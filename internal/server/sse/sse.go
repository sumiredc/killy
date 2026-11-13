// Package sse writes server-sent event streams and tells open streams when their state changed.
package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HeartbeatInterval is how often a stream sends ": hb", so Cloudflare and the ingress do not close
// an idle stream.
const HeartbeatInterval = 20 * time.Second

// Writer writes one event stream. It is not safe for concurrent use.
type Writer struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

// Start sends the stream's headers and lifts the server's write deadline for this response, which
// lives for as long as the client stays connected.
func Start(w http.ResponseWriter) (*Writer, error) {
	rc := http.NewResponseController(w)
	// A response writer without deadlines (such as a test recorder) has nothing to lift.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return nil, err
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	sw := &Writer{w: w, rc: rc}
	return sw, sw.flush()
}

// Event writes one event with data encoded as JSON, and flushes it.
func (w *Writer) Event(name string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.w, "event: %s\ndata: %s\n\n", name, b); err != nil {
		return err
	}
	return w.flush()
}

// Heartbeat writes the ": hb" comment and flushes it.
func (w *Writer) Heartbeat() error {
	if _, err := w.w.Write([]byte(": hb\n\n")); err != nil {
		return err
	}
	return w.flush()
}

func (w *Writer) flush() error {
	if err := w.rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	return nil
}

// Broker tells the open streams of a device that its state changed. It is safe for concurrent use.
type Broker struct {
	mu   sync.Mutex
	subs map[string]map[chan struct{}]bool
}

func NewBroker() *Broker {
	return &Broker{subs: map[string]map[chan struct{}]bool{}}
}

// Subscribe returns a channel that receives a value after any Notify for the device. Several
// notifications before the stream reads are merged into one, since the stream sends the whole
// state anyway. cancel must be called when the stream ends.
func (b *Broker) Subscribe(deviceID string) (changed <-chan struct{}, cancel func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	if b.subs[deviceID] == nil {
		b.subs[deviceID] = map[chan struct{}]bool{}
	}
	b.subs[deviceID][ch] = true
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subs[deviceID], ch)
		if len(b.subs[deviceID]) == 0 {
			delete(b.subs, deviceID)
		}
	}
}

// Notify tells every open stream of the given devices to send its state again. It never blocks.
func (b *Broker) Notify(deviceIDs ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range deviceIDs {
		for ch := range b.subs[id] {
			select {
			case ch <- struct{}{}:
			default:
				// A notification is already waiting; the stream will send the latest state then.
			}
		}
	}
}

// Subscribers returns how many streams of the device are open.
func (b *Broker) Subscribers(deviceID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs[deviceID])
}
