package main

import (
	"fmt"
	"os"

	commsout "github.com/trilam/leah/internal/comms/out"
	"github.com/trilam/leah/internal/contracts"
)

// buildNotifier returns the composition root for daemon notifications.
// Always includes Desktop; appends VoiceNotify when LEAH_VOICE_ENABLED=1.
// Fanout dispatches to every wrapped notifier and joins errors so a TTS
// chain failure cannot suppress the desktop banner.
func buildNotifier() contracts.Notifier {
	desktop := commsout.NewDesktop()
	if os.Getenv("LEAH_VOICE_ENABLED") != "1" {
		return desktop
	}
	return &commsout.Fanout{Notifiers: []contracts.Notifier{desktop, commsout.NewVoice()}}
}

// logVoiceState emits the operator-visible line announcing whether voice
// notifications are wired in this daemon process.
func logVoiceState(out *os.File) {
	if os.Getenv("LEAH_VOICE_ENABLED") == "1" {
		_, _ = fmt.Fprintln(out, "leah-daemon: voice notifier enabled (LEAH_VOICE_ENABLED=1)")
		return
	}
	_, _ = fmt.Fprintln(out, "leah-daemon: voice notifier disabled (set LEAH_VOICE_ENABLED=1 to enable)")
}
