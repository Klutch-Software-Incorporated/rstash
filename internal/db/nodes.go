package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gosilo/internal/model"
)

// GetNode returns the node at the given path for the user, or nil if not found.
func GetNode(ctx context.Context, q Querier, userID int64, path string) (*model.Node, error) {
	var n model.Node
	err := q.QueryRowContext(ctx,
		`SELECT id, user_id, path, is_folder,
		        COALESCE(content_type, ''), COALESCE(content_length, 0),
		        etag, created_at, updated_at
		 FROM nodes WHERE user_id = ? AND path = ?`,
		userID, path,
	).Scan(&n.ID, &n.UserID, &n.Path, &n.IsFolder,
		&n.ContentType, &n.ContentLength,
		&n.ETag, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get node: %w", err)
	}
	return &n, nil
}

// UpsertNode creates or updates a node, returning the resulting row.
func UpsertNode(ctx context.Context, q Querier, userID int64, path string, isFolder bool, contentType string, contentLength int64, etag string) (*model.Node, error) {
	var n model.Node
	err := q.QueryRowContext(ctx,
		`INSERT INTO nodes (user_id, path, is_folder, content_type, content_length, etag)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, path) DO UPDATE SET
		     content_type = excluded.content_type,
		     content_length = excluded.content_length,
		     etag = excluded.etag,
		     updated_at = datetime('now')
		 RETURNING id, user_id, path, is_folder,
		           COALESCE(content_type, ''), COALESCE(content_length, 0),
		           etag, created_at, updated_at`,
		userID, path, isFolder, contentType, contentLength, etag,
	).Scan(&n.ID, &n.UserID, &n.Path, &n.IsFolder,
		&n.ContentType, &n.ContentLength,
		&n.ETag, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert node: %w", err)
	}
	return &n, nil
}

// DeleteNode removes the node at the given path for the user.
func DeleteNode(ctx context.Context, q Querier, userID int64, path string) error {
	_, err := q.ExecContext(ctx,
		"DELETE FROM nodes WHERE user_id = ? AND path = ?",
		userID, path,
	)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	return nil
}

// ListChildren returns the direct children of a folder.
func ListChildren(ctx context.Context, q Querier, userID int64, folderPath string) ([]*model.Node, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, user_id, path, is_folder,
		        COALESCE(content_type, ''), COALESCE(content_length, 0),
		        etag, created_at, updated_at
		 FROM nodes
		 WHERE user_id = ? AND path LIKE ? AND path != ?`,
		userID, folderPath+"%", folderPath,
	)
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		var n model.Node
		if err := rows.Scan(&n.ID, &n.UserID, &n.Path, &n.IsFolder,
			&n.ContentType, &n.ContentLength,
			&n.ETag, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		// Filter to direct children only: the remainder after folderPath
		// must contain no "/" (document) or exactly one trailing "/" (subfolder).
		rest := strings.TrimPrefix(n.Path, folderPath)
		slashIdx := strings.Index(rest, "/")
		if slashIdx == -1 || slashIdx == len(rest)-1 {
			nodes = append(nodes, &n)
		}
	}
	return nodes, rows.Err()
}

// UserStorageStats holds aggregate storage metrics for a user.
type UserStorageStats struct {
	FileCount  int64
	TotalBytes int64
}

// GetUserStorageStats returns total file count and storage used for a user.
func GetUserStorageStats(ctx context.Context, q Querier, userID int64) (*UserStorageStats, error) {
	var s UserStorageStats
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(content_length), 0)
		 FROM nodes WHERE user_id = ? AND is_folder = 0`,
		userID,
	).Scan(&s.FileCount, &s.TotalBytes)
	if err != nil {
		return nil, fmt.Errorf("get user storage stats: %w", err)
	}
	return &s, nil
}

// ModuleStats holds per-module storage metrics.
type ModuleStats struct {
	Module     string
	FileCount  int64
	TotalBytes int64
}

// GetUserModuleStats returns per-top-level-folder (module) storage breakdown.
func GetUserModuleStats(ctx context.Context, q Querier, userID int64) ([]*ModuleStats, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT
		    SUBSTR(path, 2, INSTR(SUBSTR(path, 2), '/') - 1) AS module,
		    COUNT(*) AS file_count,
		    COALESCE(SUM(content_length), 0) AS total_bytes
		 FROM nodes
		 WHERE user_id = ? AND is_folder = 0 AND path LIKE '/%/%'
		 GROUP BY module
		 ORDER BY total_bytes DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get user module stats: %w", err)
	}
	defer rows.Close()

	var modules []*ModuleStats
	for rows.Next() {
		var m ModuleStats
		if err := rows.Scan(&m.Module, &m.FileCount, &m.TotalBytes); err != nil {
			return nil, fmt.Errorf("scan module stats: %w", err)
		}
		modules = append(modules, &m)
	}
	return modules, rows.Err()
}

// GetRecentUserNodes returns the most recently modified non-folder nodes for a user.
func GetRecentUserNodes(ctx context.Context, q Querier, userID int64, limit int) ([]*model.Node, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, user_id, path, is_folder,
		        COALESCE(content_type, ''), COALESCE(content_length, 0),
		        etag, created_at, updated_at
		 FROM nodes
		 WHERE user_id = ? AND is_folder = 0
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recent user nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		var n model.Node
		if err := rows.Scan(&n.ID, &n.UserID, &n.Path, &n.IsFolder,
			&n.ContentType, &n.ContentLength,
			&n.ETag, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		nodes = append(nodes, &n)
	}
	return nodes, rows.Err()
}

// GetSubtreeSize returns the total content_length of all non-folder descendants
// under the given folder path.
func GetSubtreeSize(ctx context.Context, q Querier, userID int64, folderPath string) (int64, error) {
	var total int64
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(content_length), 0) FROM nodes
		 WHERE user_id = ? AND path LIKE ? || '%' AND is_folder = 0 AND path != ?`,
		userID, folderPath, folderPath,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("get subtree size: %w", err)
	}
	return total, nil
}

// ListDescendantFiles returns all non-folder nodes under a folder path (recursive).
func ListDescendantFiles(ctx context.Context, q Querier, userID int64, folderPath string) ([]*model.Node, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, user_id, path, is_folder,
		        COALESCE(content_type, ''), COALESCE(content_length, 0),
		        etag, created_at, updated_at
		 FROM nodes
		 WHERE user_id = ? AND path LIKE ? || '%' AND is_folder = 0 AND path != ?`,
		userID, folderPath, folderPath,
	)
	if err != nil {
		return nil, fmt.Errorf("list descendant files: %w", err)
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		var n model.Node
		if err := rows.Scan(&n.ID, &n.UserID, &n.Path, &n.IsFolder,
			&n.ContentType, &n.ContentLength,
			&n.ETag, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		nodes = append(nodes, &n)
	}
	return nodes, rows.Err()
}

// DeleteSubtree removes all nodes (files and folders) under a given folder path,
// including the folder itself.
func DeleteSubtree(ctx context.Context, q Querier, userID int64, folderPath string) error {
	_, err := q.ExecContext(ctx,
		"DELETE FROM nodes WHERE user_id = ? AND (path = ? OR path LIKE ? || '%')",
		userID, folderPath, folderPath,
	)
	if err != nil {
		return fmt.Errorf("delete subtree: %w", err)
	}
	return nil
}

// AncestorPaths returns all ancestor folder paths from the immediate parent
// up to and including the root "/".
// Example: "/foo/bar/baz.txt" → ["/foo/bar/", "/foo/", "/"]
func AncestorPaths(path string) []string {
	var paths []string
	for {
		if path == "/" {
			break
		}
		trimmed := strings.TrimSuffix(path, "/")
		idx := strings.LastIndex(trimmed, "/")
		if idx < 0 {
			break
		}
		path = trimmed[:idx+1]
		paths = append(paths, path)
	}
	return paths
}
