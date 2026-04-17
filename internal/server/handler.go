package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/updater"
	"github.com/descoped/dddns/internal/wanip"
)

// handlerTimeout bounds a single request. Route53 is the slow caller;
// anything longer than this is a hang we want to abort.
const handlerTimeout = 30 * time.Second

// Handler processes dyndns-style requests from UniFi's inadyn. It owns
// the CIDR allowlist, Basic-Auth check, query validation, authoritative
// WAN-IP lookup, Route53 UPSERT (via updater), audit logging, and status
// snapshot.
type Handler struct {
	cfg    *config.Config
	auth   *Authenticator
	audit  *AuditLog
	status *StatusWriter

	// Hooks overridden in tests. Not part of the public API.
	wanIP    func(iface string) (net.IP, error)
	updateIP func(ctx context.Context, cfg *config.Config, opts updater.Options) (*updater.Result, error)
	now      func() time.Time
}

// NewHandler constructs a Handler with production dependencies.
func NewHandler(cfg *config.Config, auth *Authenticator, audit *AuditLog, status *StatusWriter) *Handler {
	return &Handler{
		cfg:      cfg,
		auth:     auth,
		audit:    audit,
		status:   status,
		wanIP:    wanip.FromInterface,
		updateIP: updater.Update,
		now:      time.Now,
	}
}

// ServeHTTP implements http.Handler for the dyndns update endpoint.
// See §10 of the design doc for the full response-code table.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entry := AuditEntry{RemoteAddr: r.RemoteAddr}

	defer func() {
		if rec := recover(); rec != nil {
			entry.Action = "panic"
			entry.Err = fmt.Sprintf("%v", rec)
			h.writeDyndns(w, "911", "")
			h.emit(entry)
		}
	}()

	// L1: network origin.
	if !IsAllowed(r.RemoteAddr, h.cfg.Server.AllowedCIDRs) {
		entry.Action = "cidr-deny"
		w.WriteHeader(http.StatusForbidden)
		h.emit(entry)
		return
	}

	// Protocol: only GET.
	if r.Method != http.MethodGet {
		entry.Action = "method-deny"
		w.WriteHeader(http.StatusMethodNotAllowed)
		h.emit(entry)
		return
	}

	// L2: authentication (L3 lockout is inside Authenticator).
	_, password, ok := r.BasicAuth()
	if !ok {
		entry.AuthOutcome = "missing"
		h.writeDyndns(w, "badauth", "")
		h.emit(entry)
		return
	}
	switch h.auth.Check(password) {
	case AuthOK:
		entry.AuthOutcome = "ok"
	case AuthLockedOut:
		entry.AuthOutcome = "locked"
		h.writeDyndns(w, "badauth", "")
		h.emit(entry)
		return
	default:
		entry.AuthOutcome = "bad"
		h.writeDyndns(w, "badauth", "")
		h.emit(entry)
		return
	}

	// Query validation.
	hostname := r.URL.Query().Get("hostname")
	entry.Hostname = hostname
	entry.MyIPClaimed = r.URL.Query().Get("myip")

	if hostname == "" {
		entry.Action = "notfqdn"
		h.writeDyndns(w, "notfqdn", "")
		h.emit(entry)
		return
	}
	if hostname != h.cfg.Hostname {
		entry.Action = "nohost"
		h.writeDyndns(w, "nohost", "")
		h.emit(entry)
		return
	}

	// L4: authoritative local WAN IP. The `myip` query param is a hint
	// only — an anomaly is logged if it disagrees with the real value.
	iface := ""
	if h.cfg.Server != nil {
		iface = h.cfg.Server.WANInterface
	}
	localIP, err := h.wanIP(iface)
	if err != nil {
		entry.Action = "wanip-error"
		entry.Err = err.Error()
		h.writeDyndns(w, "dnserr", "")
		h.emit(entry)
		return
	}
	entry.MyIPVerified = localIP.String()

	// Route53 UPSERT via the shared updater.
	ctx, cancel := context.WithTimeout(r.Context(), handlerTimeout)
	defer cancel()
	result, err := h.updateIP(ctx, h.cfg, updater.Options{
		OverrideIP: localIP.String(),
		Quiet:      true, // handler logs via audit, not stdout
	})
	if err != nil {
		entry.Action = "dnserr"
		entry.Err = err.Error()
		h.writeDyndns(w, "dnserr", "")
		h.emit(entry)
		return
	}

	entry.Action = result.Action
	switch result.Action {
	case "updated":
		h.writeDyndns(w, "good", result.NewIP)
	case "nochg-cache", "nochg-dns", "dry-run":
		h.writeDyndns(w, "nochg", result.NewIP)
	default:
		entry.Err = "unknown updater action: " + result.Action
		h.writeDyndns(w, "dnserr", "")
	}
	h.emit(entry)
}

// writeDyndns writes a dyndns-protocol response: plain text, trailing
// newline, always HTTP 200. The IP is appended when non-empty (e.g.
// "good 1.2.3.4\n").
func (h *Handler) writeDyndns(w http.ResponseWriter, code, ip string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	body := code
	if ip != "" {
		body = code + " " + ip
	}
	_, _ = w.Write([]byte(body + "\n"))
}

// emit writes an audit-log line and refreshes the status file. Errors
// are swallowed — failure to emit must not prevent us from responding
// to the client.
func (h *Handler) emit(entry AuditEntry) {
	_ = h.audit.Write(entry)
	_ = h.status.Write(StatusSnapshot{
		LastRequestAt:   h.now(),
		LastRemoteAddr:  entry.RemoteAddr,
		LastAuthOutcome: entry.AuthOutcome,
		LastAction:      entry.Action,
		LastError:       entry.Err,
	})
}
