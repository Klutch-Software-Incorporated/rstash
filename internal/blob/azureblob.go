package blob

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

// azureBlobConfig holds parsed Azure Blob DSN parameters.
//
// DSN is the single source of truth for explicit credentials. When neither
// Key nor SASToken is set, authentication falls through to Azure's
// DefaultAzureCredential chain (env service principal, managed identity on
// App Service / VM / Functions, Azure CLI, etc.). This is the expected mode
// for production deployments on Azure — set only `account` in the DSN and
// grant the App Service's identity the Storage Blob Data Contributor role.
type azureBlobConfig struct {
	Container   string
	AccountName string
	Key         string // shared-key auth; optional
	SASToken    string // SAS auth; optional
	Endpoint    string // defaults to {account}.blob.core.windows.net
	Prefix      string
	UseTLS      bool
}

// parseAzureBlobDSN parses an Azure Blob DSN path (the part after
// "azureblob:") into an azureBlobConfig.
//
// Format: container-name?account=NAME&key=KEY&sas=TOKEN&endpoint=HOST&prefix=P&tls=bool
func parseAzureBlobDSN(dsnPath string) (azureBlobConfig, error) {
	cfg := azureBlobConfig{
		UseTLS: true,
	}

	container := dsnPath
	var queryStr string
	if i := strings.IndexByte(dsnPath, '?'); i >= 0 {
		container = dsnPath[:i]
		queryStr = dsnPath[i+1:]
	}

	container = strings.TrimSpace(container)
	if container == "" {
		return cfg, fmt.Errorf("azureblob DSN: container name is required")
	}
	cfg.Container = container

	if queryStr == "" {
		return cfg, fmt.Errorf("azureblob DSN: account parameter is required")
	}

	params, err := url.ParseQuery(queryStr)
	if err != nil {
		return cfg, fmt.Errorf("azureblob DSN: invalid query parameters: %w", err)
	}

	cfg.AccountName = strings.TrimSpace(params.Get("account"))
	if cfg.AccountName == "" {
		return cfg, fmt.Errorf("azureblob DSN: account parameter is required")
	}

	if v := params.Get("key"); v != "" {
		cfg.Key = v
	}
	if v := params.Get("sas"); v != "" {
		cfg.SASToken = v
	}
	if v := params.Get("endpoint"); v != "" {
		cfg.Endpoint = v
	}
	if v := params.Get("prefix"); v != "" {
		cfg.Prefix = strings.Trim(v, "/")
	}
	if v := params.Get("tls"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, fmt.Errorf("azureblob DSN: invalid tls value %q: %w", v, err)
		}
		cfg.UseTLS = b
	}

	if cfg.Endpoint == "" {
		cfg.Endpoint = cfg.AccountName + ".blob.core.windows.net"
	}

	return cfg, nil
}

// serviceURL builds the https://{endpoint} (or http://) prefix used by the
// azblob SDK to locate the storage service.
func (c azureBlobConfig) serviceURL() string {
	scheme := "https"
	if !c.UseTLS {
		scheme = "http"
	}
	// When using Azurite with the default storage emulator, blob URLs take
	// the form http://host:port/{accountName}/{container}/.... The azblob
	// SDK handles this automatically when the URL is constructed as
	// http://host:port/{accountName}.
	if !c.UseTLS {
		return fmt.Sprintf("%s://%s/%s", scheme, c.Endpoint, c.AccountName)
	}
	return fmt.Sprintf("%s://%s", scheme, c.Endpoint)
}

// AzureBlobStore stores blobs in an Azure Blob Storage container.
type AzureBlobStore struct {
	client    *container.Client
	container string
	prefix    string
}

// NewAzureBlobStore creates an AzureBlobStore from a DSN path (the part
// after "azureblob:"). Probes the container as a startup connectivity
// check and surfaces any auth / network / not-found error.
func NewAzureBlobStore(dsnPath string) (*AzureBlobStore, error) {
	cfg, err := parseAzureBlobDSN(dsnPath)
	if err != nil {
		return nil, err
	}

	svcURL := cfg.serviceURL()
	containerURL := svcURL + "/" + cfg.Container

	var client *container.Client
	switch {
	case cfg.Key != "":
		cred, err := azblob.NewSharedKeyCredential(cfg.AccountName, cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("azureblob: create shared-key credential: %w", err)
		}
		client, err = container.NewClientWithSharedKeyCredential(containerURL, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("azureblob: create container client (shared key): %w", err)
		}

	case cfg.SASToken != "":
		// SAS tokens are appended to the URL as query parameters. The SDK
		// treats the resulting URL as pre-authorized, no separate credential
		// object needed.
		sasURL := containerURL + "?" + strings.TrimPrefix(cfg.SASToken, "?")
		c, err := container.NewClientWithNoCredential(sasURL, nil)
		if err != nil {
			return nil, fmt.Errorf("azureblob: create container client (SAS): %w", err)
		}
		client = c

	default:
		// DefaultAzureCredential chain: env SP -> managed identity -> Azure
		// CLI -> a few fallbacks. On Azure App Service with a system-assigned
		// identity granted Storage Blob Data Contributor, this is all that's
		// needed.
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("azureblob: create default credential: %w", err)
		}
		client, err = container.NewClient(containerURL, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("azureblob: create container client (default credential): %w", err)
		}
	}

	// Connectivity probe. GetProperties requires Read permission on the
	// container and fails fast on bad credentials, wrong endpoint, or
	// missing container.
	if _, err := client.GetProperties(context.Background(), nil); err != nil {
		return nil, fmt.Errorf("azureblob: probe container %q: %w", cfg.Container, err)
	}

	return &AzureBlobStore{
		client:    client,
		container: cfg.Container,
		prefix:    cfg.Prefix,
	}, nil
}

// blobName constructs the blob name for a given user and path. Leading
// slashes in path are stripped so we don't produce keys like "42//notes/foo".
// Identical layout to the S3 backend so a container-to-bucket copy (or vice
// versa) is a straight byte-for-byte move — no key rewriting.
func (s *AzureBlobStore) blobName(userID int64, path string) string {
	key := fmt.Sprintf("%d/%s", userID, strings.TrimLeft(path, "/"))
	if s.prefix != "" {
		key = s.prefix + "/" + key
	}
	return key
}

// Get retrieves a blob. The returned ReadCloser streams from the service.
func (s *AzureBlobStore) Get(ctx context.Context, userID int64, path string) (io.ReadCloser, error) {
	name := s.blobName(userID, path)
	resp, err := s.client.NewBlobClient(name).DownloadStream(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	return resp.Body, nil
}

// Put stores a blob. Overwrites any existing blob at the same name.
func (s *AzureBlobStore) Put(ctx context.Context, userID int64, path string, data []byte) error {
	name := s.blobName(userID, path)
	if _, err := s.client.NewBlockBlobClient(name).UploadBuffer(ctx, data, nil); err != nil {
		return fmt.Errorf("put blob: %w", err)
	}
	return nil
}

// Delete removes a blob. Returns nil if the blob doesn't exist (matches
// s3 behavior — Delete on a missing key is not treated as an error, since
// the caller's goal is "blob is gone" which is already true).
func (s *AzureBlobStore) Delete(ctx context.Context, userID int64, path string) error {
	name := s.blobName(userID, path)
	_, err := s.client.NewBlobClient(name).Delete(ctx, nil)
	if err != nil {
		// The SDK returns a BlobNotFound error when the blob doesn't exist.
		// Same goal-state as a successful delete, so swallow it.
		var respErr *azcore.ResponseError
		if ok := asResponseError(err, &respErr); ok && respErr.ErrorCode == "BlobNotFound" {
			return nil
		}
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

// DeleteTree removes every blob under the prefix corresponding to the given
// folder path. Lists via a flat pager, then loop-deletes. Batch deletion
// (up to 256 per request via ContainerBatchClient) is deferred until
// profiles show tree-delete as a hot path.
func (s *AzureBlobStore) DeleteTree(ctx context.Context, userID int64, folderPath string) error {
	prefix := s.blobName(userID, folderPath)
	pager := s.client.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		Prefix: &prefix,
	})

	var firstErr error
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("delete blob tree: list: %w", err)
		}
		for _, b := range page.Segment.BlobItems {
			if b == nil || b.Name == nil {
				continue
			}
			if _, err := s.client.NewBlobClient(*b.Name).Delete(ctx, nil); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("delete blob tree: delete %q: %w", *b.Name, err)
				}
			}
		}
	}
	return firstErr
}

// Close is a no-op — the azblob client uses HTTP with no persistent resources.
func (s *AzureBlobStore) Close() error {
	return nil
}

// asResponseError unwraps a typed Azure SDK response error.
// Separated out to keep the Delete swallow-NotFound logic readable.
func asResponseError(err error, target **azcore.ResponseError) bool {
	for e := err; e != nil; {
		if re, ok := e.(*azcore.ResponseError); ok {
			*target = re
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := e.(unwrapper)
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}
