package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"sort"
	"strings"

	"gosilo/internal/db"
	"gosilo/internal/storage"
	"gosilo/internal/ui"
)

type filesHandler struct {
	deps *UIDeps
}

// FilesHandler creates a new handler for the file browser.
func FilesHandler(deps *UIDeps) *filesHandler {
	return &filesHandler{deps: deps}
}

type filesContent struct {
	Username    string
	CurrentPath string
	Breadcrumbs []breadcrumb
	Items       []*fileItem
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
				fi.Size = humanSize(subtreeSize)
			}
		} else {
			fi.ContentType = item.ContentType
			if item.ContentLength != nil {
				fi.Size = humanSize(*item.ContentLength)
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

	h.deps.Renderer.Render(w, "files", ui.PageData{
		Title:       "Files — " + storagePath,
		CurrentUser: userInfo(CurrentUser(r)),
		CSRFToken:   CSRFToken(r),
		Flash:       GetFlash(w, r),
		Content: filesContent{
			Username:    username,
			CurrentPath: storagePath,
			Breadcrumbs: breadcrumbs,
			Items:       items,
		},
	})
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
	if !ValidateCSRF(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
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
			SetFlash(w, "Delete failed: "+err.Error())
		} else {
			name := strings.TrimSuffix(path.Base(strings.TrimSuffix(deletePath, "/")), "/")
			SetFlash(w, fmt.Sprintf("Deleted %s/ (%d files)", name, count))
		}
	} else {
		// Single file delete.
		_, err := h.deps.Storage.DeleteDocument(r.Context(), user.ID, deletePath, storage.Conditions{})
		if err != nil {
			slog.Error("failed to delete document", "error", err, "path", deletePath)
			SetFlash(w, "Delete failed: "+err.Error())
		} else {
			name := path.Base(deletePath)
			SetFlash(w, fmt.Sprintf("Deleted %s", name))
		}
	}

	// Redirect to parent folder.
	parentFolder := path.Dir(strings.TrimSuffix(deletePath, "/"))
	if !strings.HasSuffix(parentFolder, "/") {
		parentFolder += "/"
	}
	http.Redirect(w, r, "/files"+parentFolder, http.StatusSeeOther)
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

// humanSize formats bytes into a human-readable string.
func humanSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
