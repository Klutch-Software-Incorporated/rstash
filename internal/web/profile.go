package web

import (
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gosilo/internal/auth"
	"gosilo/internal/config"
	"gosilo/internal/storage"
	"gosilo/internal/ui"
)

var moduleNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var folderNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// fallbackMIME supplements Go's mime package for common extensions it doesn't know.
var fallbackMIME = map[string]string{
	".md":         "text/markdown",
	".markdown":   "text/markdown",
	".yaml":       "application/yaml",
	".yml":        "application/yaml",
	".toml":       "application/toml",
	".go":         "text/x-go",
	".rs":         "text/x-rust",
	".sh":         "text/x-shellscript",
	".bash":       "text/x-shellscript",
	".zsh":        "text/x-shellscript",
	".fish":       "text/x-shellscript",
	".ps1":        "text/x-powershell",
	".bat":        "text/x-bat",
	".cmd":        "text/x-bat",
	".dockerfile": "text/x-dockerfile",
	".tf":         "text/x-terraform",
	".tsx":        "text/tsx",
	".jsx":        "text/jsx",
}

// knownFilenames maps extensionless filenames to MIME types.
var knownFilenames = map[string]string{
	"dockerfile": "text/x-dockerfile",
	"makefile":   "text/x-makefile",
}

type tokenRow struct {
	TokenPrefix string
	TokenFull   string
	ClientID    string
	Scopes      string
	CreatedAt   string
}

type sessionRow struct {
	TokenPrefix string
	TokenFull   string
	CreatedAt   string
	ExpiresAt   string
	IsCurrent   bool
}

type breadcrumb struct {
	Name string
	Path string
}

type fileItem struct {
	Name        string
	Path        string
	BrowseURL   string
	IsFolder    bool
	Size        string
	ContentType string
	ETag        string
}

type searchResultRow struct {
	Name        string
	Path        string
	BrowseURL   string
	Size        string
	ContentType string
}

type profileHandler struct {
	deps *UIDeps
}

// ProfileHandler returns handler methods for user-scoped profile pages.
func ProfileHandler(deps *UIDeps) *profileHandler {
	return &profileHandler{deps: deps}
}

// urlPrefix returns the base URL prefix for the target user's profile.
func urlPrefix(r *http.Request) string {
	return "/~" + TargetUser(r).Username
}

// --- Dashboard ---

type profileDashboardContent struct {
	// Common (always shown)
	Stats        *homeStats
	Activity     []*activityEvent
	RecentFiles  []*recentFileRow
	LargestFiles []*recentFileRow
	SessionCount int64
	QuotaMode    string
	Tokens       []*tokenRow
	BaseURL      string
	URLPrefix    string
	IsSelf       bool
	Host         string
	Username     string

	// Admin-only fields (when viewing another user)
	TargetUser *profileTargetUser
	AuditLog   []*auditRow
}

type profileTargetUser struct {
	ID                int64
	Username          string
	IsAdmin           bool
	Disabled          bool
	Approved          bool
	CreatedAt         string
	LastLoginAt       string
	LastLoginIP       string
	StorageUsed       string
	FileCount         int64
	TOSAcceptedAt     string
	PrivacyAcceptedAt string
}

func (h *profileHandler) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	isSelf := IsSelf(r)
	ctx := r.Context()
	prefix := urlPrefix(r)

	stats, err := h.deps.Repo.GetUserStorageStats(ctx, target.ID)
	if err != nil {
		slog.Error("failed to get storage stats", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tokenCount, err := h.deps.Repo.CountUserOAuthTokens(ctx, target.ID)
	if err != nil {
		slog.Error("failed to count oauth tokens", "error", err)
	}

	sessCount, _ := h.deps.Auth.CountUserSessions(ctx, target.ID)

	recentNodes, _ := h.deps.Repo.GetRecentUserNodes(ctx, target.ID, 10)
	fileRows := make([]*recentFileRow, len(recentNodes))
	for i, n := range recentNodes {
		fileRows[i] = &recentFileRow{
			Name:        path.Base(n.Path),
			Path:        n.Path,
			BrowseURL:   prefix + "/files" + path.Dir(n.Path) + "/#file-" + url.PathEscape(path.Base(n.Path)),
			Size:        formatBytes(n.ContentLength),
			ContentType: n.ContentType,
			UpdatedAt:   n.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	largestNodes, _ := h.deps.Repo.GetLargestUserNodes(ctx, target.ID, 10)
	largestRows := make([]*recentFileRow, len(largestNodes))
	for i, n := range largestNodes {
		largestRows[i] = &recentFileRow{
			Name:        path.Base(n.Path),
			Path:        n.Path,
			BrowseURL:   prefix + "/files" + path.Dir(n.Path) + "/#file-" + url.PathEscape(path.Base(n.Path)),
			Size:        formatBytes(n.ContentLength),
			ContentType: n.ContentType,
			UpdatedAt:   n.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	activity := buildActivityFeed(ctx, h.deps.Repo, target.ID)

	oauthTokens, err := h.deps.Repo.ListOAuthTokensByUserID(ctx, target.ID)
	if err != nil {
		slog.Error("failed to list oauth tokens", "error", err)
		oauthTokens = nil
	}
	tokenRows := make([]*tokenRow, len(oauthTokens))
	for i, t := range oauthTokens {
		tokenRows[i] = &tokenRow{
			TokenPrefix: t.Token[:8] + "...",
			TokenFull:   t.Token,
			ClientID:    t.ClientID,
			Scopes:      strings.ReplaceAll(t.Scopes, " ", ", "),
			CreatedAt:   t.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	hs := &homeStats{
		FileCount:    stats.FileCount,
		StorageUsed:  formatBytes(stats.TotalBytes),
		StorageBytes: stats.TotalBytes,
		OAuthApps:    tokenCount,
	}
	snap := h.deps.Settings.Load()
	if snap.QuotaMode == "user" {
		limit := snap.QuotaUser
		if target.StorageQuota > 0 {
			limit = target.StorageQuota
		}
		hs.StorageQuota = formatBytes(limit)
		hs.QuotaBytes = limit
		if limit > 0 {
			pct := int(stats.TotalBytes * 100 / limit)
			if pct > 100 {
				pct = 100
			}
			hs.QuotaPercent = pct
		}
	}

	host := h.deps.Config.BaseURL
	if parsed, err := url.Parse(h.deps.Config.BaseURL); err == nil {
		host = parsed.Host
	}

	content := &profileDashboardContent{
		Stats:        hs,
		Activity:     activity,
		RecentFiles:  fileRows,
		LargestFiles: largestRows,
		SessionCount: sessCount,
		QuotaMode:    snap.QuotaMode,
		Tokens:       tokenRows,
		BaseURL:      h.deps.Config.BaseURL,
		URLPrefix:    prefix,
		IsSelf:       isSelf,
		Host:         host,
		Username:     target.Username,
	}

	// Admin viewing another user: add extra detail.
	if !isSelf && CurrentUser(r).IsAdmin {
		idStr := strconv.FormatInt(target.ID, 10)
		auditEntries, _ := h.deps.Repo.ListAuditEntriesByTarget(ctx, "user", idStr, 25)
		aRows := make([]*auditRow, len(auditEntries))
		for i, e := range auditEntries {
			aRows[i] = &auditRow{
				ActorUsername: e.ActorUsername,
				Action:       e.Action,
				TargetType:   e.TargetType,
				TargetID:     e.TargetID,
				Details:      e.Details,
				CreatedAt:    e.CreatedAt.Format("2006-01-02 15:04:05"),
			}
		}

		pt := &profileTargetUser{
			ID:        target.ID,
			Username:  target.Username,
			IsAdmin:   target.IsAdmin,
			Disabled:  target.Disabled,
			Approved:  target.Approved,
			CreatedAt: target.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		if target.LastLoginAt != nil {
			pt.LastLoginAt = (*target.LastLoginAt).Format("2006-01-02 15:04:05")
		}
		if target.LastLoginIP != nil {
			pt.LastLoginIP = *target.LastLoginIP
		}
		if target.TOSAcceptedAt != nil {
			pt.TOSAcceptedAt = (*target.TOSAcceptedAt).Format("2006-01-02 15:04:05")
		}
		if target.PrivacyAcceptedAt != nil {
			pt.PrivacyAcceptedAt = (*target.PrivacyAcceptedAt).Format("2006-01-02 15:04:05")
		}
		pt.StorageUsed = hs.StorageUsed
		pt.FileCount = stats.FileCount

		content.TargetUser = pt
		content.AuditLog = aRows
	}

	title := target.Username + " — Gosilo"
	h.deps.Renderer.Render(w, "profile_dashboard", h.deps.pageData(w, r, title, content))
}

// --- Settings ---

type profileSettingsContent struct {
	BaseURL   string
	Host      string
	Username  string
	CreatedAt string
	Tokens    []*tokenRow
	Sessions  []*sessionRow
	QuotaMode string
	Stats     *homeStats
	URLPrefix string
	IsSelf    bool
	IsAdmin   bool

	// Admin-only when viewing another user
	TargetUser *profileTargetUser
}

func (h *profileHandler) ShowSettings(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	isSelf := IsSelf(r)
	currentUser := CurrentUser(r)
	sess := CurrentSession(r)
	ctx := r.Context()
	prefix := urlPrefix(r)

	stats, err := h.deps.Repo.GetUserStorageStats(ctx, target.ID)
	if err != nil {
		slog.Error("failed to get storage stats", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	hs := &homeStats{
		FileCount:    stats.FileCount,
		StorageUsed:  formatBytes(stats.TotalBytes),
		StorageBytes: stats.TotalBytes,
	}
	snap := h.deps.Settings.Load()
	if snap.QuotaMode == "user" {
		limit := snap.QuotaUser
		if target.StorageQuota > 0 {
			limit = target.StorageQuota
		}
		hs.StorageQuota = formatBytes(limit)
		hs.QuotaBytes = limit
		if limit > 0 {
			pct := int(stats.TotalBytes * 100 / limit)
			if pct > 100 {
				pct = 100
			}
			hs.QuotaPercent = pct
		}
	}

	tokenCount, err := h.deps.Repo.CountUserOAuthTokens(ctx, target.ID)
	if err != nil {
		slog.Error("failed to count oauth tokens", "error", err)
	}
	hs.OAuthApps = tokenCount

	// OAuth tokens.
	oauthTokens, err := h.deps.Repo.ListOAuthTokensByUserID(ctx, target.ID)
	if err != nil {
		slog.Error("failed to list oauth tokens", "error", err)
		oauthTokens = nil
	}
	tokenRows := make([]*tokenRow, len(oauthTokens))
	for i, t := range oauthTokens {
		tokenRows[i] = &tokenRow{
			TokenPrefix: t.Token[:8] + "...",
			TokenFull:   t.Token,
			ClientID:    t.ClientID,
			Scopes:      strings.ReplaceAll(t.Scopes, " ", ", "),
			CreatedAt:   t.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	// Sessions.
	sessions, err := h.deps.Auth.ListUserSessions(ctx, target.ID)
	if err != nil {
		slog.Error("failed to list user sessions", "error", err)
		sessions = nil
	}
	sessRows := make([]*sessionRow, len(sessions))
	for i, s := range sessions {
		sessRows[i] = &sessionRow{
			TokenPrefix: s.Token[:8] + "...",
			TokenFull:   s.Token,
			CreatedAt:   s.CreatedAt.Format("2006-01-02 15:04:05"),
			ExpiresAt:   s.ExpiresAt.Format("2006-01-02 15:04:05"),
			IsCurrent:   isSelf && sess != nil && s.Token == sess.Token,
		}
	}

	host := h.deps.Config.BaseURL
	if parsed, err := url.Parse(h.deps.Config.BaseURL); err == nil {
		host = parsed.Host
	}

	content := &profileSettingsContent{
		BaseURL:   h.deps.Config.BaseURL,
		Host:      host,
		Username:  target.Username,
		CreatedAt: target.CreatedAt.Format("2006-01-02 15:04:05"),
		Tokens:    tokenRows,
		Sessions:  sessRows,
		QuotaMode: snap.QuotaMode,
		Stats:     hs,
		URLPrefix: prefix,
		IsSelf:    isSelf,
		IsAdmin:   currentUser.IsAdmin,
	}

	if !isSelf && currentUser.IsAdmin {
		pt := &profileTargetUser{
			ID:        target.ID,
			Username:  target.Username,
			IsAdmin:   target.IsAdmin,
			Disabled:  target.Disabled,
			Approved:  target.Approved,
			CreatedAt: target.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		if target.LastLoginAt != nil {
			pt.LastLoginAt = (*target.LastLoginAt).Format("2006-01-02 15:04:05")
		}
		if target.LastLoginIP != nil {
			pt.LastLoginIP = *target.LastLoginIP
		}
		if target.TOSAcceptedAt != nil {
			pt.TOSAcceptedAt = (*target.TOSAcceptedAt).Format("2006-01-02 15:04:05")
		}
		if target.PrivacyAcceptedAt != nil {
			pt.PrivacyAcceptedAt = (*target.PrivacyAcceptedAt).Format("2006-01-02 15:04:05")
		}
		pt.StorageUsed = hs.StorageUsed
		pt.FileCount = stats.FileCount
		content.TargetUser = pt
	}

	title := "Settings — " + target.Username
	h.deps.Renderer.Render(w, "profile_settings", h.deps.pageData(w, r, title, content))
}

// --- Files ---

type profileFilesContent struct {
	Username        string
	CurrentPath     string
	IsRoot          bool
	IsPublic        bool
	ParentBrowseURL string
	Breadcrumbs     []breadcrumb
	Items           []*fileItem
	URLPrefix       string
	PublicWritesOff bool
}

type profileSearchContent struct {
	Query     string
	Results   []*searchResultRow
	URLPrefix string
}

func (h *profileHandler) BrowseFiles(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	prefix := urlPrefix(r)

	rawPath := r.PathValue("path")
	storagePath := "/" + rawPath

	if storagePath == "/" || strings.HasSuffix(storagePath, "/") {
		h.browseFolder(w, r, target.ID, target.Username, storagePath, prefix)
		return
	}

	h.downloadFile(w, r, target.ID, storagePath)
}

func (h *profileHandler) browseFolder(w http.ResponseWriter, r *http.Request, userID int64, username, storagePath, prefix string) {
	desc, _, err := h.deps.Storage.GetFolder(r.Context(), userID, storagePath, storage.Conditions{})
	if err != nil {
		slog.Error("failed to list folder", "error", err, "path", storagePath)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var items []*fileItem
	for name, item := range desc.Items {
		isFolder := strings.HasSuffix(name, "/")
		fi := &fileItem{
			Name:      name,
			Path:      storagePath + name,
			BrowseURL: prefix + "/files" + storagePath + name,
			IsFolder:  isFolder,
			ETag:      item.ETag,
		}
		if isFolder {
			subtreeSize, err := h.deps.Repo.GetSubtreeSize(r.Context(), userID, storagePath+name)
			if err != nil {
				slog.Error("failed to get subtree size", "error", err, "path", storagePath+name)
			} else {
				fi.Size = formatBytes(subtreeSize)
			}
		} else {
			fi.ContentType = item.ContentType
			if item.ContentLength != nil {
				fi.Size = formatBytes(*item.ContentLength)
			}
		}
		items = append(items, fi)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].IsFolder != items[j].IsFolder {
			return items[i].IsFolder
		}
		return items[i].Name < items[j].Name
	})

	breadcrumbs := profileBreadcrumbs(storagePath, prefix)

	parentURL := prefix + "/files/"
	if len(breadcrumbs) > 1 {
		parentURL = breadcrumbs[len(breadcrumbs)-2].Path
	}

	publicWritesOff := strings.HasPrefix(storagePath, "/public/") && h.deps.Settings.Load().PublicWrites == "off"

	h.deps.Renderer.Render(w, "profile_files", h.deps.pageData(w, r, "Files — "+storagePath, profileFilesContent{
		Username:        username,
		CurrentPath:     storagePath,
		IsRoot:          storagePath == "/",
		IsPublic:        strings.HasPrefix(storagePath, "/public/") || storagePath == "/public/",
		ParentBrowseURL: parentURL,
		Breadcrumbs:     breadcrumbs,
		Items:           items,
		URLPrefix:       prefix,
		PublicWritesOff: publicWritesOff,
	}))
}

func (h *profileHandler) downloadFile(w http.ResponseWriter, r *http.Request, userID int64, storagePath string) {
	result, err := h.deps.Storage.GetDocument(r.Context(), userID, storagePath, storage.Conditions{})
	if err != nil {
		if err == storage.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		slog.Error("failed to get document", "error", err, "path", storagePath)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer result.Content.Close()

	filename := path.Base(storagePath)
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", result.ContentLength))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	io.Copy(w, result.Content)
}

func (h *profileHandler) SearchFiles(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	prefix := urlPrefix(r)

	query := r.URL.Query().Get("q")
	var results []*searchResultRow

	if query != "" {
		nodes, err := h.deps.Repo.SearchUserNodes(r.Context(), target.ID, query, 50)
		if err != nil {
			slog.Error("failed to search nodes", "error", err)
		} else {
			results = make([]*searchResultRow, len(nodes))
			for i, n := range nodes {
				results[i] = &searchResultRow{
					Name:        path.Base(n.Path),
					Path:        n.Path,
					BrowseURL:   prefix + "/files" + path.Dir(n.Path) + "/#file-" + url.PathEscape(path.Base(n.Path)),
					Size:        formatBytes(n.ContentLength),
					ContentType: n.ContentType,
				}
			}
		}
	}

	h.deps.Renderer.Render(w, "profile_files_search", h.deps.pageData(w, r, "Search Files", profileSearchContent{
		Query:     query,
		Results:   results,
		URLPrefix: prefix,
	}))
}

// --- File POST handlers ---

func (h *profileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	prefix := urlPrefix(r)

	deletePath := r.FormValue("path")
	if deletePath == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if strings.HasSuffix(deletePath, "/") {
		count, err := h.deps.Storage.DeleteFolder(r.Context(), target.ID, deletePath)
		if err != nil {
			slog.Error("failed to delete folder", "error", err, "path", deletePath)
			ui.SetFlashError(w, "Delete failed: "+err.Error())
		} else {
			name := strings.TrimSuffix(path.Base(strings.TrimSuffix(deletePath, "/")), "/")
			ui.SetFlash(w, fmt.Sprintf("Deleted %s/ (%d files)", name, count))
		}
	} else {
		_, err := h.deps.Storage.DeleteDocument(r.Context(), target.ID, deletePath, storage.Conditions{})
		if err != nil {
			slog.Error("failed to delete document", "error", err, "path", deletePath)
			ui.SetFlashError(w, "Delete failed: "+err.Error())
		} else {
			name := path.Base(deletePath)
			ui.SetFlash(w, fmt.Sprintf("Deleted %s", name))
		}
	}

	parentFolder := path.Dir(strings.TrimSuffix(deletePath, "/"))
	if !strings.HasSuffix(parentFolder, "/") {
		parentFolder += "/"
	}
	http.Redirect(w, r, prefix+"/files"+parentFolder, http.StatusSeeOther)
}

func (h *profileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	prefix := urlPrefix(r)

	maxSize := h.deps.Settings.Load().MaxUploadSize
	if maxSize <= 0 {
		maxSize = 50 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	if err := r.ParseMultipartForm(maxSize); err != nil {
		ui.SetFlashError(w, "Upload failed: file too large or invalid form data.")
		http.Redirect(w, r, prefix+"/files/", http.StatusSeeOther)
		return
	}

	folder := r.FormValue("folder")
	if folder == "" {
		folder = "/"
	}
	if !strings.HasSuffix(folder, "/") {
		folder += "/"
	}

	if folder == "/" {
		ui.SetFlashError(w, "Cannot upload to root. Please create or select a module first.")
		http.Redirect(w, r, prefix+"/files/", http.StatusSeeOther)
		return
	}

	if strings.HasPrefix(folder, "/public/") && h.deps.Settings.Load().PublicWrites == "off" {
		ui.SetFlashError(w, "Uploads to public paths are disabled by the server operator.")
		http.Redirect(w, r, prefix+"/files"+folder, http.StatusSeeOther)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		ui.SetFlashError(w, "No files selected.")
		http.Redirect(w, r, prefix+"/files"+folder, http.StatusSeeOther)
		return
	}

	uploaded := 0
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			slog.Error("failed to open uploaded file", "error", err, "filename", fh.Filename)
			continue
		}

		contentType := detectContentType(fh)
		docPath := folder + fh.Filename
		_, err = h.deps.Storage.PutDocument(r.Context(), target.ID, docPath, f, contentType, storage.Conditions{})
		f.Close()
		if err != nil {
			slog.Error("failed to store uploaded file", "error", err, "path", docPath)
			continue
		}
		uploaded++
	}

	if uploaded == len(files) {
		ui.SetFlash(w, fmt.Sprintf("Uploaded %d file(s).", uploaded))
	} else {
		ui.SetFlash(w, fmt.Sprintf("Uploaded %d of %d file(s). Some failed.", uploaded, len(files)))
	}
	http.Redirect(w, r, prefix+"/files"+folder, http.StatusSeeOther)
}

func (h *profileHandler) BulkDeleteFiles(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	prefix := urlPrefix(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	paths := r.Form["paths[]"]
	if len(paths) == 0 {
		ui.SetFlash(w, "No items selected.")
		http.Redirect(w, r, prefix+"/files/", http.StatusSeeOther)
		return
	}

	deleted := 0
	for _, p := range paths {
		if strings.HasSuffix(p, "/") {
			_, err := h.deps.Storage.DeleteFolder(r.Context(), target.ID, p)
			if err != nil {
				slog.Error("bulk delete folder failed", "error", err, "path", p)
				continue
			}
		} else {
			_, err := h.deps.Storage.DeleteDocument(r.Context(), target.ID, p, storage.Conditions{})
			if err != nil {
				slog.Error("bulk delete file failed", "error", err, "path", p)
				continue
			}
		}
		deleted++
	}

	ui.SetFlash(w, fmt.Sprintf("Deleted %d item(s).", deleted))

	referer := r.Header.Get("Referer")
	if referer != "" {
		http.Redirect(w, r, referer, http.StatusSeeOther)
	} else {
		http.Redirect(w, r, prefix+"/files/", http.StatusSeeOther)
	}
}

func (h *profileHandler) CreateModule(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	prefix := urlPrefix(r)

	name := strings.TrimSpace(r.FormValue("name"))
	if !moduleNameRe.MatchString(name) {
		ui.SetFlashError(w, "Invalid module name. Use only letters, numbers, hyphens, and underscores.")
		http.Redirect(w, r, prefix+"/files/", http.StatusSeeOther)
		return
	}

	folderPath := "/" + name + "/"
	err := h.deps.Storage.CreateFolder(r.Context(), target.ID, folderPath)
	if err != nil {
		if err == storage.ErrConflict {
			ui.SetFlashError(w, fmt.Sprintf("Module %q already exists.", name))
		} else {
			slog.Error("failed to create module", "error", err, "name", name)
			ui.SetFlashError(w, "Failed to create module.")
		}
		http.Redirect(w, r, prefix+"/files/", http.StatusSeeOther)
		return
	}

	ui.SetFlash(w, fmt.Sprintf("Created module %s/", name))
	http.Redirect(w, r, prefix+"/files/", http.StatusSeeOther)
}

func (h *profileHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	prefix := urlPrefix(r)

	parent := r.FormValue("parent")
	if parent == "" || !strings.HasSuffix(parent, "/") {
		ui.SetFlashError(w, "Invalid parent folder.")
		http.Redirect(w, r, prefix+"/files/", http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if !folderNameRe.MatchString(name) {
		ui.SetFlashError(w, "Invalid folder name. Use only letters, numbers, hyphens, underscores, and dots.")
		http.Redirect(w, r, prefix+"/files"+parent, http.StatusSeeOther)
		return
	}

	folderPath := parent + name + "/"
	err := h.deps.Storage.CreateFolder(r.Context(), target.ID, folderPath)
	if err != nil {
		if err == storage.ErrConflict {
			ui.SetFlashError(w, fmt.Sprintf("Folder %q already exists.", name))
		} else {
			slog.Error("failed to create folder", "error", err, "path", folderPath)
			ui.SetFlashError(w, "Failed to create folder.")
		}
		http.Redirect(w, r, prefix+"/files"+parent, http.StatusSeeOther)
		return
	}

	ui.SetFlash(w, fmt.Sprintf("Created folder %s/", name))
	http.Redirect(w, r, prefix+"/files"+parent, http.StatusSeeOther)
}

// --- Settings POST handlers ---

func (h *profileHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if !IsSelf(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	target := TargetUser(r)
	prefix := urlPrefix(r)
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if !h.deps.Auth.CheckPassword(target, currentPassword) {
		ui.SetFlashError(w, "Current password is incorrect.")
		http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
		return
	}

	if msg := validatePassword(newPassword); msg != "" {
		ui.SetFlashError(w, msg)
		http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
		return
	}

	if newPassword != confirmPassword {
		ui.SetFlashError(w, "New passwords do not match.")
		http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
		return
	}

	if err := h.deps.Auth.UpdatePassword(r.Context(), target.ID, newPassword); err != nil {
		slog.Error("failed to update password", "error", err)
		ui.SetFlashError(w, "An error occurred. Please try again.")
		http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
		return
	}

	// Invalidate all sessions (including current) and issue a fresh one.
	if err := h.deps.Auth.TerminateAllSessions(r.Context(), target.ID); err != nil {
		slog.Error("failed to terminate sessions after password change", "error", err)
	}
	newSess, err := h.deps.Auth.CreateSession(r.Context(), target.ID, ClientIP(r))
	if err != nil {
		slog.Error("failed to create new session after password change", "error", err)
	} else {
		auth.SetSessionCookie(w, newSess.Token, h.deps.SecureCookies)
	}

	ui.SetFlash(w, "Password changed successfully.")
	http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
}

func (h *profileHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	prefix := urlPrefix(r)
	token := r.PathValue("token")

	t, err := h.deps.Repo.GetOAuthToken(r.Context(), token)
	if err != nil {
		slog.Error("failed to get oauth token", "error", err)
		ui.SetFlashError(w, "Failed to revoke token.")
		http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
		return
	}
	if t == nil || t.UserID != target.ID {
		ui.SetFlashError(w, "Token not found.")
		http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
		return
	}

	if err := h.deps.Repo.DeleteOAuthToken(r.Context(), token); err != nil {
		slog.Error("failed to revoke oauth token", "error", err)
		ui.SetFlashError(w, "Failed to revoke token.")
		http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
		return
	}
	_ = h.deps.Repo.DeleteRefreshTokenByAccessToken(r.Context(), token)

	ui.SetFlash(w, "Token revoked.")
	http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
}

func (h *profileHandler) TerminateSession(w http.ResponseWriter, r *http.Request) {
	target := TargetUser(r)
	prefix := urlPrefix(r)
	token := r.PathValue("token")

	if IsSelf(r) {
		sess := CurrentSession(r)
		if sess != nil && token == sess.Token {
			ui.SetFlashError(w, "You cannot terminate your current session.")
			http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
			return
		}

		// Verify the session belongs to the target user.
		targetSess, err := h.deps.Auth.GetSession(r.Context(), token)
		if err != nil || targetSess == nil || targetSess.UserID != target.ID {
			ui.SetFlashError(w, "Session not found.")
			http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
			return
		}
	}

	if err := h.deps.Auth.TerminateSession(r.Context(), token); err != nil {
		slog.Error("failed to terminate session", "error", err)
		ui.SetFlashError(w, "Failed to terminate session.")
	} else {
		if !IsSelf(r) {
			h.audit(r, "session.terminated", "session", token[:8]+"...", "")
		}
		ui.SetFlash(w, "Session terminated.")
	}

	http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
}

func (h *profileHandler) TerminateAllSessions(w http.ResponseWriter, r *http.Request) {
	if IsSelf(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	target := TargetUser(r)
	prefix := urlPrefix(r)

	if err := h.deps.Auth.TerminateAllSessions(r.Context(), target.ID); err != nil {
		slog.Error("failed to terminate all sessions", "error", err)
		ui.SetFlashError(w, "Failed to terminate sessions.")
	} else {
		idStr := strconv.FormatInt(target.ID, 10)
		h.audit(r, "session.terminated", "user", idStr, "all sessions")
		ui.SetFlash(w, "All sessions terminated.")
	}

	http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
}

// --- Admin actions on profile ---

func (h *profileHandler) ToggleAdmin(w http.ResponseWriter, r *http.Request) {
	if IsSelf(r) || !CurrentUser(r).IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	target := TargetUser(r)
	newAdmin := !target.IsAdmin
	if err := h.deps.Auth.ToggleAdmin(r.Context(), target.ID, newAdmin); err != nil {
		slog.Error("failed to toggle admin", "error", err)
		ui.SetFlashError(w, "Failed to toggle admin status.")
		http.Redirect(w, r, urlPrefix(r)+"/settings", http.StatusSeeOther)
		return
	}

	// Force re-login so the session reflects the new privilege level.
	if err := h.deps.Auth.TerminateAllSessions(r.Context(), target.ID); err != nil {
		slog.Error("failed to terminate sessions after admin toggle", "error", err)
	}

	idStr := strconv.FormatInt(target.ID, 10)
	h.audit(r, "user.admin_toggled", "user", idStr, fmt.Sprintf("admin=%v", newAdmin))

	if newAdmin {
		ui.SetFlash(w, fmt.Sprintf("%s is now an admin.", target.Username))
	} else {
		ui.SetFlash(w, fmt.Sprintf("%s is no longer an admin.", target.Username))
	}
	http.Redirect(w, r, urlPrefix(r)+"/settings", http.StatusSeeOther)
}

func (h *profileHandler) ToggleDisabled(w http.ResponseWriter, r *http.Request) {
	if IsSelf(r) || !CurrentUser(r).IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	target := TargetUser(r)
	newDisabled := !target.Disabled
	if err := h.deps.Auth.SetDisabled(r.Context(), target.ID, newDisabled); err != nil {
		slog.Error("failed to toggle disabled", "error", err)
		ui.SetFlashError(w, "Failed to toggle disabled status.")
		http.Redirect(w, r, urlPrefix(r)+"/settings", http.StatusSeeOther)
		return
	}

	idStr := strconv.FormatInt(target.ID, 10)
	if newDisabled {
		if err := h.deps.Auth.TerminateAllSessions(r.Context(), target.ID); err != nil {
			slog.Error("failed to terminate sessions on disable", "error", err)
		}
		_ = h.deps.Repo.DeleteOAuthTokensByUserID(r.Context(), target.ID)
		_ = h.deps.Repo.DeleteRefreshTokensByUserID(r.Context(), target.ID)
		h.audit(r, "user.disabled", "user", idStr, target.Username)
		ui.SetFlash(w, fmt.Sprintf("%s has been disabled.", target.Username))
	} else {
		// Clear any stale sessions from before the account was disabled.
		if err := h.deps.Auth.TerminateAllSessions(r.Context(), target.ID); err != nil {
			slog.Error("failed to terminate sessions on enable", "error", err)
		}
		h.audit(r, "user.enabled", "user", idStr, target.Username)
		ui.SetFlash(w, fmt.Sprintf("%s has been enabled.", target.Username))
	}
	http.Redirect(w, r, urlPrefix(r)+"/settings", http.StatusSeeOther)
}

func (h *profileHandler) ApproveUser(w http.ResponseWriter, r *http.Request) {
	if IsSelf(r) || !CurrentUser(r).IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	target := TargetUser(r)
	if err := h.deps.Auth.SetApproved(r.Context(), target.ID, true); err != nil {
		slog.Error("failed to approve user", "error", err)
		ui.SetFlashError(w, "Failed to approve user.")
		http.Redirect(w, r, urlPrefix(r)+"/", http.StatusSeeOther)
		return
	}

	idStr := strconv.FormatInt(target.ID, 10)
	h.audit(r, "user.approved", "user", idStr, target.Username)
	ui.SetFlash(w, fmt.Sprintf("%s has been approved.", target.Username))
	http.Redirect(w, r, urlPrefix(r)+"/", http.StatusSeeOther)
}

func (h *profileHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if IsSelf(r) || !CurrentUser(r).IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	target := TargetUser(r)
	idStr := strconv.FormatInt(target.ID, 10)

	if err := h.deps.Auth.DeleteUser(r.Context(), target.ID); err != nil {
		slog.Error("failed to delete user", "error", err)
		ui.SetFlashError(w, "Failed to delete user.")
		http.Redirect(w, r, urlPrefix(r)+"/settings", http.StatusSeeOther)
		return
	}

	h.audit(r, "user.deleted", "user", idStr, target.Username)
	ui.SetFlash(w, "User deleted.")
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *profileHandler) SetQuota(w http.ResponseWriter, r *http.Request) {
	if IsSelf(r) || !CurrentUser(r).IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	target := TargetUser(r)
	prefix := urlPrefix(r)
	idStr := strconv.FormatInt(target.ID, 10)

	quotaStr := r.FormValue("quota")
	var quotaBytes int64
	if quotaStr != "" && quotaStr != "0" {
		parsed, err := config.ParseByteSize(quotaStr)
		if err != nil {
			ui.SetFlashError(w, fmt.Sprintf("Invalid quota value: %s", quotaStr))
			http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
			return
		}
		quotaBytes = parsed
	}

	if err := h.deps.Repo.UpdateUserQuota(r.Context(), target.ID, quotaBytes); err != nil {
		slog.Error("failed to update user quota", "error", err)
		ui.SetFlashError(w, "Failed to update quota.")
		http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
		return
	}

	h.audit(r, "user.quota_changed", "user", idStr, quotaStr)

	if quotaBytes > 0 {
		ui.SetFlash(w, fmt.Sprintf("Quota updated to %s.", formatBytes(quotaBytes)))
	} else {
		ui.SetFlash(w, "Quota reset to server default.")
	}
	http.Redirect(w, r, prefix+"/settings", http.StatusSeeOther)
}

// audit logs admin actions to the audit trail.
func (h *profileHandler) audit(r *http.Request, action, targetType, targetID, details string) {
	user := CurrentUser(r)
	if user == nil {
		return
	}
	if err := h.deps.Repo.InsertAuditEntry(r.Context(), user.ID, action, targetType, targetID, details); err != nil {
		slog.Error("failed to write audit log", "error", err, "action", action)
	}
}

// profileBreadcrumbs creates navigation breadcrumbs with the user-scoped URL prefix.
func profileBreadcrumbs(storagePath, prefix string) []breadcrumb {
	var crumbs []breadcrumb
	trimmed := strings.Trim(storagePath, "/")
	if trimmed == "" {
		return crumbs
	}
	parts := strings.Split(trimmed, "/")
	for i, part := range parts {
		href := prefix + "/files/" + strings.Join(parts[:i+1], "/") + "/"
		crumbs = append(crumbs, breadcrumb{Name: part, Path: href})
	}
	return crumbs
}

// detectContentType determines a MIME type from a multipart file header.
func detectContentType(fh *multipart.FileHeader) string {
	contentType := fh.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		ext := strings.ToLower(path.Ext(fh.Filename))
		if detected := mime.TypeByExtension(ext); detected != "" {
			contentType = detected
		} else if detected, ok := fallbackMIME[ext]; ok {
			contentType = detected
		} else if ext == "" {
			if detected, ok := knownFilenames[strings.ToLower(fh.Filename)]; ok {
				contentType = detected
			}
		}
	}
	return contentType
}
