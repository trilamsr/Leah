// Package ipc is the daemon-side AF_UNIX socket transport for the
// SwiftUI HUD. Length-prefixed JSON frames per spec §10.7; 256 KB cap
// protects the parser buffer from runaway widget payloads.
package ipc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const MaxFrameBytes = 256 * 1024

const (
	// Conversational frames per spec §10.7.
	KindAsk      = "ask"
	KindDiag     = "diag.state"
	KindError    = "error"
	KindProseDelta = "prose.delta"
	KindProseEnd   = "prose.end"
	KindTurnEnd    = "turn.end"
	KindVerifyKey  = "verify-key"

	KindWidgetMount       = "widget.mount"
	KindWidgetUpdate      = "widget.update"
	KindWidgetStale       = "widget.stale"
	KindWidgetError       = "widget.error"
	KindWidgetDismiss     = "widget.dismiss"
	KindWidgetUnmount     = "widget.unmount"
	KindNotificationToast = "notification.toast"

	// TTS kinds — §17.17. Daemon receives KindTTSSpeak / KindTTSCancel from
	// the HUD; emits KindTTSCloudFrame chunks (ElevenLabs path), a single
	// KindTTSAppleSpeak trigger (Apple Ava — HUD plays locally), and
	// terminal KindTTSSpeakDone / KindTTSSpeakErr / KindTTSCancelOK.
	KindTTSSpeak       = "tts.speak"
	KindTTSCancel      = "tts.cancel"
	KindTTSCloudFrame  = "tts.cloud.frame"
	KindTTSAppleSpeak  = "tts.apple.speak"
	KindTTSSpeakDone   = "tts.speak.done"
	KindTTSSpeakErr    = "tts.speak.err"
	KindTTSCancelOK    = "tts.cancel.ok"
)

// Frame is the wire format. Seq is signed int64 so negative values can be
// rejected explicitly at handler boundary (uint64 silently coerced them to
// huge positives and the bad-faith client never saw a structured error).
type Frame struct {
	Kind    string          `json:"kind"`
	TurnID  string          `json:"turn_id"`
	Seq     int64           `json:"seq"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func WriteFrame(w io.Writer, f Frame) error {
	body, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	if len(body) > MaxFrameBytes {
		return fmt.Errorf("frame size %d exceeds cap %d", len(body), MaxFrameBytes)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("write hdr: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

func ReadFrame(r io.Reader) (Frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, fmt.Errorf("read hdr: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxFrameBytes {
		return Frame{}, fmt.Errorf("frame size %d exceeds cap %d", n, MaxFrameBytes)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return Frame{}, fmt.Errorf("read body: %w", err)
	}
	var f Frame
	if err := json.Unmarshal(body, &f); err != nil {
		return Frame{}, fmt.Errorf("unmarshal: %w", err)
	}
	return f, nil
}
