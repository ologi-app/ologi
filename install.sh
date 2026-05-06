#!/bin/sh
# Ologi Voice CLI installer.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/ologi-app/ologi/main/install.sh | sh
#
# Mac users: prefer `brew install ologi-app/tap/ologi`. This script
# also works on Mac (downloads the same tarball brew uses), but the
# brew path gets you `brew upgrade` for free.

set -e

VERSION="${OLOGI_VERSION:-latest}"
GITHUB_REPO="ologi-app/ologi"
INSTALL_DIR="${OLOGI_INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS
case "$(uname -s)" in
  Darwin)  OS="darwin" ;;
  Linux)   OS="linux" ;;
  *)
    echo "ologi: unsupported OS: $(uname -s)" >&2
    echo "  Supported: macOS, Linux (X11 sessions only)." >&2
    exit 1
    ;;
esac

# Detect arch
case "$(uname -m)" in
  arm64|aarch64) ARCH="arm64" ;;
  x86_64|amd64)  ARCH="amd64" ;;
  *)
    echo "ologi: unsupported arch: $(uname -m)" >&2
    exit 1
    ;;
esac

# Resolve "latest" to a concrete version
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
    | sed -n 's/.*"tag_name": *"\(v[^"]*\)".*/\1/p')
  if [ -z "$VERSION" ]; then
    echo "ologi: could not determine latest version (rate-limited?)" >&2
    exit 1
  fi
fi

NUM_VERSION="${VERSION#v}"
TARBALL="ologi-${NUM_VERSION}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${TARBALL}"

echo "ologi: installing ${VERSION} (${OS}/${ARCH})"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

if ! curl -fsSL "$URL" -o "$TMPDIR/$TARBALL"; then
  echo "ologi: download failed" >&2
  echo "  URL: $URL" >&2
  echo "  (check the GitHub Releases page for available platforms)" >&2
  exit 1
fi

tar -xzf "$TMPDIR/$TARBALL" -C "$TMPDIR"

mkdir -p "$INSTALL_DIR"
mv "$TMPDIR/ologi" "$INSTALL_DIR/ologi"
chmod +x "$INSTALL_DIR/ologi"

echo "ologi: installed to ${INSTALL_DIR}/ologi"

# Linux post-install hints
if [ "$OS" = "linux" ]; then
  if ! command -v xdotool >/dev/null 2>&1; then
    echo ""
    echo "ologi: WARNING — xdotool isn't on PATH."
    echo "  ologi shells out to xdotool to type transcripts into focused windows"
    echo "  and to detect the active app. Install it:"
    echo "    apt:  sudo apt install xdotool"
    echo "    dnf:  sudo dnf install xdotool"
    echo "    pacman: sudo pacman -S xdotool"
  fi

  if [ -z "${DISPLAY:-}" ] && [ -n "${WAYLAND_DISPLAY:-}" ]; then
    echo ""
    echo "ologi: WARNING — pure Wayland session detected (no DISPLAY)."
    echo "  ologi v1 requires X11. Log out, choose 'Xorg' / 'X11' in your display"
    echo "  manager's session picker, log back in, then try \`ologi login\`."
  fi
fi

# PATH hint
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo ""
    echo "ologi: NOTE — ${INSTALL_DIR} is not in your PATH."
    echo "  Add this to your shell profile (.zshrc / .bashrc):"
    echo "    export PATH=\"\$PATH:${INSTALL_DIR}\""
    ;;
esac

echo ""
echo "ologi: ready. Next: \`ologi login\`"
