//go:build !darwin

// Stub implementations for non-darwin builds. On Linux/Windows the
// background daemon mode isn't supported in v1 — every entry point
// returns ErrNotSupported. Callers (cmd_voice.go) surface the error
// to the user.
package launchd

import "errors"

const Label = "app.ologi.voice"

// ErrNotSupported is returned by every launchd operation on non-darwin
// builds. The CLI's voiceStart / voiceStop / voiceAutostart / voiceStatus
// commands surface this so users know to use foreground mode instead.
var ErrNotSupported = errors.New("background daemon mode isn't supported on this platform — run `ologi voice run` instead")

// PlistSpec mirrors the darwin shape so cmd_voice.go compiles unchanged.
type PlistSpec struct {
	Label      string
	BinaryPath string
	Args       []string
	HomeDir    string
	Autostart  bool
	Env        map[string]string
}

func IsLoaded() (bool, error)         { return false, nil }
func Bootstrap() error                { return ErrNotSupported }
func Bootout() error                  { return ErrNotSupported }
func WritePlist(spec PlistSpec) error { return ErrNotSupported }
func RemovePlist() error              { return ErrNotSupported }
func Kickstart() error                { return ErrNotSupported }
