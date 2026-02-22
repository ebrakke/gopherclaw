package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadURLName(t *testing.T) {
	r := NewReadURL()
	if r.Name() != "read_url" {
		t.Errorf("expected 'read_url', got %q", r.Name())
	}
}

func TestReadURLExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1>Hello World</h1><p>This is a test.</p></body></html>`))
	}))
	defer server.Close()

	r := NewReadURL()
	args, _ := json.Marshal(map[string]string{"url": server.URL})
	result, err := r.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Hello World") {
		t.Errorf("expected 'Hello World' in result, got %q", result)
	}
	if !strings.Contains(result, "This is a test") {
		t.Errorf("expected 'This is a test' in result, got %q", result)
	}
}

func TestReadURLMissingURL(t *testing.T) {
	r := NewReadURL()
	args, _ := json.Marshal(map[string]string{})
	_, err := r.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestReadURLTruncation(t *testing.T) {
	long := strings.Repeat("x", 60000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><p>" + long + "</p></body></html>"))
	}))
	defer server.Close()

	r := NewReadURL()
	args, _ := json.Marshal(map[string]string{"url": server.URL})
	result, err := r.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) > 51000 {
		t.Errorf("expected truncation, got length %d", len(result))
	}
}
