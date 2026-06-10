// Package session is the voice-loop state machine. It arbitrates between
// the wake/listener stream and the Reasoner+TTS reply path, owning barge-in
// cancellation so a fresh utterance during TTS cancels the in-flight reply.
//
// See docs/engineer/specs/2026-06-10-voice-comm.md §9 for the state×event
// table this package implements and §4 for the barge-in invariant.
package session
