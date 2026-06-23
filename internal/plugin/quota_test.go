package plugin

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/pkg/leahplugin"
)

func TestQuotaMeter_RPCCapHolds(t *testing.T) {
	now := atomic.Int64{}
	now.Store(time.Now().Unix())
	q := NewQuotaMeter(func() time.Time { return time.Unix(now.Load(), 0) })
	id := leahplugin.PluginID("p1")
	for i := 0; i < 60; i++ {
		if err := q.ChargeRPC(id); err != nil {
			t.Fatalf("rpc %d: %v", i, err)
		}
	}
	if err := q.ChargeRPC(id); !errors.Is(err, ErrOverQuota) {
		t.Fatalf("rpc 61: want ErrOverQuota, got %v", err)
	}
	// still paused next call
	if err := q.ChargeRPC(id); !errors.Is(err, ErrOverQuota) {
		t.Fatalf("rpc 62 (paused): want ErrOverQuota, got %v", err)
	}
}

func TestQuotaMeter_WindowRollover(t *testing.T) {
	now := atomic.Int64{}
	now.Store(1000)
	q := NewQuotaMeter(func() time.Time { return time.Unix(now.Load(), 0) })
	id := leahplugin.PluginID("p1")
	for i := 0; i < 60; i++ {
		_ = q.ChargeRPC(id)
	}
	if err := q.ChargeRPC(id); !errors.Is(err, ErrOverQuota) {
		t.Fatalf("want over-quota at 61, got %v", err)
	}
	// advance past pause + window
	now.Store(1000 + 121)
	if err := q.ChargeRPC(id); err != nil {
		t.Fatalf("after rollover: %v", err)
	}
}

func TestQuotaMeter_BytesCapHolds(t *testing.T) {
	now := atomic.Int64{}
	now.Store(time.Now().Unix())
	q := NewQuotaMeter(func() time.Time { return time.Unix(now.Load(), 0) })
	id := leahplugin.PluginID("p1")
	q.Configure(id, leahplugin.Quota{RPCPerMinute: 60, StreamBytesPerMinute: 1 << 20})
	if err := q.ChargeBytes(id, 1<<20); err != nil {
		t.Fatalf("1MB: %v", err)
	}
	if err := q.ChargeBytes(id, 1); !errors.Is(err, ErrOverQuota) {
		t.Fatalf("1MB+1: want ErrOverQuota, got %v", err)
	}
}

func TestQuotaMeter_ConfigureCustomLimit(t *testing.T) {
	q := NewQuotaMeter(time.Now)
	id := leahplugin.PluginID("p1")
	q.Configure(id, leahplugin.Quota{RPCPerMinute: 2, StreamBytesPerMinute: 100})
	if err := q.ChargeRPC(id); err != nil {
		t.Fatal(err)
	}
	if err := q.ChargeRPC(id); err != nil {
		t.Fatal(err)
	}
	if err := q.ChargeRPC(id); !errors.Is(err, ErrOverQuota) {
		t.Fatalf("want ErrOverQuota at 3rd RPC, got %v", err)
	}
}

func TestQuotaMeter_ConfigureFallsBackToDefaultOnZero(t *testing.T) {
	q := NewQuotaMeter(time.Now)
	id := leahplugin.PluginID("p1")
	q.Configure(id, leahplugin.Quota{}) // both zero
	for i := 0; i < 60; i++ {
		if err := q.ChargeRPC(id); err != nil {
			t.Fatalf("rpc %d hit zero-config quota, expected default 60: %v", i, err)
		}
	}
}

func TestQuotaMeter_ConcurrentRPCNeverExceedsCap(t *testing.T) {
	q := NewQuotaMeter(time.Now)
	id := leahplugin.PluginID("p1")
	var ok atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := q.ChargeRPC(id); err == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 60 {
		t.Fatalf("got %d accepted RPCs under race, want 60", ok.Load())
	}
}

func TestQuotaMeter_ChargeBytesIgnoresNonPositive(t *testing.T) {
	q := NewQuotaMeter(time.Now)
	id := leahplugin.PluginID("p1")
	if err := q.ChargeBytes(id, 0); err != nil {
		t.Fatalf("0 bytes: %v", err)
	}
	if err := q.ChargeBytes(id, -10); err != nil {
		t.Fatalf("neg bytes: %v", err)
	}
}
