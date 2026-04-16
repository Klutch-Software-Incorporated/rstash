package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"rstash/internal/model"
	"rstash/internal/ui"
	"rstash/internal/webhooks"
)

type webhooksHandler struct {
	deps *UIDeps
}

// WebhooksHandler returns handler methods for the admin Webhooks page.
func WebhooksHandler(deps *UIDeps) *webhooksHandler {
	return &webhooksHandler{deps: deps}
}

type webhooksListContent struct {
	Subscriptions []*webhookRow
}

type webhookRow struct {
	ID            int64
	Name          string
	URL           string
	Events        string
	Active        bool
	LastSuccess   string
	LastError     string
	LastErrorMsg  string
	FailureCount  int
	CreatedAt     string
}

type webhookFormContent struct {
	Subscription *model.WebhookSubscription
	Error        string
	RawSecret    string // shown once after create/regenerate
}

// knownEvents is the closed set of events users can subscribe to in v1.
var knownEvents = []string{
	"user.claimed",
	"user.email_changed",
	"user.disabled",
	"user.enabled",
	"user.deleted",
}

// ShowList renders GET /admin/webhooks.
func (h *webhooksHandler) ShowList(w http.ResponseWriter, r *http.Request) {
	subs, err := h.deps.Repo.ListWebhookSubscriptions(r.Context())
	if err != nil {
		slog.Error("list webhooks", "error", err)
		h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to load subscriptions.")
		return
	}

	rows := make([]*webhookRow, 0, len(subs))
	for _, s := range subs {
		row := &webhookRow{
			ID:           s.ID,
			Name:         s.Name,
			URL:          s.URL,
			Events:       s.Events,
			Active:       s.Active,
			LastErrorMsg: s.LastError,
			FailureCount: s.FailureCount,
			CreatedAt:    s.CreatedAt.Format("2006-01-02 15:04"),
		}
		if s.LastSuccessAt != nil {
			row.LastSuccess = s.LastSuccessAt.Format("2006-01-02 15:04")
		}
		if s.LastErrorAt != nil {
			row.LastError = s.LastErrorAt.Format("2006-01-02 15:04")
		}
		rows = append(rows, row)
	}

	h.deps.Renderer.Render(w, "admin_webhooks", h.deps.adminPageData(w, r, "Webhooks — Admin", "webhooks", &webhooksListContent{Subscriptions: rows}))
}

// ShowNew renders GET /admin/webhooks/new.
func (h *webhooksHandler) ShowNew(w http.ResponseWriter, r *http.Request) {
	h.deps.Renderer.Render(w, "admin_webhook_form", h.deps.adminPageData(w, r, "New Webhook — Admin", "webhooks", &webhookFormContent{
		Subscription: &model.WebhookSubscription{Active: true},
	}))
}

// DoCreate handles POST /admin/webhooks.
func (h *webhooksHandler) DoCreate(w http.ResponseWriter, r *http.Request) {
	sub := &model.WebhookSubscription{
		Name:   strings.TrimSpace(r.FormValue("name")),
		URL:    strings.TrimSpace(r.FormValue("url")),
		Events: normalizeEvents(r.Form["events"]),
		Active: r.FormValue("active") == "on",
	}

	renderForm := func(msg string) {
		h.deps.Renderer.Render(w, "admin_webhook_form", h.deps.adminPageData(w, r, "New Webhook — Admin", "webhooks", &webhookFormContent{
			Subscription: sub,
			Error:        msg,
		}))
	}

	if msg := validateWebhookForm(sub); msg != "" {
		renderForm(msg)
		return
	}

	secret, err := webhooks.GenerateSecret()
	if err != nil {
		slog.Error("webhook: generate secret", "error", err)
		renderForm("Failed to generate secret.")
		return
	}
	sub.Secret = secret

	if err := h.deps.Repo.CreateWebhookSubscription(r.Context(), sub); err != nil {
		slog.Error("create webhook subscription", "error", err)
		renderForm("Failed to save subscription.")
		return
	}
	auditWebhook(h.deps, r, "admin.webhook.created", sub)

	h.deps.Renderer.Render(w, "admin_webhook_secret", h.deps.adminPageData(w, r, "Webhook Secret — Admin", "webhooks", &webhookFormContent{
		Subscription: sub,
		RawSecret:    secret,
	}))
}

// ShowEdit renders GET /admin/webhooks/{id}.
func (h *webhooksHandler) ShowEdit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sub, err := h.deps.Repo.GetWebhookSubscription(r.Context(), id)
	if err != nil || sub == nil {
		http.NotFound(w, r)
		return
	}
	h.deps.Renderer.Render(w, "admin_webhook_edit", h.deps.adminPageData(w, r, "Edit Webhook — Admin", "webhooks", &webhookFormContent{
		Subscription: sub,
	}))
}

// DoUpdate handles POST /admin/webhooks/{id}.
func (h *webhooksHandler) DoUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sub, err := h.deps.Repo.GetWebhookSubscription(r.Context(), id)
	if err != nil || sub == nil {
		http.NotFound(w, r)
		return
	}

	sub.Name = strings.TrimSpace(r.FormValue("name"))
	sub.URL = strings.TrimSpace(r.FormValue("url"))
	sub.Events = normalizeEvents(r.Form["events"])
	sub.Active = r.FormValue("active") == "on"

	if msg := validateWebhookForm(sub); msg != "" {
		h.deps.Renderer.Render(w, "admin_webhook_edit", h.deps.adminPageData(w, r, "Edit Webhook — Admin", "webhooks", &webhookFormContent{
			Subscription: sub,
			Error:        msg,
		}))
		return
	}

	if err := h.deps.Repo.UpdateWebhookSubscription(r.Context(), sub); err != nil {
		slog.Error("update webhook subscription", "error", err)
		h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to save subscription.")
		return
	}
	auditWebhook(h.deps, r, "admin.webhook.updated", sub)
	ui.SetFlash(w, "Webhook subscription updated.")
	http.Redirect(w, r, "/admin/webhooks", http.StatusSeeOther)
}

// DoDelete handles POST /admin/webhooks/{id}/delete.
func (h *webhooksHandler) DoDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sub, err := h.deps.Repo.GetWebhookSubscription(r.Context(), id)
	if err != nil || sub == nil {
		http.NotFound(w, r)
		return
	}
	if err := h.deps.Repo.DeleteWebhookSubscription(r.Context(), id); err != nil {
		slog.Error("delete webhook subscription", "error", err)
		h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to delete subscription.")
		return
	}
	auditWebhook(h.deps, r, "admin.webhook.deleted", sub)
	ui.SetFlash(w, "Webhook subscription deleted.")
	http.Redirect(w, r, "/admin/webhooks", http.StatusSeeOther)
}

// --- helpers ---

func validateWebhookForm(sub *model.WebhookSubscription) string {
	if sub.Name == "" {
		return "Name is required."
	}
	if sub.URL == "" {
		return "URL is required."
	}
	u, err := url.Parse(sub.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "URL must be a full http(s) URL."
	}
	if sub.Events == "" {
		return "Select at least one event."
	}
	return ""
}

func normalizeEvents(values []string) string {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			set[v] = true
		}
	}
	// If '*' selected, collapse to just '*'.
	if set["*"] {
		return "*"
	}
	parts := make([]string, 0, len(set))
	for _, e := range knownEvents {
		if set[e] {
			parts = append(parts, e)
		}
	}
	return strings.Join(parts, " ")
}

// KnownWebhookEvents is exposed for the admin UI to render event checkboxes.
func KnownWebhookEvents() []string { return knownEvents }

func auditWebhook(deps *UIDeps, r *http.Request, action string, sub *model.WebhookSubscription) {
	actor := CurrentUser(r)
	actorID := int64(0)
	if actor != nil {
		actorID = actor.ID
	}
	deps.Repo.Audit(r.Context(), actorID, action, "webhook", fmt.Sprintf("%d", sub.ID), sub.Name)
}
