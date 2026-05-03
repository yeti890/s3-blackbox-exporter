package config

import (
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxObjectSizeBytes = 512 * 1024 * 1024

type Config struct {
	Endpoint            string
	AccessKey           string
	SecretKey           string
	Bucket              string
	Region              string
	BasePrefix          string
	ListenAddress       string
	Interval            time.Duration
	Timeout             time.Duration
	ObjectSizeBytes     int64
	InsecureSkipVerify  bool
	DisableSSL          bool
	PathStyle           bool
	ClusterName         string
	AvailabilityZone    string
	RetryMode           string
	RetryMaxAttempts    int
	RetryMaxBackoff     time.Duration
	DisableInitialProbe bool
	TLSClientConfig     *tls.Config
}

func Load() (Config, error) {
	cfg := Config{
		Endpoint:            normalizeEndpoint(getenv("ENDPOINT", ""), getBool("DISABLE_SSL", false)),
		AccessKey:           getenv("ACCESS_KEY", ""),
		SecretKey:           firstNonEmpty(os.Getenv("SECRET_KEY"), os.Getenv("SERCRET_KEY")),
		Bucket:              getenv("BUCKET", ""),
		Region:              getenv("REGION", "us-east-1"),
		BasePrefix:          strings.Trim(firstNonEmpty(os.Getenv("BASE_PREFIX"), os.Getenv("PREFIX"), "s3-blackbox-exporter"), "/"),
		ListenAddress:       getenv("LISTEN_ADDRESS", ":9241"),
		Interval:            getDuration("INTERVAL", 30*time.Second),
		Timeout:             getDuration("TIMEOUT", 10*time.Second),
		ObjectSizeBytes:     getInt64("OBJECT_SIZE_BYTES", 1024*1024),
		InsecureSkipVerify:  getBool("INSECURE_SKIP_VERIFY", false),
		DisableSSL:          getBool("DISABLE_SSL", false),
		PathStyle:           getBool("PATH_STYLE", true),
		ClusterName:         getenv("CLUSTER_NAME", "default"),
		AvailabilityZone:    getenv("AZ", "unknown"),
		RetryMode:           strings.ToLower(getenv("RETRY_MODE", "nop")),
		RetryMaxAttempts:    getInt("RETRY_MAX_ATTEMPTS", 1),
		RetryMaxBackoff:     getDuration("RETRY_MAX_BACKOFF", 2*time.Second),
		DisableInitialProbe: getBool("DISABLE_INITIAL_PROBE", false),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	cfg.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("ENDPOINT is required")
	}
	if c.AccessKey == "" {
		return fmt.Errorf("ACCESS_KEY is required")
	}
	if c.SecretKey == "" {
		return fmt.Errorf("SECRET_KEY is required; SERCRET_KEY is also supported for backward compatibility")
	}
	if c.Bucket == "" {
		return fmt.Errorf("BUCKET is required")
	}
	if c.Region == "" {
		return fmt.Errorf("REGION is required")
	}
	if c.ClusterName == "" {
		return fmt.Errorf("CLUSTER_NAME must not be empty")
	}
	if c.AvailabilityZone == "" {
		return fmt.Errorf("AZ must not be empty")
	}
	if c.Interval < time.Second {
		return fmt.Errorf("INTERVAL must be >= 1s")
	}
	if c.Timeout < time.Second {
		return fmt.Errorf("TIMEOUT must be >= 1s")
	}
	if c.ObjectSizeBytes <= 0 {
		return fmt.Errorf("OBJECT_SIZE_BYTES must be > 0")
	}
	if c.ObjectSizeBytes > maxObjectSizeBytes {
		return fmt.Errorf("OBJECT_SIZE_BYTES must be <= %d", maxObjectSizeBytes)
	}
	if c.RetryMode != "nop" && c.RetryMode != "standard" {
		return fmt.Errorf("RETRY_MODE must be one of: nop, standard")
	}
	if c.RetryMaxAttempts < 1 {
		return fmt.Errorf("RETRY_MAX_ATTEMPTS must be >= 1")
	}
	if c.RetryMaxBackoff <= 0 {
		return fmt.Errorf("RETRY_MAX_BACKOFF must be > 0")
	}
	return nil
}

func normalizeEndpoint(endpoint string, disableSSL bool) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return ""
	}
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	if disableSSL {
		return "http://" + endpoint
	}
	return "https://" + endpoint
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func getBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
