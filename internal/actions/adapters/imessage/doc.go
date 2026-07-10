/*
Package imessage is Leah's adapter for outbound iMessage sends from the host
Mac via osascript. Every send routes through the operator-attestation gate
(Attestor) before any subprocess is spawned; recipient validation and the
rolling rate limit run BEFORE attestation so typos and bursts do not burn
consent prompts. macOS Messages.app holds the operator's iMessage account —
there is no OAuth token. The constructor performs no I/O.
*/
package imessage
