// Package listener owns the STT side of the voice loop: it converts a hot
// microphone stream into a series of Segment values (partial transcripts plus
// one Final per utterance) on a single channel.
//
// W11 ships the interface, a deterministic in-memory Fake for tests, and a
// Real placeholder that returns ErrNotImplemented. W12 wires the
// whisper-stream backend behind the same Listener interface so session-layer
// code never imports the binary directly.
//
// See docs/engineer/specs/2026-06-10-voice-comm.md §3.1 for the surface and
// §4 for the barge-in semantics this listener feeds.
package listener
