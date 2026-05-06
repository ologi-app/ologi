//go:build linux

// Linux source-app detection: shell out to xdotool to get the active
// window title. Browser tab host detection is Mac-only for v1 (it
// relied on AppleScript). Linux returns just the window title.
package sourceapp

import (
	"context"
	"os/exec"
	"strings"
)

func detectImpl() string {
	ctx, cancel := context.WithTimeout(context.Background(), DetectTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "xdotool", "getactivewindow", "getwindowname").Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(out))
	return name
}
