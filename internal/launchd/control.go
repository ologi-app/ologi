//go:build darwin

package launchd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// domainTarget returns the gui/<uid> string for launchctl.
func domainTarget() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

// serviceTarget returns gui/<uid>/app.ologi.voice.
func serviceTarget() string {
	return domainTarget() + "/" + Label
}

// Bootstrap loads the plist. Must be called after WritePlist.
// If already loaded, returns nil (idempotent-ish).
func Bootstrap() error {
	path := PlistPath()
	cmd := exec.Command("launchctl", "bootstrap", domainTarget(), path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// launchctl returns non-zero when the service is already
		// loaded. Detect that and treat as success.
		if strings.Contains(string(out), "already loaded") || strings.Contains(string(out), "Service is disabled") {
			return nil
		}
		return fmt.Errorf("bootstrap: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Bootout unloads the plist. Returns nil if not loaded.
func Bootout() error {
	cmd := exec.Command("launchctl", "bootout", serviceTarget())
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "Could not find service") ||
			strings.Contains(string(out), "No such process") {
			return nil
		}
		return fmt.Errorf("bootout: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Kickstart forces a restart of the loaded service. Use after a binary
// upgrade while the service is loaded.
func Kickstart() error {
	cmd := exec.Command("launchctl", "kickstart", "-k", serviceTarget())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kickstart: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// IsLoaded returns true if the service is currently loaded (whether
// running or not).
func IsLoaded() (bool, error) {
	cmd := exec.Command("launchctl", "print", serviceTarget())
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// launchctl exits non-zero when the service isn't found — treat as
	// "not loaded" rather than an error.
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, err
}

// Print returns the raw `launchctl print <service>` output for status
// reporting. Returns "" if the service isn't loaded.
func Print() (string, error) {
	cmd := exec.Command("launchctl", "print", serviceTarget())
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Not loaded — return "" for the caller to show "stopped".
		return "", nil
	}
	return string(out), nil
}
