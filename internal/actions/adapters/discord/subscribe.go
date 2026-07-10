package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
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

// pendingMessage carries a parsed Message plus the URL of an unfetched
// voice attachment. Fetching happens on the dispatch goroutine, never
// on the gateway read loop (a slow CDN fetch must not stall reads).
type pendingMessage struct {
	msg      Message
	voiceURL string
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
	start := time.Now()
	conn, err := a.dialer.Dial(ctx, url, tok)
	a.m.ObserveAPI("subscribe_dial", time.Since(start).Seconds())
	if err != nil {
		a.record(AuditRow{Kind: "discord_subscribe", Reason: "dial_failed"})
		return fmt.Errorf("discord: gateway dial: %w", err)
	}

	wanted := make(map[string]struct{}, len(channelIDs))
	for _, id := range channelIDs {
		wanted[id] = struct{}{}
	}

	queue := make(chan pendingMessage, inboundQueueDepth)
	telemetry.SafeGo(nil, nil, "discord-dispatch", func() { a.dispatchLoop(ctx, queue, handler) })
	telemetry.SafeGo(nil, nil, "discord-read", func() { a.readLoop(ctx, conn, wanted, queue) })

	a.record(AuditRow{Kind: "discord_subscribe", Success: true})
	return nil
}

// readLoop owns the gateway socket. It MUST NOT block on the handler queue —
// a slow handler dropping events is preferable to a stalled read that lets
// Discord time us out and tear down the session.
func (a *Adapter) readLoop(ctx context.Context, conn WebSocketConn, wanted map[string]struct{}, queue chan<- pendingMessage) {
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
		pm, ok := a.parseEvent(raw, wanted)
		if !ok {
			continue
		}
		select {
		case queue <- pm:
		default:
			// Handler is behind. Drop the event so the gateway read stays hot.
			a.record(AuditRow{Kind: "discord_inbound", Reason: "handler_slow_drop", ChannelHash: shortHash(pm.msg.ChannelID), GuildHash: shortHash(pm.msg.GuildID)})
		}
	}
}

func (a *Adapter) dispatchLoop(ctx context.Context, queue <-chan pendingMessage, handler func(Message)) {
	for {
		select {
		case <-ctx.Done():
			return
		case pm, ok := <-queue:
			if !ok {
				return
			}
			if pm.voiceURL != "" {
				// Fetch on the dispatch goroutine so a slow CDN response
				// cannot stall the gateway read loop. Failure leaves Voice nil.
				pm.msg.Voice = a.fetchVoiceAttachment(ctx, pm.voiceURL)
			}
			handler(pm.msg)
		}
	}
}

// parseEvent decodes a Discord gateway frame and applies the guild
// allowlist + channel filter. Non-MESSAGE_CREATE ops are dropped silently;
// non-allowlisted guilds increment a counter row without ever surfacing
// the message to the handler. Voice attachments are detected here and
// their URL stashed for the dispatch goroutine to fetch — the read loop
// must NOT block on a CDN download.
func (a *Adapter) parseEvent(raw []byte, wanted map[string]struct{}) (pendingMessage, bool) {
	var env struct {
		Op int             `json:"op"`
		T  string          `json:"t"`
		D  json.RawMessage `json:"d"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return pendingMessage{}, false
	}
	if env.T != "MESSAGE_CREATE" {
		return pendingMessage{}, false
	}
	var d struct {
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id"`
		Content   string `json:"content"`
		Author    struct {
			ID string `json:"id"`
		} `json:"author"`
		Timestamp   string `json:"timestamp"`
		Attachments []struct {
			URL         string `json:"url"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			Size        int    `json:"size"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(env.D, &d); err != nil {
		return pendingMessage{}, false
	}
	if !a.guildAllowed(d.GuildID) {
		a.record(AuditRow{Kind: "discord_inbound", Reason: "guild_not_allowed", GuildHash: shortHash(d.GuildID)})
		return pendingMessage{}, false
	}
	if len(wanted) > 0 {
		if _, ok := wanted[d.ChannelID]; !ok {
			return pendingMessage{}, false
		}
	}
	ts, _ := time.Parse(time.RFC3339Nano, d.Timestamp)
	var voiceURL string
	for _, att := range d.Attachments {
		if isVoiceAttachment(att.ContentType, att.Filename) && att.Size <= maxVoiceBytes {
			voiceURL = att.URL
			break
		}
	}
	return pendingMessage{
		msg: Message{
			ChannelID: d.ChannelID,
			GuildID:   d.GuildID,
			AuthorID:  d.Author.ID,
			Body:      d.Content,
			Timestamp: ts,
		},
		voiceURL: voiceURL,
	}, true
}
