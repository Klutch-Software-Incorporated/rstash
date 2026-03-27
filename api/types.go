package api

// FolderItem represents an item in a remoteStorage folder listing.
type FolderItem struct {
	ETag          string `json:"ETag"`
	ContentType   string `json:"Content-Type,omitempty"`
	ContentLength *int64 `json:"Content-Length,omitempty"`
	LastModified  string `json:"Last-Modified,omitempty"`
}

// FolderDescription is the JSON-LD folder listing per the remoteStorage spec.
type FolderDescription struct {
	Context string                `json:"@context"`
	Items   map[string]FolderItem `json:"items"`
}
