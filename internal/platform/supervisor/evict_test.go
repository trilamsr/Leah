package supervisor

import (
	"reflect"
	"testing"
)

func TestEvictionLadder_OrderMatchesSpec(t *testing.T) {
	want := []EvictAction{
		EvictAdapterCache,
		EvictLiveVision,
		EvictPluginLRU,
		EvictWhisperContinuous,
		EvictVoiceSession,
	}
	got := EvictionLadder(PSIWarning)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ladder: want %v, got %v", want, got)
	}
}

func TestEvictionLadder_NominalIsEmpty(t *testing.T) {
	if got := EvictionLadder(PSINominal); len(got) != 0 {
		t.Fatalf("nominal pressure should have no actions, got %v", got)
	}
}

func TestEvictionLadder_CriticalSkipsToVoiceSession(t *testing.T) {
	got := EvictionLadder(PSICritical)
	if len(got) == 0 || got[len(got)-1] != EvictVoiceSession {
		t.Fatalf("critical ladder must end with EvictVoiceSession, got %v", got)
	}
}
