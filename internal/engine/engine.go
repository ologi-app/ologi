package engine

import (
	"log"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/ologi/hypertask-cli/internal/aai"
	"github.com/ologi/hypertask-cli/internal/audio"
	"github.com/ologi/hypertask-cli/internal/keylistener"
	"github.com/ologi/hypertask-cli/internal/sound"
	"github.com/ologi/hypertask-cli/internal/typewriter"
)

type EventType int

const (
	EventStatusChanged EventType = iota
	EventRecordingStarted
	EventRecordingStopped
	EventPartialText
	EventFinalText
	EventError
)

type EngineEvent struct {
	Type      EventType
	Text      string
	Error     error
	State     string // "idle", "recording", "transcribing"
	Mode      string // "stream" or "batch"
	Duration  time.Duration
	Timestamp time.Time
}

// Config is the engine's runtime config — a subset of the full user
// settings, covering only what the engine actually consumes.
type Config struct {
	Hotkey       string
	Language     string
	SampleRate   int
	Device       string
	Channel      int
	Mode         string // "stream" for v1
	StartSound   string
	StopSound    string
	Replacements []ReplacementEntry
}

// ReplacementEntry is one user dictionary row. The engine applies these
// locally AFTER the AAI transcript arrives, BEFORE typing; the server
// independently applies the same entries on POST /sessions.
type ReplacementEntry struct {
	Pattern     string
	Replacement string
}

// ApplyReplacements returns `text` with the dictionary applied.
// Sorts by Pattern alphabetically (deterministic). Case-insensitive.
// Mirrors the Plan A server's applyReplacements behavior.
func (c Config) ApplyReplacements(text string) string {
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
		re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(r.Pattern))
		out = re.ReplaceAllString(out, r.Replacement)
	}
	return out
}

// Session describes a completed dictation, handed to the OnSession callback.
type Session struct {
	Mode       string // "stream" or "batch"
	StartedAt  time.Time
	EndedAt    time.Time
	DurationMs int64
	Text       string // AFTER replacements — what the typewriter typed
}

// OnSessionFunc is invoked once per completed dictation, AFTER typewriter output.
type OnSessionFunc func(s Session)

// TokenSource returns a fresh scoped AAI token. Called on each dictation.
// If nil, the engine uses OLOGI_DEV_AAI_KEY env var as a dev fallback.
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

func (e *Engine) Events() <-chan EngineEvent {
	return e.events
}

func (e *Engine) Config() Config {
	return e.cfg
}

func (e *Engine) emit(ev EngineEvent) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	log.Printf("[engine] emit: type=%d state=%q text=%q (chan len=%d)", ev.Type, ev.State, ev.Text, len(e.events))
	select {
	case e.events <- ev:
		log.Printf("[engine] emit: delivered")
	default:
		log.Printf("[engine] emit: DROPPED (channel full)")
	}
}

// Run starts the engine event loop. Blocks until Stop() is called.
// PortAudio must already be initialized before calling Run.
func (e *Engine) Run() {
	defer close(e.doneCh)

	keys, err := keylistener.StartKeyListener(e.cfg.Hotkey)
	if err != nil {
		e.emit(EngineEvent{Type: EventError, Error: err})
		return
	}

	sound.InitSounds(e.cfg.StartSound, e.cfg.StopSound)
	tw := typewriter.NewTypeWriter()

	log.Printf("[engine] ready — double-%s to dictate (stream)", e.cfg.Hotkey)
	e.emit(EngineEvent{Type: EventStatusChanged, State: "idle"})

	var capture *audio.AudioCapture
	var recording bool
	var sessionMode string
	var recordStart time.Time

	var aaiClient *aai.StreamingClient
	var committed string
	var partial string

	var batch *aai.BatchRecorder

	for {
		select {
		case <-e.stopCh:
			if recording {
				capture.Stop()
				if aaiClient != nil {
					aaiClient.Stop()
				}
			}
			return

		case ev := <-keys:
			if ev.Down {
				if recording {
					continue
				}

				sessionMode = "stream"
				if ev.Shift {
					sessionMode = "batch"
				}

				if sessionMode == "batch" {
					log.Printf("[engine] batch mode is not supported in v1; falling back to stream")
					sessionMode = "stream"
				}

				log.Printf("[engine] recording... (mode: %s)", sessionMode)
				recording = true
				recordStart = time.Now()
				sound.PlayStartSound()

				e.emit(EngineEvent{
					Type:  EventRecordingStarted,
					State: "recording",
					Mode:  sessionMode,
				})

				var actualRate int
				capture, actualRate, err = audio.NewAudioCapture(e.cfg.SampleRate, e.cfg.Device, e.cfg.Channel, nil)
				if err != nil {
					log.Printf("[engine] audio capture failed: %v", err)
					e.emit(EngineEvent{Type: EventError, Error: err})
					recording = false
					e.emit(EngineEvent{Type: EventStatusChanged, State: "idle"})
					continue
				}

				if sessionMode == "stream" {
					committed = ""
					partial = ""
					handler := func(text string, isFinal bool) {
						if isFinal {
							committed += text + " "
							partial = ""
						} else {
							partial = text
						}
						// Only emit partials for live display; final is emitted on key release
						e.emit(EngineEvent{Type: EventPartialText, Text: committed + partial, Mode: "stream"})
					}
					// Before opening the WS, get a fresh scoped token.
					token := os.Getenv("OLOGI_DEV_AAI_KEY") // dev fallback
					if e.tokenSource != nil {
						t, tokErr := e.tokenSource()
						if tokErr != nil {
							log.Printf("[engine] token mint failed: %v", tokErr)
							e.emit(EngineEvent{Type: EventError, Error: tokErr})
							capture.Stop()
							capture = nil
							recording = false
							e.emit(EngineEvent{Type: EventStatusChanged, State: "idle"})
							continue
						}
						token = t
					}
					aaiClient, err = aai.NewStreamingClient(token, e.cfg.Language, actualRate, handler)
					if err != nil {
						log.Printf("[engine] assemblyai connect failed: %v", err)
						e.emit(EngineEvent{Type: EventError, Error: err})
						capture.Stop()
						capture = nil
						recording = false
						e.emit(EngineEvent{Type: EventStatusChanged, State: "idle"})
						continue
					}
					capture.SetWriter(aaiClient)
				} else {
					// Unreachable: the batch-mode trap above coerces sessionMode
					// back to "stream". Retained as a compile-time guard so the
					// batch package import stays live and the branch remains
					// wireable if the trap is ever lifted.
					batch = aai.NewBatchRecorder("", e.cfg.Language, actualRate)
					capture.SetWriter(batch)
				}

				if err := capture.Start(); err != nil {
					log.Printf("[engine] audio start failed: %v", err)
					e.emit(EngineEvent{Type: EventError, Error: err})
					capture.Stop()
					if aaiClient != nil {
						aaiClient.Stop()
						aaiClient = nil
					}
					capture = nil
					recording = false
					e.emit(EngineEvent{Type: EventStatusChanged, State: "idle"})
					continue
				}

			} else {
				if !recording {
					continue
				}
				log.Println("[engine] stopped")
				recording = false
				duration := time.Since(recordStart)
				sound.PlayStopSound()

				e.emit(EngineEvent{
					Type:     EventRecordingStopped,
					State:    "transcribing",
					Mode:     sessionMode,
					Duration: duration,
				})

				if sessionMode == "stream" {
					capture.Stop()
					aaiClient.WaitForFormattedTurn(500 * time.Millisecond)
					aaiClient.Stop()
					fullText := e.cfg.ApplyReplacements(committed + partial)
					if fullText != "" {
						tw.Type(fullText)
						sess := Session{
							Mode:       "stream",
							StartedAt:  recordStart,
							EndedAt:    time.Now(),
							DurationMs: duration.Milliseconds(),
							Text:       fullText,
						}
						if e.onSession != nil {
							// Fire-and-forget so a slow HTTP POST doesn't block the engine.
							go e.onSession(sess)
						}
						e.emit(EngineEvent{Type: EventFinalText, Text: fullText, Mode: "stream"})
					}
					committed = ""
					partial = ""
					aaiClient = nil
				}

				e.emit(EngineEvent{Type: EventStatusChanged, State: "idle"})
				capture = nil
				_ = batch
			}
		}
	}
}

func (e *Engine) Stop() {
	select {
	case <-e.stopCh:
	default:
		close(e.stopCh)
	}
	<-e.doneCh
}
