//go:build darwin

package sourceapp

import (
	"context"
	"net/url"
	"os/exec"
	"strings"
)

// browserTabHost returns the host (e.g. "claude.ai") of the active tab
// of the focused browser, or "" if none can be determined.
//
// Wraps each AppleScript call in a DetectTimeout context — some browsers
// occasionally block on the first automation call, and we'd rather
// report no-source than hang the dictation-stop codepath.
func browserTabHost(bundleID string) string {
	scripts := map[string]string{
		"com.google.Chrome":          `tell application "Google Chrome" to if (count of windows) > 0 then return URL of active tab of front window`,
		"com.apple.Safari":           `tell application "Safari" to if (count of windows) > 0 then return URL of front document`,
		"org.mozilla.firefox":        `tell application "Firefox" to if (count of windows) > 0 then return URL of active tab of front window`,
		"company.thebrowser.Browser": `tell application "Arc" to if (count of windows) > 0 then return URL of active tab of front window`,
	}
	script, ok := scripts[bundleID]
	if !ok {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), DetectTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if err != nil {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(string(out)))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
