package web

import (
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"gosilo/internal/db"
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

type filesHandler struct {
	deps *UIDeps
}

// FilesHandler creates a new handler for the file browser.
func FilesHandler(deps *UIDeps) *filesHandler {
	return &filesHandler{deps: deps}
}

type filesContent struct {
	Username        string
	CurrentPath     string
	IsRoot          bool
	IsPublic        bool
	ParentBrowseURL string
	Breadcrumbs     []breadcrumb
	Items           []*fileItem
	PublicWritesOff bool
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

// Browse handles GET /files/{path...} — folder listing or file download.
func (h *filesHandler) Browse(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)

	rawPath := r.PathValue("path")
	storagePath := "/" + rawPath

	// If path is empty or ends with "/" it's a folder.
	if storagePath == "/" || strings.HasSuffix(storagePath, "/") {
		h.browseFolder(w, r, user.ID, user.Username, storagePath)
		return
	}

	// Otherwise it's a file download.
	h.downloadFile(w, r, user.ID, storagePath)
}

func (h *filesHandler) browseFolder(w http.ResponseWriter, r *http.Request, userID int64, username, storagePath string) {
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
			BrowseURL: "/files" + storagePath + name,
			IsFolder:  isFolder,
			ETag:      item.ETag,
		}
		if isFolder {
			subtreeSize, err := db.GetSubtreeSize(r.Context(), h.deps.DB, userID, storagePath+name)
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

	// Sort: folders first, then alphabetical.
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsFolder != items[j].IsFolder {
			return items[i].IsFolder
		}
		return items[i].Name < items[j].Name
	})

	breadcrumbs := buildBreadcrumbs(storagePath)

	parentURL := "/files/"
	if len(breadcrumbs) > 1 {
		parentURL = breadcrumbs[len(breadcrumbs)-2].Path
	}

	publicWritesOff := strings.HasPrefix(storagePath, "/public/") && h.deps.Settings.Load().PublicWrites == "off"

	h.deps.Renderer.Render(w, "files", h.deps.pageData(w, r, "Files — "+storagePath, filesContent{
		Username:        username,
		CurrentPath:     storagePath,
		IsRoot:          storagePath == "/",
		IsPublic:        strings.HasPrefix(storagePath, "/public/") || storagePath == "/public/",
		ParentBrowseURL: parentURL,
		Breadcrumbs:     breadcrumbs,
		Items:           items,
		PublicWritesOff: publicWritesOff,
	}))
}

func (h *filesHandler) downloadFile(w http.ResponseWriter, r *http.Request, userID int64, storagePath string) {
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

// Delete handles POST /files/delete for both files and folders.
func (h *filesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)

	deletePath := r.FormValue("path")
	if deletePath == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if strings.HasSuffix(deletePath, "/") {
		// Folder delete (recursive).
		count, err := h.deps.Storage.DeleteFolder(r.Context(), user.ID, deletePath)
		if err != nil {
			slog.Error("failed to delete folder", "error", err, "path", deletePath)
			ui.SetFlashError(w, "Delete failed: "+err.Error())
		} else {
			name := strings.TrimSuffix(path.Base(strings.TrimSuffix(deletePath, "/")), "/")
			ui.SetFlash(w, fmt.Sprintf("Deleted %s/ (%d files)", name, count))
		}
	} else {
		// Single file delete.
		_, err := h.deps.Storage.DeleteDocument(r.Context(), user.ID, deletePath, storage.Conditions{})
		if err != nil {
			slog.Error("failed to delete document", "error", err, "path", deletePath)
			ui.SetFlashError(w, "Delete failed: "+err.Error())
		} else {
			name := path.Base(deletePath)
			ui.SetFlash(w, fmt.Sprintf("Deleted %s", name))
		}
	}

	// Redirect to parent folder.
	parentFolder := path.Dir(strings.TrimSuffix(deletePath, "/"))
	if !strings.HasSuffix(parentFolder, "/") {
		parentFolder += "/"
	}
	http.Redirect(w, r, "/files"+parentFolder, http.StatusSeeOther)
}

// Upload handles POST /files/upload — multipart file upload.
func (h *filesHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)

	// Limit request body size.
	maxSize := h.deps.Settings.Load().MaxUploadSize
	if maxSize <= 0 {
		maxSize = 50 << 20 // 50MB default
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	if err := r.ParseMultipartForm(maxSize); err != nil {
		ui.SetFlashError(w, "Upload failed: file too large or invalid form data.")
		http.Redirect(w, r, "/files/", http.StatusSeeOther)
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
		http.Redirect(w, r, "/files/", http.StatusSeeOther)
		return
	}

	if strings.HasPrefix(folder, "/public/") && h.deps.Settings.Load().PublicWrites == "off" {
		ui.SetFlashError(w, "Uploads to public paths are disabled by the server operator.")
		http.Redirect(w, r, "/files"+folder, http.StatusSeeOther)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		ui.SetFlashError(w, "No files selected.")
		http.Redirect(w, r, "/files"+folder, http.StatusSeeOther)
		return
	}

	uploaded := 0
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			slog.Error("failed to open uploaded file", "error", err, "filename", fh.Filename)
			continue
		}

		contentType := fh.Header.Get("Content-Type")
		if contentType == "" || contentType == "application/octet-stream" {
			ext := strings.ToLower(path.Ext(fh.Filename))
			if detected := mime.TypeByExtension(ext); detected != "" {
				contentType = detected
			} else if detected, ok := fallbackMIME[ext]; ok {
				contentType = detected
			} else if ext == "" {
				// Extensionless known filenames (e.g. Dockerfile, Makefile).
				if detected, ok := knownFilenames[strings.ToLower(fh.Filename)]; ok {
					contentType = detected
				}
			}
		}

		docPath := folder + fh.Filename
		_, err = h.deps.Storage.PutDocument(r.Context(), user.ID, docPath, f, contentType, storage.Conditions{})
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
	http.Redirect(w, r, "/files"+folder, http.StatusSeeOther)
}

// CreateModule handles POST /files/create-module — create a top-level module folder.
func (h *filesHandler) CreateModule(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)

	name := strings.TrimSpace(r.FormValue("name"))
	if !moduleNameRe.MatchString(name) {
		ui.SetFlashError(w, "Invalid module name. Use only letters, numbers, hyphens, and underscores.")
		http.Redirect(w, r, "/files/", http.StatusSeeOther)
		return
	}

	folderPath := "/" + name + "/"
	err := h.deps.Storage.CreateFolder(r.Context(), user.ID, folderPath)
	if err != nil {
		if err == storage.ErrConflict {
			ui.SetFlashError(w, fmt.Sprintf("Module %q already exists.", name))
		} else {
			slog.Error("failed to create module", "error", err, "name", name)
			ui.SetFlashError(w, "Failed to create module.")
		}
		http.Redirect(w, r, "/files/", http.StatusSeeOther)
		return
	}

	ui.SetFlash(w, fmt.Sprintf("Created module %s/", name))
	http.Redirect(w, r, "/files/", http.StatusSeeOther)
}

// CreateFolder handles POST /files/create-folder — create a sub-folder within an existing folder.
func (h *filesHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)

	parent := r.FormValue("parent")
	if parent == "" || !strings.HasSuffix(parent, "/") {
		ui.SetFlashError(w, "Invalid parent folder.")
		http.Redirect(w, r, "/files/", http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if !folderNameRe.MatchString(name) {
		ui.SetFlashError(w, "Invalid folder name. Use only letters, numbers, hyphens, underscores, and dots.")
		http.Redirect(w, r, "/files"+parent, http.StatusSeeOther)
		return
	}

	folderPath := parent + name + "/"
	err := h.deps.Storage.CreateFolder(r.Context(), user.ID, folderPath)
	if err != nil {
		if err == storage.ErrConflict {
			ui.SetFlashError(w, fmt.Sprintf("Folder %q already exists.", name))
		} else {
			slog.Error("failed to create folder", "error", err, "path", folderPath)
			ui.SetFlashError(w, "Failed to create folder.")
		}
		http.Redirect(w, r, "/files"+parent, http.StatusSeeOther)
		return
	}

	ui.SetFlash(w, fmt.Sprintf("Created folder %s/", name))
	http.Redirect(w, r, "/files"+parent, http.StatusSeeOther)
}

type searchContent struct {
	Query   string
	Results []*searchResultRow
}

type searchResultRow struct {
	Name        string
	Path        string
	BrowseURL   string
	Size        string
	ContentType string
}

// Search handles GET /files/search — file search.
func (h *filesHandler) Search(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)

	query := r.URL.Query().Get("q")
	var results []*searchResultRow

	if query != "" {
		nodes, err := db.SearchUserNodes(r.Context(), h.deps.DB, user.ID, query, 50)
		if err != nil {
			slog.Error("failed to search nodes", "error", err)
		} else {
			results = make([]*searchResultRow, len(nodes))
			for i, n := range nodes {
				results[i] = &searchResultRow{
					Name:        path.Base(n.Path),
					Path:        n.Path,
					BrowseURL:   "/files" + path.Dir(n.Path) + "/#file-" + url.PathEscape(path.Base(n.Path)),
					Size:        formatBytes(n.ContentLength),
					ContentType: n.ContentType,
				}
			}
		}
	}

	h.deps.Renderer.Render(w, "files_search", h.deps.pageData(w, r, "Search Files", searchContent{
		Query:   query,
		Results: results,
	}))
}

// BulkDelete handles POST /files/bulk-delete — delete multiple files/folders.
func (h *filesHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	paths := r.Form["paths[]"]
	if len(paths) == 0 {
		ui.SetFlash(w, "No items selected.")
		http.Redirect(w, r, "/files/", http.StatusSeeOther)
		return
	}

	deleted := 0
	for _, p := range paths {
		if strings.HasSuffix(p, "/") {
			_, err := h.deps.Storage.DeleteFolder(r.Context(), user.ID, p)
			if err != nil {
				slog.Error("bulk delete folder failed", "error", err, "path", p)
				continue
			}
		} else {
			_, err := h.deps.Storage.DeleteDocument(r.Context(), user.ID, p, storage.Conditions{})
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
		http.Redirect(w, r, "/files/", http.StatusSeeOther)
	}
}

// buildBreadcrumbs creates navigation breadcrumbs from a storage path.
func buildBreadcrumbs(storagePath string) []breadcrumb {
	var crumbs []breadcrumb
	// Remove leading/trailing slashes and split.
	trimmed := strings.Trim(storagePath, "/")
	if trimmed == "" {
		return crumbs
	}
	parts := strings.Split(trimmed, "/")
	for i, part := range parts {
		href := "/files/" + strings.Join(parts[:i+1], "/") + "/"
		crumbs = append(crumbs, breadcrumb{Name: part, Path: href})
	}
	return crumbs
}
