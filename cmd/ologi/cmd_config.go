package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ologi-app/ologi/internal/config"
)

func cmdConfig(args []string) {
	cfg, err := config.Load()
	if os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "ologi: not logged in — run 'ologi login'")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ologi: read config: %v\n", err)
		os.Exit(1)
	}
	if cfg.DeviceID == "" {
		fmt.Fprintln(os.Stderr, "ologi: config missing device_id — run 'ologi login' again")
		os.Exit(2)
	}

	server := strings.TrimSuffix(cfg.ServerURL, "/")
	if server == "" {
		server = defaultServerURL
	}
	url := fmt.Sprintf("%s/voice?device=%s#settings", server, cfg.DeviceID)

	fmt.Fprintf(os.Stderr, "Opening %s\n", url)
	if err := openURL(url); err != nil {
		fmt.Fprintf(os.Stderr, "ologi: could not open browser: %v\n", err)
		fmt.Fprintln(os.Stderr, "Visit the URL above manually.")
		os.Exit(1)
	}
}
