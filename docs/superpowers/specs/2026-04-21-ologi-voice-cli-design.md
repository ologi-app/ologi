# Ologi Voice CLI — design spec

**Date:** 2026-04-21
**Status:** Draft — pending user review
**Owner:** solo
**Scope:** The macOS CLI half of Ologi Voice. Forks the existing `ptt` Go app from `~/Documents/stt/` into a new `packages/ologi-cli/` monorepo package. Binary named `ologi` with voice-only subcommands for v1. Distributed via a private Homebrew tap. Integrates with the Plan A web API for auth, config, AssemblyAI-token minting, and session posting. Managed by macOS `launchd` as a background LaunchAgent.

---

## TL;DR

`~/Documents/stt/ptt` is a mature Go app that captures mic audio via PortAudio, streams it to AssemblyAI over WebSocket with the user's own API key, types the transcript into the focused app via CGEvent, and stores history/metrics/replacements locally in `~/.config/ptt/*`. It has a bubbletea TUI wrapping all of that.

Plan A already shipped the web side: schema, 13 API endpoints under `/api/voice/*`, a tabbed dashboard at `voice.ologi.app`, a first-run wizard, a CLI-approval page, and the marketing landing.

Plan B ships the CLI to complete the loop. It forks the `ptt` engine, strips everything Plan A took over (TUI, local history, local metrics, local replacements config), and wires the engine to the Plan A API:
- `POST /api/voice/login/start` + `/complete` + `/approve` for device-code login.
- `GET /api/voice/config` for settings + dictionary.
- `POST /api/voice/realtime-token` for per-session AssemblyAI streaming tokens.
- `POST /api/voice/sessions` to finalize each dictation.

The binary is named `ologi`, not `ologi-voice`. Voice commands sit under a `voice` subcommand (`ologi voice run`, `ologi voice start`, `ologi voice stop`). No TUI, no plugin management, no task subcommands in v1 — those land in later plans as additive subcommands on the same binary. This avoids a rename later and costs zero extra today.

Daemon lifecycle runs under `launchd` via a LaunchAgent plist. `ologi voice run` is a plain foreground blocking listener; `launchd` (invoked from `ologi voice start`) runs it in the background and handles crash-restart and start-at-login. Users can still run `ologi voice run` manually from a terminal for debugging.

Distribution is a private `ologi/homebrew-tap` with a CI-built signed+notarized macOS binary (arm64 + amd64) shipped on GitHub Releases and pinned in the formula.

MVP-strict: no TUI, no in-binary plugin management, no tasks, no Windows, no Linux, no self-update (brew handles it).

---

## Background

### What's in `~/Documents/stt/`

~1.6k LOC Go, single package (`main`). Key files:

- `audio.go` — PortAudio capture, 16kHz PCM16, mono via channel-select for multi-channel devices.
- `keylistener_darwin.go` — `right_option` double-tap detection via CGEvent taps; `shift+` modifier = batch mode.
- `transcribe.go` — WebSocket client to `wss://streaming.assemblyai.com/v3/ws`, speech model `universal-streaming-english` (or `universal-streaming-multilingual` for non-English).
- `batch.go` — POST-based fallback: accumulate PCM → wrap in WAV → `v2/upload` → `v2/transcript` → poll.
- `typewriter_darwin.go` — CGEvent keystroke injection into whichever app has focus. The secret sauce; types into ChatGPT, Claude.ai, terminals, editors, etc. indistinguishably from a human keypress.
- `sound_darwin.go` — plays the start/stop Tink/Pop sounds via AppKit's NSSound.
- `engine.go` — state machine tying keybind + audio + transcriber + typewriter together; dispatches events on a channel.
- `config.go` — TOML at `~/.config/ptt/config.toml` holding API key, language, device, keybind, replacements dictionary.
- `history.go` / `history_test.go` — jsonl at `~/.config/ptt/history.jsonl` (last 100 entries, rotated).
- `metrics.go` — JSON at `~/.config/ptt/metrics.json` (per-day session rollup).
- `tui_*.go` — bubbletea TUI (device picker, history browser, metrics view, settings, substitutions editor, status pane).
- `main.go` — subcommand parser (`--config`, `--version`, `--test-mic`, `--record-key`, `--sounds`, `--devices`, flags `--headless`, `--stream`, `--batch`, `-r`, `--mic`).

### What Plan A already shipped

- **Schema**: `voice_sessions`, `voice_transcripts`, `voice_settings`, `voice_replacements`, `voice_devices`, `voice_device_codes` + `voice_mode` / `voice_device_code_status` enums.
- **API**:
  - `POST /api/voice/login/start` — body `{device_name, platform, cli_version}` → `{device_code, verification_url, interval_ms}`.
  - `POST /api/voice/login/complete` — body `{device_code}` → `{status: 'pending'|'denied'|'expired'|'ok', api_key?, device_id?}`. Single-use on success.
  - `POST /api/voice/login/approve` (web, Firebase-session-authed) + `/deny`.
  - `GET /api/voice/config` — Bearer-authed — returns `{settings_version, hotkey, language, mic_device, start_sound, stop_sound, replacements: [{pattern, replacement}]}`.
  - `POST /api/voice/realtime-token` — Bearer-authed — returns `{token}` (60s AssemblyAI scoped token).
  - `POST /api/voice/sessions` — Bearer-authed — body `{mode, started_at, ended_at, duration_ms, source_app?, text}` → `{session_id, canonical_text, settings_version}`. Server applies replacements canonically and computes word_count + wpm.
  - `GET /api/voice/sessions` — Firebase-session-authed (web only) — paginated.
  - `GET /api/voice/stats` — web only.
  - `PATCH /api/voice/settings`, `POST/DELETE /api/voice/replacements/:id` — web only.
  - `GET /api/voice/devices` + `DELETE /api/voice/devices/:id` — web only.
- **Web**: tabbed dashboard (History / Stats / Settings), first-run wizard modal that polls `/sessions` and auto-closes on first row, device-code approve page at `/voice/link?code=<code>`, marketing landing at `ologi.app/voice`.
- **App-switcher** grown to 2×3 with a mic-glyph Voice tile.

### What the CLI needs to do

Reduce the fork to its essence and add a thin Ologi API client. Minimum loop:

1. Boot: read `~/.config/ologi/config.toml` for the `ht_*` key. Call `GET /api/voice/config` → cache hotkey, language, replacements, mic, sounds in memory.
2. Install key listener for the configured hotkey. Block.
3. On key-down: mint a realtime token (`POST /api/voice/realtime-token`); open the WS to AssemblyAI with that token; start PortAudio capture; wire captured frames into the WS.
4. On each partial transcript: update the typewriter's staged text.
5. On each final transcript ("turn complete" from AAI): keep it staged, continue accumulating.
6. On key-up: stop audio, flush AAI, apply replacements to `committed + partial`, type the final text via CGEvent, `POST /api/voice/sessions`. If the response carries a new `settings_version`, refetch `/config`.
7. Loop.

Plus the auxiliary commands — login, logout, launchd integration, status.

---

## Design decisions (locked in brainstorming)

| # | Decision | Choice | Why |
|---|---|---|---|
| D1 | Binary name | `ologi` (not `ologi-voice`) — voice functionality under a `voice` subcommand | Zero-cost today; avoids rename-dance when we grow (plugin mgmt, TUI, tasks port). Forever-name; claim the namespace. |
| D2 | v1 scope | Voice subsystem only: `login`, `logout`, `voice run`, `voice start`, `voice stop`, `voice autostart on/off`, `status`, `--version` | MVP-strict. Plugin management and TUI are Plan C; tasks port is Plan D. |
| D3 | Daemon lifecycle | macOS `launchd` LaunchAgent. `ologi voice run` is the blocking foreground listener; `ologi voice start/stop` wrap `launchctl bootstrap/bootout` | Every serious macOS background tool does this. Survives TUI quits, reboots, crashes; gets `StartAtLogin` via `RunAtLoad`. Cleaner than self-daemonization. |
| D4 | Config location | `~/.config/ologi/config.toml` mode 0600, holding `{api_key, device_name, device_id, server_url?}` | Future-proof — `ologi` binary owns this directory regardless of which subcommands exist. API key at mode 0600 is acceptable for MVP; Keychain migration is a later plan. |
| D5 | launchd plist label | `app.ologi.voice` at `~/Library/LaunchAgents/app.ologi.voice.plist` | Subcommand-scoped label so later daemons (eventual `app.ologi.*`) don't collide. |
| D6 | AssemblyAI token flow | Per-session scoped token minted server-side | Already shipped (Plan A's `POST /api/voice/realtime-token`). CLI never sees our real AAI key. |
| D7 | Dictionary (replacements) | Server-owned. CLI pulls on boot and after any session POST where the response's `settings_version` advanced | Plan A already made the server canonical. CLI just follows. |
| D8 | Source-app attribution | Native CGo call to `[NSWorkspace frontmostApplication].bundleIdentifier` or `localizedName`. For Chrome/Safari/Firefox, best-effort AppleScript to read the active tab URL (200ms timeout; on failure just use the app name) | Cheap, rich. Flows into the dashboard's history-row badge. |
| D9 | Distribution | Private `ologi/homebrew-tap` with a formula pinning a signed + notarized GitHub Release tarball | Standard macOS CLI pattern. Not Homebrew core (new project, would be rejected). |
| D10 | Signing + notarization | Apple Developer ID code sign + `notarytool` submit + staple, both automated in CI | Without it, first-run gets the "unidentified developer" Gatekeeper dialog. One-time ~$99/yr + CI secrets; after setup it's zero-touch. |
| D11 | CI build target | GitHub Actions workflow in the hypertask monorepo. On `voice-cli-v*` tag: cross-compile darwin/arm64 + darwin/amd64, sign, notarize, upload to GitHub Release, bump the tap formula via a follow-up commit | One tag = one release. No manual steps. |
| D12 | Relation to `~/Documents/stt` | Copy into `packages/ologi-cli/` as a new in-monorepo Go package. The standalone `stt` repo becomes historical. | One codebase, one deploy pipeline, shared tooling. |
| D13 | Module layout | `cmd/ologi/main.go` (entry), `internal/audio`, `internal/keylistener`, `internal/sound`, `internal/typewriter`, `internal/aai`, `internal/engine`, `internal/api` (Ologi client), `internal/config`, `internal/launchd`, `internal/sourceapp` | Idiomatic Go; allows later non-voice subcommands to add siblings under `internal/*` without touching voice code. |
| D14 | What gets deleted from the ptt source | All `tui_*.go`, `history.go`/`history_test.go`, `metrics.go`, `--headless` flag (daemon has only one mode), local `replacements` from the TOML | Those concerns moved server-side in Plan A. Fork carries only the engine. |
| D15 | What gets added | `internal/api` (device-code login, config, realtime-token, sessions), `internal/launchd` (plist install/load/unload/kickstart), `internal/sourceapp` (frontmost-app detection), `ologi` subcommand router | Wraps the engine in account-aware plumbing. |
| D16 | Authentication | `ht_*` API keys via Bearer header. Same shape the tasks CLI and Claude Code plugin already use. Minted during device-code login, stored in `~/.config/ologi/config.toml` at mode 0600 | Reuses Plan A + existing `api_keys` table. |
| D17 | Login flow | Device-code: `ologi login` POSTs `/login/start` → gets `device_code` + `verification_url` → opens the URL in the default browser (`open` on macOS) → polls `/login/complete` every `interval_ms` (2s) until `approved`, `denied`, or `expired` | Already shipped server-side. CLI just drives it. |
| D18 | Logout | `ologi logout` calls `DELETE /api/voice/devices/:id` (device ID in config), unloads the launchd plist if installed, deletes `~/.config/ologi/config.toml` | Clean slate. Revokes the API key server-side so a stolen config file can't be re-used after logout. |
| D19 | Server URL | Configurable via `OLOGI_SERVER_URL` env var, defaults to `https://voice.ologi.app`. For dev, developers set `OLOGI_SERVER_URL=http://voice.ologi.localhost:3005` | One env knob covers dev/prod/staging. |
| D20 | Hotkey default | `right_option` double-tap (inherited from `ptt`). Changeable via the web Settings tab, pulled via `/config` on next boot | Preserves muscle memory for ptt users. |
| D21 | Batch mode | `shift+<hotkey>` modifier recorded in the original ptt engine, preserved in the fork | Matches existing behavior. Not a new feature. |
| D22 | Logging | `~/Library/Logs/ologi-voice.log` when launchd-managed (via plist `StandardOutPath` + `StandardErrorPath`). When run via terminal, logs to stderr as usual | macOS convention. |
| D23 | Versioning | Semantic version baked in at build time via `-ldflags "-X main.version=..."`. `ologi --version` prints it. Used in `POST /api/voice/login/start`'s `cli_version` field for server telemetry | Observability. |
| D24 | Accessibility + mic permission prompts | macOS handles these on first `CGEventPost` + first PortAudio `stream.Start`. `ologi voice run` logs a warning if Accessibility looks denied (via `AXIsProcessTrusted()`); the first-run wizard on the web mentions both | Can't bypass; the prompts are correct behavior. |
| D25 | Fail-closed on auth errors | If `/config` returns 401 at startup, print `ologi: unauthenticated — run 'ologi login'` and exit 1. If a `/sessions` POST returns 401 mid-run, log the error and keep running (the transcript just didn't persist — dictation still typed into the focused app) | Better than crashing. User notices the dashboard is stale and re-authenticates. |
| D26 | Concurrency — one daemon per user | `ologi voice start` refuses if the launchd service is already loaded. `ologi voice run` (foreground) refuses if the launchd service is loaded (to avoid two copies fighting for the mic). | Simplest correct behavior. |

---

## Non-goals (v1 of the CLI)

- **No TUI.** `ologi` with no args prints usage, not an interactive screen. TUI lands in Plan C.
- **No `ologi plugin install` / plugin management.** Plan C.
- **No `ologi tasks ...`.** Plan D; until then, the existing Node CLI in `packages/cli/` coexists.
- **No Windows / Linux.** macOS only. Voice capture is CoreAudio-PortAudio-specific and the typewriter is CGEvent-specific. Cross-platform is its own project.
- **No in-CLI settings editor.** Settings are changed via the web Settings tab. CLI just consumes them.
- **No in-CLI history browser.** History lives on `voice.ologi.app/#history`.
- **No `stats` subcommand beyond a one-liner `ologi status` health check.** Stats UI lives on the web.
- **No self-update.** `brew upgrade ologi` handles that.
- **No Keychain storage of the API key.** Mode-0600 TOML for v1; Keychain is a later plan.
- **No offline mode / transcript queuing.** If the server is unreachable, the session POST fails; the typewriter still typed the text. The user loses the history row for that session. Acceptable v1 failure mode.
- **No multi-turn conversation transcripts.** One session = one transcript, matching Plan A's schema.
- **No arbitrary speech-model configuration.** `en` → `universal-streaming-english`; anything else → `universal-streaming-multilingual`. Same as existing ptt.
- **No local session logs.** `~/.config/ptt/history.jsonl` is NOT recreated. The server is the only source of truth.

---

## Architecture

### Runtime topology

```
┌──────────────────────────────────────────────────────────────────────┐
│ macOS user's laptop                                                  │
│                                                                      │
│  ┌──────────────────────────┐    ┌─────────────────────────────┐     │
│  │ launchctl (launchd)      │    │ ~/.config/ologi/config.toml │     │
│  │ app.ologi.voice plist    │    │ api_key, device_name/id      │     │
│  │   ↓ invokes on demand    │    └─────────────────────────────┘     │
│  │   /usr/local/bin/ologi   │                                        │
│  │     voice run            │                                        │
│  └──────────┬───────────────┘                                        │
│             │                                                        │
│             ▼                                                        │
│  ┌──────────────────────────┐      ┌─────────────────────────────┐   │
│  │ ologi voice run (proc)   │◄─────┤ keylistener (CGEvent tap)   │   │
│  │  engine event loop       │      └─────────────────────────────┘   │
│  │   ┌──────────────────┐   │      ┌─────────────────────────────┐   │
│  │   │ audio (portaudio)│───┼─────►│ aai (WS client)             │   │
│  │   └──────────────────┘   │      │  mints realtime token       │   │
│  │   ┌──────────────────┐   │      │  via POST /realtime-token   │   │
│  │   │ typewriter       │◄──┼──────┤  streams to AAI             │   │
│  │   │ (CGEventPost)    │   │      └──────────┬──────────────────┘   │
│  │   └──────────────────┘   │                 │                      │
│  │   ┌──────────────────┐   │                 │                      │
│  │   │ sourceapp        │   │                 │                      │
│  │   │ (NSWorkspace)    │   │                 │                      │
│  │   └──────────────────┘   │                 │                      │
│  └──────────────────────────┘                 │                      │
│             │                                  │                      │
│             │                                  │                      │
│             │                                  │                      │
└─────────────┼──────────────────────────────────┼─────────────────────-┘
              │                                  │
              │ HTTPS (Bearer ht_*)              │ WSS
              ▼                                  ▼
┌──────────────────────────────────┐   ┌─────────────────────────────┐
│ voice.ologi.app                  │   │ streaming.assemblyai.com    │
│ /api/voice/{config,              │   │ (scoped 60s token)          │
│              realtime-token,     │   └─────────────────────────────┘
│              sessions,           │
│              login/*,            │
│              devices/*}          │
└──────────────────────────────────┘
```

Two external connections per dictation:
1. HTTPS to `voice.ologi.app` — one request to mint the realtime token; one request at the end to POST the session.
2. WSS to `streaming.assemblyai.com` — continuous audio stream during the dictation.

No audio ever touches our server.

### Module layout

```
packages/ologi-cli/
├── cmd/
│   └── ologi/
│       └── main.go                  ← entry point, subcommand router
├── internal/
│   ├── api/                         ← Ologi HTTP client
│   │   ├── client.go                ← base, auth, errors
│   │   ├── login.go                 ← start / poll-complete / logout
│   │   ├── config.go                ← GET /config
│   │   ├── token.go                 ← POST /realtime-token
│   │   └── sessions.go              ← POST /sessions
│   ├── audio/                       ← forked from stt/audio.go
│   ├── keylistener/                 ← forked from stt/keylistener_darwin.go
│   ├── sound/                       ← forked from stt/sound_darwin.go
│   ├── typewriter/                  ← forked from stt/typewriter_darwin.go
│   ├── aai/                         ← forked from stt/transcribe.go (streaming)
│   │   ├── streaming.go             ← WS client, now takes scoped token
│   │   └── batch.go                 ← forked from stt/batch.go; optional for shift+hotkey
│   ├── engine/                      ← forked from stt/engine.go, trimmed
│   │   └── engine.go                ← event loop; hooks into api.PostSession
│   ├── sourceapp/                   ← NEW — frontmost-app detection
│   │   ├── sourceapp_darwin.go      ← CGo: NSWorkspace frontmostApplication
│   │   └── browser_tab_darwin.go    ← AppleScript helper for browser URLs
│   ├── launchd/                     ← NEW — plist install + launchctl wrappers
│   │   ├── plist.go                 ← renders plist from template
│   │   └── control.go               ← bootstrap / bootout / kickstart / print
│   └── config/                      ← NEW — ~/.config/ologi/config.toml
│       ├── load.go
│       └── save.go
├── Formula/                         ← lives in the SEPARATE ologi/homebrew-tap repo,
│                                        documented here for reference
│   └── ologi.rb
├── .github/workflows/release.yml    ← IN THIS MONOREPO; cuts releases on tag
├── go.mod
├── go.sum
└── README.md
```

`cmd/ologi/main.go` is a thin dispatcher. Subcommands are implemented as files next to it (`cmd_login.go`, `cmd_voice.go`, `cmd_status.go`) or as methods on the CLI struct — details in the plan.

### launchd integration

The LaunchAgent plist is templated and written on first `ologi voice start`. Content:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key><string>app.ologi.voice</string>
    <key>ProgramArguments</key>
    <array>
      <string>/opt/homebrew/bin/ologi</string>
      <string>voice</string>
      <string>run</string>
    </array>
    <key>RunAtLoad</key><{{if .Autostart}}true{{else}}false{{end}}/>
    <key>KeepAlive</key>
    <dict>
      <key>Crashed</key><true/>
      <key>SuccessfulExit</key><false/>
    </dict>
    <key>StandardOutPath</key><string>{{.HomeDir}}/Library/Logs/ologi-voice.log</string>
    <key>StandardErrorPath</key><string>{{.HomeDir}}/Library/Logs/ologi-voice.log</string>
    <key>EnvironmentVariables</key>
    <dict>
      {{range $k, $v := .Env}}<key>{{$k}}</key><string>{{$v}}</string>{{end}}
    </dict>
  </dict>
</plist>
```

Template fields:
- `HomeDir` — `os.UserHomeDir()`. Can't use `~` in a plist.
- `Autostart` — whether `RunAtLoad` is true. Toggled by `ologi voice autostart on|off`.
- `Env` — any env vars the daemon needs (primarily `OLOGI_SERVER_URL` for dev builds; empty in prod).

Control commands:

```
ologi voice start
    = write plist if missing
    + launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/app.ologi.voice.plist
    + (if already loaded) launchctl kickstart -k gui/$(id -u)/app.ologi.voice

ologi voice stop
    = launchctl bootout gui/$(id -u)/app.ologi.voice

ologi voice autostart on
    = rewrite plist with RunAtLoad=true
    + if currently loaded, bootout + bootstrap to pick up the new plist

ologi voice autostart off
    = rewrite plist with RunAtLoad=false
    + bootout + bootstrap

ologi voice status
    = launchctl print gui/$(id -u)/app.ologi.voice → parse running/stopped/last-exit
```

The binary path in `ProgramArguments` is derived at plist-render time via `os.Executable()`, not hardcoded. So Homebrew's prefix (arm64: `/opt/homebrew/bin`, amd64: `/usr/local/bin`) is correct.

### Login flow (device code)

```
ologi login
    ├─ prompt user for a friendly device name (default: `hostname` output)
    ├─ POST /api/voice/login/start {device_name, platform:"darwin", cli_version}
    │   ← {device_code:"XK3NQ9WR", verification_url:"https://ologi.app/voice/link?code=XK3NQ9WR", interval_ms:2000}
    ├─ print the code + URL to stderr
    ├─ `open <verification_url>` (macOS default-browser launch)
    └─ poll POST /api/voice/login/complete {device_code} every 2s:
          status:"pending"  → keep polling, show a spinner
          status:"denied"   → print "denied by user" and exit 2
          status:"expired"  → print "code expired, try again" and exit 2
          status:"ok"       → persist {api_key, device_id, device_name} to
                                ~/.config/ologi/config.toml at mode 0600, print
                                "linked as <device_name>" and exit 0
```

The `verification_url` is rendered by the server pointing at apex (`ologi.app` or in dev the detected host) + `/voice/link?code=...`. The `/voice/link` page (Plan A) shows Approve/Deny buttons, which POST to `/api/voice/login/approve|deny`, which flip `voice_device_codes.status`.

Timeouts:
- Server-side: code TTL = 10 min (Plan A).
- Client-side: poll for at most 10 min, then give up with `exit 2` and a helpful message.
- Spinner shows elapsed/remaining.

### Token mint + AAI streaming flow

On each hotkey press:

```
engine detects key-down
    → api.MintRealtimeToken()  // POST /api/voice/realtime-token → {token}
    → aai.NewStreamingClient(token, sampleRate, language, onPartial, onFinal)
        // Authorization: Bearer <token>
        // wss://streaming.assemblyai.com/v3/ws?sample_rate=16000&speech_model=universal-streaming-english
    → audio.Start() with WS as the writer sink
```

The only diff from the existing `ptt/transcribe.go` is the auth header — uses the scoped `Bearer <token>` instead of the user's full API key. Once connected, message handling is unchanged: `Begin` / `Turn` / `Termination`.

On key-up:

```
audio.Stop()
→ aai.Stop() (sends {"type":"Terminate"}, waits for Termination, closes WS)
→ text = replacements.Apply(committed + partial)    // local preview applied
→ typewriter.Type(text)                             // CGEventPost keystrokes
→ api.PostSession(session)                          // server stores canonical text
    ← {session_id, canonical_text, settings_version}
→ if settings_version > cached_settings_version:
      go api.FetchConfig()  // refresh replacements, hotkey, language, etc.
```

Note: the typewriter types the locally-predicted text (after applying the client-side cached replacements) so there's no visible delay. The server independently applies the *canonical* replacements to the same raw text — if the client and server agree, both produce the same string, and the history row matches what the user saw typed. If they disagree (e.g., the client's replacement cache is stale), the server's version is authoritative in the history; the already-typed text is whatever the user sees. Both cases are fine for v1.

### Session upload payload

```jsonc
POST /api/voice/sessions
Authorization: Bearer ht_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
Content-Type: application/json

{
  "mode": "stream",
  "started_at": "2026-04-21T14:42:00.123Z",
  "ended_at":   "2026-04-21T14:42:14.507Z",
  "duration_ms": 14384,
  "source_app": "Google Chrome / claude.ai",
  "text": "Rewrite the intro so it leads with the outcome, not the mechanic."
}
```

`source_app` comes from the `sourceapp` package:

1. On recording start, `NSWorkspace.sharedWorkspace.frontmostApplication` → `localizedName`. Call this `appName`.
2. If `bundleIdentifier` matches a known browser (`com.google.Chrome`, `com.apple.Safari`, `org.mozilla.firefox`, `company.thebrowser.Browser` (Arc)), run a 200ms-timeout AppleScript to read the active tab's URL. Extract the host. Format as `"<appName> / <host>"`.
3. On any timeout/error, fall back to just `appName`.
4. On full failure, send `null`.

AppleScript example for Chrome:
```
tell application "Google Chrome"
  if (count of windows) > 0 then return URL of active tab of front window
end tell
```

Each browser gets a one-liner; ~5 lines of scripts total.

### Error handling

- **No config file / no API key** on `ologi voice run` startup: `ologi: unauthenticated — run 'ologi login'`; exit 1.
- **`GET /config` returns 401**: same as above; exit 1 (config file exists but key revoked).
- **`POST /realtime-token` fails**: log the error, play the stop-sound, stay in idle state. Next hotkey press retries. No retry loop in-session.
- **AAI WS connection fails mid-session**: engine aborts the session; audio.Stop; no POST to `/sessions`; log warning. The user's keystroke press is "cancelled."
- **`POST /sessions` fails**: log the error with the text body (so it's in the log file for potential recovery); typewriter already typed it. User sees a warning via stderr, but voice still typed.
- **launchctl bootstrap fails**: print the error verbatim and exit 3 with a hint ("check Accessibility permission in System Settings").

---

## CLI subcommand surface (v1)

All commands output terse human-readable output. `--json` flag adds machine-readable output where useful. Exit codes: `0` success, `1` user error, `2` auth or device-code flow failure, `3` platform error (launchd, filesystem, permissions).

```
ologi login
    Interactive login. Prompts for a device name (default: hostname). Opens the
    approval URL in the default browser and polls until approved, denied, or
    expired. Writes ~/.config/ologi/config.toml on success.

ologi logout
    Revokes the API key server-side (DELETE /api/voice/devices/:id), unloads
    the launchd plist if present, removes ~/.config/ologi/config.toml.

ologi status
ologi status --json
    Prints a one-line health summary:
        account: brentryczak@gmail.com (brent-mbp)
        voice:   running (since 10:42 am, 1h 14m)
    --json emits {"account": {...}, "voice": {...}} for scripting.

ologi --version
    Prints `ologi <semver> (<git-sha>)`.

ologi voice run
    The blocking foreground listener. Invoked by launchd; usable from a
    terminal for debugging. Captures audio, streams to AssemblyAI, types the
    transcript. Refuses to run if the launchd daemon is already loaded.

ologi voice start
    Writes/updates the plist, launchctl bootstrap. Runs the daemon in the
    background; returns immediately. If already running, kickstart.

ologi voice stop
    launchctl bootout. Returns when the daemon has exited.

ologi voice autostart on
ologi voice autostart off
    Toggles RunAtLoad in the plist. If currently loaded, bootout + bootstrap to
    apply.

ologi voice status
ologi voice status --json
    Prints the launchctl print output summary — loaded? running? last exit
    code? --json emits a structured version.
```

Explicit non-commands (won't exist in v1):
- `ologi voice login` — top-level `ologi login` serves all subsystems.
- `ologi voice record-key` — changing the hotkey is a web-Settings concern.
- `ologi voice test-mic` — debugging is a v1.1 follow-up if needed.
- `ologi voice history` — history is on the web.
- Bare `ologi` — prints usage; no TUI in v1.

---

## Homebrew tap

Separate repo: `github.com/ologi/homebrew-tap` (private or public, TBD; private is fine since Homebrew supports private taps).

Formula (`Formula/ologi.rb`):

```ruby
class Ologi < Formula
  desc "Ologi — talk your way through your AI conversations"
  homepage "https://ologi.app/voice"
  version "0.1.0"

  depends_on "portaudio"
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/ologi/ologi-cli/releases/download/v0.1.0/ologi-0.1.0-darwin-arm64.tar.gz"
      sha256 "<filled-by-CI>"
    end
    on_intel do
      url "https://github.com/ologi/ologi-cli/releases/download/v0.1.0/ologi-0.1.0-darwin-amd64.tar.gz"
      sha256 "<filled-by-CI>"
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

Install flow:

```
brew install ologi-app/tap/ologi
    = brew tap ologi/tap
    + brew install ologi
```

Updates:

```
brew upgrade ologi
```

### GitHub Actions release workflow

`.github/workflows/release.yml` in the hypertask monorepo (paths: only fires when `packages/ologi-cli/**` changes, OR when a `voice-cli-v*` tag is pushed).

On tag push:
1. Run Go tests (`go test ./...` inside `packages/ologi-cli/`).
2. Cross-compile for `darwin/arm64` and `darwin/amd64`:
   ```
   CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build \
     -ldflags "-X main.version=${TAG}" \
     -o ologi-arm64 ./cmd/ologi
   ```
   Note: `CGO_ENABLED=1` because PortAudio + CoreGraphics require cgo. The Actions runner `macos-14` (arm64) natively builds arm64; `macos-13` (intel) natively builds amd64. Use a matrix job.
3. Sign each binary with `codesign -s "Developer ID Application: <team>" --options runtime ...` using the `$APPLE_DEVELOPER_ID_CERT` secret.
4. Notarize each: zip → `xcrun notarytool submit --wait` with `$APPLE_NOTARY_PROFILE` secret → staple.
5. Tar.gz each: `ologi-<version>-darwin-<arch>.tar.gz` containing just the binary.
6. `gh release create v<version> --generate-notes` with both tarballs attached.
7. Checkout the tap repo, bump `Formula/ologi.rb` with the new version + SHAs, commit, push.

GitHub secrets required:
- `APPLE_DEVELOPER_ID_CERT` — base64-encoded .p12 cert + private key.
- `APPLE_DEVELOPER_ID_CERT_PASSWORD` — the .p12 password.
- `APPLE_NOTARY_KEY_ID`, `APPLE_NOTARY_ISSUER`, `APPLE_NOTARY_KEY_BASE64` — App Store Connect API key for notarization.
- `TAP_REPO_TOKEN` — fine-grained PAT with push to `ologi/homebrew-tap`.

---

## Testing

### Go unit tests

`go test ./...` inside `packages/ologi-cli/`. No external services.

- `internal/api/*`: HTTP client tests with a stub server (`httptest.NewServer`). Cover auth headers, JSON encoding, error paths, retry behaviors, 401 handling.
- `internal/config/*`: round-trip save/load; ensure 0600 perms are set; ensure no secrets in error messages.
- `internal/launchd/*`: plist rendering from template (snapshot test); exec `launchctl` calls are mocked via an interface.
- `internal/engine/*`: state machine tests with stubbed audio/AAI/typewriter/API collaborators.
- `internal/sourceapp/*`: hard to unit-test (CGo calls into macOS APIs); rely on manual smoke.

### Manual smoke (macOS required)

Developer runs through this once per release candidate:

1. `ologi --version` prints the expected version.
2. `ologi login` against a dev server (`OLOGI_SERVER_URL=http://voice.ologi.localhost:3005`). Browser pops open, Approve, CLI reports "linked." `~/.config/ologi/config.toml` created at mode 0600.
3. `ologi voice run` (foreground). Double-tap right_option in any app, speak, release. Confirm:
   - Text appears in the focused app.
   - A session row appears on `voice.ologi.app/#history` within ~2s.
4. `Ctrl-C` exits cleanly.
5. `ologi voice start`. Daemon is running under launchd; `ologi voice status` shows `running`. Repeat step 3; still works.
6. `ologi voice stop`. Daemon exits.
7. `ologi voice autostart on`. Reboot. Daemon is running after login.
8. `ologi logout`. Device row removed from dashboard; plist gone; config.toml gone.
9. `ologi voice run` now prints the auth error and exits 1.

### CI smoke (GitHub Actions)

On every push to `packages/ologi-cli/**`:
- Run `go vet`, `go test`, `go build` for darwin/arm64.
- Do NOT try to run the binary in Actions (no mic, no Accessibility, no launchd access).

### E2E with the shipped web API

Out of scope for v1. The web dashboard (Plan A) already lets us see whether the loop works. A future `ologi voice test` subcommand could do a golden-path E2E, but we skip it for MVP.

---

## Rollout

Two distinct deliverables — both required for a first release:

1. **The `packages/ologi-cli/` Go package** in this monorepo.
2. **The `ologi/homebrew-tap` repo** with the formula.
3. **CI release workflow** in the monorepo that cuts releases on tag push.

Plus the Apple Developer signing setup (one-time, adjacent to this work).

Order of PRs (see the plan for task-level detail):

1. PR 1 — Go package scaffolding + forked engine (no API client yet; `ologi voice run` works against a hardcoded AAI key for smoke).
2. PR 2 — `internal/api` + `internal/config` + `ologi login` / `ologi logout` / `ologi status`.
3. PR 3 — Wire the engine to the API. `ologi voice run` goes through the full login → token → session flow.
4. PR 4 — `internal/launchd` + `ologi voice start` / `stop` / `autostart` / `status`.
5. PR 5 — `internal/sourceapp` + source-app attribution in session POSTs.
6. PR 6 — Release workflow + tap repo + first signed release (`voice-cli-v0.1.0`).

Each PR ships individually and leaves `main` green.

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| AssemblyAI realtime-token authentication differs slightly from the direct-key API (different headers, different handshake) | Plan A's `/realtime-token` endpoint is trivial to test: `curl -X POST $URL -H "Authorization: Bearer <ht key>"`. Before touching CLI code, verify a freshly-minted token dials the AAI WS correctly using a 10-line Go script. |
| CGEvent requires Accessibility permission; first-run UX is ugly | Detect via `AXIsProcessTrusted()`, log a friendly message pointing to System Settings → Privacy & Security → Accessibility. Document in the first-run wizard (Plan A already mentions this). |
| PortAudio CGo link needs the `portaudio` shared lib on the build machine + in signing metadata | GitHub Actions macos-14 / macos-13 runners have Homebrew preinstalled; `brew install portaudio` in the CI job covers builds. The binary dynamically links `libportaudio`; users install via `brew install ologi` which depends on it via the formula's `depends_on "portaudio"` line. |
| launchctl interface varies between macOS versions (old: `load`, new: `bootstrap`) | Target macOS 11+ (Big Sur, 2020). All users on 11+ have `bootstrap/bootout` available. Check version via `sw_vers -productVersion` on startup if we suspect drift. |
| launchd plist breakage from path/permission issues | `ologi voice start` validates the plist renders, writes atomically, then attempts bootstrap. If bootstrap fails, back out: delete plist + print the error. Never leave the user with a broken plist. |
| Apple Developer signing setup is a one-time ~half-day cost | Noted in non-goals as "orthogonal"; the code work proceeds without it. Unsigned binaries still work for the developer themselves via `xattr -d com.apple.quarantine`. |
| Source-app AppleScript timing / permissions | AppleScript calls into other apps can themselves prompt for Automation permission on first use. The `sourceapp` package wraps every call in a 200ms timeout and a recover(); worst case we emit `null` source_app. Non-fatal. |
| User logs out from the web (revokes the api_key via /devices/:id DELETE) while the CLI daemon is running | On the next `/sessions` POST the daemon gets a 401. Log the error, keep running (typewriter still types). User sees history isn't persisting and re-logs-in from the terminal. Acceptable. |
| `brew upgrade ologi` while the daemon is running replaces the binary; launchd's loaded process is still the old one | Standard macOS behavior. `launchctl kickstart -k gui/$(id -u)/app.ologi.voice` (which `ologi voice start` does on every call) restarts the daemon with the new binary. Tell users to `ologi voice start` after upgrade. |
| Concurrent `ologi voice run` instances fighting over the mic | `ologi voice run` checks if launchd has `app.ologi.voice` loaded; refuses with a helpful error if so. Two foreground runs are still possible but it's clearly a user error; both will log AAI/mic errors. |

---

## Open questions

All answerable without blocking — notes for follow-ups:

1. **Keychain vs TOML for the API key.** Plan B ships TOML at 0600. v1.1 could migrate to Keychain for best-practice.
2. **Multiple accounts per machine.** Plan B supports one at a time. `ologi login --profile <name>` could select among profiles later; not in v1.
3. **Crash telemetry.** Plan B logs to `~/Library/Logs/ologi-voice.log`. Sending crash reports up to the server is a later plan; don't add surveillance in v1.
4. **Server-side AAI provider swap.** Plan A's `/realtime-token` endpoint abstracts this; the CLI is provider-agnostic. Changing providers requires zero CLI changes.
5. **Public homebrew-core tap.** Private tap is v1. Submitting to homebrew-core requires the project to be "notable" per their guidelines; revisit when Ologi has meaningful adoption.
6. **Auto-update from within the binary.** Could add `ologi update` that does `brew upgrade ologi` on the user's behalf. Not v1.
7. **Test fixture for the AAI streaming handshake.** Hard to mock faithfully; deferring to manual smoke for v1.

---

## References

- `docs/superpowers/specs/2026-04-21-ologi-voice-design.md` — the parent product spec.
- `docs/superpowers/plans/2026-04-21-ologi-voice-web.md` — Plan A, shipped.
- `~/Documents/stt/` — the existing `ptt` Go app; source for the engine fork.
- `packages/cli/` — existing Node tasks CLI; will eventually be ported in Plan D.
- `tools/hypertask-local/` — Claude Code plugin; will be embedded + installable in Plan C.
- Apple's `launchd.plist(5)` man page — LaunchAgent reference.
- AssemblyAI Universal Streaming v3 docs — streaming contract.
- `apps/web/src/app/api/voice/realtime-token/route.ts` — server-side token mint.
- `apps/web/src/app/api/voice/sessions/route.ts` — server-side session finalizer.
- `apps/web/src/app/api/voice/login/*` — device-code login endpoints.
- `apps/web/src/app/(dashboard)/voice/link/page.tsx` — browser-side approval UI.
