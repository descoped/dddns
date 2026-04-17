package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StatusSnapshot is the last-request summary written to serve-status.json.
// It is consumed by `dddns serve status` (reader added in D1).
type StatusSnapshot struct {
	LastRequestAt   time.Time `json:"last_request_at"`
	LastRemoteAddr  string    `json:"last_remote_addr,omitempty"`
	LastAuthOutcome string    `json:"last_auth_outcome,omitempty"`
	LastAction      string    `json:"last_action,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
}

// StatusWriter overwrites a single JSON file with the outcome of the
// most recent request. Writes are atomic (write-to-temp + rename) so a
// concurrent reader never sees a partially-written file.
type StatusWriter struct {
	path string
	mu   sync.Mutex
}

// NewStatusWriter constructs a StatusWriter that targets the given path.
// The directory must exist; it is not created lazily.
func NewStatusWriter(path string) *StatusWriter {
	return &StatusWriter{path: path}
}

// ReadStatus loads the latest StatusSnapshot from path. An absent file
// or malformed JSON is returned as an error — callers typically print
// the error verbatim (e.g. "status file not found — the server has not
// recorded any request yet").
func ReadStatus(path string) (StatusSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StatusSnapshot{}, fmt.Errorf("read status: %w", err)
	}
	var snap StatusSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return StatusSnapshot{}, fmt.Errorf("parse status: %w", err)
	}
	return snap, nil
}

// Write serializes snap as pretty-printed JSON and replaces the target
// file atomically.
func (s *StatusWriter) Write(snap StatusSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".serve-status-*.tmp")
	if err != nil {
		return fmt.Errorf("create tmp status: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write tmp status: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close tmp status: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename tmp status: %w", err)
	}
	return nil
}
