package probe

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/yeti89/s3-blackbox-exporter/internal/config"
	"github.com/yeti89/s3-blackbox-exporter/internal/keygen"
	"github.com/yeti89/s3-blackbox-exporter/internal/metrics"
	"github.com/yeti89/s3-blackbox-exporter/internal/s3client"
)

const (
	opHeadBucket = "head_bucket"
	opPutObject  = "put_object"
	opHeadObject = "head_object"
	opListBucket = "list_bucket"
	opGetObject  = "get_object"
	opDelete     = "delete_object"
)

var errorTypes = []string{
	"none",
	"timeout",
	"dns",
	"tcp",
	"tls",
	"auth",
	"http_4xx",
	"http_5xx",
	"checksum",
	"unexpected_status",
	"body_read",
	"skipped",
	"unknown",
}

var statusClasses = []string{"2xx", "4xx", "5xx", "timeout", "other"}

type Runner struct {
	cfg      config.Config
	client   *s3.Client
	metrics  *metrics.Metrics
	logger   *slog.Logger
	statuses *s3client.StatusRecorder
}

type opResult struct {
	operation  string
	statusCode int
	errType    string
	err        error
	duration   time.Duration
	success    bool
}

func NewRunner(ctx context.Context, cfg config.Config, m *metrics.Metrics, logger *slog.Logger) (*Runner, error) {
	client, statuses, err := s3client.New(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}

	m.ObjectSizeBytes.WithLabelValues(cfg.ClusterName, cfg.AvailabilityZone, cfg.Endpoint, cfg.Bucket).Set(float64(cfg.ObjectSizeBytes))

	return &Runner{cfg: cfg, client: client, metrics: m, logger: logger, statuses: statuses}, nil
}

func (r *Runner) RunForever(ctx context.Context) {
	if !r.cfg.DisableInitialProbe {
		r.RunOnce(ctx)
	}

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.RunOnce(ctx)
		}
	}
}

func (r *Runner) RunOnce(ctx context.Context) {
	started := time.Now()
	cycleOK := true

	defer func() {
		r.recordCycle(cycleOK, time.Since(started))
	}()

	key, err := keygen.NewObjectKey(r.cfg.BasePrefix, r.cfg.ClusterName, r.cfg.AvailabilityZone)
	if err != nil {
		r.logger.Error("generate object key failed", "error", err)
		cycleOK = false
		return
	}

	body, err := randomBytes(r.cfg.ObjectSizeBytes)
	if err != nil {
		r.logger.Error("generate test object failed", "error", err)
		cycleOK = false
		return
	}
	wantSHA := sha256.Sum256(body)

	record := func(result opResult) {
		r.record(result)
		if !result.success {
			cycleOK = false
		}
	}

	record(r.headBucket(ctx))

	putResult := r.putObject(ctx, key, body)
	record(putResult)
	if !putResult.success {
		record(skipped(opHeadObject, "put_object failed"))
		record(skipped(opListBucket, "put_object failed"))
		record(skipped(opGetObject, "put_object failed"))
		record(skipped(opDelete, "put_object failed"))
		return
	}

	deleteDone := false
	defer func() {
		if !deleteDone {
			result := r.deleteObject(ctx, key)
			if !result.success {
				r.logger.Warn("best-effort cleanup failed", "key", key, "error", result.err)
			}
		}
	}()

	record(r.headObject(ctx, key))
	record(r.listExactKey(ctx, key))
	record(r.getObject(ctx, key, wantSHA[:]))

	deleteResult := r.deleteObject(ctx, key)
	record(deleteResult)
	deleteDone = deleteResult.success
}

func (r *Runner) headBucket(parent context.Context) opResult {
	return r.runS3Op(parent, opHeadBucket, []int{http.StatusOK, http.StatusNoContent}, func(ctx context.Context) (int, error) {
		r.statuses.Reset()
		_, err := r.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(r.cfg.Bucket)})
		return statusCodeFromResponseOrError(r.statuses.Last(), err), err
	})
}

func (r *Runner) putObject(parent context.Context, key string, body []byte) opResult {
	return r.runS3Op(parent, opPutObject, []int{http.StatusOK, http.StatusNoContent}, func(ctx context.Context) (int, error) {
		r.statuses.Reset()
		_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(r.cfg.Bucket),
			Key:    aws.String(key),
			Body:   bytes.NewReader(body),
		})
		return statusCodeFromResponseOrError(r.statuses.Last(), err), err
	})
}

func (r *Runner) headObject(parent context.Context, key string) opResult {
	return r.runS3Op(parent, opHeadObject, []int{http.StatusOK}, func(ctx context.Context) (int, error) {
		r.statuses.Reset()
		out, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(r.cfg.Bucket),
			Key:    aws.String(key),
		})
		if err == nil && out.ContentLength != nil && *out.ContentLength != r.cfg.ObjectSizeBytes {
			return statusCodeFromResponseOrError(r.statuses.Last(), nil), fmt.Errorf("unexpected content length: got=%d want=%d", *out.ContentLength, r.cfg.ObjectSizeBytes)
		}
		return statusCodeFromResponseOrError(r.statuses.Last(), err), err
	})
}

func (r *Runner) listExactKey(parent context.Context, key string) opResult {
	return r.runS3Op(parent, opListBucket, []int{http.StatusOK}, func(ctx context.Context) (int, error) {
		r.statuses.Reset()
		out, err := r.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:  aws.String(r.cfg.Bucket),
			Prefix:  aws.String(key),
			MaxKeys: aws.Int32(1),
		})
		if err != nil {
			return statusCodeFromResponseOrError(r.statuses.Last(), err), err
		}
		if len(out.Contents) != 1 || aws.ToString(out.Contents[0].Key) != key {
			return statusCodeFromResponseOrError(r.statuses.Last(), nil), fmt.Errorf("created object is not visible in list result")
		}
		return statusCodeFromResponseOrError(r.statuses.Last(), nil), nil
	})
}

func (r *Runner) getObject(parent context.Context, key string, wantSHA []byte) opResult {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, r.cfg.Timeout)
	defer cancel()

	r.statuses.Reset()
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	statusCode := statusCodeFromResponseOrError(r.statuses.Last(), err)
	if err != nil {
		return makeResult(opGetObject, started, statusCode, err, http.StatusOK)
	}
	defer out.Body.Close()

	gotSHA := sha256.New()
	if _, err := io.Copy(gotSHA, out.Body); err != nil {
		return opResult{operation: opGetObject, statusCode: statusCode, errType: "body_read", err: err, duration: time.Since(started), success: false}
	}

	if !bytes.Equal(gotSHA.Sum(nil), wantSHA) {
		return opResult{
			operation:  opGetObject,
			statusCode: statusCode,
			errType:    "checksum",
			err:        fmt.Errorf("checksum mismatch: got=%s want=%s", hex.EncodeToString(gotSHA.Sum(nil)), hex.EncodeToString(wantSHA)),
			duration:   time.Since(started),
			success:    false,
		}
	}

	return opResult{operation: opGetObject, statusCode: statusCode, errType: "none", duration: time.Since(started), success: true}
}

func (r *Runner) deleteObject(parent context.Context, key string) opResult {
	return r.runS3Op(parent, opDelete, []int{http.StatusOK, http.StatusNoContent}, func(ctx context.Context) (int, error) {
		r.statuses.Reset()
		_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(r.cfg.Bucket),
			Key:    aws.String(key),
		})
		return statusCodeFromResponseOrError(r.statuses.Last(), err), err
	})
}

func (r *Runner) runS3Op(parent context.Context, operation string, expected []int, fn func(context.Context) (int, error)) opResult {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, r.cfg.Timeout)
	defer cancel()

	statusCode, err := fn(ctx)
	return makeResult(operation, started, statusCode, err, expected...)
}

func makeResult(operation string, started time.Time, statusCode int, err error, expected ...int) opResult {
	if err != nil {
		return opResult{operation: operation, statusCode: statusCode, errType: classifyError(err, statusCode), err: err, duration: time.Since(started), success: false}
	}
	if !expectedStatus(statusCode, expected) {
		return opResult{operation: operation, statusCode: statusCode, errType: "unexpected_status", err: fmt.Errorf("unexpected status code %d", statusCode), duration: time.Since(started), success: false}
	}
	return opResult{operation: operation, statusCode: statusCode, errType: "none", duration: time.Since(started), success: true}
}

func skipped(operation, reason string) opResult {
	return opResult{operation: operation, statusCode: 0, errType: "skipped", err: errors.New(reason), success: false}
}

func (r *Runner) recordCycle(success bool, duration time.Duration) {
	labels := []string{r.cfg.ClusterName, r.cfg.AvailabilityZone, r.cfg.Endpoint, r.cfg.Bucket}
	if success {
		r.metrics.CycleSuccess.WithLabelValues(labels...).Set(1)
	} else {
		r.metrics.CycleSuccess.WithLabelValues(labels...).Set(0)
	}
	r.metrics.CycleDuration.WithLabelValues(labels...).Observe(duration.Seconds())
}

func (r *Runner) record(result opResult) {
	labels := []string{r.cfg.ClusterName, r.cfg.AvailabilityZone, r.cfg.Endpoint, r.cfg.Bucket, result.operation}
	statusClass := classifyStatus(result.statusCode, result.errType)

	if result.success {
		r.metrics.ProbeSuccess.WithLabelValues(labels...).Set(1)
	} else {
		r.metrics.ProbeSuccess.WithLabelValues(labels...).Set(0)
		r.metrics.ProbeErrorTotal.WithLabelValues(append(labels, result.errType)...).Inc()
		r.logger.Warn("s3 probe operation failed", "operation", result.operation, "status_code", result.statusCode, "error_type", result.errType, "error", result.err)
	}

	r.metrics.ProbeDuration.WithLabelValues(labels...).Observe(result.duration.Seconds())
	r.metrics.LastHTTPStatusCode.WithLabelValues(labels...).Set(float64(result.statusCode))
	r.metrics.LastRunTimestamp.WithLabelValues(labels...).Set(float64(time.Now().Unix()))

	for _, class := range statusClasses {
		value := 0.0
		if class == statusClass {
			value = 1
		}
		r.metrics.HTTPStatusClass.WithLabelValues(append(labels, class)...).Set(value)
	}

	for _, errType := range errorTypes {
		value := 0.0
		if errType == result.errType {
			value = 1
		}
		r.metrics.ProbeErrorState.WithLabelValues(append(labels, errType)...).Set(value)
	}

	codeLabel := strconv.Itoa(result.statusCode)
	if result.statusCode == 0 {
		codeLabel = "none"
	}
	r.metrics.HTTPStatusTotal.WithLabelValues(append(labels, codeLabel, statusClass)...).Inc()
}

func randomBytes(size int64) ([]byte, error) {
	data := make([]byte, size)
	_, err := rand.Read(data)
	return data, err
}

func expectedStatus(statusCode int, expected []int) bool {
	for _, code := range expected {
		if statusCode == code {
			return true
		}
	}
	return false
}

func statusCodeFromResponseOrError(lastStatus int, err error) int {
	if lastStatus > 0 {
		return lastStatus
	}
	if err == nil {
		return http.StatusOK
	}
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) {
		return responseErr.HTTPStatusCode()
	}
	return 0
}

func classifyStatus(statusCode int, errType string) string {
	if errType == "timeout" {
		return "timeout"
	}
	switch {
	case statusCode >= 200 && statusCode <= 299:
		return "2xx"
	case statusCode >= 400 && statusCode <= 499:
		return "4xx"
	case statusCode >= 500 && statusCode <= 599:
		return "5xx"
	default:
		return "other"
	}
}

func classifyError(err error, statusCode int) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && strings.Contains(strings.ToLower(opErr.Op), "dial") {
		return "tcp"
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := strings.ToLower(apiErr.ErrorCode())
		if code == "accessdenied" || code == "invalidaccesskeyid" || code == "signaturedoesnotmatch" || code == "authfailure" {
			return "auth"
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "tls") || strings.Contains(message, "certificate") || strings.Contains(message, "x509") {
		return "tls"
	}
	if statusCode >= 400 && statusCode <= 499 {
		return "http_4xx"
	}
	if statusCode >= 500 && statusCode <= 599 {
		return "http_5xx"
	}
	return "unknown"
}
