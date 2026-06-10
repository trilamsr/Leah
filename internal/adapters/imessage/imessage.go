package imessage

import (
	"context"
	"errors"
	"time"
)

const ScopeSend = "imessage:send"

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

type OSExec interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

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

type Adapter struct{}

func New(cfg Config) (*Adapter, error) {
	_ = cfg
	return &Adapter{}, nil
}

func (a *Adapter) Send(ctx context.Context, msg Message) error {
	_ = ctx
	_ = msg
	return nil
}
