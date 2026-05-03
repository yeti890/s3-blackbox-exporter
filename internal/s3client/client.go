package s3client

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/yeti89/s3-blackbox-exporter/internal/config"
)

type StatusRecorder struct {
	next http.RoundTripper
	mu   sync.Mutex
	last int
}

func New(ctx context.Context, cfg config.Config) (*s3.Client, *StatusRecorder, error) {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     cfg.TLSClientConfig,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}
	recorder := &StatusRecorder{next: transport}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
		awsconfig.WithRetryer(retryer(cfg)),
	)
	if err != nil {
		return nil, nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.PathStyle
		o.HTTPClient = &http.Client{
			Timeout:   cfg.Timeout + 2*time.Second,
			Transport: recorder,
		}
	})

	return client, recorder, nil
}

func retryer(cfg config.Config) func() aws.Retryer {
	if cfg.RetryMode == "standard" {
		return func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = cfg.RetryMaxAttempts
				o.MaxBackoff = cfg.RetryMaxBackoff
			})
		}
	}
	return func() aws.Retryer { return aws.NopRetryer{} }
}

func (s *StatusRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := s.next.RoundTrip(req)
	if resp != nil {
		s.mu.Lock()
		s.last = resp.StatusCode
		s.mu.Unlock()
	}
	return resp, err
}

func (s *StatusRecorder) Reset() {
	s.mu.Lock()
	s.last = 0
	s.mu.Unlock()
}

func (s *StatusRecorder) Last() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}
