package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"gosilo/internal/db"
	"gosilo/internal/ui"
)

type homeHandler struct {
	deps *UIDeps
}

// HomeHandler returns handler methods for the home page.
func HomeHandler(deps *UIDeps) *homeHandler {
	return &homeHandler{deps: deps}
}

type homeContent struct {
	Stats    *homeStats
	Modules  []*moduleRow
	Activity []*activityEvent
}

type homeStats struct {
	FileCount   int64
	StorageUsed string
	OAuthApps   int64
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

// Show handles GET / — account overview for logged-in users, server info for
// logged-out visitors.
func (h *homeHandler) Show(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	user := CurrentUser(r)
	if user == nil {
		h.deps.Renderer.Render(w, "home", ui.PageData{
			Title:            "Gosilo",
			CSRFToken:        CSRFToken(r),
			Flash:            GetFlash(w, r),
			RegistrationMode: h.deps.Config.RegistrationMode,
		})
		return
	}

	ctx := r.Context()

	stats, err := db.GetUserStorageStats(ctx, h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to get storage stats", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tokenCount, err := db.CountUserOAuthTokens(ctx, h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to count oauth tokens", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	modules, err := db.GetUserModuleStats(ctx, h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to get module stats", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	moduleRows := make([]*moduleRow, len(modules))
	for i, m := range modules {
		moduleRows[i] = &moduleRow{
			Name:      m.Module,
			FileCount: m.FileCount,
			Size:      formatBytes(m.TotalBytes),
		}
	}

	activity := buildActivityFeed(ctx, h.deps.DB, user.ID)

	h.deps.Renderer.Render(w, "home", ui.PageData{
		Title:            "Gosilo",
		CurrentUser:      userInfo(user),
		CSRFToken:        CSRFToken(r),
		Flash:            GetFlash(w, r),
		RegistrationMode: h.deps.Config.RegistrationMode,
		Content: &homeContent{
			Stats: &homeStats{
				FileCount:   stats.FileCount,
				StorageUsed: formatBytes(stats.TotalBytes),
				OAuthApps:   tokenCount,
			},
			Modules:  moduleRows,
			Activity: activity,
		},
	})
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
			events = append(events, &activityEvent{
				Type:      "login",
				Summary:   "Logged in",
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

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
