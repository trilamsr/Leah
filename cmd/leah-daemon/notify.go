package main

import (
	"fmt"
	"os"

	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/notify"
)

// buildNotifier returns the composition root for daemon notifications.
// Always includes Desktop; appends VoiceNotify when LEAH_VOICE_ENABLED=1.
// Fanout dispatches to every wrapped notifier and joins errors so a TTS
// chain failure cannot suppress the desktop banner.
func buildNotifier() daemonloop.Notifier {
	desktop := notify.NewDesktop()
	if os.Getenv("LEAH_VOICE_ENABLED") != "1" {
		return desktop
	}
	return &notify.Fanout{Notifiers: []notify.Notifier{desktop, notify.NewVoice()}}
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
