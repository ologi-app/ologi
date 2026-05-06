# ologi — Ologi Voice CLI

Talk your way through your AI conversations.

| Platform | Status |
|---|---|
| macOS (Apple Silicon + Intel) | ✅ Supported |
| Linux (X11 sessions) | ✅ Supported (v0.2.0+) |
| Linux (Wayland sessions) | ❌ Not supported in v1 — switch to X11 |
| Windows | ❌ Not planned |

## Install

### macOS (recommended)

```sh
brew install ologi-app/tap/ologi
```

### Linux

```sh
curl -fsSL https://raw.githubusercontent.com/ologi-app/ologi/main/install.sh | sh
```

The installer downloads the right tarball for your arch (`amd64` or `arm64`)
and drops the binary in `~/.local/bin/ologi`. You'll also need:

- `xdotool` (for typing into focused windows + detecting the active app):
  - Debian / Ubuntu: `sudo apt install xdotool`
  - Fedora: `sudo dnf install xdotool`
  - Arch: `sudo pacman -S xdotool`
- `libportaudio2` and `libxi6` (audio capture + global hotkey):
  - Most distros include these, but if not: `sudo apt install libportaudio2 libxi6`
- An **X11 session**. Pure Wayland sessions can't satisfy the global-hotkey
  + key-injection requirements that ologi needs. Log out, pick "Xorg" /
  "X11" in your display manager's session selector, log back in.

### macOS (alternative — no brew)

The same `install.sh` works on macOS. Prefer brew if you have it (you get
`brew upgrade ologi` for free).

## Use

```sh
ologi login                  # link this device — opens a browser tab to approve
ologi voice run              # foreground listener — Ctrl+C to stop
ologi voice start            # macOS only: launchd-managed background daemon
```

Configure your hotkey, tap mode (single/double), and microphone in the web
dashboard's **Settings** tab on `voice.ologi.app`. Changes to mic + tap mode
apply on the next dictation; hotkey changes need an `ologi voice stop &&
start` (the keylistener binds at startup).

See `ologi --help` for the full command list.

## Releasing

Releases are cut by pushing a `v*` tag:

```sh
git tag v0.2.0
git push --tags
```

CI (`.github/workflows/release.yml`) builds + uploads tarballs for
`darwin-arm64`, `darwin-amd64`, `linux-amd64`, and `linux-arm64`. Each
release ends up at:

```
https://github.com/ologi-app/ologi/releases/download/<tag>/ologi-<version>-<os>-<arch>.tar.gz
```

After a release lands, bump the Homebrew formula at
`ologi-app/homebrew-tap` to point at the new SHAs.
