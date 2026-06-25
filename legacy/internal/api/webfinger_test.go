package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rstash/internal/api"
	"rstash/internal/config"
)

func webfingerServer() *httptest.Server {
	cfg := &config.Config{
		BaseURL: "https://storage.example.com",
	}
	return httptest.NewServer(api.WebFinger(cfg))
}

func TestWebFinger_ValidRequest(t *testing.T) {
	ts := webfingerServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?resource=acct:alice@storage.example.com")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/jrd+json" {
		t.Fatalf("expected Content-Type application/jrd+json, got %q", ct)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body["subject"] != "acct:alice@storage.example.com" {
		t.Fatalf("unexpected subject: %v", body["subject"])
	}

	links, ok := body["links"].([]any)
	if !ok || len(links) == 0 {
		t.Fatal("expected links array")
	}

	link := links[0].(map[string]any)
	if link["rel"] != "http://tools.ietf.org/id/draft-dejong-remotestorage" {
		t.Fatalf("unexpected rel: %v", link["rel"])
	}
	if link["href"] != "https://storage.example.com/storage/alice" {
		t.Fatalf("unexpected href: %v", link["href"])
	}
	if link["type"] != "draft-dejong-remotestorage-26" {
		t.Fatalf("unexpected type: %v", link["type"])
	}

	props, ok := link["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties object")
	}
	if props["http://remotestorage.io/spec/version"] != "draft-dejong-remotestorage-26" {
		t.Fatalf("unexpected version: %v", props["http://remotestorage.io/spec/version"])
	}
	if props["http://tools.ietf.org/html/rfc6749#section-4.2"] != "https://storage.example.com/oauth/authorize" {
		t.Fatalf("unexpected auth URL: %v", props["http://tools.ietf.org/html/rfc6749#section-4.2"])
	}
}

func TestWebFinger_MissingResource(t *testing.T) {
	ts := webfingerServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing resource, got %d", resp.StatusCode)
	}
}

func TestWebFinger_InvalidResourceFormat(t *testing.T) {
	ts := webfingerServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?resource=mailto:alice@example.com")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-acct resource, got %d", resp.StatusCode)
	}
}

func TestWebFinger_NoAtSign(t *testing.T) {
	ts := webfingerServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?resource=acct:alicenodomain")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for resource without @, got %d", resp.StatusCode)
	}
}

func TestWebFinger_PostNotAllowed(t *testing.T) {
	ts := webfingerServer()
	defer ts.Close()

	resp, err := http.Post(ts.URL+"?resource=acct:alice@example.com", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST, got %d", resp.StatusCode)
	}
}
