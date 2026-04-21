package aai

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const assemblyAIURL = "wss://streaming.assemblyai.com/v3/ws"

type TranscriptHandler func(text string, isFinal bool)

type StreamingClient struct {
	conn     *websocket.Conn
	handler  TranscriptHandler
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	writeMu  sync.Mutex
	lastText string
	waiter   *formatWaiter
}

type aaiMessage struct {
	Type            string `json:"type"`
	Transcript      string `json:"transcript"`
	EndOfTurn       bool   `json:"end_of_turn"`
	TurnIsFormatted bool   `json:"turn_is_formatted"`
}

func NewStreamingClient(token, language string, sampleRate int, handler TranscriptHandler) (*StreamingClient, error) {
	model := "universal-streaming-english"
	if language != "" && language != "en" {
		model = "universal-streaming-multilingual"
	}

	url := fmt.Sprintf("%s?sample_rate=%d&speech_model=%s&format_turns=true&token=%s", assemblyAIURL, sampleRate, model, token)

	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("connecting to assemblyai (status %d): %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("connecting to assemblyai: %w", err)
	}

	c := &StreamingClient{
		conn:    conn,
		handler: handler,
		done:    make(chan struct{}),
		waiter:  newFormatWaiter(),
	}
	go c.readLoop()
	return c, nil
}

func (c *StreamingClient) readLoop() {
	defer close(c.done)
	defer c.waiter.MarkReceived()
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[assemblyai] read error: %v", err)
			}
			return
		}

		var msg aaiMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "Begin":
			log.Println("[assemblyai] session started")
		case "Turn":
			if msg.Transcript == "" {
				continue
			}
			c.mu.Lock()
			c.lastText = msg.Transcript
			c.mu.Unlock()

			switch {
			case !msg.EndOfTurn:
				// Streaming partial — unchanged behavior.
				c.handler(msg.Transcript, false)
			case !msg.TurnIsFormatted:
				// Unformatted end-of-turn. Emit as partial so the live
				// preview stays snappy; the formatted version will commit.
				c.waiter.MarkPending()
				c.handler(msg.Transcript, false)
			default:
				// Formatted end-of-turn — this is what we commit.
				// MarkReceived fires AFTER the handler so any goroutine
				// waiting on WaitForFormattedTurn only unblocks once the
				// committed text has been updated.
				c.handler(msg.Transcript, true)
				c.mu.Lock()
				c.lastText = ""
				c.mu.Unlock()
				c.waiter.MarkReceived()
			}
		case "Termination":
			log.Println("[assemblyai] session terminated")
			return
		}
	}
}

// WaitForFormattedTurn blocks until any pending unformatted end-of-turn
// has received its formatted counterpart, or timeout elapses. Returns
// immediately if no turn is awaiting formatting.
func (c *StreamingClient) WaitForFormattedTurn(timeout time.Duration) {
	c.waiter.Wait(timeout)
}


func (c *StreamingClient) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	err := c.conn.WriteMessage(websocket.BinaryMessage, p)
	c.writeMu.Unlock()
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// formatWaiter coordinates waiting for a formatted turn message to arrive
// after its unformatted counterpart. One-shot semantics: MarkPending opens
// a wait window; MarkReceived closes it and wakes any waiter.
type formatWaiter struct {
	mu      sync.Mutex
	pending chan struct{}
}

func newFormatWaiter() *formatWaiter {
	return &formatWaiter{}
}

// MarkPending indicates we are now awaiting a formatted turn.
// Idempotent: calling twice without an intervening MarkReceived is a no-op.
func (w *formatWaiter) MarkPending() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending == nil {
		w.pending = make(chan struct{})
	}
}

// MarkReceived signals that the formatted turn arrived. Safe to call when
// nothing is pending.
func (w *formatWaiter) MarkReceived() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending != nil {
		close(w.pending)
		w.pending = nil
	}
}

// Wait blocks until MarkReceived is called or timeout elapses. Returns
// immediately if nothing is pending.
func (w *formatWaiter) Wait(timeout time.Duration) {
	w.mu.Lock()
	ch := w.pending
	w.mu.Unlock()
	if ch == nil {
		return
	}
	// ch is captured under the lock; MarkReceived only closes it (never
	// replaces a non-nil channel), so reading ch here without the lock is safe.
	select {
	case <-ch:
	case <-time.After(timeout):
	}
}

func (c *StreamingClient) Stop() {
	c.once.Do(func() {
		// Don't Flush — the engine captures committed+partial on stop
		c.writeMu.Lock()
		_ = c.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"Terminate"}`))
		c.writeMu.Unlock()
		select {
		case <-c.done:
		case <-time.After(3 * time.Second):
		}
		c.conn.Close()
	})
}
