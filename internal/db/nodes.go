package db

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"rstash/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetNode returns the node at the given path for the user, or nil if not found.
func (r *Repository) GetNode(ctx context.Context, userID int64, path string) (*model.Node, error) {
	var n model.Node
	err := r.db.WithContext(ctx).Where("user_id = ? AND path = ?", userID, path).First(&n).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get node: %w", err)
	}
	return &n, nil
}

// UpsertNode creates or updates a document node, returning the resulting row.
func (r *Repository) UpsertNode(ctx context.Context, userID int64, path string, contentType string, contentLength int64, etag string) (*model.Node, error) {
	now := time.Now().UTC()
	n := model.Node{
		UserID:        userID,
		Path:          path,
		ContentType:   contentType,
		ContentLength: contentLength,
		ETag:          etag,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"content_type", "content_length", "e_tag", "updated_at",
		}),
	}).Create(&n)
	if result.Error != nil {
		return nil, fmt.Errorf("upsert node: %w", result.Error)
	}

	// Re-read to get the actual row (with correct ID, timestamps).
	var out model.Node
	if err := r.db.WithContext(ctx).Where("user_id = ? AND path = ?", userID, path).First(&out).Error; err != nil {
		return nil, fmt.Errorf("upsert node read-back: %w", err)
	}
	return &out, nil
}

// DeleteNode removes the node at the given path for the user.
func (r *Repository) DeleteNode(ctx context.Context, userID int64, path string) error {
	if err := r.db.WithContext(ctx).Where("user_id = ? AND path = ?", userID, path).Delete(&model.Node{}).Error; err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	return nil
}

// VirtualChild represents a direct child of a folder: either a document node
// or a virtual subfolder derived from document paths.
type VirtualChild struct {
	Name          string    // e.g. "file.txt" or "subfolder/"
	IsFolder      bool
	ETag          string
	ContentType   string
	ContentLength int64
	UpdatedAt     time.Time
}

// ListVirtualChildren returns the direct children of a folder by querying all
// descendant documents and deriving virtual subfolders from their paths.
// Folders are implicit — they exist because documents exist beneath them.
func (r *Repository) ListVirtualChildren(ctx context.Context, userID int64, folderPath string) ([]*VirtualChild, error) {
	pattern := folderPath + "%"
	var nodes []*model.Node
	err := r.db.WithContext(ctx).Where("user_id = ? AND path LIKE ? AND path != ?", userID, pattern, folderPath).Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("list virtual children: %w", err)
	}

	// Separate direct documents from subfolder descendants.
	type folderAcc struct {
		childETags map[string]string // name → etag of all direct children in the subfolder
	}
	folders := make(map[string]*folderAcc)
	var result []*VirtualChild

	for _, n := range nodes {
		rest := strings.TrimPrefix(n.Path, folderPath)
		slashIdx := strings.Index(rest, "/")

		if slashIdx == -1 {
			// Direct document child (no slash in relative path).
			result = append(result, &VirtualChild{
				Name:          rest,
				IsFolder:      false,
				ETag:          n.ETag,
				ContentType:   n.ContentType,
				ContentLength: n.ContentLength,
				UpdatedAt:     n.UpdatedAt,
			})
		} else {
			// Belongs to a subfolder. Extract the immediate subfolder name.
			subfolderName := rest[:slashIdx+1] // e.g. "vacation/"
			acc, ok := folders[subfolderName]
			if !ok {
				acc = &folderAcc{childETags: make(map[string]string)}
				folders[subfolderName] = acc
			}
			// Collect this document's contribution to the subfolder's ETag.
			// We use the document's path relative to the subfolder as the key.
			subRest := rest[slashIdx+1:]
			acc.childETags[subRest] = n.ETag
		}
	}

	// Build virtual folder children with computed ETags.
	for name, acc := range folders {
		result = append(result, &VirtualChild{
			Name:     name,
			IsFolder: true,
			ETag:     computeSubfolderETag(acc.childETags),
		})
	}

	return result, nil
}

// computeSubfolderETag computes a folder ETag from all descendant document ETags.
// This produces a deterministic hash that changes when any descendant changes.
func computeSubfolderETag(descendantETags map[string]string) string {
	// We reuse FolderETag's approach but with all descendants, not just direct children.
	// This means a subfolder's ETag is based on ALL documents beneath it, which ensures
	// any change deep in the tree propagates up.
	names := make([]string, 0, len(descendantETags))
	for name := range descendantETags {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(descendantETags[name])
	}

	h := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", h[:8])
}

// ErrPathConflict is returned when a document path conflicts with existing
// documents that imply a folder at that path, or vice versa.
var ErrPathConflict = fmt.Errorf("path conflict")

// CheckPathConflict checks that creating a document at the given path won't
// conflict with existing documents that imply a folder at the same path, or
// with an existing document that sits where a folder segment would be.
func (r *Repository) CheckPathConflict(ctx context.Context, userID int64, path string) error {
	// Check 1: Would this document path shadow an existing virtual folder?
	// e.g. creating "/a/b" when "/a/b/c.txt" exists.
	pattern := path + "/%"
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Node{}).
		Where("user_id = ? AND path LIKE ?", userID, pattern).
		Count(&count).Error; err != nil {
		return fmt.Errorf("check path conflict (children): %w", err)
	}
	if count > 0 {
		return ErrPathConflict
	}

	// Check 2: Would any ancestor segment of this path conflict with an existing document?
	// e.g. creating "/a/b/c.txt" when document "/a/b" exists.
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i := 1; i < len(segments); i++ {
		ancestorDoc := "/" + strings.Join(segments[:i], "/")
		var n int64
		if err := r.db.WithContext(ctx).Model(&model.Node{}).
			Where("user_id = ? AND path = ?", userID, ancestorDoc).
			Count(&n).Error; err != nil {
			return fmt.Errorf("check path conflict (ancestor): %w", err)
		}
		if n > 0 {
			return ErrPathConflict
		}
	}

	return nil
}

// UserStorageStats holds aggregate storage metrics for a user.
type UserStorageStats struct {
	FileCount  int64
	TotalBytes int64
}

// GetUserStorageStats returns total file count and storage used for a user.
func (r *Repository) GetUserStorageStats(ctx context.Context, userID int64) (*UserStorageStats, error) {
	var s UserStorageStats
	err := r.db.WithContext(ctx).Model(&model.Node{}).
		Where("user_id = ?", userID).
		Select("COUNT(*) AS file_count, COALESCE(SUM(content_length), 0) AS total_bytes").
		Scan(&s).Error
	if err != nil {
		return nil, fmt.Errorf("get user storage stats: %w", err)
	}
	return &s, nil
}

// CountDocumentNodes returns the total number of document nodes across all users.
func (r *Repository) CountDocumentNodes(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Node{}).Count(&n).Error
	return n, err
}

// ModuleStats holds per-module storage metrics.
type ModuleStats struct {
	Module     string
	FileCount  int64
	TotalBytes int64
}

// GetUserModuleStats returns per-top-level-folder (module) storage breakdown.
// Computed in Go to avoid dialect-specific SUBSTR/INSTR.
func (r *Repository) GetUserModuleStats(ctx context.Context, userID int64) ([]*ModuleStats, error) {
	var nodes []*model.Node
	pattern := "/%/%"
	err := r.db.WithContext(ctx).Where("user_id = ? AND path LIKE ?", userID, pattern).Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("get user module stats: %w", err)
	}

	moduleMap := make(map[string]*ModuleStats)
	for _, n := range nodes {
		// Extract module name from path like "/modulename/rest..."
		trimmed := strings.TrimPrefix(n.Path, "/")
		idx := strings.Index(trimmed, "/")
		if idx < 0 {
			continue
		}
		module := trimmed[:idx]
		ms, ok := moduleMap[module]
		if !ok {
			ms = &ModuleStats{Module: module}
			moduleMap[module] = ms
		}
		ms.FileCount++
		ms.TotalBytes += n.ContentLength
	}

	var result []*ModuleStats
	for _, ms := range moduleMap {
		result = append(result, ms)
	}
	// Sort by total_bytes descending.
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].TotalBytes > result[i].TotalBytes {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result, nil
}

// GetRecentUserNodes returns the most recently modified non-folder nodes for a user.
func (r *Repository) GetRecentUserNodes(ctx context.Context, userID int64, limit int) ([]*model.Node, error) {
	var nodes []*model.Node
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Limit(limit).
		Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("get recent user nodes: %w", err)
	}
	return nodes, nil
}

// GetLargestUserNodes returns the largest non-folder nodes for a user by content_length.
func (r *Repository) GetLargestUserNodes(ctx context.Context, userID int64, limit int) ([]*model.Node, error) {
	var nodes []*model.Node
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("content_length DESC").
		Limit(limit).
		Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("get largest user nodes: %w", err)
	}
	return nodes, nil
}

// GetSubtreeSize returns the total content_length of all non-folder descendants
// under the given folder path.
func (r *Repository) GetSubtreeSize(ctx context.Context, userID int64, folderPath string) (int64, error) {
	pattern := folderPath + "%"
	var total *int64
	err := r.db.WithContext(ctx).Model(&model.Node{}).
		Where("user_id = ? AND path LIKE ? AND path != ?", userID, pattern, folderPath).
		Select("COALESCE(SUM(content_length), 0)").
		Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("get subtree size: %w", err)
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

// ListDescendantFiles returns all non-folder nodes under a folder path (recursive).
func (r *Repository) ListDescendantFiles(ctx context.Context, userID int64, folderPath string) ([]*model.Node, error) {
	pattern := folderPath + "%"
	var nodes []*model.Node
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND path LIKE ? AND path != ?", userID, pattern, folderPath).
		Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("list descendant files: %w", err)
	}
	return nodes, nil
}

// DeleteSubtree removes all nodes (files and folders) under a given folder path,
// including the folder itself.
func (r *Repository) DeleteSubtree(ctx context.Context, userID int64, folderPath string) error {
	pattern := folderPath + "%"
	if err := r.db.WithContext(ctx).Where("user_id = ? AND (path = ? OR path LIKE ?)", userID, folderPath, pattern).Delete(&model.Node{}).Error; err != nil {
		return fmt.Errorf("delete subtree: %w", err)
	}
	return nil
}

// globToLike converts a user search query with glob wildcards (* and ?)
// into a SQL LIKE pattern.
func globToLike(query string) string {
	hasWild := strings.ContainsAny(query, "*?")
	r := strings.NewReplacer("%", `\%`, "_", `\_`)
	pattern := r.Replace(query)
	pattern = strings.ReplaceAll(pattern, "*", "%")
	pattern = strings.ReplaceAll(pattern, "?", "_")
	if !hasWild {
		pattern = "%" + pattern + "%"
	}
	return pattern
}

// SearchUserNodes searches non-folder nodes by path, supporting glob wildcards.
func (r *Repository) SearchUserNodes(ctx context.Context, userID int64, query string, limit int) ([]*model.Node, error) {
	pattern := globToLike(query)
	var nodes []*model.Node
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND path LIKE ?", userID, pattern).
		Order("updated_at DESC").
		Limit(limit).
		Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("search user nodes: %w", err)
	}
	return nodes, nil
}

// AncestorPaths returns all ancestor folder paths from the immediate parent
// up to and including the root "/".
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
