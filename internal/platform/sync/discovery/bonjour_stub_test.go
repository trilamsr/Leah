//go:build !darwin

package discovery

import (
	"context"
	"testing"
	"time"
)

// Round-trip exercises publish→browse via the stub registry. Darwin builds
// route Publish through the cgo bridge and exercise it via the daemon
// integration smoke; the in-memory contract lives here so CI Linux always
// covers the publish→browse path.
func TestPublishBrowseRoundTrip(t *testing.T) {
	d := &stubDiscovery{records: map[DeviceID]peerRecord{}}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := d.Publish(ctx, "mac-a", 51820); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	ch, err := d.Browse(ctx)
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	select {
	case p := <-ch:
		if p.ID() != "mac-a" {
			t.Fatalf("Peer.ID = %q, want mac-a", p.ID())
		}
		if p.Endpoint().Port() != 51820 {
			t.Fatalf("Peer.Endpoint().Port = %d, want 51820", p.Endpoint().Port())
		}
		if p.Status() != StatusOnline {
			t.Fatalf("Peer.Status = %v, want online", p.Status())
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Browse: no peer received within 200ms")
	}
}

func TestPublishNotifiesActiveBrowser(t *testing.T) {
	d := &stubDiscovery{records: map[DeviceID]peerRecord{}}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	ch, err := d.Browse(ctx)
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if err := d.Publish(ctx, "mac-b", 51821); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case p := <-ch:
		if p.ID() != "mac-b" {
			t.Fatalf("expected mac-b, got %q", p.ID())
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Publish-after-Browse failed to notify subscriber")
	}
}
