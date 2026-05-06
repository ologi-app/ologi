//go:build linux

// Linux typewriter: shells out to xdotool. xdotool is a thin wrapper
// over libxtst that handles keysym mapping + Unicode for us, so we
// don't have to reimplement that in CGo. It's a runtime dependency
// (apt install xdotool / dnf install xdotool) — the install.sh script
// flags it if missing.
//
// One subprocess per HandleTranscript / Type call. With partials
// arriving at AAI's natural cadence (~every few hundred ms), that's
// a few exec.Command per dictation session — fine.
package typewriter

import (
	"log"
	"os/exec"
	"strconv"
	"sync"
	"unicode/utf8"
)

type TypeWriter struct {
	mu             sync.Mutex
	lastPartialLen int
}

func NewTypeWriter() *TypeWriter {
	if _, err := exec.LookPath("xdotool"); err != nil {
		log.Printf("[typewriter] WARNING: xdotool not found in PATH. Install it (apt install xdotool / dnf install xdotool) so transcripts can be typed into focused windows.")
	}
	return &TypeWriter{}
}

func (tw *TypeWriter) HandleTranscript(text string, isFinal bool) {
	if text == "" {
		return
	}
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.lastPartialLen > 0 {
		runBackspace(tw.lastPartialLen)
	}
	runType(text)

	if isFinal {
		tw.lastPartialLen = 0
	} else {
		tw.lastPartialLen = utf8.RuneCountInString(text)
	}
}

func (tw *TypeWriter) Type(text string) {
	if text == "" {
		return
	}
	tw.mu.Lock()
	defer tw.mu.Unlock()
	runType(text)
}

func (tw *TypeWriter) Reset() {
	tw.mu.Lock()
	tw.lastPartialLen = 0
	tw.mu.Unlock()
}

// runType sends `text` to the focused window. The trailing `--`
// keeps xdotool from treating leading dashes in `text` as flags.
func runType(text string) {
	cmd := exec.Command("xdotool", "type", "--clearmodifiers", "--delay", "1", "--", text)
	if err := cmd.Run(); err != nil {
		log.Printf("[typewriter] xdotool type failed: %v", err)
	}
}

func runBackspace(n int) {
	if n <= 0 {
		return
	}
	cmd := exec.Command("xdotool", "key", "--clearmodifiers", "--repeat", strconv.Itoa(n), "--delay", "1", "BackSpace")
	if err := cmd.Run(); err != nil {
		log.Printf("[typewriter] xdotool key BackSpace failed: %v", err)
	}
}
