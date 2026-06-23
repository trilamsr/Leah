package discovery

import (
	"testing"
)

func TestServiceTypeMatchesSpec(t *testing.T) {
	if ServiceType != "_leah-sync._tcp" {
		t.Fatalf("ServiceType: want _leah-sync._tcp, got %q", ServiceType)
	}
}

func TestPeerStatusString(t *testing.T) {
	cases := map[PeerStatus]string{
		StatusOnline:      "online",
		StatusIdle:        "idle",
		StatusPaused:      "paused",
		StatusUnreachable: "unreachable",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("PeerStatus(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}
