// Package keylistener provides global hotkey detection. Per-OS impls
// (keylistener_darwin.go, keylistener_linux.go) emit raw KeyEvents on
// the channel returned by StartKeyListener; DoubleTapFilter wraps that
// channel with the shared press-release-press-and-hold gesture used by
// the engine.
package keylistener

import "time"

// KeyEvent is a single press or release of the configured hotkey.
// Down=true on press, Down=false on release. Shift is true if Shift was
// held at the time of the event (used to distinguish stream vs batch).
type KeyEvent struct {
	Down  bool
	Shift bool
}

// DoubleTapFilter converts raw key events into double-tap-and-hold
// engagement events. Pattern: press, release, press (held) within
// 300ms → emit Down. Subsequent release → emit Up. Anything that
// breaks the pattern resets to idle.
//
// Single-tap mode skips this entirely — the listener returns the raw
// channel so press = engage, release = disengage. See Mac/Linux
// StartKeyListener for the conditional.
func DoubleTapFilter(raw chan KeyEvent) chan KeyEvent {
	out := make(chan KeyEvent, 16)
	const window = 300 * time.Millisecond

	go func() {
		type state int
		const (
			idle state = iota
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
					st = sawFirstUp
					firstUpTime = time.Now()
				}
			case sawFirstUp:
				if ev.Down {
					if time.Since(firstUpTime) <= window {
						st = engaged
						engageShift = ev.Shift
						select {
						case out <- KeyEvent{Down: true, Shift: engageShift}:
						default:
						}
					} else {
						st = sawFirstDown
					}
				}
			case engaged:
				if !ev.Down {
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
