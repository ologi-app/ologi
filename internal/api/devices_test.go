package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPatchDevice_uploadsMicListAndVersion(t *testing.T) {
	var seenPath, seenAuth, seenBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ht_testkey")
	if err := c.PatchDevice("dev-1", []string{"Built-in", "Logitech"}, "0.1.0"); err != nil {
		t.Fatalf("PatchDevice: %v", err)
	}
	if seenPath != "/api/voice/devices/dev-1" {
		t.Errorf("path = %q", seenPath)
	}
	if seenAuth != "Bearer ht_testkey" {
		t.Errorf("auth = %q", seenAuth)
	}
	if !strings.Contains(seenBody, `"available_mics":["Built-in","Logitech"]`) {
		t.Errorf("body missing available_mics: %s", seenBody)
	}
	if !strings.Contains(seenBody, `"cli_version":"0.1.0"`) {
		t.Errorf("body missing cli_version: %s", seenBody)
	}
}

func TestGetConfig_decodesEffectiveSettings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"settings_version": 7,
			"hotkey":           "left_command",
			"language":         "en",
			"mic_device":       "Device Mic",
			"start_sound":      "Tink",
			"stop_sound":       "Pop",
			"replacements":     []any{},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ht_testkey")
	cfg, err := c.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg.Hotkey != "left_command" {
		t.Errorf("Hotkey = %q", cfg.Hotkey)
	}
	if cfg.MicDevice == nil || *cfg.MicDevice != "Device Mic" {
		t.Errorf("MicDevice = %v", cfg.MicDevice)
	}
	if cfg.SettingsVersion != 7 {
		t.Errorf("SettingsVersion = %d", cfg.SettingsVersion)
	}
}
