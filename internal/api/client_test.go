package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSendsBearerAuth(t *testing.T) {
	var gotAuth, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ht_test")
	c.Version = "0.0.1-test"
	if _, err := c.do("GET", "/api/voice/config", nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotAuth != "Bearer ht_test" {
		t.Errorf("auth header: got %q, want %q", gotAuth, "Bearer ht_test")
	}
	if !strings.Contains(gotUA, "ologi/0.0.1-test") {
		t.Errorf("user-agent: got %q, want contains ologi/0.0.1-test", gotUA)
	}
}

func TestClientReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ht_test")
	_, err := c.do("GET", "/api/voice/config", nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("got %T, want *APIError", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("status: got %d, want 401", apiErr.StatusCode)
	}
	if apiErr.Message != "Unauthorized" {
		t.Errorf("message: got %q, want %q", apiErr.Message, "Unauthorized")
	}
}

func TestClientAuthErrorHelper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "ht_test").do("GET", "/x", nil)
	if !IsAuthError(err) {
		t.Errorf("IsAuthError(%v) = false, want true", err)
	}
}
