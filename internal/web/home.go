package web

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"sort"

	"gosilo/internal/db"
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

	h.deps.Renderer.Render(w, "home", h.deps.pageData(w, r, "Gosilo", nil))
}

func buildActivityFeed(ctx context.Context, database *sql.DB, userID int64) []*activityEvent {
	const perSource = 15
	var events []*activityEvent

	nodes, err := db.GetRecentUserNodes(ctx, database, userID, perSource)
	if err != nil {
		slog.Error("failed to get recent nodes", "error", err)
	} else {
		for _, n := range nodes {
			events = append(events, &activityEvent{
				Type:      "file_update",
				Summary:   "Updated " + n.Path,
				Timestamp: n.UpdatedAt,
			})
		}
	}

	sessions, err := db.GetRecentUserSessions(ctx, database, userID, perSource)
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
				Timestamp: s.CreatedAt,
			})
		}
	}

	tokens, err := db.GetRecentUserOAuthTokens(ctx, database, userID, perSource)
	if err != nil {
		slog.Error("failed to get recent oauth tokens", "error", err)
	} else {
		for _, t := range tokens {
			events = append(events, &activityEvent{
				Type:      "oauth_grant",
				Summary:   "Authorized app: " + t.ClientID,
				Timestamp: t.CreatedAt,
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
