package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAttributionTransportAddsHeaders(t *testing.T) {
	t.Parallel()

	var gotReferer, gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("HTTP-Referer")
		gotTitle = r.Header.Get("X-Title")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	transport := &attributionTransport{
		base:        http.DefaultTransport,
		httpReferer: "https://example.com/app",
		appTitle:    "nib",
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	resp.Body.Close()

	if gotReferer != "https://example.com/app" {
		t.Errorf("HTTP-Referer = %q, want %q", gotReferer, "https://example.com/app")
	}
	if gotTitle != "nib" {
		t.Errorf("X-Title = %q, want %q", gotTitle, "nib")
	}
}

func TestAttributionTransportOmitsEmptyHeaders(t *testing.T) {
	t.Parallel()

	var gotReferer, gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("HTTP-Referer")
		gotTitle = r.Header.Get("X-Title")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{
		Transport: &attributionTransport{base: http.DefaultTransport},
		Timeout:   5 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	resp.Body.Close()

	if gotReferer != "" {
		t.Errorf("HTTP-Referer = %q, want empty", gotReferer)
	}
	if gotTitle != "" {
		t.Errorf("X-Title = %q, want empty", gotTitle)
	}
}

func TestNewHTTPClientTimeout(t *testing.T) {
	t.Parallel()

	client := newHTTPClient(30, "", "")
	if client.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", client.Timeout)
	}

	noTimeout := newHTTPClient(0, "", "")
	if noTimeout.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0 (unset)", noTimeout.Timeout)
	}
}

func TestNewHTTPClientUsesAttributionTransport(t *testing.T) {
	t.Parallel()

	client := newHTTPClient(0, "https://ref.example", "title")
	transport, ok := client.Transport.(*attributionTransport)
	if !ok {
		t.Fatalf("Transport type = %T, want *attributionTransport", client.Transport)
	}
	if transport.httpReferer != "https://ref.example" {
		t.Errorf("httpReferer = %q, want %q", transport.httpReferer, "https://ref.example")
	}
	if transport.appTitle != "title" {
		t.Errorf("appTitle = %q, want %q", transport.appTitle, "title")
	}
}
