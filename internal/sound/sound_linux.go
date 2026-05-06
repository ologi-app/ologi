//go:build linux

// Linux sound: silent for v1. Mac uses /System/Library/Sounds/ via
// AudioToolbox; the freedesktop equivalent (canberra-gtk-play with a
// theme name) varies too much across distros to ship reliably. Users
// can ask for sounds later if they actually want them.
package sound

func InitSounds(start, stop string) {}
func PlayStartSound()                {}
func PlayStopSound()                 {}
