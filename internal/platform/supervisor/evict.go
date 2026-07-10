package supervisor

type PSILevel int

const (
	PSINominal PSILevel = iota
	PSIWarning
	PSICritical
)

type EvictAction int

const (
	EvictAdapterCache EvictAction = iota
	EvictLiveVision
	EvictPluginLRU
	EvictWhisperContinuous
	EvictVoiceSession
)

// EvictionLadder is spec §9.5: shed cheapest first; harsher pressure preserves order.
func EvictionLadder(p PSILevel) []EvictAction {
	if p == PSINominal {
		return nil
	}
	return []EvictAction{
		EvictAdapterCache,
		EvictLiveVision,
		EvictPluginLRU,
		EvictWhisperContinuous,
		EvictVoiceSession,
	}
}
