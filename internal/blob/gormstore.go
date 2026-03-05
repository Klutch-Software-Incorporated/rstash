package blob

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"gosilo/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GORMStore stores blobs in any GORM-supported database.
type GORMStore struct {
	db *gorm.DB
}

// NewGORMStore creates a new GORMStore using the given GORM database connection.
// It runs AutoMigrate to ensure the blobs table exists.
func NewGORMStore(db *gorm.DB) (*GORMStore, error) {
	if err := db.AutoMigrate(&model.Blob{}); err != nil {
		return nil, fmt.Errorf("migrate blobs table: %w", err)
	}
	return &GORMStore{db: db}, nil
}

// Get retrieves a blob.
func (s *GORMStore) Get(ctx context.Context, userID int64, path string) (io.ReadCloser, error) {
	var b model.Blob
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND path = ?", userID, path).
		First(&b).Error
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	return io.NopCloser(bytes.NewReader(b.Data)), nil
}

// Put stores a blob (insert or replace).
func (s *GORMStore) Put(ctx context.Context, userID int64, path string, data []byte) error {
	b := model.Blob{
		UserID: userID,
		Path:   path,
		Data:   data,
	}

	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "path"}},
			DoUpdates: clause.AssignmentColumns([]string{"data"}),
		}).
		Create(&b).Error
	if err != nil {
		return fmt.Errorf("put blob: %w", err)
	}
	return nil
}

// Delete removes a blob.
func (s *GORMStore) Delete(ctx context.Context, userID int64, path string) error {
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND path = ?", userID, path).
		Delete(&model.Blob{}).Error
	if err != nil {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

// DeleteTree removes all blobs under a folder path prefix.
func (s *GORMStore) DeleteTree(ctx context.Context, userID int64, folderPath string) error {
	pattern := folderPath + "%"
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND path LIKE ?", userID, pattern).
		Delete(&model.Blob{}).Error
	if err != nil {
		return fmt.Errorf("delete blob tree: %w", err)
	}
	return nil
}

// Count returns the total number of blobs stored.
func (s *GORMStore) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&model.Blob{}).Count(&n).Error
	return n, err
}

// Close is a no-op — the caller owns the *gorm.DB lifecycle.
func (s *GORMStore) Close() error {
	return nil
}
