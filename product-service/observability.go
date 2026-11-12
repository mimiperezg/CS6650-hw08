package main

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// The RED signals for every endpoint: Rate, Errors and Duration. Each
// request is labeled with its route template (/v1/products/:productId), not
// its raw path, so every product ID adds to the same series instead of
// creating a new one.
var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests handled, by method, endpoint and status code.",
	}, []string{"method", "endpoint", "status"})

	requestErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_request_errors_total",
		Help: "HTTP requests that returned a non-2xx status code.",
	}, []string{"method", "endpoint", "status"})

	// The default buckets start at 5ms, which is slower than nearly every
	// request this in-memory service handles, so p50 and p95 would both land
	// in the first bucket. These run from 0.1ms to about 3.3s, doubling.
	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Time to handle an HTTP request, in seconds.",
		Buckets: prometheus.ExponentialBuckets(0.0001, 2, 16),
	}, []string{"method", "endpoint"})
)

// observeRequests records each request's RED signals twice: as Prometheus
// metrics, for live dashboards, and as one JSON log line, for debugging
// after the fact. It must run before the recovery middleware, so that a
// panic's 500 is recorded too.
func observeRequests(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		endpoint := c.FullPath()
		// Prometheus scrapes every few seconds; counting those scrapes would
		// add a busy endpoint to the dashboard that no user ever calls.
		if endpoint == metricsPath {
			c.Next()
			return
		}
		// FullPath is empty when no route matched, i.e. Gin's 404.
		if endpoint == "" {
			endpoint = "unmatched"
		}

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		method := c.Request.Method
		status := c.Writer.Status()
		statusLabel := strconv.Itoa(status)

		requestsTotal.WithLabelValues(method, endpoint, statusLabel).Inc()
		if status < 200 || status > 299 {
			requestErrorsTotal.WithLabelValues(method, endpoint, statusLabel).Inc()
		}
		requestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())

		logger.Info("request",
			"method", method,
			"endpoint", endpoint,
			"path", c.Request.URL.Path,
			"status", status,
			"duration_ms", float64(duration)/float64(time.Millisecond),
		)
	}
}
