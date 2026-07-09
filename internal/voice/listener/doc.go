// Package listener owns the STT side of the voice loop: it converts a hot
// microphone stream into a series of Segment values (partial transcripts plus
// one Final per utterance) on a single channel.
//
// Current surface: the interface, a deterministic in-memory Fake for tests,
// and a whisper-stream backend behind the same Listener interface so
// session-layer code never imports the binary directly.
package listener
