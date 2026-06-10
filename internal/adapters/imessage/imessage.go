package imessage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const ScopeSend = "imessage:send"

const (
	maxSendsPerWindow = 10
	rateWindow        = 60 * time.Second
)

const permissionDeniedSignal = "not authorized to send Apple events"

var recipientRe = regexp.MustCompile(`^[+\w@.-]+$`)

var (
	ErrAttestationDenied = errors.New("imessage: attestation denied")
	ErrPermissionDenied  = errors.New("imessage: automation permission denied")
	ErrSendFailed        = errors.New("imessage: send failed")
	ErrRateLimited       = errors.New("imessage: rate limit exceeded")
	ErrInvalidRecipient  = errors.New("imessage: invalid recipient")
)

type Message struct {
	To   string
	Body string
}

type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

// OSExec carries the script via stdin so AppleScript never enters argv.
type OSExec interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

// AuditRow omits plaintext recipient and body bytes by design (spec §6).
type AuditRow struct {
	Kind          string
	Success       bool
	RecipientHash string
	BodyLen       int
	Reason        string
}

type AuditSink interface {
	Record(row AuditRow)
}

type Config struct {
	Attestor Attestor
	OSExec   OSExec
	Audit    AuditSink
	Now      func() time.Time
}

type Adapter struct {
	att   Attestor
	exec  OSExec
	audit AuditSink
	now   func() time.Time

	mu    sync.Mutex
	sends []time.Time
}

func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("imessage: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.OSExec == nil {
		return nil, errors.New("imessage: Config.OSExec required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Adapter{att: cfg.Attestor, exec: cfg.OSExec, audit: cfg.Audit, now: now}, nil
}

// Send order is load-bearing: validate -> rate-limit -> attest -> exec.
func (a *Adapter) Send(ctx context.Context, msg Message) error {
	hash := recipientHash(msg.To)
	bodyLen := utf8.RuneCountInString(msg.Body)

	if msg.To == "" || !recipientRe.MatchString(msg.To) {
		a.record(AuditRow{Kind: "imessage_send", RecipientHash: hash, BodyLen: bodyLen, Reason: "invalid_recipient"})
		return ErrInvalidRecipient
	}
	if !a.allow() {
		a.record(AuditRow{Kind: "imessage_send", RecipientHash: hash, BodyLen: bodyLen, Reason: "rate_limited"})
		return ErrRateLimited
	}
	if err := a.att.Attest(ctx, ScopeSend); err != nil {
		a.record(AuditRow{Kind: "imessage_send", RecipientHash: hash, BodyLen: bodyLen, Reason: "attestation_denied"})
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	script := buildScript(msg.To, msg.Body)
	if _, err := a.exec.Run(ctx, "osascript", []string{"-"}, []byte(script)); err != nil {
		if strings.Contains(err.Error(), permissionDeniedSignal) {
			a.record(AuditRow{Kind: "imessage_send", RecipientHash: hash, BodyLen: bodyLen, Reason: "permission_denied"})
			return fmt.Errorf("%w: %v", ErrPermissionDenied, err)
		}
		a.record(AuditRow{Kind: "imessage_send", RecipientHash: hash, BodyLen: bodyLen, Reason: err.Error()})
		return fmt.Errorf("%w: %v", ErrSendFailed, err)
	}
	a.record(AuditRow{Kind: "imessage_send", Success: true, RecipientHash: hash, BodyLen: bodyLen})
	return nil
}

func (a *Adapter) record(r AuditRow) {
	if a.audit == nil {
		return
	}
	a.audit.Record(r)
}

func (a *Adapter) allow() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cutoff := a.now().Add(-rateWindow)
	kept := a.sends[:0]
	for _, t := range a.sends {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	a.sends = kept
	if len(a.sends) >= maxSendsPerWindow {
		return false
	}
	a.sends = append(a.sends, a.now())
	return true
}

func recipientHash(to string) string {
	sum := sha256.Sum256([]byte(to))
	return hex.EncodeToString(sum[:])[:8]
}

func buildScript(to, body string) string {
	return `tell application "Messages"
	set targetService to first service whose service type = iMessage
	set targetBuddy to buddy "` + escapeAS(to) + `" of targetService
	send "` + escapeAS(body) + `" to targetBuddy
end tell
`
}

// escapeAS: backslash MUST run before quote so escaped quotes are not double-escaped.
func escapeAS(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
