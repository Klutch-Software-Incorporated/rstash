package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal counts HTTP requests by method, status, and route group.
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rstash_http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "status", "route"})

	// HTTPRequestDuration measures HTTP request duration in seconds.
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "rstash_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	// StorageUsedBytes is a gauge for total storage used in bytes.
	StorageUsedBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rstash_storage_used_bytes",
		Help: "Total storage used in bytes.",
	})

	// StorageAvailableBytes is a gauge for available disk space.
	StorageAvailableBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rstash_storage_available_bytes",
		Help: "Available disk space in bytes.",
	})

	// ActiveSessions is a gauge for the number of active sessions.
	ActiveSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rstash_active_sessions",
		Help: "Number of active (non-expired) sessions.",
	})

	// ActiveTokens is a gauge for the number of active OAuth tokens.
	ActiveTokens = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rstash_active_tokens",
		Help: "Number of active (non-expired) OAuth tokens.",
	})

	// UsersTotal is a gauge for the total number of users.
	UsersTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rstash_users_total",
		Help: "Total number of registered users.",
	})

	// EmailsSentTotal counts emails sent by type and status.
	EmailsSentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rstash_emails_sent_total",
		Help: "Total emails sent.",
	}, []string{"type", "status"})

	// LoginFailuresTotal counts failed login attempts.
	LoginFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rstash_login_failures_total",
		Help: "Total failed login attempts.",
	})

	// UserSignupsTotal counts user registrations.
	UserSignupsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rstash_user_signups_total",
		Help: "Total user registrations.",
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
