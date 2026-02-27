package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"gosilo/internal/db"
	"gosilo/internal/storage"
)

// storageAudit logs a storage operation to the audit log.
func storageAudit(r *http.Request, database *sql.DB, userID int64, action, path string) {
	db.Audit(r.Context(), database, userID, action, "storage", path, "")
}

// Storage handles GET/PUT/DELETE/HEAD requests on /storage/{user}/{path...}.
func Storage(database *sql.DB, svc *storage.Service, maxUploadSize func() int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := r.PathValue("user")
		pathVal := r.PathValue("path")
		storagePath := "/" + pathVal

		// Reject dangerous path components.
		if strings.Contains(storagePath, "..") || strings.Contains(storagePath, "//") {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		isFolder := strings.HasSuffix(storagePath, "/")
		isPublic := isPublicPath(storagePath)
		isReadOnly := r.Method == http.MethodGet || r.Method == http.MethodHead

		// Public documents are readable without auth (not folders, per spec).
		needsAuth := !(isPublic && isReadOnly && !isFolder)

		var tokenUserID int64
		if needsAuth {
			bearer := extractBearer(r)
			if bearer == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			oauthToken, err := db.GetOAuthToken(r.Context(), database, bearer)
			if err != nil {
				slog.Error("lookup oauth token", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if oauthToken == nil {
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			isWrite := r.Method == http.MethodPut || r.Method == http.MethodDelete
			if !CheckScope(oauthToken.Scopes, storagePath, isWrite) {
				http.Error(w, "insufficient scope", http.StatusForbidden)
				return
			}
			tokenUserID = oauthToken.UserID
		}

		// Look up user by username.
		user, err := db.GetUserByUsername(r.Context(), database, username)
		if err != nil {
			slog.Error("lookup user", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		// Reject disabled or unapproved accounts.
		if needsAuth && (user.Disabled || !user.Approved) {
			http.Error(w, "account disabled", http.StatusForbidden)
			return
		}

		// Ownership check: token must belong to the path's user.
		if needsAuth && tokenUserID != user.ID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		cond := parseConditions(r)

		switch r.Method {
		case http.MethodGet:
			if isFolder {
				handleGetFolder(w, r, svc, user.ID, storagePath, cond)
			} else {
				handleGetDocument(w, r, svc, user.ID, storagePath, cond)
			}
		case http.MethodHead:
			if isFolder {
				handleGetFolder(w, r, svc, user.ID, storagePath, cond)
			} else {
				handleHeadDocument(w, r, svc, user.ID, storagePath, cond)
			}
		case http.MethodPut:
			if isFolder {
				http.Error(w, "cannot PUT a folder", http.StatusBadRequest)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize())
			putErr := handlePutDocumentErr(w, r, svc, user.ID, storagePath, cond)
			if putErr != nil && errors.Is(putErr, storage.ErrContentRejected) {
				storageAudit(r, database, user.ID, "storage.upload_rejected", storagePath)
			} else if putErr == nil {
				storageAudit(r, database, user.ID, "storage.put", storagePath)
			}
		case http.MethodDelete:
			handleDeleteDocument(w, r, svc, user.ID, storagePath, cond)
			storageAudit(r, database, user.ID, "storage.delete", storagePath)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func parseConditions(r *http.Request) storage.Conditions {
	var cond storage.Conditions
	if im := r.Header.Get("If-Match"); im != "" {
		cond.IfMatch = storage.UnquoteETag(im)
	}
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		if inm == "*" {
			cond.IfNoneMatch = "*"
		} else {
			cond.IfNoneMatch = storage.UnquoteETag(inm)
		}
	}
	return cond
}

func handleGetDocument(w http.ResponseWriter, r *http.Request, svc *storage.Service, userID int64, path string, cond storage.Conditions) {
	result, err := svc.GetDocument(r.Context(), userID, path, cond)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer result.Content.Close()

	w.Header().Set("ETag", storage.QuoteETag(result.ETag))
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", result.ContentLength))
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, result.Content)
}

func handleHeadDocument(w http.ResponseWriter, r *http.Request, svc *storage.Service, userID int64, path string, cond storage.Conditions) {
	result, err := svc.HeadDocument(r.Context(), userID, path, cond)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("ETag", storage.QuoteETag(result.ETag))
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", result.ContentLength))
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
}

func handleGetFolder(w http.ResponseWriter, r *http.Request, svc *storage.Service, userID int64, path string, cond storage.Conditions) {
	desc, etag, err := svc.GetFolder(r.Context(), userID, path, cond)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	body, err := json.Marshal(desc)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("ETag", storage.QuoteETag(etag))
	w.Header().Set("Content-Type", "application/ld+json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func handlePutDocument(w http.ResponseWriter, r *http.Request, svc *storage.Service, userID int64, path string, cond storage.Conditions) {
	handlePutDocumentErr(w, r, svc, userID, path, cond)
}

func handlePutDocumentErr(w http.ResponseWriter, r *http.Request, svc *storage.Service, userID int64, path string, cond storage.Conditions) error {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	result, err := svc.PutDocument(r.Context(), userID, path, r.Body, contentType, cond)
	if err != nil {
		writeServiceError(w, err)
		return err
	}

	w.Header().Set("ETag", storage.QuoteETag(result.ETag))
	if result.IsNew {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	return nil
}

func handleDeleteDocument(w http.ResponseWriter, r *http.Request, svc *storage.Service, userID int64, path string, cond storage.Conditions) {
	result, err := svc.DeleteDocument(r.Context(), userID, path, cond)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("ETag", storage.QuoteETag(result.ETag))
	w.WriteHeader(http.StatusOK)
}

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	return ""
}

func isPublicPath(path string) bool {
	return strings.HasPrefix(path, "/public/")
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, storage.ErrPreconditionFailed):
		http.Error(w, "precondition failed", http.StatusPreconditionFailed)
	case errors.Is(err, storage.ErrConflict):
		http.Error(w, "conflict", http.StatusConflict)
	case errors.Is(err, storage.ErrNotModified):
		w.WriteHeader(http.StatusNotModified)
	case errors.Is(err, storage.ErrPayloadTooLarge):
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
	case errors.Is(err, storage.ErrQuotaExceeded):
		http.Error(w, "quota exceeded", http.StatusRequestEntityTooLarge)
	case errors.Is(err, storage.ErrContentRejected):
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
	default:
		slog.Error("storage error", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
