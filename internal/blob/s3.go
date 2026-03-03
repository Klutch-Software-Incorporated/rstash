package blob

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// s3Config holds parsed S3 DSN parameters.
type s3Config struct {
	Bucket    string
	Region    string
	Endpoint  string
	Prefix    string
	AccessKey string
	SecretKey string
	UseTLS    bool
}

// parseS3DSN parses an S3 DSN path (everything after "s3:") into an s3Config.
// Format: bucket-name?region=us-east-1&endpoint=s3.amazonaws.com&prefix=optional/prefix
func parseS3DSN(dsnPath string) (s3Config, error) {
	cfg := s3Config{
		Region:   "us-east-1",
		Endpoint: "s3.amazonaws.com",
		UseTLS:   true,
	}

	// Split bucket from query params.
	bucket := dsnPath
	var queryStr string
	if i := strings.IndexByte(dsnPath, '?'); i >= 0 {
		bucket = dsnPath[:i]
		queryStr = dsnPath[i+1:]
	}

	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return cfg, fmt.Errorf("s3 DSN: bucket name is required")
	}
	cfg.Bucket = bucket

	if queryStr == "" {
		// Fall back to env vars for credentials.
		cfg.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
		cfg.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
		return cfg, nil
	}

	params, err := url.ParseQuery(queryStr)
	if err != nil {
		return cfg, fmt.Errorf("s3 DSN: invalid query parameters: %w", err)
	}

	if v := params.Get("region"); v != "" {
		cfg.Region = v
	}
	if v := params.Get("endpoint"); v != "" {
		cfg.Endpoint = v
	}
	if v := params.Get("prefix"); v != "" {
		cfg.Prefix = strings.Trim(v, "/")
	}
	if v := params.Get("access_key"); v != "" {
		cfg.AccessKey = v
	} else {
		cfg.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	if v := params.Get("secret_key"); v != "" {
		cfg.SecretKey = v
	} else {
		cfg.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	if v := params.Get("tls"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, fmt.Errorf("s3 DSN: invalid tls value %q: %w", v, err)
		}
		cfg.UseTLS = b
	}

	return cfg, nil
}

// S3Store stores blobs in an S3-compatible object store.
type S3Store struct {
	client *minio.Client
	bucket string
	prefix string
}

// NewS3Store creates a new S3Store from a DSN path (the part after "s3:").
func NewS3Store(dsnPath string) (*S3Store, error) {
	cfg, err := parseS3DSN(dsnPath)
	if err != nil {
		return nil, err
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Region: cfg.Region,
		Secure: cfg.UseTLS,
	})
	if err != nil {
		return nil, fmt.Errorf("s3: create client: %w", err)
	}

	// Verify bucket exists as a startup connectivity check.
	exists, err := client.BucketExists(context.Background(), cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("s3: check bucket %q: %w", cfg.Bucket, err)
	}
	if !exists {
		return nil, fmt.Errorf("s3: bucket %q does not exist", cfg.Bucket)
	}

	return &S3Store{
		client: client,
		bucket: cfg.Bucket,
		prefix: cfg.Prefix,
	}, nil
}

// objectKey constructs the S3 object key for a given user and path.
// Leading slashes are stripped to avoid double-slash keys (e.g. "42//test/file").
func (s *S3Store) objectKey(userID int64, path string) string {
	key := fmt.Sprintf("%d/%s", userID, strings.TrimLeft(path, "/"))
	if s.prefix != "" {
		key = s.prefix + "/" + key
	}
	return key
}

// Get retrieves a blob from S3.
func (s *S3Store) Get(ctx context.Context, userID int64, path string) (io.ReadCloser, error) {
	key := s.objectKey(userID, path)
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	// Stat to surface errors eagerly (e.g. NoSuchKey).
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		return nil, fmt.Errorf("get blob: %w", err)
	}
	return obj, nil
}

// Put stores a blob in S3.
func (s *S3Store) Put(ctx context.Context, userID int64, path string, content io.Reader) error {
	key := s.objectKey(userID, path)
	_, err := s.client.PutObject(ctx, s.bucket, key, content, -1, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("put blob: %w", err)
	}
	return nil
}

// Delete removes a blob from S3.
func (s *S3Store) Delete(ctx context.Context, userID int64, path string) error {
	key := s.objectKey(userID, path)
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

// DeleteTree removes all blobs under a folder path prefix from S3.
func (s *S3Store) DeleteTree(ctx context.Context, userID int64, folderPath string) error {
	prefix := s.objectKey(userID, folderPath)

	// List all objects under the prefix.
	objectsCh := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	// Feed listed objects into RemoveObjects for batch deletion.
	removeCh := make(chan minio.ObjectInfo)
	errCh := s.client.RemoveObjects(ctx, s.bucket, removeCh, minio.RemoveObjectsOptions{})

	go func() {
		defer close(removeCh)
		for obj := range objectsCh {
			if obj.Err != nil {
				continue
			}
			removeCh <- obj
		}
	}()

	// Collect any deletion errors.
	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = fmt.Errorf("delete blob tree: %w", e.Err)
		}
	}
	return firstErr
}

// Close is a no-op — minio.Client uses HTTP with no persistent resources.
func (s *S3Store) Close() error {
	return nil
}
