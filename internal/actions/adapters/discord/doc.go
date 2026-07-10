// Package discord is Leah's Discord adapter — operator-attested REST posting,
// channel enumeration, and gateway Subscribe against Discord API v10. Current
// surface: PostMessage, ListChannels, and Subscribe via a pluggable
// WebSocketDialer seam so tests stay hermetic and the real gorilla/websocket
// (or discordgo) binding can land later without churning this adapter's
// surface. Voice attachments are a follow-up.
//
// Every RPC routes through Attestor.Attest(scope) BEFORE the bot token leaves
// the TokenSource; token-load on a denied call would leak the secret into a
// logger or panic trace. Outbound posts are capped at 10 per rolling 60s per
// channel as a spam-blast firebreak if the daemon is compromised. Inbound
// gateway reads NEVER block on a slow handler — a full dispatch queue drops
// the event with a hashed audit row so Discord cannot time the session out.
package discord
