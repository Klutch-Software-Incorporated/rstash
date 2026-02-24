// Command rs-upload performs a remoteStorage file upload using WebFinger
// discovery and the OAuth 2.0 implicit grant flow.
//
// Usage:
//
//	go run ./cmd/rs-upload acct:user@host module/path ./local-file
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"bytes"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintf(os.Stderr, "Usage: rs-upload acct:user@host module/path ./local-file\n")
		os.Exit(1)
	}

	acct := os.Args[1]
	storagePath := os.Args[2]
	localFile := os.Args[3]

	if !strings.HasPrefix(acct, "acct:") {
		fmt.Fprintf(os.Stderr, "Error: first argument must start with acct:\n")
		os.Exit(1)
	}

	// Derive module (first path segment) for scope.
	module := strings.SplitN(storagePath, "/", 2)[0]
	if module == "" {
		fmt.Fprintf(os.Stderr, "Error: storage path must include a module (e.g. documents/file.txt)\n")
		os.Exit(1)
	}

	// Detect content type from file extension.
	contentType := mime.TypeByExtension(filepath.Ext(localFile))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Read file early so we fail fast on bad paths.
	fileData, err := os.ReadFile(localFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Step 1: WebFinger discovery.
	host := strings.SplitN(strings.TrimPrefix(acct, "acct:"), "@", 2)
	if len(host) != 2 {
		fmt.Fprintf(os.Stderr, "Error: invalid acct URI (expected acct:user@host)\n")
		os.Exit(1)
	}

	storageBase, authURL, err := webfinger(host[1], acct)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WebFinger error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Storage base: %s\n", storageBase)
	fmt.Printf("Auth URL:     %s\n", authURL)

	// Step 2: OAuth implicit grant.
	token, err := oauthImplicit(authURL, module)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OAuth error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Got token:    %s...%s\n", token[:4], token[len(token)-4:])

	// Step 3: Upload file.
	putURL := storageBase + "/" + storagePath
	err = upload(putURL, token, contentType, fileData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Upload error: %v\n", err)
		os.Exit(1)
	}
}

// webfinger performs WebFinger discovery and returns the storage base URL and
// OAuth authorize URL.
func webfinger(host, resource string) (storageBase, authURL string, err error) {
	wfURL := "http://" + host + "/.well-known/webfinger?resource=" + url.QueryEscape(resource)
	fmt.Printf("WebFinger:    %s\n", wfURL)

	resp, err := http.Get(wfURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}

	var wf struct {
		Links []struct {
			Href       string            `json:"href"`
			Properties map[string]string `json:"properties"`
		} `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wf); err != nil {
		return "", "", fmt.Errorf("decoding response: %w", err)
	}

	if len(wf.Links) == 0 {
		return "", "", fmt.Errorf("no links in WebFinger response")
	}

	link := wf.Links[0]
	storageBase = link.Href
	authURL = link.Properties["http://tools.ietf.org/html/rfc6749#section-4.2"]
	if storageBase == "" || authURL == "" {
		return "", "", fmt.Errorf("missing href or oauth property in WebFinger link")
	}

	return storageBase, authURL, nil
}

// oauthImplicit runs the OAuth 2.0 implicit grant flow by opening the user's
// browser and starting a local callback server.
func oauthImplicit(authURL, module string) (string, error) {
	// Generate random state parameter.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	state := hex.EncodeToString(stateBytes)

	// Start local callback server on a random port.
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", fmt.Errorf("starting callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	tokenCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", callbackPage)
	mux.HandleFunc("/callback/receive", receiveHandler(state, tokenCh, errCh))

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	// Build authorize URL and open browser.
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)
	authorizeURL := fmt.Sprintf("%s?response_type=token&redirect_uri=%s&scope=%s:rw&state=%s",
		authURL,
		url.QueryEscape(redirectURI),
		url.QueryEscape(module),
		url.QueryEscape(state),
	)

	fmt.Printf("Opening browser for authorization...\n")
	if err := openBrowser(authorizeURL); err != nil {
		fmt.Printf("Could not open browser automatically.\nPlease visit: %s\n", authorizeURL)
	}

	// Wait for token.
	select {
	case token := <-tokenCh:
		srv.Shutdown(context.Background())
		return token, nil
	case err := <-errCh:
		srv.Shutdown(context.Background())
		return "", err
	case <-time.After(5 * time.Minute):
		srv.Shutdown(context.Background())
		return "", fmt.Errorf("timed out waiting for authorization (5 minutes)")
	}
}

// callbackPage serves the HTML page that extracts the access_token from the
// URL fragment and forwards it to /callback/receive.
func callbackPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html><body>
<p>Completing authorization...</p>
<script>
  var h = location.hash.substring(1);
  var params = new URLSearchParams(h);
  var token = params.get('access_token');
  var state = params.get('state');
  if (token) {
    location.href = '/callback/receive?token=' + encodeURIComponent(token) + '&state=' + encodeURIComponent(state);
  } else {
    document.body.innerText = 'Authorization failed: ' + (params.get('error') || 'no token in response');
  }
</script>
</body></html>`)
}

// receiveHandler returns an http.HandlerFunc that receives the token forwarded
// by the callback page's JavaScript.
func receiveHandler(expectedState string, tokenCh chan<- string, errCh chan<- error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		state := r.URL.Query().Get("state")

		if state != expectedState {
			errCh <- fmt.Errorf("state mismatch (CSRF protection)")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}

		if token == "" {
			errCh <- fmt.Errorf("empty token received")
			http.Error(w, "no token", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><body>
<p>Authorization complete. You may close this tab.</p>
</body></html>`)

		tokenCh <- token
	}
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(rawURL string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL).Start()
	case "darwin":
		return exec.Command("open", rawURL).Start()
	default:
		return exec.Command("xdg-open", rawURL).Start()
	}
}

// upload PUTs the file data to the given storage URL.
func upload(putURL, token, contentType string, data []byte) error {
	fmt.Printf("Uploading:    PUT %s (%s, %d bytes)\n", putURL, contentType, len(data))

	req, err := http.NewRequest(http.MethodPut, putURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, body)
	}

	etag := resp.Header.Get("ETag")
	fmt.Printf("Success!      %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
	if etag != "" {
		fmt.Printf("ETag:         %s\n", etag)
	}
	return nil
}
