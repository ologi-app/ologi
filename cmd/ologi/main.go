package main

import (
	"fmt"
	"os"
)

// version is set at build time via -ldflags "-X main.version=<semver>".
var version = "dev"

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Printf("ologi %s\n", version)
			return
		case "--help", "-h", "help":
			printUsage(os.Stdout)
			return
		}
	}
	printUsage(os.Stderr)
	os.Exit(1)
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `ologi — talk your way through your AI conversations

Usage:
  ologi login                   Link this device to your Ologi account
  ologi logout                  Revoke the link, remove local config
  ologi status                  Show account + voice daemon status
  ologi voice run               Start the foreground listener (what launchd invokes)
  ologi voice start             Start the launchd-managed background daemon
  ologi voice stop              Stop the daemon
  ologi voice autostart on|off  Toggle start-at-login
  ologi voice status            Show the daemon's launchctl status
  ologi --version               Print the binary version
  ologi --help                  This message
`)
}
