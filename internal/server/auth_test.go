package server

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testSecret = "super-secret-value"

// fakeClock returns a stubbed now() that tests advance manually.
type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }
func (f *fakeClock) advance(d time.Duration) {
	f.t = f.t.Add(d)
}

func newTestAuth(t *testing.T) (*Authenticator, *fakeClock) {
	t.Helper()
	fc := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	a := NewAuthenticator(testSecret)
	a.now = fc.now
	return a, fc
}

func TestAuth_OK(t *testing.T) {
	a, _ := newTestAuth(t)
	if got := a.Check(testSecret); got != AuthOK {
		t.Errorf("Check(correct) = %v, want AuthOK", got)
	}
}

func TestAuth_BadCredentials(t *testing.T) {
	a, _ := newTestAuth(t)
	if got := a.Check("wrong"); got != AuthBadCredentials {
		t.Errorf("Check(wrong) = %v, want AuthBadCredentials", got)
	}
}

func TestAuth_EmptyPassword(t *testing.T) {
	a, _ := newTestAuth(t)
	if got := a.Check(""); got != AuthBadCredentials {
		t.Errorf("Check(empty) = %v, want AuthBadCredentials", got)
	}
}

// TestAuth_LockoutTriggersAfterThreshold verifies that exactly
// MaxFailuresPerWindow failures inside FailureWindow cause the next
// attempt to return AuthLockedOut, even with the correct password.
func TestAuth_LockoutTriggersAfterThreshold(t *testing.T) {
	a, fc := newTestAuth(t)

	for i := 0; i < MaxFailuresPerWindow; i++ {
		if got := a.Check("wrong"); got != AuthBadCredentials {
			t.Fatalf("failure %d: got %v, want AuthBadCredentials", i, got)
		}
		fc.advance(1 * time.Second)
	}

	// Correct password should still be rejected while lockout is active.
	if got := a.Check(testSecret); got != AuthLockedOut {
		t.Errorf("Check(correct) during lockout = %v, want AuthLockedOut", got)
	}
}

// TestAuth_FailuresOutsideWindowDoNotCount verifies that failures that
// age out of the sliding window do not contribute to the lockout tally.
func TestAuth_FailuresOutsideWindowDoNotCount(t *testing.T) {
	a, fc := newTestAuth(t)

	// 4 failures right now.
	for i := 0; i < 4; i++ {
		a.Check("wrong")
	}
	// Advance beyond the window so those 4 age out.
	fc.advance(FailureWindow + 1*time.Second)
	// A 5th failure now should NOT trip the lockout.
	a.Check("wrong")
	if got := a.Check(testSecret); got != AuthOK {
		t.Errorf("Check(correct) after aged-out failures = %v, want AuthOK", got)
	}
}

// TestAuth_LockoutExpires verifies that after LockoutDuration, auth
// resumes normally.
func TestAuth_LockoutExpires(t *testing.T) {
	a, fc := newTestAuth(t)
	// Trip the lockout.
	for i := 0; i < MaxFailuresPerWindow; i++ {
		a.Check("wrong")
	}
	if got := a.Check(testSecret); got != AuthLockedOut {
		t.Fatalf("precondition: expected lockout, got %v", got)
	}
	// Advance beyond LockoutDuration.
	fc.advance(LockoutDuration + 1*time.Second)
	if got := a.Check(testSecret); got != AuthOK {
		t.Errorf("Check(correct) after lockout expired = %v, want AuthOK", got)
	}
}

// TestAuth_SuccessClearsFailureTally verifies that a good password
// resets the sliding window — legitimate callers do not accumulate
// historical typos.
func TestAuth_SuccessClearsFailureTally(t *testing.T) {
	a, _ := newTestAuth(t)
	for i := 0; i < MaxFailuresPerWindow-1; i++ {
		a.Check("wrong")
	}
	// One success clears the count.
	if got := a.Check(testSecret); got != AuthOK {
		t.Fatalf("Check(correct) = %v, want AuthOK", got)
	}
	// Now MaxFailuresPerWindow-1 more failures should not lock us out.
	for i := 0; i < MaxFailuresPerWindow-1; i++ {
		a.Check("wrong")
	}
	if got := a.Check(testSecret); got != AuthOK {
		t.Errorf("Check(correct) = %v, want AuthOK (success should have cleared tally)", got)
	}
}

// TestAuth_ConcurrentSafety runs many Check calls in parallel to exercise
// the lock. The race detector (go test -race) will flag any data race.
func TestAuth_ConcurrentSafety(t *testing.T) {
	a := NewAuthenticator(testSecret)

	const (
		workers       = 20
		callsPerGroup = 100
	)
	var okCount, badCount, lockedCount atomic.Int64

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		// Half the workers send correct passwords, half wrong. We don't
		// assert exact counts (lockout can race a correct attempt) — we
		// just want to prove there's no data race and Check never panics.
		pw := testSecret
		if w%2 == 0 {
			pw = "wrong"
		}
		go func(pw string) {
			defer wg.Done()
			for i := 0; i < callsPerGroup; i++ {
				switch a.Check(pw) {
				case AuthOK:
					okCount.Add(1)
				case AuthBadCredentials:
					badCount.Add(1)
				case AuthLockedOut:
					lockedCount.Add(1)
				}
			}
		}(pw)
	}
	wg.Wait()

	total := okCount.Load() + badCount.Load() + lockedCount.Load()
	if total != int64(workers*callsPerGroup) {
		t.Errorf("lost calls: total=%d want=%d", total, workers*callsPerGroup)
	}
}
