package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/yeti89/s3-blackbox-exporter/internal/config"
	metricsx "github.com/yeti89/s3-blackbox-exporter/internal/metrics"
	"github.com/yeti89/s3-blackbox-exporter/internal/probe"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := prometheus.NewRegistry()
	m := metricsx.New(version, commit, date)
	m.MustRegister(reg)

	runner, err := probe.NewRunner(ctx, cfg, m, logger)
	if err != nil {
		logger.Error("create probe runner failed", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "s3-blackbox-exporter %s\nmetrics: /metrics\nhealth: /healthz\n", version)
	})

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go runner.RunForever(ctx)

	go func() {
		logger.Info("s3-blackbox-exporter started",
			"version", version,
			"commit", commit,
			"date", date,
			"listen_address", cfg.ListenAddress,
			"endpoint", cfg.Endpoint,
			"bucket", cfg.Bucket,
			"cluster", cfg.ClusterName,
			"az", cfg.AvailabilityZone,
			"base_prefix", cfg.BasePrefix,
			"retry_mode", cfg.RetryMode,
			"retry_max_attempts", cfg.RetryMaxAttempts,
			"request_checksum_calculation", cfg.RequestChecksumCalculation,
			"response_checksum_validation", cfg.ResponseChecksumValidation,
			"interval", cfg.Interval.String(),
			"timeout", cfg.Timeout.String(),
			"object_size_bytes", cfg.ObjectSizeBytes,
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown failed", "error", err)
	}
	logger.Info("s3-blackbox-exporter stopped")
}
