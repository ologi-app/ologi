// Package sourceapp detects the currently-focused macOS application,
// with best-effort augmentation for browsers (active tab host).
//
// Never crashes, never blocks indefinitely. On any error returns "".
// Callers treat the empty string as "no attribution available".
package sourceapp

import "time"

// DetectTimeout bounds the total cost of one Detect() call. AppleScript
// can occasionally hang for several seconds on the first call into an
// unresponsive browser; we cap the overall effort here.
const DetectTimeout = 250 * time.Millisecond

// Detect returns a human-friendly "<App Name> / <host>" when a browser
// is focused and its active tab URL can be read, otherwise just
// "<App Name>", otherwise "".
//
// Examples:
//   "Google Chrome / claude.ai"
//   "iTerm2"
//   ""  (nothing detected)
func Detect() string {
	return detectImpl()
}
