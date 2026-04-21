package launchd

import (
	"strings"
	"testing"
)

func TestRenderPlistAutostart(t *testing.T) {
	out, err := RenderPlist(PlistSpec{
		Label:      "app.ologi.voice",
		BinaryPath: "/opt/homebrew/bin/ologi",
		Args:       []string{"voice", "run"},
		HomeDir:    "/Users/test",
		Autostart:  true,
		Env:        map[string]string{"OLOGI_SERVER_URL": "https://voice.ologi.app"},
	})
	if err != nil {
		t.Fatalf("RenderPlist: %v", err)
	}
	if !strings.Contains(out, "<string>app.ologi.voice</string>") {
		t.Error("missing label")
	}
	if !strings.Contains(out, "<string>/opt/homebrew/bin/ologi</string>") {
		t.Error("missing binary path")
	}
	if !strings.Contains(out, "<string>voice</string>") || !strings.Contains(out, "<string>run</string>") {
		t.Error("missing args")
	}
	if !strings.Contains(out, "<key>RunAtLoad</key><true/>") {
		t.Error("missing RunAtLoad=true")
	}
	if !strings.Contains(out, "/Users/test/Library/Logs/ologi-voice.log") {
		t.Error("missing log path")
	}
	if !strings.Contains(out, "<key>OLOGI_SERVER_URL</key><string>https://voice.ologi.app</string>") {
		t.Error("missing env var")
	}
}

func TestRenderPlistNoAutostart(t *testing.T) {
	out, err := RenderPlist(PlistSpec{
		Label:      "app.ologi.voice",
		BinaryPath: "/usr/local/bin/ologi",
		Args:       []string{"voice", "run"},
		HomeDir:    "/Users/test",
		Autostart:  false,
	})
	if err != nil {
		t.Fatalf("RenderPlist: %v", err)
	}
	if !strings.Contains(out, "<key>RunAtLoad</key><false/>") {
		t.Error("missing RunAtLoad=false")
	}
}
