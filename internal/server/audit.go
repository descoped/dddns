package server

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AuditMaxSize is the default rotation threshold for the audit log —
// when the file reaches this size it is renamed to path+".old" before
// the next write appends to a fresh file. Matches the operational log
// policy in scripts/install-on-unifi-os.sh.
const AuditMaxSize int64 = 10 * 1024 * 1024

// AuditEntry is one line of the JSONL audit log. The handler fills in
// the relevant fields for the request it just processed; omitted fields
// are elided from the serialized form.
type AuditEntry struct {
	Timestamp       time.Time `json:"ts"`
	RemoteAddr      string    `json:"remote"`
	Hostname        string    `json:"hostname,omitempty"`
	MyIPClaimed     string    `json:"myip_claimed,omitempty"`
	MyIPVerified    string    `json:"myip_verified,omitempty"`
	AuthOutcome     string    `json:"auth,omitempty"`
	Action          string    `json:"action,omitempty"`
	Route53ChangeID string    `json:"route53_change_id,omitempty"`
	Err             string    `json:"error,omitempty"`
}

// AuditLog is an append-only JSONL writer with size-based rotation. All
// writes are serialized under a mutex; the on-disk append is a single
// os.File.Write on O_APPEND-opened FD, which is atomic for typical audit
// line sizes (well below PIPE_BUF).
type AuditLog struct {
	path    string
	maxSize int64

	mu  sync.Mutex
	now func() time.Time // injectable for tests
}

// NewAuditLog constructs an AuditLog writing to path with the default
// rotation threshold.
func NewAuditLog(path string) *AuditLog {
	return &AuditLog{
		path:    path,
		maxSize: AuditMaxSize,
		now:     time.Now,
	}
}

// Write serializes entry as one JSON line and appends it to the log,
// rotating first if the file has reached the size threshold.
// entry.Timestamp is overwritten with the current time.
func (a *AuditLog) Write(entry AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	entry.Timestamp = a.now()

	if info, err := os.Stat(a.path); err == nil && info.Size() >= a.maxSize {
		// Overwrite any prior .old; one rotation keep is sufficient per the
		// plan (operational log has the same policy).
		_ = os.Rename(a.path, a.path+".old")
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}
