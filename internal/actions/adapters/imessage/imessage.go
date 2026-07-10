package imessage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/trilam/leah/internal/platform/contracts"
	"github.com/trilam/leah/internal/platform/ratelimit"
	"github.com/trilam/leah/internal/platform/telemetry/connectadapter"
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
	Attestor contracts.Attestor
	OSExec   OSExec
	Audit    AuditSink
	Now      func() time.Time
	// Metrics is optional — nil is a no-op (connectadapter contract), so
	// existing callers keep working without a registry.
	Metrics *connectadapter.Metrics
}

type Adapter struct {
	att     contracts.Attestor
	exec    OSExec
	audit   AuditSink
	m       *connectadapter.Metrics
	limiter *ratelimit.Window
}

func New(config Config) (*Adapter, error) {
	if config.Attestor == nil {
		return nil, errors.New("imessage: Config.Attestor required (operator-attestation gate)")
	}
	if config.OSExec == nil {
		return nil, errors.New("imessage: Config.OSExec required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Adapter{att: config.Attestor, exec: config.OSExec, audit: config.Audit, m: config.Metrics, limiter: ratelimit.NewWindow(rateWindow, maxSendsPerWindow, now)}, nil
}

// Send order is load-bearing: validate -> rate-limit -> attest -> exec.
func (a *Adapter) Send(ctx context.Context, msg Message) error {
	hash := recipientHash(msg.To)
	bodyLen := utf8.RuneCountInString(msg.Body)

	if msg.To == "" || !recipientRe.MatchString(msg.To) {
		a.record(AuditRow{Kind: "imessage_send", RecipientHash: hash, BodyLen: bodyLen, Reason: "invalid_recipient"})
		return ErrInvalidRecipient
	}
	if !a.limiter.Allow("") {
		a.record(AuditRow{Kind: "imessage_send", RecipientHash: hash, BodyLen: bodyLen, Reason: "rate_limited"})
		return ErrRateLimited
	}
	if err := a.att.Attest(ctx, ScopeSend); err != nil {
		a.record(AuditRow{Kind: "imessage_send", RecipientHash: hash, BodyLen: bodyLen, Reason: "attestation_denied"})
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	script := buildScript(msg.To, msg.Body)
	start := time.Now()
	_, err := a.exec.Run(ctx, "osascript", []string{"-"}, []byte(script))
	a.m.ObserveAPI("send", time.Since(start).Seconds())
	if err != nil {
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
