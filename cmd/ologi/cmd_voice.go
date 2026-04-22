package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ologi-app/ologi/internal/engine"
	"github.com/ologi-app/ologi/internal/launchd"
	"github.com/ologi-app/ologi/internal/sourceapp"
)

func cmdVoice(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "ologi voice: missing subcommand (run|start|stop|autostart|status)")
		os.Exit(1)
	}
	switch args[0] {
	case "run":
		voiceRun()
	case "start":
		voiceStart(false)
	case "stop":
		voiceStop()
	case "autostart":
		if len(args) < 2 || (args[1] != "on" && args[1] != "off") {
			fmt.Fprintln(os.Stderr, "ologi voice autostart: 'on' or 'off' required")
			os.Exit(1)
		}
		voiceAutostart(args[1] == "on")
	case "status":
		voiceStatus()
	default:
		fmt.Fprintf(os.Stderr, "ologi voice: unknown subcommand %q\n", args[0])
		os.Exit(1)
	}
}

// voiceRun is the blocking foreground listener. launchd invokes this.
func voiceRun() {
	// If the daemon is already loaded under launchd, refuse to start a
	// second copy — the two would fight for the mic.
	if loaded, _ := launchd.IsLoaded(); loaded {
		fmt.Fprintln(os.Stderr, "ologi: voice daemon is already running under launchd (use 'ologi voice stop' first)")
		os.Exit(3)
	}

	cfg := loadConfigOrDie()
	c := newClient(cfg)
	rt := &engine.Runtime{Client: c, Detect: sourceapp.Detect}

	engineCfg, err := rt.Boot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ologi: boot: %v\n", err)
		os.Exit(1)
	}

	eng := engine.NewEngine(engineCfg, rt.OnSession, rt.MintToken)
	go eng.Run()

	// Drain events (we don't surface them in v1).
	go func() {
		for range eng.Events() {
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	eng.Stop()
}

func voiceStart(autostart bool) {
	// Ensure config exists — refusing to start without an account.
	_ = loadConfigOrDie()

	binPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ologi: locate own binary: %v\n", err)
		os.Exit(3)
	}
	home, _ := os.UserHomeDir()

	env := map[string]string{}
	if v := os.Getenv("OLOGI_SERVER_URL"); v != "" {
		env["OLOGI_SERVER_URL"] = v
	}

	spec := launchd.PlistSpec{
		Label:      launchd.Label,
		BinaryPath: binPath,
		Args:       []string{"voice", "run"},
		HomeDir:    home,
		Autostart:  autostart,
		Env:        env,
	}
	if err := launchd.WritePlist(spec); err != nil {
		fmt.Fprintf(os.Stderr, "ologi: write plist: %v\n", err)
		os.Exit(3)
	}

	if err := launchd.Bootstrap(); err != nil {
		fmt.Fprintf(os.Stderr, "ologi: bootstrap: %v\n", err)
		os.Exit(3)
	}
	// If it was already loaded, kickstart to pick up the new plist.
	_ = launchd.Kickstart()

	fmt.Fprintln(os.Stderr, "✓ voice daemon started")
}

func voiceStop() {
	if err := launchd.Bootout(); err != nil {
		fmt.Fprintf(os.Stderr, "ologi: bootout: %v\n", err)
		os.Exit(3)
	}
	fmt.Fprintln(os.Stderr, "✓ voice daemon stopped")
}

func voiceAutostart(on bool) {
	voiceStart(on)
	if on {
		fmt.Fprintln(os.Stderr, "✓ will start at login")
	} else {
		fmt.Fprintln(os.Stderr, "✓ will not start at login")
	}
}

func voiceStatus() {
	loaded, _ := launchd.IsLoaded()
	if !loaded {
		fmt.Println("stopped")
		return
	}
	fmt.Println("running")
	fmt.Println("logs: ~/Library/Logs/ologi-voice.log")
}
