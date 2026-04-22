package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ologi-app/ologi/internal/api"
	"github.com/ologi-app/ologi/internal/config"
	"github.com/ologi-app/ologi/internal/launchd"
)

func cmdLogin(args []string) {
	// Default device name is the machine's hostname.
	defaultName, _ := os.Hostname()
	fmt.Fprintf(os.Stderr, "Device name [%s]: ", defaultName)
	name := readLine()
	if name == "" {
		name = defaultName
	}

	// Pre-create a client with no API key for /login/start.
	serverOverride := os.Getenv("OLOGI_SERVER_URL")
	if serverOverride == "" {
		serverOverride = defaultServerURL
	}
	pre := api.NewClient(serverOverride, "")
	pre.Version = version

	start, err := pre.LoginStart(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ologi: login/start failed: %v\n", err)
		os.Exit(2)
	}

	fmt.Fprintf(os.Stderr, "\nDevice code: %s\n", start.DeviceCode)
	fmt.Fprintf(os.Stderr, "Approval URL: %s\n", start.VerificationURL)
	fmt.Fprintln(os.Stderr, "\nOpening the approval URL in your browser… (if it doesn't open, visit the URL manually)")

	// `open` on macOS — don't hard-fail if it can't launch.
	_ = exec.Command("open", start.VerificationURL).Start()

	// Poll loop. Cap at 10 minutes.
	interval := time.Duration(start.IntervalMs) * time.Millisecond
	if interval < time.Second {
		interval = 2 * time.Second
	}
	deadline := time.Now().Add(10 * time.Minute)

	// Let ^C abort cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	dots := 0
	for {
		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "\nologi: cancelled")
			os.Exit(2)
		default:
		}

		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "\nologi: code expired (10 min) — please retry")
			os.Exit(2)
		}

		resp, err := pre.LoginPoll(start.DeviceCode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nologi: poll error: %v\n", err)
			os.Exit(2)
		}
		switch resp.Status {
		case "pending":
			dots++
			if dots%5 == 0 {
				fmt.Fprint(os.Stderr, ".")
			}
			time.Sleep(interval)
			continue
		case "denied":
			fmt.Fprintln(os.Stderr, "\nologi: denied")
			os.Exit(2)
		case "expired":
			fmt.Fprintln(os.Stderr, "\nologi: expired — please retry")
			os.Exit(2)
		case "ok":
			err := config.Save(config.Config{
				APIKey:     resp.APIKey,
				DeviceID:   resp.DeviceID,
				DeviceName: name,
				ServerURL:  strings.TrimSuffix(serverOverride, "/"),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nologi: save config: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "\n✓ linked as %q\n", name)
			return
		default:
			fmt.Fprintf(os.Stderr, "\nologi: unexpected status %q\n", resp.Status)
			os.Exit(2)
		}
	}
}

func cmdLogout(args []string) {
	cfg, err := config.Load()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "ologi: not logged in (nothing to do)")
			return
		}
		fmt.Fprintf(os.Stderr, "ologi: read config: %v\n", err)
		os.Exit(1)
	}

	// Unload the daemon if it's running; ignore errors.
	_ = launchd.Bootout()
	_ = launchd.RemovePlist()

	// Revoke server-side.
	if cfg.DeviceID != "" {
		c := newClient(cfg)
		if err := c.DeleteDevice(cfg.DeviceID); err != nil {
			fmt.Fprintf(os.Stderr, "ologi: warning — could not revoke device server-side: %v\n", err)
		}
	}

	if err := config.Remove(); err != nil {
		fmt.Fprintf(os.Stderr, "ologi: remove config: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "✓ logged out")
}

func cmdStatus(args []string) {
	cfg, err := config.Load()
	if os.IsNotExist(err) {
		fmt.Println("account: (not logged in)")
		fmt.Println("voice:   (stopped)")
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ologi: read config: %v\n", err)
		os.Exit(1)
	}

	who := cfg.DeviceName
	if who == "" {
		who = "(unnamed device)"
	}
	fmt.Printf("account: %s\n", who)

	loaded, _ := launchd.IsLoaded()
	if loaded {
		fmt.Println("voice:   running")
	} else {
		fmt.Println("voice:   stopped")
	}
}

// readLine reads a line from stdin, trimming trailing whitespace.
// Empty on EOF or error.
func readLine() string {
	var buf [256]byte
	n, err := os.Stdin.Read(buf[:])
	if err != nil || n == 0 {
		return ""
	}
	return strings.TrimRight(string(buf[:n]), "\r\n\t ")
}
