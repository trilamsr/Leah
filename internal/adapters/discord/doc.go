// Package discord is Leah's Discord adapter — operator-attested REST posting
// and channel enumeration against Discord API v10. W65 covers PostMessage +
// ListChannels; gateway Subscribe + voice attachments land in W66.
//
// Every RPC routes through Attestor.Attest(scope) BEFORE the bot token leaves
// the TokenSource; token-load on a denied call would leak the secret into a
// logger or panic trace. Outbound posts are capped at 10 per rolling 60s per
// channel as a spam-blast firebreak if the daemon is compromised.
package discord
