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
