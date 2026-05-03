package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	CycleSuccess       *prometheus.GaugeVec
	CycleDuration      *prometheus.HistogramVec
	ProbeSuccess       *prometheus.GaugeVec
	ProbeDuration      *prometheus.HistogramVec
	HTTPStatusTotal    *prometheus.CounterVec
	HTTPStatusClass    *prometheus.GaugeVec
	LastHTTPStatusCode *prometheus.GaugeVec
	ProbeErrorTotal    *prometheus.CounterVec
	ProbeErrorState    *prometheus.GaugeVec
	ObjectSizeBytes    *prometheus.GaugeVec
	LastRunTimestamp   *prometheus.GaugeVec
	BuildInfo          *prometheus.GaugeVec
}

func New(version, commit, date string) *Metrics {
	m := &Metrics{
		CycleSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "s3_probe_cycle_success",
			Help: "S3 synthetic probe full cycle success: 1 if all critical operations in the cycle succeeded, 0 otherwise.",
		}, []string{"cluster", "az", "endpoint", "bucket"}),

		CycleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "s3_probe_cycle_duration_seconds",
			Help:    "S3 synthetic probe full cycle duration in seconds.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"cluster", "az", "endpoint", "bucket"}),

		ProbeSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "s3_probe_success",
			Help: "S3 synthetic probe operation success: 1 success, 0 failure.",
		}, []string{"cluster", "az", "endpoint", "bucket", "operation"}),

		ProbeDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "s3_probe_duration_seconds",
			Help:    "S3 synthetic probe operation latency in seconds.",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"cluster", "az", "endpoint", "bucket", "operation"}),

		HTTPStatusTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "s3_probe_http_status_total",
			Help: "S3 synthetic probe HTTP status counter by code and class.",
		}, []string{"cluster", "az", "endpoint", "bucket", "operation", "code", "class"}),

		HTTPStatusClass: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "s3_probe_http_status_class",
			Help: "Last S3 synthetic probe HTTP status class: 1 for last observed class, 0 otherwise.",
		}, []string{"cluster", "az", "endpoint", "bucket", "operation", "class"}),

		LastHTTPStatusCode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "s3_probe_last_http_status_code",
			Help: "Last S3 synthetic probe HTTP status code. 0 means no HTTP response, usually timeout, DNS, TCP or TLS error.",
		}, []string{"cluster", "az", "endpoint", "bucket", "operation"}),

		ProbeErrorTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "s3_probe_error_total",
			Help: "S3 synthetic probe error counter by normalized error type.",
		}, []string{"cluster", "az", "endpoint", "bucket", "operation", "type"}),

		ProbeErrorState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "s3_probe_error",
			Help: "Last S3 synthetic probe normalized error state: 1 active for last probe, 0 inactive.",
		}, []string{"cluster", "az", "endpoint", "bucket", "operation", "type"}),

		ObjectSizeBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "s3_probe_object_size_bytes",
			Help: "Synthetic S3 test object size in bytes.",
		}, []string{"cluster", "az", "endpoint", "bucket"}),

		LastRunTimestamp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "s3_probe_last_run_timestamp_seconds",
			Help: "Unix timestamp of the last completed S3 synthetic probe operation.",
		}, []string{"cluster", "az", "endpoint", "bucket", "operation"}),

		BuildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "s3_probe_exporter_build_info",
			Help: "S3 probe exporter build information.",
		}, []string{"version", "commit", "date"}),
	}

	m.BuildInfo.WithLabelValues(version, commit, date).Set(1)
	return m
}

func (m *Metrics) MustRegister(reg *prometheus.Registry) {
	reg.MustRegister(
		m.CycleSuccess,
		m.CycleDuration,
		m.ProbeSuccess,
		m.ProbeDuration,
		m.HTTPStatusTotal,
		m.HTTPStatusClass,
		m.LastHTTPStatusCode,
		m.ProbeErrorTotal,
		m.ProbeErrorState,
		m.ObjectSizeBytes,
		m.LastRunTimestamp,
		m.BuildInfo,
	)
}
