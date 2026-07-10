package duplex

import "sync"

// bargeArbiter gates a TTS-halt callback on mic VAD detecting voice
// while TTS is active. Latching ttsActive=false after one halt enforces
// the 80ms spec §1.7 budget — repeated VAD frames within the same TTS
// window do not double-fire the cancel path.
type bargeArbiter struct {
	mu        sync.Mutex
	ttsActive bool
	onHalt    func()
}

func newBargeArbiter() *bargeArbiter { return &bargeArbiter{} }

func (a *bargeArbiter) ttsStarted() {
	a.mu.Lock()
	a.ttsActive = true
	a.mu.Unlock()
}

func (a *bargeArbiter) ttsEnded() {
	a.mu.Lock()
	a.ttsActive = false
	a.mu.Unlock()
}

func (a *bargeArbiter) micVoiceDetected() {
	a.mu.Lock()
	fire := a.ttsActive && a.onHalt != nil
	if fire {
		a.ttsActive = false
	}
	cb := a.onHalt
	a.mu.Unlock()
	if fire {
		cb()
	}
}
