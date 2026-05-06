//go:build darwin

package main

import "os/exec"

// openURL launches the default browser to the given URL. macOS uses
// `open`. Errors propagate so callers can hint the user to visit the
// URL manually.
func openURL(url string) error {
	return exec.Command("open", url).Start()
}
