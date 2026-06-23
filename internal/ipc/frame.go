// Package ipc is the daemon-side AF_UNIX socket transport for the
// SwiftUI HUD. Length-prefixed JSON frames per spec §10.7; 256 KB cap
// protects the parser buffer from runaway widget payloads.
package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const MaxFrameBytes = 256 * 1024

// ErrFrameSizeExceeded reports a frame larger than MaxFrameBytes. Server
// closes the conn on this error (header desync is unrecoverable). Use
// errors.Is to detect rather than string-matching the message.
var ErrFrameSizeExceeded = errors.New("ipc: frame size exceeds cap")

const (
	// Conversational frames per spec §10.7.
	KindAsk             = "ask"
	KindDiag            = "diag.state"
	KindDiagResponse    = "diag.state.response"
	KindError           = "error"
	KindProseDelta      = "prose.delta"
	KindProseEnd        = "prose.end"
	KindTurnEnd         = "turn.end"
	KindVerifyKey       = "verify-key"
	KindVerifyKeyResult = "verify-key.result"

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
	KindTTSSpeak      = "tts.speak"
	KindTTSCancel     = "tts.cancel"
	KindTTSCloudFrame = "tts.cloud.frame"
	KindTTSAppleSpeak = "tts.apple.speak"
	KindTTSSpeakDone  = "tts.speak.done"
	KindTTSSpeakErr   = "tts.speak.err"
	KindTTSCancelOK   = "tts.cancel.ok"

	// Voice duplex kinds — §1.3.2. Daemon receives KindVoiceStart / KindVoiceBarge /
	// KindVoiceEnd from the HUD; emits KindVoicePartial transcript updates and
	// KindVoiceTTSChunk audio frames between them.
	KindVoiceStart    = "voice.start"
	KindVoicePartial  = "voice.partial"
	KindVoiceTTSChunk = "voice.tts.chunk"
	KindVoiceBarge    = "voice.barge"
	KindVoiceEnd      = "voice.end"

	// Vision kinds — §4.6. KindVisionSnap is a one-shot screenshot Ask;
	// KindVisionStreamStart / KindVisionStreamFrame drive live-screen and
	// live-camera capture; KindVisionConsentRequired is the daemon→HUD
	// nudge when the first per-session cloud upload needs operator approval.
	KindVisionSnap            = "vision.snap"
	KindVisionStreamStart     = "vision.stream.start"
	KindVisionStreamFrame     = "vision.stream.frame"
	KindVisionConsentRequired = "vision.consent.required"

	// Sync kinds — §10.7 sync.* (Bonjour + OTP pair). PeerList drains the
	// current discovery snapshot; PairStart runs the OTP handshake against
	// the matching peer; PairAck commits the sync_peer row.
	KindSyncPeerList  = "sync.peer.list"
	KindSyncPairStart = "sync.pair.start"
	KindSyncPairAck   = "sync.pair.ack"

	// Recommend kinds — Settings RecommendationsPane (T07) bridge into
	// internal/learn.Recommender. List/Apply/Dismiss drive the queued→surfaced
	// →accepted lifecycle; AntiAdd / AntiList expose the operator-source
	// anti-rule surface so a kind dismissed by the user never re-surfaces.
	KindRecommendList     = "recommend.list"
	KindRecommendApply    = "recommend.apply"
	KindRecommendDismiss  = "recommend.dismiss"
	KindRecommendAntiAdd  = "recommend.anti.add"
	KindRecommendAntiList = "recommend.anti.list"

	// Plugin kinds — §15 plugin lifecycle. List snapshots host state;
	// Install/Uninstall manage bundles; Enable/Disable toggle without
	// uninstall; Logs tails plugin stdout for the PluginsPane.
	KindPluginList      = "plugin.list"
	KindPluginInstall   = "plugin.install"
	KindPluginEnable    = "plugin.enable"
	KindPluginDisable   = "plugin.disable"
	KindPluginUninstall = "plugin.uninstall"
	KindPluginLogs      = "plugin.logs"

	// A2A kinds — §14 agent-to-agent. PeerList snapshots paired peers;
	// PairStart drives the §5.4 hello handshake; PeerPause toggles delivery;
	// PeerUnpair revokes the trust record.
	KindA2APeerList   = "a2a.peer.list"
	KindA2APairStart  = "a2a.pair.start"
	KindA2APeerPause  = "a2a.peer.pause"
	KindA2APeerUnpair = "a2a.peer.unpair"
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
		return fmt.Errorf("%w: %d > %d", ErrFrameSizeExceeded, len(body), MaxFrameBytes)
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
		return Frame{}, fmt.Errorf("%w: %d > %d", ErrFrameSizeExceeded, n, MaxFrameBytes)
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
