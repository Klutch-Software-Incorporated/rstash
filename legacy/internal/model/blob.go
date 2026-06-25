package model

// Blob represents a stored blob for the GORM-based blob store.
type Blob struct {
	UserID int64  `gorm:"primaryKey;autoIncrement:false"`
	Path   string `gorm:"primaryKey;size:512"`
	Data   []byte `gorm:"not null"`
}
