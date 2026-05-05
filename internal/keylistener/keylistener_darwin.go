package keylistener

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

extern void goKeyCallback(int keyDown, int shiftHeld);
extern void goRecordCallback(int keycode, uint64_t flags);

static CGEventRef recordCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *userInfo) {
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		CGEventTapEnable(*(CFMachPortRef *)userInfo, true);
		return event;
	}
	if (type == kCGEventFlagsChanged || type == kCGEventKeyDown) {
		CGKeyCode keycode = (CGKeyCode)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
		CGEventFlags flags = CGEventGetFlags(event);
		goRecordCallback((int)keycode, (uint64_t)flags);
	}
	return event;
}

static int startRecordTap(void) {
	static CFMachPortRef tap;
	static char userInfo[sizeof(CFMachPortRef)];

	CGEventMask mask = CGEventMaskBit(kCGEventFlagsChanged) | CGEventMaskBit(kCGEventKeyDown);
	tap = CGEventTapCreate(
		kCGSessionEventTap,
		kCGHeadInsertEventTap,
		kCGEventTapOptionListenOnly,
		mask,
		recordCallback,
		userInfo
	);
	if (!tap) return -1;

	*(CFMachPortRef *)userInfo = tap;
	CFRunLoopSourceRef source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
	CFRunLoopAddSource(CFRunLoopGetCurrent(), source, kCFRunLoopCommonModes);
	CGEventTapEnable(tap, true);
	CFRelease(source);
	CFRunLoopRun();
	return 0;
}

static CGEventRef eventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *userInfo) {
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		// Re-enable the tap
		CGEventTapEnable(*(CFMachPortRef *)userInfo, true);
		return event;
	}

	CGKeyCode keycode = (CGKeyCode)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);

	int targetKeycode = *(int *)((char *)userInfo + sizeof(CFMachPortRef));
	uint64_t flagMask = *(uint64_t *)((char *)userInfo + sizeof(CFMachPortRef) + sizeof(int));

	if ((int)keycode != targetKeycode) {
		return event;
	}

	CGEventFlags flags = CGEventGetFlags(event);

	if (flagMask != 0 && type == kCGEventFlagsChanged) {
		// Modifier key: detect down/up via flag presence.
		int isDown = (flags & flagMask) != 0;
		int shiftHeld = (flags & kCGEventFlagMaskShift) != 0;
		goKeyCallback(isDown, shiftHeld);
	} else if (flagMask == 0 && (type == kCGEventKeyDown || type == kCGEventKeyUp)) {
		// Regular key: kCGEventKeyDown = pressed, kCGEventKeyUp = released.
		int isDown = (type == kCGEventKeyDown);
		int shiftHeld = (flags & kCGEventFlagMaskShift) != 0;
		goKeyCallback(isDown, shiftHeld);
	}

	return event;
}

static int startEventTap(int keycode, uint64_t flagMask) {
	static CFMachPortRef tap;
	static int storedKeycode;
	storedKeycode = keycode;

	// We need to pass both the tap ref, keycode, and flagMask to the callback.
	// For simplicity, use a struct-like layout in a static buffer.
	static char userInfo[sizeof(CFMachPortRef) + sizeof(int) + sizeof(uint64_t)];

	CGEventMask mask = CGEventMaskBit(kCGEventFlagsChanged) | CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventKeyUp);

	tap = CGEventTapCreate(
		kCGSessionEventTap,
		kCGHeadInsertEventTap,
		kCGEventTapOptionListenOnly,
		mask,
		eventCallback,
		userInfo
	);

	if (!tap) {
		return -1; // No accessibility permission
	}

	// Store tap, keycode, and flagMask in userInfo for the callback
	*(CFMachPortRef *)userInfo = tap;
	*(int *)(userInfo + sizeof(CFMachPortRef)) = keycode;
	*(uint64_t *)(userInfo + sizeof(CFMachPortRef) + sizeof(int)) = flagMask;

	CFRunLoopSourceRef source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
	CFRunLoopAddSource(CFRunLoopGetCurrent(), source, kCFRunLoopCommonModes);
	CGEventTapEnable(tap, true);
	CFRelease(source);

	CFRunLoopRun(); // Blocks forever
	return 0;
}
*/
import "C"

import (
	"fmt"
	"log"
	"time"
)

// KeyEvent represents a key press or release.
type KeyEvent struct {
	Down  bool
	Shift bool // whether shift was held during this event
}

var keyEventCh chan KeyEvent

//export goKeyCallback
func goKeyCallback(keyDown C.int, shiftHeld C.int) {
	if keyEventCh != nil {
		log.Printf("[keys] raw event: down=%v shift=%v", keyDown != 0, shiftHeld != 0)
		select {
		case keyEventCh <- KeyEvent{Down: keyDown != 0, Shift: shiftHeld != 0}:
		default:
		}
	}
}

// recordEventCh receives raw keycode events during --record-key.
var recordEventCh chan RecordedKey

// RecordedKey holds info about a detected modifier key press.
type RecordedKey struct {
	Keycode int
	Flags   uint64
}

//export goRecordCallback
func goRecordCallback(keycode C.int, flags C.uint64_t) {
	if recordEventCh != nil {
		select {
		case recordEventCh <- RecordedKey{Keycode: int(keycode), Flags: uint64(flags)}:
		default:
		}
	}
}

// keycodeToBindName maps macOS keycodes back to config names.
func keycodeToBindName(keycode int) string {
	names := map[int]string{
		61: "right_option",
		58: "left_option",
		54: "right_command",
		55: "left_command",
		62: "right_control",
		59: "left_control",
		60: "right_shift",
		56: "left_shift",
		63: "fn",
		// Function keys
		122: "f1",
		120: "f2",
		99:  "f3",
		118: "f4",
		96:  "f5",
		97:  "f6",
		98:  "f7",
		100: "f8",
		101: "f9",
		109: "f10",
		103: "f11",
		111: "f12",
	}
	if name, ok := names[keycode]; ok {
		return name
	}
	return ""
}

// RecordKey listens for any modifier key press and returns its config name.
func RecordKey() (string, error) {
	recordEventCh = make(chan RecordedKey, 16)

	go func() {
		result := C.startRecordTap()
		if result == -1 {
			log.Fatal("[keys] failed to create event tap — grant Accessibility permission in System Settings > Privacy & Security > Accessibility")
		}
	}()

	// Wait for a key-down event
	for ev := range recordEventCh {
		name := keycodeToBindName(ev.Keycode)
		if name != "" {
			return name, nil
		}
		return "", fmt.Errorf("detected keycode %d which is not a supported key", ev.Keycode)
	}
	return "", fmt.Errorf("no key detected")
}

// keycodeForBind maps config keybind names to macOS keycodes.
func keycodeForBind(name string) (int, error) {
	codes := map[string]int{
		"right_option":  61,
		"left_option":   58,
		"right_command": 54,
		"left_command":  55,
		"right_control": 62,
		"left_control":  59,
		"right_shift":   60,
		"left_shift":    56,
		"fn":            63,
		"f1":            122,
		"f2":            120,
		"f3":            99,
		"f4":            118,
		"f5":            96,
		"f6":            97,
		"f7":            98,
		"f8":            100,
		"f9":            101,
		"f10":           109,
		"f11":           103,
		"f12":           111,
	}
	code, ok := codes[name]
	if !ok {
		return 0, fmt.Errorf("unknown keybind %q", name)
	}
	return code, nil
}

// flagMaskForBind maps keybind names to their CGEvent flag masks.
// Returns 0 for regular (non-modifier) keys like F1-F12.
func flagMaskForBind(name string) (uint64, error) {
	masks := map[string]uint64{
		"right_option":  0x80000,  // kCGEventFlagMaskAlternate
		"left_option":   0x80000,  // kCGEventFlagMaskAlternate
		"right_command": 0x100000, // kCGEventFlagMaskCommand
		"left_command":  0x100000, // kCGEventFlagMaskCommand
		"right_control": 0x40000,  // kCGEventFlagMaskControl
		"left_control":  0x40000,  // kCGEventFlagMaskControl
		"right_shift":   0x20000,  // kCGEventFlagMaskShift
		"left_shift":    0x20000,  // kCGEventFlagMaskShift
		"fn":            0x800000, // kCGEventFlagMaskSecondaryFn
		// Regular keys use 0 — matched by keycode only.
		"f1": 0, "f2": 0, "f3": 0, "f4": 0, "f5": 0, "f6": 0,
		"f7": 0, "f8": 0, "f9": 0, "f10": 0, "f11": 0, "f12": 0,
	}
	mask, ok := masks[name]
	if !ok {
		return 0, fmt.Errorf("unknown keybind %q for flag mask", name)
	}
	return mask, nil
}

// StartKeyListener begins monitoring the global keybind. Key events are sent to
// the returned channel. The EventTap runs on its own goroutine with a CFRunLoop.
// StartKeyListener begins watching for the given key. tapMode is "single"
// (raw press/release passes through) or "double" (engages on
// double-tap-and-hold; see DoubleTapFilter).
func StartKeyListener(keybind, tapMode string) (chan KeyEvent, error) {
	code, err := keycodeForBind(keybind)
	if err != nil {
		return nil, err
	}
	mask, err := flagMaskForBind(keybind)
	if err != nil {
		return nil, err
	}

	keyEventCh = make(chan KeyEvent, 16)

	go func() {
		log.Printf("[keys] listening for %s (keycode %d, flagMask 0x%x)", keybind, code, mask)
		result := C.startEventTap(C.int(code), C.uint64_t(mask))
		if result == -1 {
			log.Fatal("[keys] failed to create event tap — grant Accessibility permission in System Settings > Privacy & Security > Accessibility")
		}
	}()

	// In single-tap mode, every press/release pair is a session. In
	// double-tap mode (default), gate engagement behind double-tap-and-hold.
	if tapMode == "single" {
		return keyEventCh, nil
	}
	return DoubleTapFilter(keyEventCh), nil
}

// DoubleTapFilter converts raw key events into double-tap-and-hold events.
// Pattern: press, release, press (hold) within 300ms → engaged.
// Release after engaged → disengaged.
func DoubleTapFilter(raw chan KeyEvent) chan KeyEvent {
	out := make(chan KeyEvent, 16)
	const window = 300 * time.Millisecond

	go func() {
		// States: idle → sawFirstDown → sawFirstUp → engaged
		type state int
		const (
			idle        state = iota
			sawFirstDown
			sawFirstUp
			engaged
		)

		st := idle
		var firstUpTime time.Time

		var engageShift bool
		for ev := range raw {
			switch st {
			case idle:
				if ev.Down {
					st = sawFirstDown
				}
			case sawFirstDown:
				if !ev.Down {
					// First tap released
					st = sawFirstUp
					firstUpTime = time.Now()
				}
			case sawFirstUp:
				if ev.Down {
					if time.Since(firstUpTime) <= window {
						// Second press within window — engage!
						st = engaged
						engageShift = ev.Shift
						select {
						case out <- KeyEvent{Down: true, Shift: engageShift}:
						default:
						}
					} else {
						// Too slow — treat as new first tap
						st = sawFirstDown
					}
				}
			case engaged:
				if !ev.Down {
					// Released after engaged
					st = idle
					select {
					case out <- KeyEvent{Down: false, Shift: engageShift}:
					default:
					}
				}
			}
		}
	}()

	return out
}
