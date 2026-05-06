//go:build linux

package main

import "os/exec"

// openURL launches the default browser to the given URL. Linux uses
// xdg-open (most desktops have it; falls back to GNOME's gio or KDE's
// kde-open if not).
func openURL(url string) error {
	if err := exec.Command("xdg-open", url).Start(); err == nil {
		return nil
	}
	if err := exec.Command("gio", "open", url).Start(); err == nil {
		return nil
	}
	return exec.Command("kde-open", url).Start()
}
