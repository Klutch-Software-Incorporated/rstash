// Command rs-upload-pkce performs a remoteStorage file upload using WebFinger
// discovery and the OAuth 2.0 authorization code grant with PKCE (S256).
//
// This is the recommended flow for public clients (OAuth 2.1). Unlike the
// implicit flow (rs-upload), the access token never appears in a URL fragment
// or browser history.
//
// Usage:
//
//	go run ./cmd/rs-upload-pkce acct:user@host module/path ./local-file
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
		fmt.Fprintf(os.Stderr, "Usage: rs-upload-pkce acct:user@host module/path ./local-file\n")
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

	// Derive token endpoint from auth URL (same origin + /oauth/token).
	tokenURL, err := deriveTokenURL(authURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deriving token URL: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Token URL:    %s\n", tokenURL)

	// Step 2: OAuth authorization code + PKCE.
	token, err := oauthAuthCode(authURL, tokenURL, module)
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

// deriveTokenURL returns the /oauth/token endpoint from the authorize URL's origin.
func deriveTokenURL(authURL string) (string, error) {
	u, err := url.Parse(authURL)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host + "/oauth/token", nil
}

// generatePKCE creates a code_verifier and its S256 code_challenge.
func generatePKCE() (verifier, challenge string, err error) {
	// 32 bytes = 43 base64url characters, well within the 43-128 range.
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// oauthAuthCode runs the OAuth 2.0 authorization code + PKCE flow by opening
// the user's browser and starting a local callback server.
func oauthAuthCode(authURL, tokenURL, module string) (string, error) {
	// Generate PKCE pair.
	codeVerifier, codeChallenge, err := generatePKCE()
	if err != nil {
		return "", fmt.Errorf("generating PKCE: %w", err)
	}

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
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", callbackHandler(state, codeCh, errCh))

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	// Build authorize URL with PKCE parameters.
	authorizeURL := fmt.Sprintf(
		"%s?response_type=code&redirect_uri=%s&scope=%s:rw&state=%s&code_challenge=%s&code_challenge_method=S256",
		authURL,
		url.QueryEscape(redirectURI),
		url.QueryEscape(module),
		url.QueryEscape(state),
		url.QueryEscape(codeChallenge),
	)

	fmt.Printf("Opening browser for authorization...\n")
	if err := openBrowser(authorizeURL); err != nil {
		fmt.Printf("Could not open browser automatically.\nPlease visit: %s\n", authorizeURL)
	}

	// Wait for authorization code.
	var code string
	select {
	case code = <-codeCh:
		srv.Shutdown(context.Background())
	case err := <-errCh:
		srv.Shutdown(context.Background())
		return "", err
	case <-time.After(5 * time.Minute):
		srv.Shutdown(context.Background())
		return "", fmt.Errorf("timed out waiting for authorization (5 minutes)")
	}

	fmt.Printf("Got auth code: %s...%s\n", code[:4], code[len(code)-4:])

	// Exchange authorization code for access token.
	token, err := exchangeCode(tokenURL, code, codeVerifier, redirectURI)
	if err != nil {
		return "", err
	}
	return token, nil
}

// callbackHandler handles the redirect from the authorization server.
// In the code flow, the authorization code arrives in query params (not the
// fragment), so no JavaScript extraction is needed.
func callbackHandler(expectedState string, codeCh chan<- string, errCh chan<- error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// Check for error response.
		if errCode := q.Get("error"); errCode != "" {
			desc := q.Get("error_description")
			if desc == "" {
				desc = errCode
			}
			errCh <- fmt.Errorf("authorization denied: %s", desc)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<!DOCTYPE html>
<html><body>
<p>Authorization denied: %s. You may close this tab.</p>
</body></html>`, desc)
			return
		}

		state := q.Get("state")
		if state != expectedState {
			errCh <- fmt.Errorf("state mismatch (CSRF protection)")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}

		code := q.Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no authorization code in response")
			http.Error(w, "no code", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><body>
<p>Authorization complete. You may close this tab.</p>
</body></html>`)

		codeCh <- code
	}
}

// exchangeCode POSTs to the token endpoint to exchange the authorization code
// (with PKCE code_verifier) for an access token.
func exchangeCode(tokenURL, code, codeVerifier, redirectURI string) (string, error) {
	fmt.Printf("Exchanging code at %s...\n", tokenURL)

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {codeVerifier},
		"redirect_uri":  {redirectURI},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}

	var tokenResp struct {
		AccessToken      string `json:"access_token"`
		TokenType        string `json:"token_type"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("decoding token response: %w (body: %s)", err, body)
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("token error: %s — %s", tokenResp.Error, tokenResp.ErrorDescription)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access_token in response (status %d, body: %s)", resp.StatusCode, body)
	}

	return tokenResp.AccessToken, nil
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
