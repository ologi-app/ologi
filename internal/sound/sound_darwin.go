//go:build darwin

package sound

/*
#cgo LDFLAGS: -framework AudioToolbox
#include <AudioToolbox/AudioToolbox.h>
#include <CoreFoundation/CoreFoundation.h>

static void playSound(const char *path) {
	CFStringRef cfPath = CFStringCreateWithCString(NULL, path, kCFStringEncodingUTF8);
	CFURLRef url = CFURLCreateWithFileSystemPath(NULL, cfPath, kCFURLPOSIXPathStyle, false);
	SystemSoundID soundID;
	AudioServicesCreateSystemSoundID(url, &soundID);
	AudioServicesPlaySystemSound(soundID);
	CFRelease(url);
	CFRelease(cfPath);
	// Note: soundID leaks slightly, but for two short sounds it's negligible
}
*/
import "C"

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"
)

var sounds = []string{
	"Basso", "Blow", "Bottle", "Frog", "Funk", "Glass",
	"Hero", "Morse", "Ping", "Pop", "Purr", "Sosumi",
	"Submarine", "Tink",
}

var startSoundName string
var stopSoundName string

// InitSounds sets the sound names. Call once at startup.
func InitSounds(startName, stopName string) {
	if startName != "" {
		startSoundName = startName
	}
	if stopName != "" {
		stopSoundName = stopName
	}
}

func playNamedSound(name string) {
	path := C.CString("/System/Library/Sounds/" + name + ".aiff")
	defer C.free(unsafe.Pointer(path))
	C.playSound(path)
}

func PlayStartSound() {
	playNamedSound(startSoundName)
}

func PlayStopSound() {
	playNamedSound(stopSoundName)
}

func TestSounds() {
	fmt.Println("Available sounds:")
	for i, s := range sounds {
		marker := ""
		if s == startSoundName {
			marker += " [start]"
		}
		if s == stopSoundName {
			marker += " [stop]"
		}
		fmt.Printf("  %2d) %s%s\n", i+1, s, marker)
	}
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  <number>    — preview sound")
	fmt.Println("  <number>!   — set as start sound")
	fmt.Println("  <number>?   — set as stop sound")
	fmt.Println("  q           — quit")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "q" || input == "" {
			return
		}

		setStart := strings.HasSuffix(input, "!")
		setStop := strings.HasSuffix(input, "?")
		numStr := strings.TrimRight(input, "!?")

		n := 0
		for _, c := range numStr {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			} else {
				n = -1
				break
			}
		}

		if n < 1 || n > len(sounds) {
			fmt.Println("  invalid — enter 1-", len(sounds))
			continue
		}

		name := sounds[n-1]

		if setStart {
			startSoundName = name
			fmt.Printf("  start sound → %s\n", name)
			playNamedSound(name)
			time.Sleep(300 * time.Millisecond)
		} else if setStop {
			stopSoundName = name
			fmt.Printf("  stop sound → %s\n", name)
			playNamedSound(name)
			time.Sleep(300 * time.Millisecond)
		} else {
			fmt.Printf("  playing %s...\n", name)
			playNamedSound(name)
			time.Sleep(500 * time.Millisecond)
		}
	}
}
