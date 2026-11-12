package kill

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

// OutcomeStore keeps the first outcome of every kill ID.
type OutcomeStore interface {
	Lookup(id string) (protocol.Outcome, bool)
	// Save keeps o for id. It keeps o in memory even when it returns an error, so the same id
	// gets o again until the agent restarts.
	Save(id string, o protocol.Outcome, at time.Time) error
}

// FileStore is an OutcomeStore backed by an append-only JSON Lines file
// (/var/lib/killy/outcomes.jsonl in production). Records never expire. It is safe for concurrent use.
type FileStore struct {
	mu   sync.Mutex
	mem  map[string]protocol.Outcome
	file appendFile
	// lineOpen is set when the file ends in a record cut off mid-write, so the next record starts
	// on a new line instead of being glued onto the broken one.
	lineOpen bool
}

type appendFile interface {
	Write(b []byte) (int, error)
	Sync() error
	Close() error
}

type record struct {
	ID      string    `json:"id"`
	Outcome string    `json:"outcome"`
	Reason  string    `json:"reason,omitempty"`
	Reaped  int       `json:"reaped,omitempty"`
	At      time.Time `json:"at"`
}

// OpenFileStore loads the records at path, or starts empty when the file does not exist. Lines
// that are not valid records are skipped: they are what a write cut off by a crash or a power loss
// leaves. It fails when the file exists but cannot be read.
func OpenFileStore(path string) (*FileStore, error) {
	s := &FileStore{mem: map[string]protocol.Outcome{}}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("read kill outcomes: %w", err)
	default:
		s.load(data)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create the directory for kill outcomes: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open kill outcomes: %w", err)
	}
	s.file = f
	return s, nil
}

func (s *FileStore) load(data []byte) {
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		var r record
		if json.Unmarshal(line, &r) != nil || r.ID == "" {
			continue
		}
		if _, seen := s.mem[r.ID]; !seen {
			s.mem[r.ID] = protocol.Outcome{Outcome: r.Outcome, Reason: r.Reason, Reaped: r.Reaped}
		}
	}
	s.lineOpen = len(data) > 0 && data[len(data)-1] != '\n'
}

func (s *FileStore) Lookup(id string) (protocol.Outcome, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.mem[id]
	return o, ok
}

// Save appends one record and syncs it to disk before returning, so the outcome survives a crash
// right after it is reported. The write is tried once.
func (s *FileStore) Save(id string, o protocol.Outcome, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mem[id] = o
	line, err := json.Marshal(record{ID: id, Outcome: o.Outcome, Reason: o.Reason, Reaped: o.Reaped, At: at.UTC()})
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if s.lineOpen {
		line = append([]byte{'\n'}, line...)
	}
	// One Write call per record, so with O_APPEND a record is never interleaved with another.
	if _, err := s.file.Write(line); err != nil {
		// Part of the line may have been written.
		s.lineOpen = true
		return fmt.Errorf("write kill outcome: %w", err)
	}
	s.lineOpen = false
	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("sync kill outcomes: %w", err)
	}
	return nil
}

func (s *FileStore) Close() error {
	return s.file.Close()
}
