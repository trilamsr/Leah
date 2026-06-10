package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const ScopeSubscribe = "discord:subscribe"

const defaultGatewayURL = "wss://gateway.discord.gg/?v=10&encoding=json"

// inboundQueueDepth is small on purpose: backpressure on the gateway reader
// is unsafe (Discord disconnects slow consumers), so a slow handler must
// drop the next event rather than block the read loop.
const inboundQueueDepth = 32

// Message is the inbound surface from a subscribed channel. Voice payloads
// arrive as raw bytes (decoded by the STT pipeline, not this adapter).
type Message struct {
	ChannelID string
	GuildID   string
	AuthorID  string
	Body      string
	Voice     []byte
	Timestamp time.Time
}

// WebSocketConn is the read seam over a live Discord gateway connection.
// Production wires gorilla/websocket; tests inject a fake that yields canned
// frames. ReadMessage MUST return a terminal error after Close.
type WebSocketConn interface {
	ReadMessage() ([]byte, error)
	Close() error
}

// WebSocketDialer opens a gateway connection. Token is passed so the dialer
// can attach it (Discord's IDENTIFY flow) without leaking the secret back up
// through the adapter — keeps the bot token on the dialer side of the seam.
type WebSocketDialer interface {
	Dial(ctx context.Context, url, token string) (WebSocketConn, error)
}

// Subscribe opens a single shared gateway session and routes inbound
// MESSAGE_CREATE events from the requested channels through handler.
// Attestation runs once at subscription grain — per-message prompts would
// be unusable. Empty GuildAllowlist drops everything; the operator must
// opt in explicitly.
func (a *Adapter) Subscribe(ctx context.Context, channelIDs []string, handler func(Message)) error {
	if a.dialer == nil {
		return errors.New("discord: Subscribe requires Config.WebSocketDialer")
	}
	if handler == nil {
		return errors.New("discord: Subscribe handler must not be nil")
	}

	if err := a.att.Attest(ctx, ScopeSubscribe); err != nil {
		a.record(AuditRow{Kind: "discord_subscribe", Reason: "attestation_denied"})
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		a.record(AuditRow{Kind: "discord_subscribe", Reason: "token_load_failed"})
		return fmt.Errorf("discord: token load: %w", err)
	}

	url := a.gatewayURL
	if url == "" {
		url = defaultGatewayURL
	}
	conn, err := a.dialer.Dial(ctx, url, tok)
	if err != nil {
		a.record(AuditRow{Kind: "discord_subscribe", Reason: "dial_failed"})
		return fmt.Errorf("discord: gateway dial: %w", err)
	}

	wanted := make(map[string]struct{}, len(channelIDs))
	for _, id := range channelIDs {
		wanted[id] = struct{}{}
	}

	queue := make(chan Message, inboundQueueDepth)
	go a.dispatchLoop(ctx, queue, handler)
	go a.readLoop(ctx, conn, wanted, queue)

	a.record(AuditRow{Kind: "discord_subscribe", Success: true})
	return nil
}

// readLoop owns the gateway socket. It MUST NOT block on the handler queue —
// a slow handler dropping events is preferable to a stalled read that lets
// Discord time us out and tear down the session.
func (a *Adapter) readLoop(ctx context.Context, conn WebSocketConn, wanted map[string]struct{}, queue chan<- Message) {
	defer func() { _ = conn.Close() }()
	defer close(queue)

	done := ctx.Done()
	closed := make(chan struct{})
	go func() {
		select {
		case <-done:
			_ = conn.Close()
		case <-closed:
			return
		}
	}()
	defer close(closed)

	for {
		raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		msg, ok := a.parseEvent(raw, wanted)
		if !ok {
			continue
		}
		select {
		case queue <- msg:
		default:
			// Handler is behind. Drop the event so the gateway read stays hot.
			a.record(AuditRow{Kind: "discord_inbound", Reason: "handler_slow_drop", ChannelHash: shortHash(msg.ChannelID), GuildHash: shortHash(msg.GuildID)})
		}
	}
}

func (a *Adapter) dispatchLoop(ctx context.Context, queue <-chan Message, handler func(Message)) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-queue:
			if !ok {
				return
			}
			handler(msg)
		}
	}
}

// parseEvent decodes a Discord gateway frame and applies the guild
// allowlist + channel filter. Non-MESSAGE_CREATE ops are dropped silently;
// non-allowlisted guilds increment a counter row without ever surfacing
// the message to the handler.
func (a *Adapter) parseEvent(raw []byte, wanted map[string]struct{}) (Message, bool) {
	var env struct {
		Op int             `json:"op"`
		T  string          `json:"t"`
		D  json.RawMessage `json:"d"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return Message{}, false
	}
	if env.T != "MESSAGE_CREATE" {
		return Message{}, false
	}
	var d struct {
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id"`
		Content   string `json:"content"`
		Author    struct {
			ID string `json:"id"`
		} `json:"author"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(env.D, &d); err != nil {
		return Message{}, false
	}
	if !a.guildAllowed(d.GuildID) {
		a.record(AuditRow{Kind: "discord_inbound", Reason: "guild_not_allowed", GuildHash: shortHash(d.GuildID)})
		return Message{}, false
	}
	if len(wanted) > 0 {
		if _, ok := wanted[d.ChannelID]; !ok {
			return Message{}, false
		}
	}
	ts, _ := time.Parse(time.RFC3339Nano, d.Timestamp)
	return Message{
		ChannelID: d.ChannelID,
		GuildID:   d.GuildID,
		AuthorID:  d.Author.ID,
		Body:      d.Content,
		Timestamp: ts,
	}, true
}
