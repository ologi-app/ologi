//go:build darwin

package typewriter

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <unistd.h>

static inline void pttTypeChar(UniChar ch) {
	CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, 0, true);
	CGEventRef keyUp   = CGEventCreateKeyboardEvent(NULL, 0, false);
	CGEventKeyboardSetUnicodeString(keyDown, 1, &ch);
	CGEventKeyboardSetUnicodeString(keyUp, 1, &ch);
	// Clear all modifier flags so held keys (Ctrl, Option, etc.) don't affect the output
	CGEventSetFlags(keyDown, 0);
	CGEventSetFlags(keyUp, 0);
	CGEventPost(kCGHIDEventTap, keyDown);
	CGEventPost(kCGHIDEventTap, keyUp);
	CFRelease(keyDown);
	CFRelease(keyUp);
	usleep(1500); // 1.5ms between keystrokes
}

static inline void typeBackspace(void) {
	CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)51, true);
	CGEventRef keyUp   = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)51, false);
	CGEventSetFlags(keyDown, 0);
	CGEventSetFlags(keyUp, 0);
	CGEventPost(kCGHIDEventTap, keyDown);
	CGEventPost(kCGHIDEventTap, keyUp);
	CFRelease(keyDown);
	CFRelease(keyUp);
	usleep(1000); // 1ms between backspaces (faster than typing)
}
*/
import "C"

import (
	"sync"
	"unicode/utf16"
)

type TypeWriter struct {
	mu             sync.Mutex
	lastPartialLen int
}

func NewTypeWriter() *TypeWriter {
	return &TypeWriter{}
}

// HandleTranscript processes a partial or final transcript from AssemblyAI.
// For partials: erases previous partial text and types the new one.
// For finals: erases previous partial, types final text, and resets the counter.
func (tw *TypeWriter) HandleTranscript(text string, isFinal bool) {
	if text == "" {
		return
	}

	tw.mu.Lock()
	defer tw.mu.Unlock()

	// Erase previous partial
	for i := 0; i < tw.lastPartialLen; i++ {
		C.typeBackspace()
	}

	// Type new text
	runes := []rune(text)
	for _, r := range runes {
		pairs := utf16.Encode([]rune{r})
		for _, ch := range pairs {
			C.pttTypeChar(C.UniChar(ch))
		}
	}

	if isFinal {
		tw.lastPartialLen = 0
	} else {
		tw.lastPartialLen = len(runes)
	}
}

// Type outputs text at the cursor position. No partial tracking.
func (tw *TypeWriter) Type(text string) {
	if text == "" {
		return
	}
	tw.mu.Lock()
	defer tw.mu.Unlock()

	for _, r := range text {
		pairs := utf16.Encode([]rune{r})
		for _, ch := range pairs {
			C.pttTypeChar(C.UniChar(ch))
		}
	}
}

// Reset clears the partial counter without typing anything.
func (tw *TypeWriter) Reset() {
	tw.mu.Lock()
	tw.lastPartialLen = 0
	tw.mu.Unlock()
}
