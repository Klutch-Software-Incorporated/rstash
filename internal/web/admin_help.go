package web

import (
	"net/http"
	"strings"

	"gosilo/internal/cmdinfo"
)

type helpHandler struct {
	deps *UIDeps
}

// HelpHandler returns handler methods for CLI help pages.
func HelpHandler(deps *UIDeps) *helpHandler {
	return &helpHandler{deps: deps}
}

// --- Content structs ---

type helpIndexContent struct {
	Groups []helpGroup
}

type helpGroup struct {
	Title    string
	Commands []cmdinfo.CommandSummary
}

type helpCommandContent struct {
	Command cmdinfo.CommandInfo
}

// ShowIndex handles GET /admin/help — command index grouped by category.
func (h *helpHandler) ShowIndex(w http.ResponseWriter, r *http.Request) {
	// Group top-level commands (children of root) by GroupID.
	groupMap := make(map[string]*helpGroup)
	var groupOrder []string

	for _, cmd := range h.deps.CommandIndex {
		// Only top-level commands (direct children of root).
		if !strings.Contains(cmd.Parent, " ") && cmd.Parent != "" {
			gid := cmd.GroupID
			if gid == "" {
				gid = "_other"
			}
			g, ok := groupMap[gid]
			if !ok {
				title := cmd.GroupTitle
				if title == "" {
					title = "Other"
				}
				g = &helpGroup{Title: title}
				groupMap[gid] = g
				groupOrder = append(groupOrder, gid)
			}
			g.Commands = append(g.Commands, cmdinfo.CommandSummary{
				Name:     cmd.Name,
				FullName: cmd.FullName,
				Short:    cmd.Short,
			})
		}
	}

	var groups []helpGroup
	for _, gid := range groupOrder {
		groups = append(groups, *groupMap[gid])
	}

	content := &helpIndexContent{Groups: groups}
	h.deps.Renderer.Render(w, "admin_help_index", h.deps.adminPageData(w, r, "CLI Reference — Admin", "help", content))
}

// ShowCommand handles GET /admin/help/{command...} — single command detail.
func (h *helpHandler) ShowCommand(w http.ResponseWriter, r *http.Request) {
	// Convert URL path to command full name: "user/add" → "gosilo user add"
	cmdPath := r.PathValue("command")
	parts := strings.Split(cmdPath, "/")
	fullName := "gosilo"
	if cmdPath != "" {
		fullName = "gosilo " + strings.Join(parts, " ")
	}

	cmd := cmdinfo.CommandByPath(h.deps.CommandIndex, fullName)
	if cmd == nil {
		http.NotFound(w, r)
		return
	}

	content := &helpCommandContent{Command: *cmd}
	h.deps.Renderer.Render(w, "admin_help_command", h.deps.adminPageData(w, r, cmd.FullName+" — CLI Reference", "help", content))
}
