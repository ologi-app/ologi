package main

import (
	"fmt"
	"os"

	"github.com/ologi/hypertask-cli/internal/api"
	"github.com/ologi/hypertask-cli/internal/config"
)

const defaultServerURL = "https://voice.ologi.app"

// serverURL returns, in priority order:
// 1. OLOGI_SERVER_URL env var
// 2. cfg.ServerURL if set
// 3. defaultServerURL
func serverURL(cfg config.Config) string {
	if env := os.Getenv("OLOGI_SERVER_URL"); env != "" {
		return env
	}
	if cfg.ServerURL != "" {
		return cfg.ServerURL
	}
	return defaultServerURL
}

// loadConfigOrDie loads the config. On any error (including missing
// file), prints a helpful message and exits 1.
func loadConfigOrDie() config.Config {
	cfg, err := config.Load()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "ologi: not logged in — run 'ologi login'")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "ologi: read config: %v\n", err)
		os.Exit(1)
	}
	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "ologi: config missing api_key — run 'ologi login'")
		os.Exit(1)
	}
	return cfg
}

// newClient builds an API client from a config.
func newClient(cfg config.Config) *api.Client {
	c := api.NewClient(serverURL(cfg), cfg.APIKey)
	c.Version = version
	return c
}

// Stubs — replaced in Tasks 14 (login/logout/status) and 15 (voice).

func cmdLogin(args []string)  { fmt.Fprintln(os.Stderr, "ologi: login not implemented yet"); os.Exit(1) }
func cmdLogout(args []string) { fmt.Fprintln(os.Stderr, "ologi: logout not implemented yet"); os.Exit(1) }
func cmdStatus(args []string) { fmt.Fprintln(os.Stderr, "ologi: status not implemented yet"); os.Exit(1) }
func cmdVoice(args []string)  { fmt.Fprintln(os.Stderr, "ologi: voice not implemented yet"); os.Exit(1) }
