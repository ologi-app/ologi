// Package launchd writes and controls the app.ologi.voice LaunchAgent.
package launchd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// Label is the canonical launchd label.
const Label = "app.ologi.voice"

// PlistSpec fully describes the desired plist state.
type PlistSpec struct {
	Label      string
	BinaryPath string
	Args       []string
	HomeDir    string
	Autostart  bool
	Env        map[string]string
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key><string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
      <string>{{.BinaryPath}}</string>
{{range .Args}}      <string>{{.}}</string>
{{end}}    </array>
    <key>RunAtLoad</key>{{if .Autostart}}<true/>{{else}}<false/>{{end}}
    <key>KeepAlive</key>
    <dict>
      <key>Crashed</key><true/>
      <key>SuccessfulExit</key><false/>
    </dict>
    <key>StandardOutPath</key><string>{{.HomeDir}}/Library/Logs/ologi-voice.log</string>
    <key>StandardErrorPath</key><string>{{.HomeDir}}/Library/Logs/ologi-voice.log</string>
{{if .Env}}    <key>EnvironmentVariables</key>
    <dict>
{{range $k, $v := .Env}}      <key>{{$k}}</key><string>{{$v}}</string>
{{end}}    </dict>
{{end}}  </dict>
</plist>
`

var tmpl = template.Must(template.New("plist").Parse(plistTemplate))

// RenderPlist renders a spec into the XML string.
func RenderPlist(spec PlistSpec) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, spec); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// PlistPath returns ~/Library/LaunchAgents/app.ologi.voice.plist.
func PlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

// WritePlist renders and writes the plist to its canonical location,
// creating ~/Library/LaunchAgents if necessary.
func WritePlist(spec PlistSpec) error {
	out, err := RenderPlist(spec)
	if err != nil {
		return err
	}
	path := PlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir LaunchAgents: %w", err)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}
	return nil
}

// RemovePlist deletes the plist file. Returns nil if it didn't exist.
func RemovePlist() error {
	err := os.Remove(PlistPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
