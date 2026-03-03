package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"gosilo/internal/db"
	"gosilo/internal/ui"
)

type abuseHandler struct {
	deps *UIDeps
}

// AbuseHandler returns handler methods for abuse reporting.
func AbuseHandler(deps *UIDeps) *abuseHandler {
	return &abuseHandler{deps: deps}
}

type abuseReportContent struct {
	Path      string
	Submitted bool
	Error     string
}

// ShowReportForm handles GET /abuse/report.
func (h *abuseHandler) ShowReportForm(w http.ResponseWriter, r *http.Request) {
	content := &abuseReportContent{
		Path:      r.URL.Query().Get("path"),
		Submitted: r.URL.Query().Get("submitted") == "1",
	}
	h.deps.Renderer.Render(w, "abuse_report", h.deps.pageData(w, r, "Report Abuse — Gosilo", content))
}

// DoReport handles POST /abuse/report.
func (h *abuseHandler) DoReport(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	path := strings.TrimSpace(r.FormValue("path"))
	reason := r.FormValue("reason")
	description := strings.TrimSpace(r.FormValue("description"))

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "abuse_report", h.deps.pageData(w, r, "Report Abuse — Gosilo", &abuseReportContent{
			Path:  path,
			Error: msg,
		}))
	}

	if email == "" {
		renderErr("Email address is required.")
		return
	}
	if reason == "" {
		renderErr("Please select a reason.")
		return
	}

	ip := ClientIP(r)
	report, err := h.deps.Repo.CreateAbuseReport(r.Context(), email, path, reason, description, ip)
	if err != nil {
		slog.Error("failed to create abuse report", "error", err)
		renderErr("Failed to submit report. Please try again.")
		return
	}

	h.deps.Repo.Audit(r.Context(), db.SystemActorID, "abuse.reported", "abuse_report", fmt.Sprintf("%d", report.ID), path)

	http.Redirect(w, r, "/abuse/report?submitted=1", http.StatusSeeOther)
}

// --- Admin abuse report handlers ---

type adminAbuseContent struct {
	Reports     []*db.AbuseReportRow
	StatusFilter string
	OpenCount   int64
}

// ShowAbuseReports handles GET /admin/abuse.
func (h *abuseHandler) ShowAbuseReports(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	if statusFilter == "" {
		statusFilter = "open"
	}

	var dbFilter string
	if statusFilter != "all" {
		dbFilter = statusFilter
	}

	reports, err := h.deps.Repo.ListAbuseReports(r.Context(), dbFilter, 50, 0)
	if err != nil {
		slog.Error("failed to list abuse reports", "error", err)
	}

	openCount, _ := h.deps.Repo.CountOpenAbuseReports(r.Context())

	content := &adminAbuseContent{
		Reports:      reports,
		StatusFilter: statusFilter,
		OpenCount:    openCount,
	}
	h.deps.Renderer.Render(w, "admin_abuse", h.deps.adminPageData(w, r, "Abuse Reports — Admin", "abuse", content))
}

// ReviewAbuseReport handles POST /admin/abuse/{id}/review.
func (h *abuseHandler) ReviewAbuseReport(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid report ID", http.StatusBadRequest)
		return
	}

	status := r.FormValue("status")
	note := strings.TrimSpace(r.FormValue("note"))

	switch status {
	case "reviewed", "dismissed", "actioned":
		// valid
	default:
		http.Error(w, "Invalid status", http.StatusBadRequest)
		return
	}

	currentUser := CurrentUser(r)
	if currentUser == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.deps.Repo.UpdateAbuseReportStatus(r.Context(), id, status, currentUser.ID, note); err != nil {
		slog.Error("failed to review abuse report", "error", err)
		ui.SetFlashError(w, "Failed to update report.")
		http.Redirect(w, r, "/admin/abuse", http.StatusSeeOther)
		return
	}

	// Audit log.
	adminH := AdminHandler(h.deps)
	adminH.audit(r, "abuse.reviewed", "abuse_report", idStr, "status="+status)

	ui.SetFlash(w, "Report #"+idStr+" marked as "+status+".")
	http.Redirect(w, r, "/admin/abuse", http.StatusSeeOther)
}
