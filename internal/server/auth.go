package server

import (
	"crypto/subtle"
	"sync"
	"time"
)

// Lockout policy (layer L3 in the security model): if MaxFailuresPerWindow
// or more auth failures occur within FailureWindow of each other, reject
// every subsequent attempt for LockoutDuration. This is a sliding window
// keyed on the process — an attacker who can restart dddns loses little
// because the supervisor respawns on a 5-second backoff.
const (
	MaxFailuresPerWindow = 5
	FailureWindow        = 60 * time.Second
	LockoutDuration      = 5 * time.Minute
)

// AuthResult is the three-valued outcome of Authenticator.Check.
type AuthResult int

const (
	AuthOK AuthResult = iota
	AuthBadCredentials
	AuthLockedOut
)

// Authenticator verifies a Basic Auth password against a shared secret
// and enforces the sliding-window lockout. The zero value is not usable —
// construct with NewAuthenticator.
//
// All methods are safe for concurrent use.
type Authenticator struct {
	secret []byte

	mu          sync.Mutex
	failures    []time.Time
	lockedUntil time.Time
	now         func() time.Time // override for deterministic tests
}

// NewAuthenticator returns an Authenticator bound to the given shared
// secret. The secret is stored verbatim — the caller is expected to have
// already decrypted it from config.secure if applicable.
func NewAuthenticator(secret string) *Authenticator {
	return &Authenticator{
		secret: []byte(secret),
		now:    time.Now,
	}
}

// Check returns AuthOK on a matching password, AuthLockedOut if the
// Authenticator is in a lockout window, and AuthBadCredentials otherwise
// (after recording the failure for lockout tracking).
//
// A successful authentication clears the pending-failures tally —
// legitimate callers do not pay for historical typos.
func (a *Authenticator) Check(password string) AuthResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := a.now()
	if now.Before(a.lockedUntil) {
		return AuthLockedOut
	}

	if subtle.ConstantTimeCompare([]byte(password), a.secret) == 1 {
		a.failures = a.failures[:0]
		return AuthOK
	}

	// Record failure and prune the sliding window.
	a.failures = append(a.failures, now)
	cutoff := now.Add(-FailureWindow)
	kept := a.failures[:0]
	for _, t := range a.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	a.failures = kept

	if len(a.failures) >= MaxFailuresPerWindow {
		a.lockedUntil = now.Add(LockoutDuration)
		a.failures = a.failures[:0]
	}
	return AuthBadCredentials
}
