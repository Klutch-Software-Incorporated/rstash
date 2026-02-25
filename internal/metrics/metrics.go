package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal counts HTTP requests by method, status, and route group.
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gosilo_http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "status", "route"})

	// HTTPRequestDuration measures HTTP request duration in seconds.
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gosilo_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	// StorageUsedBytes is a gauge for total storage used in bytes.
	StorageUsedBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gosilo_storage_used_bytes",
		Help: "Total storage used in bytes.",
	})

	// StorageAvailableBytes is a gauge for available disk space.
	StorageAvailableBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gosilo_storage_available_bytes",
		Help: "Available disk space in bytes.",
	})

	// ActiveSessions is a gauge for the number of active sessions.
	ActiveSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gosilo_active_sessions",
		Help: "Number of active (non-expired) sessions.",
	})

	// ActiveTokens is a gauge for the number of active OAuth tokens.
	ActiveTokens = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gosilo_active_tokens",
		Help: "Number of active (non-expired) OAuth tokens.",
	})

	// UsersTotal is a gauge for the total number of users.
	UsersTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gosilo_users_total",
		Help: "Total number of registered users.",
	})
)

// RouteGroup maps a request path to a bounded set of route groups.
func RouteGroup(path string) string {
	switch {
	case strings.HasPrefix(path, "/storage/"):
		return "storage"
	case strings.HasPrefix(path, "/.well-known/"):
		return "webfinger"
	case strings.HasPrefix(path, "/oauth/"):
		return "oauth"
	case path == "/metrics":
		return "metrics"
	default:
		return "web"
	}
}
