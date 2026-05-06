//go:build linux

package keylistener

/*
#cgo LDFLAGS: -lX11 -lXi
#include <X11/Xlib.h>
#include <X11/extensions/XInput2.h>
#include <X11/keysym.h>
#include <stdlib.h>
#include <stdio.h>

extern void goKeyCallback(int keyDown, int shiftHeld);

static Display *gDpy = NULL;
static int gXiOpcode = 0;
static int gTargetKeycode = 0;

// initX11 opens the display, verifies XInput2 is available, maps the
// target keysym to a hardware keycode, and selects raw key press/
// release events on the root window so we receive them regardless of
// focus AND without grabbing the key (other apps still see it).
//
// Return codes:
//   0  ok
//  -1  XOpenDisplay failed (no X server / no DISPLAY)
//  -2  XInput extension missing
//  -3  XInput 2.0 unsupported
//  -4  keysym -> keycode failed (unmapped key)
static int initX11(unsigned long targetKeysym) {
	XInitThreads();

	gDpy = XOpenDisplay(NULL);
	if (!gDpy) return -1;

	int event, error;
	if (!XQueryExtension(gDpy, "XInputExtension", &gXiOpcode, &event, &error)) {
		return -2;
	}

	int major = 2, minor = 0;
	if (XIQueryVersion(gDpy, &major, &minor) != Success) return -3;

	gTargetKeycode = XKeysymToKeycode(gDpy, targetKeysym);
	if (gTargetKeycode == 0) return -4;

	XIEventMask mask;
	unsigned char maskBits[XIMaskLen(XI_LASTEVENT)];
	memset(maskBits, 0, sizeof(maskBits));
	XISetMask(maskBits, XI_RawKeyPress);
	XISetMask(maskBits, XI_RawKeyRelease);
	mask.deviceid = XIAllMasterDevices;
	mask.mask_len = sizeof(maskBits);
	mask.mask = maskBits;

	XISelectEvents(gDpy, DefaultRootWindow(gDpy), &mask, 1);
	XSync(gDpy, False);
	return 0;
}

// runX11Loop blocks reading X events until the display closes.
// Each matching raw key event invokes goKeyCallback. Shift state is
// queried via XQueryPointer at event time (XIRawEvent doesn't carry
// modifier state on its own).
static void runX11Loop(void) {
	XEvent ev;
	XGenericEventCookie *cookie = &ev.xcookie;
	while (1) {
		XNextEvent(gDpy, &ev);
		if (cookie->type != GenericEvent || cookie->extension != gXiOpcode) {
			continue;
		}
		if (!XGetEventData(gDpy, cookie)) continue;

		XIRawEvent *re = (XIRawEvent *)cookie->data;
		if (re->detail == gTargetKeycode) {
			int down = (cookie->evtype == XI_RawKeyPress) ? 1 : 0;

			Window root_ret, child_ret;
			int rx, ry, wx, wy;
			unsigned int mask_ret = 0;
			XQueryPointer(gDpy, DefaultRootWindow(gDpy),
				&root_ret, &child_ret, &rx, &ry, &wx, &wy, &mask_ret);
			int shift = (mask_ret & ShiftMask) ? 1 : 0;

			goKeyCallback(down, shift);
		}
		XFreeEventData(gDpy, cookie);
	}
}
*/
import "C"

import (
	"fmt"
	"log"
	"os"
)

var keyEventCh chan KeyEvent

//export goKeyCallback
func goKeyCallback(keyDown C.int, shiftHeld C.int) {
	if keyEventCh != nil {
		select {
		case keyEventCh <- KeyEvent{Down: keyDown != 0, Shift: shiftHeld != 0}:
		default:
		}
	}
}

// keysymForBind maps the config hotkey name to its X11 keysym.
// "fn" isn't supported on Linux — Macs have a real fn keycode, Linux
// laptops don't have a portable equivalent.
func keysymForBind(name string) (C.ulong, error) {
	switch name {
	case "right_option":
		return C.XK_Alt_R, nil
	case "left_option":
		return C.XK_Alt_L, nil
	case "right_command":
		return C.XK_Super_R, nil
	case "left_command":
		return C.XK_Super_L, nil
	case "right_control":
		return C.XK_Control_R, nil
	case "left_control":
		return C.XK_Control_L, nil
	case "right_shift":
		return C.XK_Shift_R, nil
	case "left_shift":
		return C.XK_Shift_L, nil
	case "f1":
		return C.XK_F1, nil
	case "f2":
		return C.XK_F2, nil
	case "f3":
		return C.XK_F3, nil
	case "f4":
		return C.XK_F4, nil
	case "f5":
		return C.XK_F5, nil
	case "f6":
		return C.XK_F6, nil
	case "f7":
		return C.XK_F7, nil
	case "f8":
		return C.XK_F8, nil
	case "f9":
		return C.XK_F9, nil
	case "f10":
		return C.XK_F10, nil
	case "f11":
		return C.XK_F11, nil
	case "f12":
		return C.XK_F12, nil
	case "fn":
		return 0, fmt.Errorf("the fn key isn't supported on Linux — pick a function key (f1-f12) or modifier (option/command/control/shift) in the web settings")
	}
	return 0, fmt.Errorf("unsupported hotkey on Linux: %q", name)
}

// StartKeyListener begins watching the configured hotkey via XInput2
// raw events. tapMode is "single" (raw passthrough) or "double" (apply
// DoubleTapFilter). Wayland sessions need XWayland AND a working DISPLAY
// — pure Wayland will fail at XOpenDisplay.
func StartKeyListener(keybind, tapMode string) (chan KeyEvent, error) {
	if os.Getenv("DISPLAY") == "" {
		hint := ""
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			hint = " (pure Wayland session detected — ologi v1 requires X11. Switch to an X11 session via your display manager.)"
		}
		return nil, fmt.Errorf("DISPLAY is unset%s", hint)
	}

	keysym, err := keysymForBind(keybind)
	if err != nil {
		return nil, err
	}

	keyEventCh = make(chan KeyEvent, 16)

	switch rc := C.initX11(keysym); rc {
	case 0:
		// ok
	case -1:
		return nil, fmt.Errorf("could not open X11 display %q (Wayland: switch to an X11 session)", os.Getenv("DISPLAY"))
	case -2:
		return nil, fmt.Errorf("XInputExtension missing — install libxi or upgrade your X server")
	case -3:
		return nil, fmt.Errorf("XInput 2.0 unavailable — your X server is too old")
	case -4:
		return nil, fmt.Errorf("could not map keysym to keycode for %q — your keyboard layout may not have this key", keybind)
	default:
		return nil, fmt.Errorf("X11 init failed: %d", rc)
	}

	go func() {
		log.Printf("[keys] listening for %s (X11 raw events)", keybind)
		C.runX11Loop()
	}()

	if tapMode == "single" {
		return keyEventCh, nil
	}
	return DoubleTapFilter(keyEventCh), nil
}

// RecordKey is a no-op on Linux — the web settings page captures
// keystrokes directly via the browser's KeyboardEvent. The CLI flow
// using --record-key was Mac-only legacy.
func RecordKey() (string, error) {
	return "", fmt.Errorf("RecordKey isn't supported on Linux — use the web settings page (KeyRecorder)")
}
