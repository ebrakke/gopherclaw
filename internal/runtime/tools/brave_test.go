package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBraveSearchName(t *testing.T) {
	b := NewBraveSearch("test-key")
	if b.Name() != "brave_search" {
		t.Errorf("expected 'brave_search', got %q", b.Name())
	}
}

func TestBraveSearchExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "test-key" {
			t.Error("missing API key header")
		}
		if r.URL.Query().Get("q") != "golang testing" {
			t.Errorf("unexpected query: %s", r.URL.Query().Get("q"))
		}
		json.NewEncoder(w).Encode(braveResponse{
			Web: braveWeb{
				Results: []braveResult{
					{Title: "Go Testing", URL: "https://go.dev/testing", Description: "How to test in Go"},
					{Title: "Go Docs", URL: "https://go.dev/doc", Description: "Go documentation"},
				},
			},
		})
	}))
	defer server.Close()

	b := NewBraveSearch("test-key")
	b.baseURL = server.URL

	args, _ := json.Marshal(map[string]any{"query": "golang testing", "count": 2})
	result, err := b.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Go Testing") {
		t.Errorf("expected 'Go Testing' in result, got %q", result)
	}
	if !strings.Contains(result, "https://go.dev/testing") {
		t.Errorf("expected URL in result, got %q", result)
	}
}

func TestBraveSearchNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(braveResponse{})
	}))
	defer server.Close()

	b := NewBraveSearch("test-key")
	b.baseURL = server.URL

	args, _ := json.Marshal(map[string]string{"query": "xyznonexistent"})
	result, err := b.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "No results") {
		t.Errorf("expected 'No results', got %q", result)
	}
}
