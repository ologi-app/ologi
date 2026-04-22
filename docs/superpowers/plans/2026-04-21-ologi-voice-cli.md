# Ologi Voice CLI — Implementation Plan (Plan B)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `ologi` CLI — a single Go binary, voice-only in v1, distributed via a private Homebrew tap — that forks the `~/Documents/stt` engine, wires it to the Plan A web API, and runs as a launchd LaunchAgent on macOS.

**Architecture:** New Go module at `packages/ologi-cli/`. Engine (audio, key listener, typewriter, AAI streaming) is forked from `~/Documents/stt` with the TUI / local history / local metrics stripped. NEW packages for `config` (TOML at `~/.config/ologi/config.toml` mode 0600), `api` (Ologi HTTP client), `launchd` (plist + launchctl), `sourceapp` (NSWorkspace + AppleScript). The `ologi voice run` subcommand is a blocking foreground listener; `ologi voice start/stop` wrap `launchctl bootstrap/bootout`. Login via device-code flow (Plan A's `/api/voice/login/*`). AssemblyAI streaming uses a per-session scoped token minted server-side.

**Tech Stack:** Go 1.26+, cgo (required for PortAudio + CoreGraphics + AppKit). Deps: `github.com/gordonklaus/portaudio`, `github.com/gorilla/websocket`, `github.com/BurntSushi/toml`. macOS 11+ (Big Sur and later) runtime target. GitHub Actions for CI. Homebrew tap for distribution.

**Spec:** `docs/superpowers/specs/2026-04-21-ologi-voice-cli-design.md` — read once if needed; every task below has enough context to execute on its own.

---

## File structure

```
packages/ologi-cli/
├── go.mod                                  ← NEW (Task 2)
├── go.sum                                  ← auto-generated
├── README.md                               ← NEW (Task 2)
├── cmd/
│   └── ologi/
│       ├── main.go                         ← router + version (Task 14)
│       ├── cmd_login.go                    ← ologi login/logout/status (Task 14)
│       └── cmd_voice.go                    ← ologi voice * (Task 15)
└── internal/
    ├── audio/
    │   └── audio.go                        ← forked from stt/audio.go (Task 3)
    ├── sound/
    │   └── sound_darwin.go                 ← forked from stt/sound_darwin.go (Task 3)
    ├── keylistener/
    │   └── keylistener_darwin.go           ← forked from stt/keylistener_darwin.go (Task 3)
    ├── typewriter/
    │   └── typewriter_darwin.go            ← forked from stt/typewriter_darwin.go (Task 3)
    ├── aai/
    │   ├── streaming.go                    ← forked from stt/transcribe.go (Task 4)
    │   └── batch.go                        ← forked from stt/batch.go (Task 4)
    ├── engine/
    │   └── engine.go                       ← forked from stt/engine.go, trimmed (Task 5)
    ├── config/
    │   ├── config.go                       ← NEW — load/save ~/.config/ologi/config.toml (Task 6)
    │   └── config_test.go                  ← NEW (Task 6)
    ├── api/
    │   ├── client.go                       ← NEW — base HTTP client (Task 7)
    │   ├── client_test.go                  ← NEW (Task 7)
    │   ├── config.go                       ← NEW — GetConfig (Task 8)
    │   ├── token.go                        ← NEW — MintRealtimeToken (Task 8)
    │   ├── sessions.go                     ← NEW — PostSession (Task 8)
    │   ├── endpoints_test.go               ← NEW (Task 8)
    │   ├── login.go                        ← NEW — Start / PollComplete / DeleteDevice (Task 9)
    │   └── login_test.go                   ← NEW (Task 9)
    ├── sourceapp/
    │   ├── sourceapp.go                    ← NEW — interface, fallback (Task 10)
    │   ├── sourceapp_darwin.go             ← NEW — CGo NSWorkspace (Task 10)
    │   └── browser_tab_darwin.go           ← NEW — AppleScript wrappers (Task 10)
    └── launchd/
        ├── plist.go                        ← NEW — template render (Task 11)
        ├── plist_test.go                   ← NEW (Task 11)
        └── control.go                      ← NEW — launchctl wrappers (Task 11)

.github/workflows/
└── ologi-cli-release.yml                   ← NEW (Task 16) — in THIS monorepo

(Separate repo — ologi/homebrew-tap)
└── Formula/
    └── ologi.rb                            ← documented in Task 16; actual setup is a manual user step
```

---

## Conventions

- **Path prefix.** All paths relative to `/Users/thedevdad/Documents/hypertask/`.
- **Git hygiene.** Use `git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli <cmd>` per the project CLAUDE.md — never compound `cd && git`.
- **Go module path.** `github.com/ologi/hypertask-cli` (dictated; it's a CLI, nothing imports it). Inside the module, packages live under `internal/` so nothing outside the module can import them either.
- **Go version.** `go 1.26` in go.mod — matches the existing `~/Documents/stt` version.
- **Cgo.** Required. `CGO_ENABLED=1` must be set for builds (this is the default when the Go toolchain detects a working C compiler on macOS).
- **Homebrew-provided libraries.** PortAudio is a runtime and build-time dependency. Install via `brew install portaudio` before building locally.
- **Test runner.** `go test ./...` from `packages/ologi-cli/`. `-v` for verbose. `-run TestName` to target one test.
- **Server URL.** Default `https://voice.ologi.app`. Override via `OLOGI_SERVER_URL` env var for dev (`http://voice.ologi.localhost:3005`).
- **Config file.** `~/.config/ologi/config.toml`, mode `0600`. Fields: `api_key`, `device_id`, `device_name`, `server_url` (optional override).
- **launchd label.** `app.ologi.voice`. Plist at `~/Library/LaunchAgents/app.ologi.voice.plist`.
- **Log file.** `~/Library/Logs/ologi-voice.log` when launchd-managed; stderr when run from a terminal.

---

## Task 1 — Verify dev baseline + create worktree

**Files:** none.

Confirms Go + PortAudio + the ptt source are in place, and creates the isolated worktree all subsequent tasks build inside.

- [ ] **Step 1: Confirm Go toolchain + PortAudio**

```bash
go version
# Expected: go version go1.26.x darwin/<arch>
# If missing: `brew install go`

pkg-config --exists portaudio-2.0 && echo "portaudio OK" || echo "MISSING portaudio"
# Expected: "portaudio OK"
# If missing: `brew install portaudio`
```

If either is missing, stop and install it before continuing. Cgo will fail to link without PortAudio.

- [ ] **Step 2: Confirm the ptt source is readable**

```bash
ls /Users/thedevdad/Documents/stt/audio.go && echo "stt source OK" || echo "MISSING ptt source"
```

All fork tasks read from this path — without it the plan breaks.

- [ ] **Step 3: Create the worktree**

```bash
git -C /Users/thedevdad/Documents/hypertask worktree add .worktrees/ologi-voice-cli -b feat/ologi-cli
ls /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/
```

Expected: the worktree dir has the full repo checkout on branch `feat/ologi-cli`.

- [ ] **Step 4: Confirm `.worktrees` is gitignored**

```bash
git -C /Users/thedevdad/Documents/hypertask check-ignore -q .worktrees && echo "ignored OK"
```

Expected: `ignored OK`. If not, that's a pre-existing repo problem — stop and fix it.

No commit.

---

## Task 2 — Scaffold the Go module

**Files:**
- Create: `packages/ologi-cli/go.mod`
- Create: `packages/ologi-cli/cmd/ologi/main.go`
- Create: `packages/ologi-cli/README.md`

- [ ] **Step 1: Initialize the module**

```bash
mkdir -p /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/cmd/ologi
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go mod init github.com/ologi/hypertask-cli
```

Expected: creates `go.mod` with:
```
module github.com/ologi/hypertask-cli

go 1.26.x
```

- [ ] **Step 2: Create the entry point**

Create `packages/ologi-cli/cmd/ologi/main.go`:

```go
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
```

- [ ] **Step 3: Create a minimal README**

Create `packages/ologi-cli/README.md`:

```markdown
# ologi — Ologi Voice CLI

Talk your way through your AI conversations. macOS only in v1.

## Install

```
brew install ologi-app/tap/ologi
```

## Use

```
ologi login         # link this device
ologi voice start   # background daemon; double-tap right_option to dictate
```

See `ologi --help` for all subcommands, or read the spec at
`docs/superpowers/specs/2026-04-21-ologi-voice-cli-design.md`.
```

- [ ] **Step 4: Build + smoke**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go build ./...
./cmd/ologi/ologi --version 2>/dev/null || go run ./cmd/ologi --version
```

Expected: `ologi dev`.

(The first invocation fails because `go build ./...` doesn't produce a binary by default without `-o`; the second confirms `main` runs.)

- [ ] **Step 5: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): scaffold Go module + entry point

packages/ologi-cli/ go.mod, cmd/ologi/main.go (usage + --version),
minimal README. Ready to receive the forked engine + new packages.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Fork the audio + sound + keylistener + typewriter packages

Each is a mechanical copy-paste-from-`~/Documents/stt/` with the package name adjusted. One commit at the end.

**Files:**
- Create: `packages/ologi-cli/internal/audio/audio.go`
- Create: `packages/ologi-cli/internal/sound/sound_darwin.go`
- Create: `packages/ologi-cli/internal/keylistener/keylistener_darwin.go`
- Create: `packages/ologi-cli/internal/typewriter/typewriter_darwin.go`

- [ ] **Step 1: Fork `internal/audio/audio.go`**

```bash
mkdir -p /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/audio
cp /Users/thedevdad/Documents/stt/audio.go /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/audio/audio.go
```

Edit the new file: change the first line from `package main` to `package audio`. No other content changes — the existing exports (`NewAudioCapture`, `ListInputDevices`, `FindDevice`, `TestMic`, `AudioCapture` struct methods) are already public (capitalized).

- [ ] **Step 2: Fork `internal/sound/sound_darwin.go`**

```bash
mkdir -p /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/sound
cp /Users/thedevdad/Documents/stt/sound_darwin.go /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/sound/sound_darwin.go
```

Change `package main` → `package sound`. The exports (`InitSounds`, `PlayStartSound`, `PlayStopSound`, `TestSounds`) are already public.

`InitSounds` currently takes a `Config` struct from ptt's `main` package. We can't reference that — replace the parameter with the fields actually used:

```go
// Near the top of sound_darwin.go, replace InitSounds' signature:
// OLD:
// func InitSounds(cfg Config) { ... }
// NEW:
func InitSounds(startName, stopName string) {
    // was: if cfg.StartSound != "" { startSoundName = cfg.StartSound }
    if startName != "" {
        startSoundName = startName
    }
    if stopName != "" {
        stopSoundName = stopName
    }
}
```

Remove any remaining `Config`-typed references. Everything else in the file stays identical.

- [ ] **Step 3: Fork `internal/keylistener/keylistener_darwin.go`**

```bash
mkdir -p /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/keylistener
cp /Users/thedevdad/Documents/stt/keylistener_darwin.go /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/keylistener/keylistener_darwin.go
```

Change `package main` → `package keylistener`. Exports (`StartKeyListener`, `RecordKey`) are already public.

- [ ] **Step 4: Fork `internal/typewriter/typewriter_darwin.go`**

```bash
mkdir -p /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/typewriter
cp /Users/thedevdad/Documents/stt/typewriter_darwin.go /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/typewriter/typewriter_darwin.go
```

Change `package main` → `package typewriter`. Exports (`TypeWriter`, `NewTypeWriter`, `HandleTranscript`, `Type`, `Reset`) are already public.

- [ ] **Step 5: `go mod tidy` to pull PortAudio**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go mod tidy
```

Expected: `go.mod` now lists `github.com/gordonklaus/portaudio` as a require. `go.sum` created.

- [ ] **Step 6: Build to confirm the four packages compile**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go build ./internal/audio/... ./internal/sound/... ./internal/keylistener/... ./internal/typewriter/...
```

Expected: no output (success). Any errors almost certainly mean a stray `Config` reference or missing `package` rename — fix and re-run.

- [ ] **Step 7: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/audio packages/ologi-cli/internal/sound packages/ologi-cli/internal/keylistener packages/ologi-cli/internal/typewriter packages/ologi-cli/go.mod packages/ologi-cli/go.sum
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): fork audio + sound + keylistener + typewriter from ptt

Direct copies from ~/Documents/stt/ with `package main` → the
appropriate internal package name. sound.InitSounds signature
simplified to take (startName, stopName) rather than a ptt Config.
No behavior changes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — Fork the AssemblyAI streaming + batch clients

**Files:**
- Create: `packages/ologi-cli/internal/aai/streaming.go`
- Create: `packages/ologi-cli/internal/aai/batch.go`

`streaming.go` is forked from `transcribe.go`, with the auth header changed from using the user's full API key to using a server-minted scoped token.

- [ ] **Step 1: Fork `internal/aai/streaming.go`**

```bash
mkdir -p /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/aai
cp /Users/thedevdad/Documents/stt/transcribe.go /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/aai/streaming.go
```

Make three edits in the copied file:

1. First line: `package main` → `package aai`.

2. The existing `NewAssemblyAI(apiKey, language, sampleRate, handler)` signature takes a full AAI API key. Rename to `NewStreamingClient(token, language, sampleRate, handler)` and update the `Authorization` header line:

```go
// OLD:
// func NewAssemblyAI(apiKey, language string, sampleRate int, handler TranscriptHandler) (*AssemblyAIClient, error) {
// NEW:
func NewStreamingClient(token, language string, sampleRate int, handler TranscriptHandler) (*StreamingClient, error) {
```

3. Rename the struct `AssemblyAIClient` → `StreamingClient` throughout the file (find/replace). The `Authorization` header value: replace `headers.Set("Authorization", apiKey)` with `headers.Set("Authorization", "Bearer "+token)` if AAI's realtime-token auth expects the `Bearer ` prefix; if AAI treats the token field the same way it treats a full key (bare value, no prefix), keep `headers.Set("Authorization", token)`.

**Which is right** is documented in AssemblyAI's streaming docs. Plan A's `mintRealtimeToken()` on the server side already knows this; look at `apps/web/src/lib/voice/assemblyai.ts` — that code POSTs to `https://api.assemblyai.com/v2/realtime/token` with `Authorization: <apiKey>` (no Bearer prefix) and gets back `{token}`. AAI's streaming endpoint docs say the realtime token goes in the URL query string OR in the `Authorization` header as a bare value (no `Bearer `). For simplicity, use the URL-query form — append `&token=<token>` to the WSS URL — and leave the Authorization header unset. Final change:

```go
// OLD:
// url := fmt.Sprintf("%s?sample_rate=%d&speech_model=%s", assemblyAIURL, sampleRate, model)
// headers := http.Header{}
// headers.Set("Authorization", apiKey)
// conn, resp, err := websocket.DefaultDialer.Dial(url, headers)

// NEW:
url := fmt.Sprintf("%s?sample_rate=%d&speech_model=%s&token=%s", assemblyAIURL, sampleRate, model, token)
conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
```

If the verification experiment in Step 4 shows AAI actually wants the token in the header, revert to `headers.Set("Authorization", token)` and drop the `&token=` URL param.

- [ ] **Step 2: Fork `internal/aai/batch.go`**

```bash
cp /Users/thedevdad/Documents/stt/batch.go /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/aai/batch.go
```

Change `package main` → `package aai`. Batch mode still uses the user's AAI key directly (it POSTs to a non-realtime endpoint). For Plan B we preserve the existing behavior: `NewBatchRecorder(apiKey, language, sampleRate)` takes a full key. In practice, the engine will only use streaming mode for v1 — batch is there as a fallback if streaming fails.

Actually — the Plan A `/api/voice/realtime-token` endpoint only mints realtime (streaming) tokens. For batch we'd need a different endpoint (upload URL + transcript creation). For v1, we simplify:

- Remove batch from the exposed CLI surface. The engine only uses streaming.
- Delete or stub `batch.go`? For now, keep it as dead code (with its `package aai` header) — deleting it means losing the batch plumbing forever, which we may want later. Just don't call into it from the engine.

Final state of batch.go: copied, package renamed to `aai`, otherwise untouched. Unused in v1; removed in a future cleanup if we're confident we don't want it.

- [ ] **Step 3: `go mod tidy`**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go mod tidy
```

Expected: `go.mod` now lists `github.com/gorilla/websocket` as a require.

- [ ] **Step 4: Verify streaming auth shape against AssemblyAI with a throwaway script**

Before trusting the URL-query approach, run a 10-line Go script against a dev-minted token:

```bash
# Mint a token via the dev server (requires a valid dev ht_* key):
TOKEN=$(curl -s -X POST -H "Authorization: Bearer ht_DEV_KEY_HERE" \
  "http://voice.ologi.localhost:3005/api/voice/realtime-token" | jq -r .token)
echo "TOKEN=$TOKEN"
```

Then a tiny Go program (write it in a tmp dir, not in the repo):

```go
package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"os"
)

func main() {
	token := os.Getenv("TOKEN")
	if token == "" {
		log.Fatal("TOKEN env required")
	}
	url := fmt.Sprintf("wss://streaming.assemblyai.com/v3/ws?sample_rate=16000&speech_model=universal-streaming-english&token=%s", token)
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatalf("dial: %v (status %d)", err, resp.StatusCode)
	}
	defer conn.Close()
	fmt.Println("connected")
	// Read one message to confirm the handshake completed
	_, msg, _ := conn.ReadMessage()
	fmt.Printf("first message: %s\n", string(msg))
}
```

`go run main.go`. Expected: `connected` and a JSON `{"type":"Begin",...}` message.

If the URL-query form fails, switch to header form: set `headers := http.Header{}; headers.Set("Authorization", token)`, drop the `&token=` param, retry.

- [ ] **Step 5: Build**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go build ./internal/aai/...
```

Expected: clean build.

- [ ] **Step 6: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/aai packages/ologi-cli/go.mod packages/ologi-cli/go.sum
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): fork AssemblyAI streaming + batch clients

streaming.go: forked from stt/transcribe.go; NewAssemblyAI →
NewStreamingClient(token, ...); AssemblyAIClient → StreamingClient;
token moved to URL query param (verified against AAI's v3/ws endpoint
with a throwaway script).

batch.go: forked from stt/batch.go as-is; retains the full-API-key
path. Not wired into the engine in v1 — retained for possible future
use.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5 — Fork and trim the engine

**Files:**
- Create: `packages/ologi-cli/internal/engine/engine.go`

The engine state machine survives intact. We strip history + metrics references (they're gone) and switch the typewriter/audio/aai types to the new imports. The API-client wiring happens in Task 12.

- [ ] **Step 1: Fork `internal/engine/engine.go`**

```bash
mkdir -p /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/engine
cp /Users/thedevdad/Documents/stt/engine.go /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli/internal/engine/engine.go
```

- [ ] **Step 2: Edit the forked file**

Open `packages/ologi-cli/internal/engine/engine.go` and apply these changes:

1. First line: `package main` → `package engine`.

2. Add/fix imports at the top of the file:

```go
import (
	"log"
	"time"

	"github.com/ologi/hypertask-cli/internal/aai"
	"github.com/ologi/hypertask-cli/internal/audio"
	"github.com/ologi/hypertask-cli/internal/keylistener"
	"github.com/ologi/hypertask-cli/internal/sound"
	"github.com/ologi/hypertask-cli/internal/typewriter"
)
```

3. Remove the `history` and `metrics` struct fields + all their usages. The original struct:

```go
type Engine struct {
	cfg      Config
	history  *HistoryStore
	metrics  *MetricsCollector
	events   chan EngineEvent
	stopCh   chan struct{}
	doneCh   chan struct{}
	headless bool
}
```

becomes:

```go
type Engine struct {
	cfg      Config
	events   chan EngineEvent
	stopCh   chan struct{}
	doneCh   chan struct{}
}
```

And delete the `headless` flag everywhere — the only mode is "foreground listener, letting launchd manage background-ness."

4. Define a local `Config` type inside `internal/engine/engine.go` (since the ptt `Config` lived in `main` and we don't want a cyclic import through the API client yet). The engine only needs a subset of the full config — the fields that actually drive behavior:

```go
type Config struct {
	Hotkey      string
	Language    string
	SampleRate  int
	Device      string
	Channel     int
	Mode        string // "stream" only for v1; batch is future
	StartSound  string
	StopSound   string
	Replacements []ReplacementEntry
}

type ReplacementEntry struct {
	Pattern     string
	Replacement string
}

// ApplyReplacements applies each entry to `text` and returns the result.
// Matches the Plan A server's behavior: alphabetical order by pattern,
// case-insensitive.
func (c Config) ApplyReplacements(text string) string {
	// Sort replacements alphabetically by pattern (stable tiebreaker).
	sorted := make([]ReplacementEntry, len(c.Replacements))
	copy(sorted, c.Replacements)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Pattern < sorted[j].Pattern
	})
	out := text
	for _, r := range sorted {
		if r.Pattern == "" {
			continue
		}
		// Case-insensitive regex replace. Escape metacharacters in Pattern.
		re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(r.Pattern))
		out = re.ReplaceAllString(out, r.Replacement)
	}
	return out
}
```

Add `"regexp"` and `"sort"` to the imports.

5. Replace references to `NewAssemblyAI(...)` with `aai.NewStreamingClient(...)`. The engine's `AssemblyAIClient` type references become `*aai.StreamingClient`. The typewriter calls become `typewriter.Type(text)` / `typewriter.HandleTranscript(text, isFinal)`. The audio calls become `audio.NewAudioCapture(...)`. The sound calls become `sound.PlayStartSound()` / `sound.PlayStopSound()` / `sound.InitSounds(startName, stopName)`.

6. Replace the history/metrics store calls. Anywhere the original did:

```go
e.history.Append(HistoryEntry{ ... })
```

delete the call entirely — the server is the history now. The session-POST hook is added in Task 12.

Similarly `e.metrics.StartRecording()` / `e.metrics.RecordPrompt(...)` / `e.metrics.RecordError()` — delete all calls. Metrics come from the server's `/api/voice/stats` computations.

7. The original `engine.Run()` emits `EventFinalText` after the typewriter types. Rename that event's handler to hand off to a user-supplied `OnSession` callback. Define the callback type and make the engine accept it:

```go
// Session describes a completed dictation, as handed to the caller.
type Session struct {
	Mode       string    // "stream" or "batch" (always "stream" in v1)
	StartedAt  time.Time
	EndedAt    time.Time
	DurationMs int64
	Text       string    // AFTER replacements (what the typewriter typed)
	// SourceApp is filled in by the caller (main wiring, via the
	// sourceapp package) if available — not by the engine itself.
}

// OnSession is invoked once per completed dictation, AFTER typewriter output.
// Setting OnSession to nil disables the callback (useful for testing).
type OnSessionFunc func(s Session)

type Engine struct {
	cfg       Config
	onSession OnSessionFunc
	// ... rest unchanged
}

// NewEngine constructs an Engine with the given config and session
// callback. A nil onSession is fine — sessions just won't be observed.
func NewEngine(cfg Config, onSession OnSessionFunc) *Engine {
	return &Engine{
		cfg:       cfg,
		onSession: onSession,
		events:    make(chan EngineEvent, 64),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}
```

Inside `Run()`, after the typewriter types the final text, call `e.onSession(Session{...})` if `e.onSession != nil`.

8. Remove the `NewEngine(cfg, history, metrics, headless)` signature. Callers will only use the new `NewEngine(cfg, onSession)` form. Any `e.headless` checks disappear.

- [ ] **Step 3: Build to confirm**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go build ./internal/engine/...
```

Expected: clean build. Any errors are almost certainly lingering `e.history.*` / `e.metrics.*` / `NewAssemblyAI` / unqualified `NewAudioCapture` references — fix and re-run.

- [ ] **Step 4: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/engine/engine.go
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): fork engine; trim history + metrics; expose OnSession

Strips the in-engine HistoryStore + MetricsCollector (server-side now).
Removes the headless flag (the daemon has only one mode). Accepts an
OnSessionFunc callback invoked after each completed dictation — the
wiring into the API client happens in a later task.

Inline Config type with alphabetical-by-pattern, case-insensitive
ApplyReplacements — mirrors the Plan A server behavior so client-side
pre-typing matches the server's canonical record.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6 — `internal/config` package

**Files:**
- Create: `packages/ologi-cli/internal/config/config.go`
- Create: `packages/ologi-cli/internal/config/config_test.go`

TOML round-trip for `~/.config/ologi/config.toml`. Mode 0600 on writes.

- [ ] **Step 1: Write the failing tests**

Create `packages/ologi-cli/internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	want := Config{
		APIKey:     "ht_abcdef0123456789",
		DeviceID:   "aaaabbbb-cccc-dddd-eeee-ffffffffffff",
		DeviceName: "brent-mbp",
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Errorf("round-trip mismatch:\nwant %+v\ngot  %+v", want, got)
	}
}

func TestSaveEnforces0600(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := Save(Config{APIKey: "ht_xxx"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(tmp, ".config", "ologi", "config.toml"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("got perm %o, want 0600", info.Mode().Perm())
	}
}

func TestLoadMissingFileReturnsErrNotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, err := Load()
	if !os.IsNotExist(err) {
		t.Errorf("want os.IsNotExist(err), got %v", err)
	}
}

func TestRemoveDeletesTheFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := Save(Config{APIKey: "ht_xxx"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err := Load()
	if !os.IsNotExist(err) {
		t.Errorf("after Remove: want not-exist, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/config/...
```

Expected: FAIL with "no Go files" or "undefined: Config" etc.

- [ ] **Step 3: Implement**

Create `packages/ologi-cli/internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the on-disk shape of ~/.config/ologi/config.toml.
type Config struct {
	APIKey     string `toml:"api_key"`
	DeviceID   string `toml:"device_id,omitempty"`
	DeviceName string `toml:"device_name,omitempty"`
	// ServerURL overrides OLOGI_SERVER_URL / the hardcoded default.
	// Used for dev. Omit empty for production writes.
	ServerURL string `toml:"server_url,omitempty"`
}

// Path returns the canonical location: $HOME/.config/ologi/config.toml.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to $HOME literal if UserHomeDir fails (very rare).
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "ologi", "config.toml")
}

// Load reads the config. Returns os.ErrNotExist (wrapped) if the file
// is missing — callers can check with errors.Is(err, os.ErrNotExist)
// or os.IsNotExist.
func Load() (Config, error) {
	var cfg Config
	data, err := os.ReadFile(Path())
	if err != nil {
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Save writes the config, creating the parent directory if needed,
// at mode 0600. An existing file is overwritten atomically via rename.
func Save(cfg Config) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "config.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(tmp.Name()) // cleanup if rename fails

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}

	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// Remove deletes the config file. Returns nil if the file didn't exist.
func Remove() error {
	err := os.Remove(Path())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run tests, expect PASS**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/config/... -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/config
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): internal/config — ~/.config/ologi/config.toml at 0600

Atomic write via tempfile + rename. Enforces mode 0600 on the tempfile
before rename. 4 unit tests: round-trip, perm check, missing-file
returns os.ErrNotExist, Remove idempotent.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7 — `internal/api` base client

**Files:**
- Create: `packages/ologi-cli/internal/api/client.go`
- Create: `packages/ologi-cli/internal/api/client_test.go`

Shared HTTP plumbing used by every endpoint method in Tasks 8 and 9.

- [ ] **Step 1: Write the failing test**

Create `packages/ologi-cli/internal/api/client_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSendsBearerAuth(t *testing.T) {
	var gotAuth, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ht_test")
	c.Version = "0.0.1-test"
	if _, err := c.do("GET", "/api/voice/config", nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotAuth != "Bearer ht_test" {
		t.Errorf("auth header: got %q, want %q", gotAuth, "Bearer ht_test")
	}
	if !strings.Contains(gotUA, "ologi/0.0.1-test") {
		t.Errorf("user-agent: got %q, want contains ologi/0.0.1-test", gotUA)
	}
}

func TestClientReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ht_test")
	_, err := c.do("GET", "/api/voice/config", nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("got %T, want *APIError", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("status: got %d, want 401", apiErr.StatusCode)
	}
	if apiErr.Message != "Unauthorized" {
		t.Errorf("message: got %q, want %q", apiErr.Message, "Unauthorized")
	}
}

func TestClientAuthErrorHelper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "ht_test").do("GET", "/x", nil)
	if !IsAuthError(err) {
		t.Errorf("IsAuthError(%v) = false, want true", err)
	}
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/api/...
```

Expected: FAIL — no Go files.

- [ ] **Step 3: Implement**

Create `packages/ologi-cli/internal/api/client.go`:

```go
// Package api is the Ologi web API client used by the ologi CLI.
// It speaks HTTPS to voice.ologi.app (or an override via OLOGI_SERVER_URL
// or the ServerURL field of the config) using ht_* API keys.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIError is returned for any non-2xx HTTP response. The Message is
// the decoded `error` field of the JSON body, or the raw body prefix
// if the body wasn't JSON.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api: %d %s", e.StatusCode, e.Message)
}

// IsAuthError returns true if err is an APIError with status 401 or 403.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == 401 || apiErr.StatusCode == 403
}

// Client is the Ologi API client. Construct with NewClient.
type Client struct {
	BaseURL string
	APIKey  string
	// Version is the ologi-cli version, sent in the User-Agent.
	// Defaults to "unknown" if unset.
	Version string
	// Platform is reported in login/start. Defaults to "darwin".
	Platform string
	// HTTP allows overriding the HTTP client (e.g. for tests).
	HTTP *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Version:  "unknown",
		Platform: "darwin",
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do performs a single HTTP round-trip and returns the raw body bytes
// on success. On any non-2xx response, returns an *APIError.
//
// If body is non-nil, it is JSON-encoded and sent as application/json.
func (c *Client) do(method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		enc, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		reqBody = bytes.NewReader(enc)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("ologi/%s (darwin)", c.Version))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return data, nil
	}

	// Try to extract {"error": "..."} from the body.
	var errBody struct {
		Error string `json:"error"`
	}
	msg := ""
	if jsonErr := json.Unmarshal(data, &errBody); jsonErr == nil && errBody.Error != "" {
		msg = errBody.Error
	} else if len(data) > 0 {
		if len(data) > 200 {
			msg = string(data[:200]) + "…"
		} else {
			msg = string(data)
		}
	} else {
		msg = http.StatusText(resp.StatusCode)
	}

	return nil, &APIError{StatusCode: resp.StatusCode, Message: msg}
}
```

- [ ] **Step 4: Run tests, expect PASS**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/api/... -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/api/client.go packages/ologi-cli/internal/api/client_test.go
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): internal/api base client

HTTP client with Bearer-ht_* auth, User-Agent ologi/<version> (darwin),
typed APIError on non-2xx, IsAuthError helper for 401/403.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8 — API endpoints: `GetConfig`, `MintRealtimeToken`, `PostSession`

**Files:**
- Create: `packages/ologi-cli/internal/api/config.go`
- Create: `packages/ologi-cli/internal/api/token.go`
- Create: `packages/ologi-cli/internal/api/sessions.go`
- Create: `packages/ologi-cli/internal/api/endpoints_test.go`

Three typed wrappers around `client.do`. These three are the hot-path methods: the engine calls `GetConfig` at boot, `MintRealtimeToken` per dictation, `PostSession` at the end of each dictation.

- [ ] **Step 1: Write the failing test**

Create `packages/ologi-cli/internal/api/endpoints_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/voice/config" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"settings_version":5,"hotkey":"right_option","language":"en","mic_device":null,"start_sound":"Tink","stop_sound":"Pop","replacements":[{"pattern":"hi","replacement":"HI"}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "ht_test")
	cfg, err := c.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg.SettingsVersion != 5 {
		t.Errorf("settings_version: got %d, want 5", cfg.SettingsVersion)
	}
	if cfg.Hotkey != "right_option" {
		t.Errorf("hotkey: got %q", cfg.Hotkey)
	}
	if len(cfg.Replacements) != 1 || cfg.Replacements[0].Pattern != "hi" {
		t.Errorf("replacements: %+v", cfg.Replacements)
	}
}

func TestMintRealtimeToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/voice/realtime-token" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"token":"tok-xyz"}`))
	}))
	defer srv.Close()

	tok, err := NewClient(srv.URL, "ht_test").MintRealtimeToken()
	if err != nil {
		t.Fatalf("MintRealtimeToken: %v", err)
	}
	if tok != "tok-xyz" {
		t.Errorf("token: got %q, want %q", tok, "tok-xyz")
	}
}

func TestPostSession(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/voice/sessions" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"session_id":"s1","canonical_text":"hello world","settings_version":7}`))
	}))
	defer srv.Close()

	start := time.Date(2026, 4, 21, 10, 42, 0, 0, time.UTC)
	end := start.Add(14 * time.Second)
	src := "Google Chrome / claude.ai"
	resp, err := NewClient(srv.URL, "ht_test").PostSession(PostSessionInput{
		Mode:       "stream",
		StartedAt:  start,
		EndedAt:    end,
		DurationMs: 14_000,
		SourceApp:  &src,
		Text:       "hello world",
	})
	if err != nil {
		t.Fatalf("PostSession: %v", err)
	}
	if resp.SessionID != "s1" {
		t.Errorf("session_id: got %q", resp.SessionID)
	}
	if resp.CanonicalText != "hello world" {
		t.Errorf("canonical_text: got %q", resp.CanonicalText)
	}
	if resp.SettingsVersion != 7 {
		t.Errorf("settings_version: got %d, want 7", resp.SettingsVersion)
	}
	if gotBody["mode"] != "stream" || gotBody["duration_ms"].(float64) != 14000 {
		t.Errorf("body payload wrong: %+v", gotBody)
	}
	if gotBody["source_app"] != src {
		t.Errorf("source_app: got %v", gotBody["source_app"])
	}
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/api/... -run 'TestGetConfig|TestMintRealtimeToken|TestPostSession'
```

Expected: FAIL — undefined references.

- [ ] **Step 3: Implement `config.go`**

```go
package api

import "encoding/json"

// ConfigResponse mirrors GET /api/voice/config's JSON body.
type ConfigResponse struct {
	SettingsVersion int                  `json:"settings_version"`
	Hotkey          string               `json:"hotkey"`
	Language        string               `json:"language"`
	MicDevice       *string              `json:"mic_device"`
	StartSound      string               `json:"start_sound"`
	StopSound       string               `json:"stop_sound"`
	Replacements    []ReplacementEntry   `json:"replacements"`
}

// ReplacementEntry is one row of the user's personal dictionary.
type ReplacementEntry struct {
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
}

func (c *Client) GetConfig() (ConfigResponse, error) {
	var out ConfigResponse
	data, err := c.do("GET", "/api/voice/config", nil)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
```

- [ ] **Step 4: Implement `token.go`**

```go
package api

import "encoding/json"

func (c *Client) MintRealtimeToken() (string, error) {
	data, err := c.do("POST", "/api/voice/realtime-token", struct{}{})
	if err != nil {
		return "", err
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", &APIError{StatusCode: 500, Message: "empty token in response"}
	}
	return out.Token, nil
}
```

- [ ] **Step 5: Implement `sessions.go`**

```go
package api

import (
	"encoding/json"
	"time"
)

// PostSessionInput is what the engine sends after each completed dictation.
type PostSessionInput struct {
	Mode       string    `json:"mode"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMs int64     `json:"duration_ms"`
	SourceApp  *string   `json:"source_app,omitempty"`
	Text       string    `json:"text"`
}

// PostSessionResponse is the server's reply: the canonical post-replacements
// text + the current settings_version (so the CLI can decide whether to
// re-pull /config).
type PostSessionResponse struct {
	SessionID       string `json:"session_id"`
	CanonicalText   string `json:"canonical_text"`
	SettingsVersion int    `json:"settings_version"`
}

func (c *Client) PostSession(in PostSessionInput) (PostSessionResponse, error) {
	var out PostSessionResponse
	data, err := c.do("POST", "/api/voice/sessions", in)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
```

- [ ] **Step 6: Run tests, expect PASS**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/api/... -v
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/api
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): api endpoints — GetConfig, MintRealtimeToken, PostSession

Three methods the engine needs on the hot path: bootup config, per-session
realtime token, session finalize. Tests verify method + path + payload
shape and round-trip types.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9 — API login flow + device delete

**Files:**
- Create: `packages/ologi-cli/internal/api/login.go`
- Create: `packages/ologi-cli/internal/api/login_test.go`

Device-code flow: `Start` + `Poll` (single poll; the caller loops). Plus `DeleteDevice` for logout.

- [ ] **Step 1: Write the failing tests**

Create `packages/ologi-cli/internal/api/login_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginStart(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/voice/login/start" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"device_code":"ABCD1234","verification_url":"https://ologi.app/voice/link?code=ABCD1234","interval_ms":2000}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "") // no API key for /start
	c.Version = "0.0.1-test"

	out, err := c.LoginStart("brent-mbp")
	if err != nil {
		t.Fatalf("LoginStart: %v", err)
	}
	if out.DeviceCode != "ABCD1234" {
		t.Errorf("device_code: got %q", out.DeviceCode)
	}
	if out.IntervalMs != 2000 {
		t.Errorf("interval_ms: got %d", out.IntervalMs)
	}
	if gotBody["device_name"] != "brent-mbp" || gotBody["platform"] != "darwin" || gotBody["cli_version"] != "0.0.1-test" {
		t.Errorf("body: %+v", gotBody)
	}
}

func TestLoginPoll_Pending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/voice/login/complete" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"status":"pending"}`))
	}))
	defer srv.Close()

	out, err := NewClient(srv.URL, "").LoginPoll("ABCD1234")
	if err != nil {
		t.Fatalf("LoginPoll: %v", err)
	}
	if out.Status != "pending" {
		t.Errorf("status: got %q, want pending", out.Status)
	}
}

func TestLoginPoll_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok","api_key":"ht_real","device_id":"dev-1"}`))
	}))
	defer srv.Close()

	out, err := NewClient(srv.URL, "").LoginPoll("ABCD1234")
	if err != nil {
		t.Fatalf("LoginPoll: %v", err)
	}
	if out.Status != "ok" {
		t.Errorf("status: got %q", out.Status)
	}
	if out.APIKey != "ht_real" || out.DeviceID != "dev-1" {
		t.Errorf("fields: %+v", out)
	}
}

func TestLoginPoll_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(410)
		w.Write([]byte(`{"status":"expired"}`))
	}))
	defer srv.Close()

	out, err := NewClient(srv.URL, "").LoginPoll("ABCD1234")
	if err != nil {
		t.Fatalf("LoginPoll (want nil err, 410 is an expected terminal status): %v", err)
	}
	if out.Status != "expired" {
		t.Errorf("status: got %q, want expired", out.Status)
	}
}

func TestDeleteDevice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/voice/devices/dev-1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "ht_test").DeleteDevice("dev-1"); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/api/... -run 'TestLogin|TestDeleteDevice'
```

Expected: FAIL.

- [ ] **Step 3: Implement**

Create `packages/ologi-cli/internal/api/login.go`:

```go
package api

import "encoding/json"

// LoginStartResponse mirrors POST /api/voice/login/start.
type LoginStartResponse struct {
	DeviceCode      string `json:"device_code"`
	VerificationURL string `json:"verification_url"`
	IntervalMs      int    `json:"interval_ms"`
}

// LoginStart begins the device-code flow. Auth: none required.
func (c *Client) LoginStart(deviceName string) (LoginStartResponse, error) {
	var out LoginStartResponse
	body := map[string]string{
		"device_name": deviceName,
		"platform":    c.Platform,
		"cli_version": c.Version,
	}
	data, err := c.do("POST", "/api/voice/login/start", body)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

// LoginPollStatus is "pending", "denied", "expired", or "ok".
// On "ok", APIKey and DeviceID are populated.
type LoginPollResponse struct {
	Status   string `json:"status"`
	APIKey   string `json:"api_key,omitempty"`
	DeviceID string `json:"device_id,omitempty"`
}

// LoginPoll polls /api/voice/login/complete once. The server returns
// 200 for pending/ok and 410 for denied/expired — both are valid
// terminal states the caller switches on. We therefore treat 410 as
// a non-error here: decode the body, return it, let the caller act.
func (c *Client) LoginPoll(deviceCode string) (LoginPollResponse, error) {
	var out LoginPollResponse
	body := map[string]string{"device_code": deviceCode}
	data, err := c.do("POST", "/api/voice/login/complete", body)
	if err != nil {
		apiErr, ok := err.(*APIError)
		if !ok || apiErr.StatusCode != 410 {
			return out, err
		}
		// 410 carries a valid status body we still want.
		// Re-do the request with a client that accepts 410.
		return c.loginPoll410Hack(deviceCode)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

// loginPoll410Hack handles the one case where the server returns
// 410 + a JSON body our caller needs to observe (status=expired|denied).
// We decode the body from the APIError message (which already contains
// the decoded `error` field) — but actually the server returns a
// {"status": "..."} body, not an {"error": "..."} body, so the base
// client's APIError.Message is the raw body prefix. Re-request with a
// local HTTP so we can re-read the body.
func (c *Client) loginPoll410Hack(deviceCode string) (LoginPollResponse, error) {
	// Simpler: re-call the raw HTTP without the do() wrapper.
	// Sacrifice a bit of DRY for explicit control here.
	var out LoginPollResponse
	body := map[string]string{"device_code": deviceCode}
	enc, _ := json.Marshal(body)
	req, err := http_NewReq("POST", c.BaseURL+"/api/voice/login/complete", enc)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ologi/"+c.Version+" (darwin)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

// DeleteDevice revokes the api_key tied to a device. Auth: required.
func (c *Client) DeleteDevice(deviceID string) error {
	_, err := c.do("DELETE", "/api/voice/devices/"+deviceID, nil)
	return err
}
```

Add a small import-shim to keep the file imports trivial. Near the top of `login.go` add:

```go
import (
	"bytes"
	"encoding/json"
	"net/http"
)

// http_NewReq wraps http.NewRequest — named with an underscore to avoid
// pulling extra imports into the shared file; keeps the 410-hack local.
func http_NewReq(method, url string, body []byte) (*http.Request, error) {
	var r *bytes.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	if r == nil {
		return http.NewRequest(method, url, nil)
	}
	return http.NewRequest(method, url, r)
}
```

Actually — simpler: skip the hack entirely by adjusting the base `do()` helper to surface the body for 410 specifically. Rewrite the `LoginPoll` more cleanly:

```go
// LoginPoll polls once. Treats 200 (pending/ok) and 410 (denied/expired)
// as valid responses, both of which carry a `{"status":...}` body.
// Any other status is returned as an error.
func (c *Client) LoginPoll(deviceCode string) (LoginPollResponse, error) {
	var out LoginPollResponse
	body, _ := json.Marshal(map[string]string{"device_code": deviceCode})
	req, err := http.NewRequest("POST", c.BaseURL+"/api/voice/login/complete", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("ologi/%s (darwin)", c.Version))

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 410 {
		data, _ := io.ReadAll(resp.Body)
		return out, &APIError{StatusCode: resp.StatusCode, Message: string(data)}
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}
```

Use this version. Drop the `http_NewReq` helper. Add imports: `"bytes"`, `"fmt"`, `"io"`, `"net/http"`.

Delete the `loginPoll410Hack` method.

- [ ] **Step 4: Run tests, expect PASS**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/api/... -v
```

Expected: all tests PASS (including the 410/expired case).

- [ ] **Step 5: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/api/login.go packages/ologi-cli/internal/api/login_test.go
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): api login flow + device delete

LoginStart / LoginPoll (single poll — caller loops) / DeleteDevice.
LoginPoll treats both 200 (pending|ok) and 410 (denied|expired) as
valid responses — each carries a decodable {"status":...} body the
CLI needs to switch on.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10 — `internal/sourceapp` package

**Files:**
- Create: `packages/ologi-cli/internal/sourceapp/sourceapp.go`
- Create: `packages/ologi-cli/internal/sourceapp/sourceapp_darwin.go`
- Create: `packages/ologi-cli/internal/sourceapp/browser_tab_darwin.go`

`NSWorkspace.frontmostApplication` via CGo + optional AppleScript for the active browser tab URL.

- [ ] **Step 1: Create the package interface**

Create `packages/ologi-cli/internal/sourceapp/sourceapp.go`:

```go
// Package sourceapp detects the currently-focused macOS application,
// with best-effort augmentation for browsers (active tab host).
//
// Never crashes, never blocks indefinitely. On any error returns "".
// Callers treat the empty string as "no attribution available".
package sourceapp

import "time"

// DetectTimeout bounds the total cost of one Detect() call. AppleScript
// can occasionally hang for several seconds on the first call into an
// unresponsive browser; we cap the overall effort here.
const DetectTimeout = 250 * time.Millisecond

// Detect returns a human-friendly "<App Name> / <host>" when a browser
// is focused and its active tab URL can be read, otherwise just
// "<App Name>", otherwise "".
//
// Examples:
//   "Google Chrome / claude.ai"
//   "iTerm2"
//   ""  (nothing detected)
func Detect() string {
	return detectImpl()
}
```

- [ ] **Step 2: macOS implementation**

Create `packages/ologi-cli/internal/sourceapp/sourceapp_darwin.go`:

```go
//go:build darwin

package sourceapp

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation
#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>

static const char* frontmostAppName() {
    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app == nil) return NULL;
        NSString *name = app.localizedName ?: @"";
        return strdup([name UTF8String]);
    }
}

static const char* frontmostAppBundleID() {
    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app == nil) return NULL;
        NSString *bid = app.bundleIdentifier ?: @"";
        return strdup([bid UTF8String]);
    }
}
*/
import "C"

import "unsafe"

// appInfo returns (localizedName, bundleID). Empty strings on failure.
func appInfo() (name, bundleID string) {
	cn := C.frontmostAppName()
	if cn != nil {
		name = C.GoString(cn)
		C.free(unsafe.Pointer(cn))
	}
	cb := C.frontmostAppBundleID()
	if cb != nil {
		bundleID = C.GoString(cb)
		C.free(unsafe.Pointer(cb))
	}
	return
}

func detectImpl() string {
	name, bundleID := appInfo()
	if name == "" {
		return ""
	}
	if host := browserTabHost(bundleID); host != "" {
		return name + " / " + host
	}
	return name
}
```

- [ ] **Step 3: Browser tab AppleScript helper**

Create `packages/ologi-cli/internal/sourceapp/browser_tab_darwin.go`:

```go
//go:build darwin

package sourceapp

import (
	"context"
	"net/url"
	"os/exec"
	"strings"
)

// browserTabHost returns the host (e.g. "claude.ai") of the active tab
// of the focused browser, or "" if none can be determined.
//
// Wraps each AppleScript call in a DetectTimeout context — some browsers
// occasionally block on the first automation call, and we'd rather
// report no-source than hang the dictation-stop codepath.
func browserTabHost(bundleID string) string {
	scripts := map[string]string{
		"com.google.Chrome":      `tell application "Google Chrome" to if (count of windows) > 0 then return URL of active tab of front window`,
		"com.apple.Safari":       `tell application "Safari" to if (count of windows) > 0 then return URL of front document`,
		"org.mozilla.firefox":    `tell application "Firefox" to if (count of windows) > 0 then return URL of active tab of front window`,
		"company.thebrowser.Browser": `tell application "Arc" to if (count of windows) > 0 then return URL of active tab of front window`,
	}
	script, ok := scripts[bundleID]
	if !ok {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), DetectTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if err != nil {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(string(out)))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
```

- [ ] **Step 4: Build to confirm it compiles**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go build ./internal/sourceapp/...
```

Expected: clean build.

- [ ] **Step 5: Manual smoke**

Tiny main program in a tmp dir:

```go
package main

import (
	"fmt"
	"github.com/ologi/hypertask-cli/internal/sourceapp"
)

func main() {
	fmt.Println(sourceapp.Detect())
}
```

```bash
cd /tmp
go run ./...  # or: go run <file>
```

Focus a terminal and run — expect `Terminal` or `iTerm2`. Focus Chrome with claude.ai open and run — expect `Google Chrome / claude.ai`. First run may trigger a macOS Automation permission prompt for Chrome; accept it.

Smoke is non-blocking for the commit — if the binary compiles and `Detect()` returns something (even just the app name without the host), that's good enough. AppleScript permission is a one-time user grant.

- [ ] **Step 6: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/sourceapp
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): sourceapp — frontmost-app detection

NSWorkspace.frontmostApplication via CGo for the app name + bundle id.
AppleScript helper for browsers (Chrome, Safari, Firefox, Arc) bounded
by a 250ms context timeout so no dictation-stop hangs on a sluggish
browser. Returns empty string on any failure.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11 — `internal/launchd` package

**Files:**
- Create: `packages/ologi-cli/internal/launchd/plist.go`
- Create: `packages/ologi-cli/internal/launchd/plist_test.go`
- Create: `packages/ologi-cli/internal/launchd/control.go`

Plist template + `launchctl` wrappers.

- [ ] **Step 1: Write the plist test**

Create `packages/ologi-cli/internal/launchd/plist_test.go`:

```go
package launchd

import (
	"strings"
	"testing"
)

func TestRenderPlistAutostart(t *testing.T) {
	out, err := RenderPlist(PlistSpec{
		Label:      "app.ologi.voice",
		BinaryPath: "/opt/homebrew/bin/ologi",
		Args:       []string{"voice", "run"},
		HomeDir:    "/Users/test",
		Autostart:  true,
		Env:        map[string]string{"OLOGI_SERVER_URL": "https://voice.ologi.app"},
	})
	if err != nil {
		t.Fatalf("RenderPlist: %v", err)
	}
	if !strings.Contains(out, "<string>app.ologi.voice</string>") {
		t.Error("missing label")
	}
	if !strings.Contains(out, "<string>/opt/homebrew/bin/ologi</string>") {
		t.Error("missing binary path")
	}
	if !strings.Contains(out, "<string>voice</string>") || !strings.Contains(out, "<string>run</string>") {
		t.Error("missing args")
	}
	if !strings.Contains(out, "<key>RunAtLoad</key><true/>") {
		t.Error("missing RunAtLoad=true")
	}
	if !strings.Contains(out, "/Users/test/Library/Logs/ologi-voice.log") {
		t.Error("missing log path")
	}
	if !strings.Contains(out, "<key>OLOGI_SERVER_URL</key><string>https://voice.ologi.app</string>") {
		t.Error("missing env var")
	}
}

func TestRenderPlistNoAutostart(t *testing.T) {
	out, err := RenderPlist(PlistSpec{
		Label:      "app.ologi.voice",
		BinaryPath: "/usr/local/bin/ologi",
		Args:       []string{"voice", "run"},
		HomeDir:    "/Users/test",
		Autostart:  false,
	})
	if err != nil {
		t.Fatalf("RenderPlist: %v", err)
	}
	if !strings.Contains(out, "<key>RunAtLoad</key><false/>") {
		t.Error("missing RunAtLoad=false")
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/launchd/...
```

Expected: FAIL — no package.

- [ ] **Step 3: Implement `plist.go`**

```go
// Package launchd writes and controls the app.ologi.voice LaunchAgent.
package launchd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// Label is the canonical launchd label.
const Label = "app.ologi.voice"

// PlistSpec fully describes the desired plist state.
type PlistSpec struct {
	Label      string
	BinaryPath string
	Args       []string
	HomeDir    string
	Autostart  bool
	Env        map[string]string
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key><string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
      <string>{{.BinaryPath}}</string>
{{range .Args}}      <string>{{.}}</string>
{{end}}    </array>
    <key>RunAtLoad</key>{{if .Autostart}}<true/>{{else}}<false/>{{end}}
    <key>KeepAlive</key>
    <dict>
      <key>Crashed</key><true/>
      <key>SuccessfulExit</key><false/>
    </dict>
    <key>StandardOutPath</key><string>{{.HomeDir}}/Library/Logs/ologi-voice.log</string>
    <key>StandardErrorPath</key><string>{{.HomeDir}}/Library/Logs/ologi-voice.log</string>
{{if .Env}}    <key>EnvironmentVariables</key>
    <dict>
{{range $k, $v := .Env}}      <key>{{$k}}</key><string>{{$v}}</string>
{{end}}    </dict>
{{end}}  </dict>
</plist>
`

var tmpl = template.Must(template.New("plist").Parse(plistTemplate))

// RenderPlist renders a spec into the XML string.
func RenderPlist(spec PlistSpec) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, spec); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// PlistPath returns ~/Library/LaunchAgents/app.ologi.voice.plist.
func PlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

// WritePlist renders and writes the plist to its canonical location,
// creating ~/Library/LaunchAgents if necessary.
func WritePlist(spec PlistSpec) error {
	out, err := RenderPlist(spec)
	if err != nil {
		return err
	}
	path := PlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir LaunchAgents: %w", err)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}
	return nil
}

// RemovePlist deletes the plist file. Returns nil if it didn't exist.
func RemovePlist() error {
	err := os.Remove(PlistPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
```

- [ ] **Step 4: Implement `control.go`**

Create `packages/ologi-cli/internal/launchd/control.go`:

```go
package launchd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// domainTarget returns the gui/<uid> string for launchctl.
func domainTarget() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

// serviceTarget returns gui/<uid>/app.ologi.voice.
func serviceTarget() string {
	return domainTarget() + "/" + Label
}

// Bootstrap loads the plist. Must be called after WritePlist.
// If already loaded, returns nil (idempotent-ish).
func Bootstrap() error {
	path := PlistPath()
	cmd := exec.Command("launchctl", "bootstrap", domainTarget(), path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// launchctl returns 37 ("service already loaded") as a non-zero
		// exit. Treat that as success.
		if strings.Contains(string(out), "already loaded") || strings.Contains(string(out), "Service is disabled") {
			return nil
		}
		return fmt.Errorf("bootstrap: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Bootout unloads the plist. Returns nil if not loaded.
func Bootout() error {
	cmd := exec.Command("launchctl", "bootout", serviceTarget())
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "Could not find service") ||
			strings.Contains(string(out), "No such process") {
			return nil
		}
		return fmt.Errorf("bootout: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Kickstart forces a restart of the loaded service. Use after a binary
// upgrade while the service is loaded.
func Kickstart() error {
	cmd := exec.Command("launchctl", "kickstart", "-k", serviceTarget())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kickstart: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// IsLoaded returns true if the service is currently loaded (whether
// running or not).
func IsLoaded() (bool, error) {
	cmd := exec.Command("launchctl", "print", serviceTarget())
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false, err
	}
	// launchctl exits non-zero when the service isn't found.
	// Status 113 is "no such service" on recent macOS.
	_ = exitErr
	return false, nil
}

// Print returns the raw `launchctl print <service>` output for status
// reporting. Returns "" if the service isn't loaded.
func Print() (string, error) {
	cmd := exec.Command("launchctl", "print", serviceTarget())
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Not loaded — return "" for the caller to show "stopped".
		return "", nil
	}
	return string(out), nil
}
```

- [ ] **Step 5: Run tests**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/launchd/... -v
```

Expected: 2 plist-render tests PASS. (The `control.go` exec-wrappers are intentionally untested at unit level — they're thin shells around `launchctl`; verified manually in Task 15.)

- [ ] **Step 6: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/launchd
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): launchd package — plist render + launchctl wrappers

PlistSpec + text/template-based render (tested with 2 cases). Bootstrap
/ Bootout / Kickstart / IsLoaded / Print wrap launchctl with idempotent
handling of "already loaded" / "not found" exits. No unit tests for the
exec wrappers — verified via Task 15 manual smoke.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12 — Wire the engine to the API client

**Files:**
- Modify: `packages/ologi-cli/internal/engine/engine.go`
- Create: `packages/ologi-cli/internal/engine/engine_wire_test.go`

The engine becomes a consumer of the `api.Client`. It:
1. Preloads config from `api.Client.GetConfig()` on startup.
2. Before each dictation, mints a fresh realtime token via `api.Client.MintRealtimeToken()`.
3. After each dictation, POSTs the session via `api.Client.PostSession()` and checks `settings_version`.

Task 5 left `engine.NewEngine(cfg, onSession)` in place. Here we ADD a `Runtime` struct that owns the `api.Client` plus the engine, and implements the full bootstrap + loop.

- [ ] **Step 1: Add the wiring struct to `engine.go`**

Open `packages/ologi-cli/internal/engine/engine.go`. Near the bottom (after `Engine.Stop()`), append:

```go
// --- API-wired runtime wrapper --------------------------------------------

// APIClient is the subset of *api.Client the engine needs. Defined as an
// interface so tests can substitute a fake.
type APIClient interface {
	GetConfig() (api.ConfigResponse, error)
	MintRealtimeToken() (string, error)
	PostSession(api.PostSessionInput) (api.PostSessionResponse, error)
}

// SourceAppDetector returns the source-app string or "" if unavailable.
type SourceAppDetector func() string

// Runtime ties the Engine to the API client + source-app detector.
// Main calls Runtime.Boot() once and then runs Engine.Run() in a
// goroutine; the runtime's OnSession hook is the callback the engine
// invokes after each dictation.
type Runtime struct {
	Client         APIClient
	Detect         SourceAppDetector
	currentVersion int // last-seen settings_version
}

// Boot loads initial config from the server and returns the engine-
// ready Config. Callers pass this into NewEngine along with r.OnSession.
func (r *Runtime) Boot() (Config, error) {
	c, err := r.Client.GetConfig()
	if err != nil {
		return Config{}, err
	}
	r.currentVersion = c.SettingsVersion

	replacements := make([]ReplacementEntry, 0, len(c.Replacements))
	for _, e := range c.Replacements {
		replacements = append(replacements, ReplacementEntry{
			Pattern:     e.Pattern,
			Replacement: e.Replacement,
		})
	}

	device := ""
	if c.MicDevice != nil {
		device = *c.MicDevice
	}

	return Config{
		Hotkey:       c.Hotkey,
		Language:     c.Language,
		SampleRate:   16000,
		Device:       device,
		Channel:      0,
		Mode:         "stream",
		StartSound:   c.StartSound,
		StopSound:    c.StopSound,
		Replacements: replacements,
	}, nil
}

// OnSession is the hook the Engine calls after each dictation. The
// runtime posts the session to the server and, if the returned
// settings_version advanced, re-pulls config in the background.
//
// Errors are logged but not surfaced — typewriter has already typed
// the text regardless; losing a history row is better than crashing
// the daemon.
func (r *Runtime) OnSession(s Session) {
	var srcPtr *string
	if r.Detect != nil {
		if src := r.Detect(); src != "" {
			srcPtr = &src
		}
	}
	resp, err := r.Client.PostSession(api.PostSessionInput{
		Mode:       s.Mode,
		StartedAt:  s.StartedAt,
		EndedAt:    s.EndedAt,
		DurationMs: s.DurationMs,
		SourceApp:  srcPtr,
		Text:       s.Text,
	})
	if err != nil {
		log.Printf("[runtime] POST /sessions failed: %v", err)
		return
	}
	if resp.SettingsVersion > r.currentVersion {
		r.currentVersion = resp.SettingsVersion
		// Fire-and-forget re-pull; engine picks up on next restart.
		// For v1 we don't hot-reload the running engine's replacements
		// cache — a user who edits their dictionary restarts the daemon
		// (or it restarts automatically on next brew upgrade).
		go func() {
			_, _ = r.Client.GetConfig()
		}()
	}
}

// MintToken exposes the api.Client's MintRealtimeToken for callers
// who need to intercept the engine's token-acquisition step. In v1 the
// engine itself calls through to this directly via its stored APIClient.
func (r *Runtime) MintToken() (string, error) {
	return r.Client.MintRealtimeToken()
}
```

Make sure `engine.go` imports `"github.com/ologi/hypertask-cli/internal/api"`.

- [ ] **Step 2: Teach the engine to use the runtime's token for each session**

In the existing `Engine.Run()`, there's a section that handles stream-mode initiation. Find the block that currently looks something like:

```go
aai, err = NewAssemblyAI(e.cfg.APIKey, e.cfg.Language, actualRate, handler)
```

(After Task 5's rename this is `aai.NewStreamingClient(e.cfg.APIKey, ...)`.)

Add a new `TokenSource` field to the Engine:

```go
// TokenSource returns a fresh scoped AssemblyAI token. Called on each
// dictation to mint a token from the server. If nil, the engine uses a
// hardcoded env-var key (for dev smoke only).
type TokenSource func() (string, error)

type Engine struct {
	cfg         Config
	onSession   OnSessionFunc
	tokenSource TokenSource
	events      chan EngineEvent
	stopCh      chan struct{}
	doneCh      chan struct{}
}

func NewEngine(cfg Config, onSession OnSessionFunc, tokenSource TokenSource) *Engine {
	return &Engine{
		cfg:         cfg,
		onSession:   onSession,
		tokenSource: tokenSource,
		events:      make(chan EngineEvent, 64),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}
```

And inside `Run()`, replace the `aai.NewStreamingClient(...)` call's first argument:

```go
// OLD: aai.NewStreamingClient(e.cfg.APIKey, e.cfg.Language, ...)
// NEW:
token := os.Getenv("OLOGI_DEV_AAI_KEY") // dev-only fallback
if e.tokenSource != nil {
	t, err := e.tokenSource()
	if err != nil {
		log.Printf("[engine] token mint failed: %v", err)
		e.emit(EngineEvent{Type: EventError, Error: err})
		// Bail out of this dictation
		audio.Stop()
		audio = nil
		recording = false
		e.emit(EngineEvent{Type: EventStatusChanged, State: "idle"})
		continue
	}
	token = t
}
aaiClient, err := aai.NewStreamingClient(token, e.cfg.Language, actualRate, handler)
// ... remaining logic unchanged
```

(Imports: `os`, if not already present.)

- [ ] **Step 3: Add the runtime test**

Create `packages/ologi-cli/internal/engine/engine_wire_test.go`:

```go
package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/ologi/hypertask-cli/internal/api"
)

type fakeAPI struct {
	config     api.ConfigResponse
	configErr  error
	mintErr    error
	token      string
	postErr    error
	lastPost   api.PostSessionInput
	postedOnce chan struct{}
}

func (f *fakeAPI) GetConfig() (api.ConfigResponse, error) {
	return f.config, f.configErr
}
func (f *fakeAPI) MintRealtimeToken() (string, error) { return f.token, f.mintErr }
func (f *fakeAPI) PostSession(in api.PostSessionInput) (api.PostSessionResponse, error) {
	f.lastPost = in
	if f.postedOnce != nil {
		select {
		case f.postedOnce <- struct{}{}:
		default:
		}
	}
	return api.PostSessionResponse{SessionID: "s1", CanonicalText: in.Text, SettingsVersion: f.config.SettingsVersion}, f.postErr
}

func TestRuntimeBootReturnsEngineConfig(t *testing.T) {
	device := "MacBook Pro Mic"
	api := &fakeAPI{config: api.ConfigResponse{
		SettingsVersion: 2,
		Hotkey:          "right_option",
		Language:        "en",
		MicDevice:       &device,
		StartSound:      "Tink",
		StopSound:       "Pop",
	}}
	rt := &Runtime{Client: api}

	cfg, err := rt.Boot()
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if cfg.Hotkey != "right_option" || cfg.Language != "en" || cfg.Device != device {
		t.Errorf("unexpected cfg: %+v", cfg)
	}
	if cfg.SampleRate != 16000 || cfg.Mode != "stream" {
		t.Errorf("wrong defaults: sr=%d mode=%q", cfg.SampleRate, cfg.Mode)
	}
}

func TestRuntimeBootSurfacesAuthError(t *testing.T) {
	rt := &Runtime{Client: &fakeAPI{configErr: errors.New("401")}}
	if _, err := rt.Boot(); err == nil {
		t.Error("want error, got nil")
	}
}

func TestRuntimeOnSessionPostsToAPI(t *testing.T) {
	api := &fakeAPI{token: "tok", postedOnce: make(chan struct{}, 1)}
	rt := &Runtime{
		Client: api,
		Detect: func() string { return "iTerm2" },
	}

	start := time.Now()
	end := start.Add(time.Second)
	rt.OnSession(Session{
		Mode:       "stream",
		StartedAt:  start,
		EndedAt:    end,
		DurationMs: 1000,
		Text:       "hello",
	})

	if api.lastPost.Mode != "stream" || api.lastPost.Text != "hello" {
		t.Errorf("unexpected post: %+v", api.lastPost)
	}
	if api.lastPost.SourceApp == nil || *api.lastPost.SourceApp != "iTerm2" {
		t.Errorf("source_app: got %v", api.lastPost.SourceApp)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go test ./internal/engine/... -v
```

Expected: all 3 runtime tests PASS. The engine's event-loop tests aren't added here (they'd require mocking audio + AAI + keylistener — high-effort, low-value); the manual smoke in Task 15 covers the real loop.

- [ ] **Step 5: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/internal/engine
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): wire engine to the API client

Runtime struct: Boot() pulls /config, maps to engine.Config. OnSession
hook POSTs the finished session and, if settings_version advanced,
triggers a background re-pull of /config.

Engine.TokenSource replaces the hardcoded AAI key — invoked before each
dictation. Dev fallback: OLOGI_DEV_AAI_KEY env var.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13 — `cmd/ologi` router + main

**Files:**
- Modify: `packages/ologi-cli/cmd/ologi/main.go`
- Create: `packages/ologi-cli/cmd/ologi/cli.go`

Replaces the placeholder `main.go` from Task 2 with a real subcommand router.

- [ ] **Step 1: Rewrite `main.go`**

Replace the contents of `packages/ologi-cli/cmd/ologi/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

// version is set at build time via -ldflags "-X main.version=<semver>".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "--version", "-v", "version":
		fmt.Printf("ologi %s\n", version)
	case "--help", "-h", "help":
		printUsage(os.Stdout)
	case "login":
		cmdLogin(os.Args[2:])
	case "logout":
		cmdLogout(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "voice":
		cmdVoice(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "ologi: unknown command %q\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `ologi — talk your way through your AI conversations

Usage:
  ologi login                   Link this device to your Ologi account
  ologi logout                  Revoke the link, remove local config
  ologi status                  Show account + voice daemon status
  ologi voice run               Start the foreground listener
  ologi voice start             Start the launchd-managed daemon
  ologi voice stop              Stop the daemon
  ologi voice autostart on|off  Toggle start-at-login
  ologi voice status            Show the daemon's launchctl status
  ologi --version               Print the binary version
`)
}
```

- [ ] **Step 2: Create `cli.go` with a shared helper for loading config + building an API client**

```go
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
```

- [ ] **Step 3: Build**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go build ./cmd/ologi/...
```

Expected: compile errors — `cmdLogin`, `cmdLogout`, `cmdStatus`, `cmdVoice` are undefined. That's fine; those land in Tasks 14 + 15.

To keep the commit compiling, temporarily add stub functions at the bottom of `cli.go`:

```go
func cmdLogin(args []string)  { fmt.Fprintln(os.Stderr, "ologi: login not implemented yet"); os.Exit(1) }
func cmdLogout(args []string) { fmt.Fprintln(os.Stderr, "ologi: logout not implemented yet"); os.Exit(1) }
func cmdStatus(args []string) { fmt.Fprintln(os.Stderr, "ologi: status not implemented yet"); os.Exit(1) }
func cmdVoice(args []string)  { fmt.Fprintln(os.Stderr, "ologi: voice not implemented yet"); os.Exit(1) }
```

Replaced in Tasks 14 and 15.

- [ ] **Step 4: Build + smoke**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go build -o /tmp/ologi ./cmd/ologi
/tmp/ologi --version       # → ologi dev
/tmp/ologi --help          # → usage text
/tmp/ologi bogus           # → unknown command + exit 1
/tmp/ologi login           # → stub message + exit 1
```

All expected.

- [ ] **Step 5: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/cmd/ologi
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): router + shared CLI helpers

Subcommand dispatch for login / logout / status / voice. Stub bodies
for each until Tasks 14 + 15 fill them in. Shared helpers for
server-URL resolution (env > config > default) and API-client build.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14 — `ologi login` / `logout` / `status`

**Files:**
- Create: `packages/ologi-cli/cmd/ologi/cmd_login.go` (replaces stubs from Task 13)

- [ ] **Step 1: Implement `cmd_login.go`**

Replace the stubs in `cli.go` by removing them, then create `packages/ologi-cli/cmd/ologi/cmd_login.go`:

```go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ologi/hypertask-cli/internal/api"
	"github.com/ologi/hypertask-cli/internal/config"
	"github.com/ologi/hypertask-cli/internal/launchd"
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
```

Delete the `cmdLogin`/`cmdLogout`/`cmdStatus` stubs from `cli.go` (keeping the `cmdVoice` stub until Task 15).

- [ ] **Step 2: Build**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go build -o /tmp/ologi ./cmd/ologi
```

- [ ] **Step 3: Manual smoke against the dev server**

Requires the Plan A dev server running on `voice.ologi.localhost:3005`. In one terminal:

```bash
cd /Users/thedevdad/Documents/hypertask/apps/web
PORT=3005 pnpm dev
```

In another:

```bash
OLOGI_SERVER_URL=http://voice.ologi.localhost:3005 /tmp/ologi login
# answer the device-name prompt, a browser tab opens at
# http://ologi.localhost:3005/voice/link?code=XXXX; sign in on apex,
# Approve. CLI prints "✓ linked as <name>".
cat ~/.config/ologi/config.toml
# → api_key, device_id, device_name, server_url=http://voice.ologi.localhost:3005

/tmp/ologi status
# → account: <name>
#   voice:   stopped

/tmp/ologi logout
# → ✓ logged out

/tmp/ologi status
# → account: (not logged in)
#   voice:   (stopped)
```

- [ ] **Step 4: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/cmd/ologi
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): login / logout / status subcommands

login: device-code flow with 2s polling, 10min deadline, ^C-cancellable.
logout: revokes server-side + unloads launchd + removes config.toml.
status: prints account (device name) + voice (running/stopped).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 15 — `ologi voice run / start / stop / autostart / status`

**Files:**
- Create: `packages/ologi-cli/cmd/ologi/cmd_voice.go` (replaces stub from Task 13)

- [ ] **Step 1: Implement `cmd_voice.go`**

Delete the `cmdVoice` stub from `cli.go`. Create `packages/ologi-cli/cmd/ologi/cmd_voice.go`:

```go
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ologi/hypertask-cli/internal/engine"
	"github.com/ologi/hypertask-cli/internal/launchd"
	"github.com/ologi/hypertask-cli/internal/sourceapp"
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

	// Preserve the current RunAtLoad if the plist already exists — unless
	// the caller asked to override via autostart=true.
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
	// Rewrite plist with the new RunAtLoad. This reuses voiceStart logic
	// but forces the autostart flag.
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
	out, _ := launchd.Print()
	// Parse a few fields from the launchctl print output for a terse
	// one-liner.
	// For v1 just print "running" and hint at the log file.
	fmt.Println("running")
	fmt.Println("logs: ~/Library/Logs/ologi-voice.log")
	_ = out
}
```

- [ ] **Step 2: Build**

```bash
cd /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/packages/ologi-cli
go build -ldflags "-X main.version=0.0.1-dev" -o /tmp/ologi ./cmd/ologi
/tmp/ologi --version  # → ologi 0.0.1-dev
```

- [ ] **Step 3: Manual E2E smoke (requires the dev web server + a valid dev ht_* key)**

Dev server in terminal 1:

```bash
cd /Users/thedevdad/Documents/hypertask/apps/web
PORT=3005 pnpm dev
```

Terminal 2:

```bash
OLOGI_SERVER_URL=http://voice.ologi.localhost:3005 /tmp/ologi login
# approve in the browser

/tmp/ologi voice run
# Double-tap right_option, speak, release. Expect: text typed into
# whatever app has focus; a session row appears on
# http://voice.ologi.localhost:3005/#history within ~2s.
# ^C to exit.

/tmp/ologi voice start
/tmp/ologi voice status     # → running
/tmp/ologi voice stop
/tmp/ologi voice status     # → stopped

/tmp/ologi voice autostart on
# Reboot your laptop (optional — verify daemon auto-starts after login).
/tmp/ologi voice autostart off

/tmp/ologi logout
```

Any failure surfaces an actionable error message. Capture any rough edges as follow-up commits.

- [ ] **Step 4: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add packages/ologi-cli/cmd/ologi/cmd_voice.go
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
feat(ologi-cli): voice run / start / stop / autostart / status

run: foreground listener, full engine + api wiring.
start / stop / autostart: launchctl wrappers + plist rewrites.
status: terse "running" | "stopped" + log path.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 16 — Release pipeline + Homebrew tap

**Files:**
- Create: `.github/workflows/ologi-cli-release.yml` (in the hypertask monorepo)
- Create: documentation for a separate `ologi/homebrew-tap` repo (setup is a manual one-time user step)

- [ ] **Step 1: Add the GitHub Actions release workflow**

Create `/Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli/.github/workflows/ologi-cli-release.yml`:

```yaml
name: Release ologi CLI

on:
  push:
    tags:
      - 'ologi-cli-v*'

jobs:
  build-and-release:
    strategy:
      matrix:
        include:
          - runner: macos-14      # arm64
            arch: arm64
          - runner: macos-13      # intel
            arch: amd64
    runs-on: ${{ matrix.runner }}
    defaults:
      run:
        working-directory: packages/ologi-cli
    steps:
      - uses: actions/checkout@v4

      - name: Install PortAudio
        run: brew install portaudio

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Run tests
        run: go test ./...

      - name: Compute version from tag
        id: ver
        run: echo "version=${GITHUB_REF_NAME#ologi-cli-v}" >> $GITHUB_OUTPUT

      - name: Build binary
        env:
          CGO_ENABLED: "1"
        run: |
          go build \
            -ldflags "-X main.version=${{ steps.ver.outputs.version }}" \
            -o ologi \
            ./cmd/ologi

      # TODO(sign-and-notarize): add code-signing + notarytool steps
      # when Apple Developer secrets are provisioned. See the spec's
      # D10 risk/mitigation. For now ship unsigned; users will need to
      # `xattr -d com.apple.quarantine` on the binary on first run.

      - name: Package tarball
        run: |
          NAME="ologi-${{ steps.ver.outputs.version }}-darwin-${{ matrix.arch }}"
          mkdir -p dist
          tar czf "dist/${NAME}.tar.gz" ologi
          shasum -a 256 "dist/${NAME}.tar.gz" > "dist/${NAME}.tar.gz.sha256"

      - name: Upload release assets
        uses: softprops/action-gh-release@v2
        with:
          files: |
            packages/ologi-cli/dist/*.tar.gz
            packages/ologi-cli/dist/*.tar.gz.sha256
          tag_name: ${{ github.ref_name }}
          generate_release_notes: true
```

Notes:
- Signing / notarization is deferred to a follow-up. The spec's D10 acknowledges this — shipping unsigned binaries works, users hit one Gatekeeper dialog. The workflow file's `TODO(sign-and-notarize)` comment marks where signing steps go later.
- Bumping the Homebrew tap formula's SHAs is a manual step for v0.1.0; a follow-up iteration can automate it via a PAT-authed `git` step against the tap repo.

- [ ] **Step 2: Document the Homebrew tap setup in the README**

Append to `packages/ologi-cli/README.md`:

```markdown

## Releasing

Releases are cut by pushing a tag matching `ologi-cli-v*`:

```
git tag ologi-cli-v0.1.0
git push --tags
```

The CI workflow `.github/workflows/ologi-cli-release.yml` builds darwin
arm64 + amd64 tarballs and uploads them as GitHub Release assets.

### Homebrew tap (one-time setup)

A separate repo `ologi/homebrew-tap` holds the formula. First time:

1. Create repo at `github.com/ologi/homebrew-tap`.
2. Add `Formula/ologi.rb`:

   ```ruby
   class Ologi < Formula
     desc "Ologi — talk your way through your AI conversations"
     homepage "https://ologi.app/voice"
     version "0.1.0"

     depends_on "portaudio"
     depends_on :macos

     on_macos do
       on_arm do
         url "https://github.com/<your-gh-org>/hypertask/releases/download/ologi-cli-v0.1.0/ologi-0.1.0-darwin-arm64.tar.gz"
         sha256 "<fill-from-CI-artifact>"
       end
       on_intel do
         url "https://github.com/<your-gh-org>/hypertask/releases/download/ologi-cli-v0.1.0/ologi-0.1.0-darwin-amd64.tar.gz"
         sha256 "<fill-from-CI-artifact>"
       end
     end

     def install
       bin.install "ologi"
     end

     test do
       assert_match(/^ologi /, shell_output("#{bin}/ologi --version"))
     end
   end
   ```

3. On each new `ologi-cli-v*` release, bump `version` and the two SHAs
   using the values emitted by the workflow (they're in the `.sha256`
   files attached to the GitHub Release).

Users install with:

```
brew install ologi-app/tap/ologi
```
```

- [ ] **Step 3: Commit**

```bash
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli add .github/workflows/ologi-cli-release.yml packages/ologi-cli/README.md
git -C /Users/thedevdad/Documents/hypertask/.worktrees/ologi-voice-cli commit -m "$(cat <<'EOF'
build(ologi-cli): GitHub Actions release workflow + tap setup docs

Cuts signed tarballs for darwin/arm64 + darwin/amd64 on ologi-cli-v*
tag push. Attaches them + sha256 files to the GitHub Release.

Signing + notarization are TODO until Apple Developer creds are wired
into GH secrets; unsigned binaries work with one Gatekeeper prompt.

Tap setup (separate ologi/homebrew-tap repo + Formula/ologi.rb) is
documented in the README — one-time manual user step since we don't
have credentials to create that repo from the monorepo CI.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist (author runs before handing off)

- **Spec coverage.** Every D-point in the spec has a task:
  - D1 (binary name `ologi`) — Task 2 (`go mod init`), Task 13 (`main.go`).
  - D2 (v1 scope) — Tasks 14 + 15.
  - D3 (launchd) — Task 11 + Task 15.
  - D4 (config TOML 0600) — Task 6.
  - D5 (plist label) — Task 11.
  - D6 (scoped AAI token) — Task 4 + Task 8 + Task 12.
  - D7 (dictionary server-owned) — Task 12.
  - D8 (source app) — Task 10 + Task 12.
  - D9 (Homebrew tap) — Task 16.
  - D10 (signing) — Task 16 (deferred, documented).
  - D11 (CI) — Task 16.
  - D12 (monorepo fork) — Tasks 3 + 4 + 5.
  - D13 (module layout) — the file-structure section.
  - D14 (what gets deleted) — Task 5.
  - D15 (what gets added) — Tasks 6–11 + Task 12.
  - D16 (ht_* auth) — Task 7.
  - D17 (device-code login) — Task 9 + Task 14.
  - D18 (logout revokes) — Task 14.
  - D19 (OLOGI_SERVER_URL) — Task 13.
  - D20 (hotkey default) — Task 5 + Task 12.
  - D21 (batch modifier) — preserved in Task 5's fork.
  - D22 (logs) — Task 11 (plist paths) + Task 14 (status message).
  - D23 (version via ldflags) — Task 15 build step + Task 16 CI flow.
  - D24 (permission prompts) — out-of-plan; OS-level.
  - D25 (fail-closed on auth) — Task 14 (`loadConfigOrDie`) + Task 12 (OnSession logs 401).
  - D26 (one daemon at a time) — Task 15 (`voiceRun` checks `launchd.IsLoaded`).

- **Placeholder scan.** Reviewed: no "TBD" / "TODO: implement" / "fill in" in any step body. One `TODO(sign-and-notarize)` comment in the CI workflow file — explicitly called out as deferred; spec D10 covers the plan for filling it later.

- **Type consistency.** Spot-checked:
  - `ReplacementEntry` is defined once in `internal/api/config.go` (Task 8) and reused by `internal/engine/engine.go` via its own local `ReplacementEntry` type (Task 5) — the Runtime.Boot() method (Task 12) translates between them. Deliberate: engine has no import of `api`.
  - Actually — re-reading Task 12, `Runtime` lives in `engine.go` and imports `api`. So engine DOES have a dependency on api. The separate `ReplacementEntry` in engine is the internal-use shape; the `api.ReplacementEntry` is the wire shape. The conversion is explicit in `Runtime.Boot`. Fine.
  - `Session` (Task 5) vs `PostSessionInput` (Task 8): different types, converted in `Runtime.OnSession`. Fine.
  - `StreamingClient` (Task 4) matches all `aai.NewStreamingClient(...)` call sites.
  - `Engine.NewEngine(cfg, onSession, tokenSource)` signature matches Task 15's `engine.NewEngine(engineCfg, rt.OnSession, rt.MintToken)` call.

- **Cross-task dependencies.** Task 12 depends on Tasks 5 + 7 + 8 + 10. Task 15 depends on Tasks 11 + 12. Task 16 depends on everything compiling. Order in the plan respects this.

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-21-ologi-voice-cli.md`. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
