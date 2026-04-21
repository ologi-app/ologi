package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/voice/config" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"settings_version":5,"hotkey":"right_option","language":"en","mic_device":null,"start_sound":"Tink","stop_sound":"Pop","replacements":[{"pattern":"hi","replacement":"HI"}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ht_test")
	cfg, err := c.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg.SettingsVersion != 5 {
		t.Errorf("settings_version: got %d, want 5", cfg.SettingsVersion)
	}
	if cfg.Hotkey != "right_option" {
		t.Errorf("hotkey: got %q", cfg.Hotkey)
	}
	if len(cfg.Replacements) != 1 || cfg.Replacements[0].Pattern != "hi" {
		t.Errorf("replacements: %+v", cfg.Replacements)
	}
}

func TestMintRealtimeToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/voice/realtime-token" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"token":"tok-xyz"}`))
	}))
	defer srv.Close()

	tok, err := NewClient(srv.URL, "ht_test").MintRealtimeToken()
	if err != nil {
		t.Fatalf("MintRealtimeToken: %v", err)
	}
	if tok != "tok-xyz" {
		t.Errorf("token: got %q, want %q", tok, "tok-xyz")
	}
}

func TestPostSession(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/voice/sessions" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"session_id":"s1","canonical_text":"hello world","settings_version":7}`))
	}))
	defer srv.Close()

	start := time.Date(2026, 4, 21, 10, 42, 0, 0, time.UTC)
	end := start.Add(14 * time.Second)
	src := "Google Chrome / claude.ai"
	resp, err := NewClient(srv.URL, "ht_test").PostSession(PostSessionInput{
		Mode:       "stream",
		StartedAt:  start,
		EndedAt:    end,
		DurationMs: 14_000,
		SourceApp:  &src,
		Text:       "hello world",
	})
	if err != nil {
		t.Fatalf("PostSession: %v", err)
	}
	if resp.SessionID != "s1" {
		t.Errorf("session_id: got %q", resp.SessionID)
	}
	if resp.CanonicalText != "hello world" {
		t.Errorf("canonical_text: got %q", resp.CanonicalText)
	}
	if resp.SettingsVersion != 7 {
		t.Errorf("settings_version: got %d, want 7", resp.SettingsVersion)
	}
	if gotBody["mode"] != "stream" || gotBody["duration_ms"].(float64) != 14000 {
		t.Errorf("body payload wrong: %+v", gotBody)
	}
	if gotBody["source_app"] != src {
		t.Errorf("source_app: got %v", gotBody["source_app"])
	}
}
