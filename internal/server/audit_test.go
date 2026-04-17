package server

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuditLog_BasicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	log := NewAuditLog(path)

	err := log.Write(AuditEntry{
		RemoteAddr:  "127.0.0.1:54321",
		Hostname:    "test.example.com",
		AuthOutcome: "ok",
		Action:      "updated",
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got AuditEntry
	if err := json.Unmarshal(raw[:len(raw)-1], &got); err != nil { // strip trailing newline
		t.Fatalf("failed to parse back: %v\nraw: %s", err, raw)
	}
	if got.RemoteAddr != "127.0.0.1:54321" {
		t.Errorf("RemoteAddr mismatch: got %q", got.RemoteAddr)
	}
	if got.Timestamp.IsZero() {
		t.Error("Timestamp should be set by Write")
	}
}

func TestAuditLog_AppendsMultipleLines(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	log := NewAuditLog(path)

	const n = 5
	for i := 0; i < n; i++ {
		if err := log.Write(AuditEntry{RemoteAddr: "127.0.0.1", Action: "nochg-cache"}); err != nil {
			t.Fatal(err)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		var e AuditEntry
		if err := json.Unmarshal(line, &e); err != nil {
			t.Errorf("line %d is not valid JSON: %v\n%s", count, err, line)
		}
		count++
	}
	if count != n {
		t.Errorf("expected %d lines, got %d", n, count)
	}
}

// TestAuditLog_RotatesAtThreshold verifies that when the file exceeds the
// configured size, it is renamed to path+".old" and the next write starts
// a fresh file.
func TestAuditLog_RotatesAtThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	log := NewAuditLog(path)
	log.maxSize = 200 // tiny threshold for the test

	// Write enough lines to exceed the threshold.
	for i := 0; i < 10; i++ {
		if err := log.Write(AuditEntry{
			RemoteAddr: "127.0.0.1:54321",
			Hostname:   "verylongvalue.example.com",
			Action:     "updated",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// An .old file should now exist.
	if _, err := os.Stat(path + ".old"); err != nil {
		t.Errorf("expected rotated file at %s: %v", path+".old", err)
	}

	// And the live file should still be valid JSONL with at least one line.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Error("live file should end with a newline")
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		var e AuditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Errorf("live file has invalid JSON line: %v\n%s", err, line)
		}
	}
}

// TestAuditLog_ConcurrentWritesSerialized verifies that parallel calls
// to Write don't interleave partial lines — every resulting line must
// parse as valid JSON. Validated under go test -race.
func TestAuditLog_ConcurrentWritesSerialized(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	log := NewAuditLog(path)

	const (
		workers      = 10
		perWorker    = 100
		totalWrites  = workers * perWorker
	)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if err := log.Write(AuditEntry{
					RemoteAddr: "127.0.0.1",
					Hostname:   "concurrent.example.com",
					Action:     "nochg-cache",
				}); err != nil {
					t.Errorf("Write failed: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		var e AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("concurrent write produced invalid JSON line %d: %v\n%s", count, err, scanner.Bytes())
		}
		count++
	}
	if count != totalWrites {
		t.Errorf("expected %d lines, got %d", totalWrites, count)
	}
}

// TestAuditLog_TimestampInjectable verifies the `now` hook feeds through
// to every written entry.
func TestAuditLog_TimestampInjectable(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	log := NewAuditLog(path)

	fixed := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	log.now = func() time.Time { return fixed }

	if err := log.Write(AuditEntry{RemoteAddr: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got AuditEntry
	if err := json.Unmarshal(raw[:len(raw)-1], &got); err != nil {
		t.Fatal(err)
	}
	if !got.Timestamp.Equal(fixed) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, fixed)
	}
}
