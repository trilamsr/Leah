package gmail

import (
	"context"
	"errors"
)

// Sentinel errors are stubbed RED so callers can compile against the surface
// while the implementation lands in the GREEN commit.
var (
	ErrAttestationDenied = errors.New("gmail: attestation denied")
	ErrMessageNotFound   = errors.New("gmail: message not found")
	ErrSendRejected      = errors.New("gmail: send rejected")
)

// Message, Config, Client, Attestor, TokenSource, Transport are declared so
// the test file compiles RED; bodies are filled in the GREEN commit.

type Message struct {
	To      string
	Subject string
	Body    string
}

type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type Transport interface {
	ListUnread(ctx context.Context, token string) ([]string, error)
	MarkRead(ctx context.Context, token, id string) error
	Send(ctx context.Context, token string, msg Message) error
}

type Config struct {
	Attestor    Attestor
	TokenSource TokenSource
	Transport   Transport
}

type Client struct{}

func New(_ Config) (*Client, error) { return nil, errors.New("gmail: not implemented") }

func (c *Client) ListUnread(_ context.Context) ([]string, error) {
	return nil, errors.New("gmail: not implemented")
}

func (c *Client) MarkRead(_ context.Context, _ string) error {
	return errors.New("gmail: not implemented")
}

func (c *Client) Send(_ context.Context, _ Message) error {
	return errors.New("gmail: not implemented")
}
