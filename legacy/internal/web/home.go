package web

import (
	"context"
	"log/slog"
	"net/http"
	"sort"

	"rstash/internal/db"
)

type homeHandler struct {
	deps *UIDeps
}

// HomeHandler returns handler methods for the home page.
func HomeHandler(deps *UIDeps) *homeHandler {
	return &homeHandler{deps: deps}
}

type homeStats struct {
	FileCount    int64
	StorageUsed  string
	StorageQuota string // empty when no quota
	QuotaPercent int
	OAuthApps    int64
	StorageBytes int64 // raw bytes for meter
	QuotaBytes   int64 // raw bytes for meter max

	// Egress (populated only when the user has a per-user egress limit set)
	EgressUsed       string // human-readable used bytes for current period
	EgressQuota      string // human-readable limit
	EgressPercent    int    // 0–100
	EgressUsedBytes  int64  // raw bytes for meter
	EgressQuotaBytes int64  // raw bytes for meter max
	EgressPeriod     string // "YYYY-MM" label for the current billing period
}

type moduleRow struct {
	Name      string
	FileCount int64
	Size      string
}

type activityEvent struct {
	Type      string // "file_update", "login", "oauth_grant"
	Summary   string
	Timestamp string
}

// Show handles GET / — redirect logged-in users to their profile,
// show server info for logged-out visitors.
func (h *homeHandler) Show(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	user := CurrentUser(r)
	if user != nil {
		http.Redirect(w, r, "/~"+user.Username+"/", http.StatusSeeOther)
		return
	}

	snap := h.deps.Settings.Load()
	content := struct {
		Subtitle string
	}{
		Subtitle: snap.HomeSubtitle,
	}
	pd := h.deps.pageData(w, r, snap.SiteName, content)
	pd.HideHeader = true
	h.deps.Renderer.Render(w, "home", pd)
}

func buildActivityFeed(ctx context.Context, repo *db.Repository, userID int64) []*activityEvent {
	const perSource = 15
	var events []*activityEvent

	nodes, err := repo.GetRecentUserNodes(ctx, userID, perSource)
	if err != nil {
		slog.Error("failed to get recent nodes", "error", err)
	} else {
		for _, n := range nodes {
			events = append(events, &activityEvent{
				Type:      "file_update",
				Summary:   "Updated " + n.Path,
				Timestamp: n.UpdatedAt.Format("2006-01-02 15:04:05"),
			})
		}
	}

	sessions, err := repo.GetRecentUserSessions(ctx, userID, perSource)
	if err != nil {
		slog.Error("failed to get recent sessions", "error", err)
	} else {
		for _, s := range sessions {
			summary := "Logged in"
			if s.IP != nil && *s.IP != "" {
				summary = "Logged in from " + *s.IP
			}
			events = append(events, &activityEvent{
				Type:      "login",
				Summary:   summary,
				Timestamp: s.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
	}

	tokens, err := repo.GetRecentUserOAuthTokens(ctx, userID, perSource)
	if err != nil {
		slog.Error("failed to get recent oauth tokens", "error", err)
	} else {
		for _, t := range tokens {
			events = append(events, &activityEvent{
				Type:      "oauth_grant",
				Summary:   "Authorized app: " + t.ClientID,
				Timestamp: t.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
	}

	// Sort by timestamp descending (ISO datetime strings sort lexicographically).
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp > events[j].Timestamp
	})

	if len(events) > 15 {
		events = events[:15]
	}

	return events
}
