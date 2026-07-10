package supervisor

import (
	"testing"
	"time"
)

func TestLeak_WarnsAt5MBPerMinFor10Min(t *testing.T) {
	d := newLeakDetector(leakConfig{
		WarnSlopeMBPerMin:  5,
		WarnSustain:        10 * time.Minute,
		EvictSlopeMBPerMin: 20,
		EvictSustain:       5 * time.Minute,
		SampleWindow:       10 * time.Minute,
	})
	base := time.Unix(0, 0)
	// 11 samples over 10 min, growing ~6 MB/min from 100 MB.
	for i := 0; i <= 20; i++ {
		at := base.Add(time.Duration(i) * 30 * time.Second)
		rss := 100 + 3*i // +6 MB/min
		d.observe("voice-stt", rss, at)
	}
	v := d.classify("voice-stt", base.Add(10*time.Minute))
	if v != leakWarn {
		t.Fatalf("want leakWarn, got %v", v)
	}
}

func TestLeak_EvictsAt20MBPerMinFor5Min(t *testing.T) {
	d := newLeakDetector(leakConfig{
		WarnSlopeMBPerMin:  5,
		WarnSustain:        10 * time.Minute,
		EvictSlopeMBPerMin: 20,
		EvictSustain:       5 * time.Minute,
		SampleWindow:       10 * time.Minute,
	})
	base := time.Unix(0, 0)
	for i := 0; i <= 10; i++ {
		at := base.Add(time.Duration(i) * 30 * time.Second)
		rss := 100 + 12*i // +24 MB/min
		d.observe("vision-live", rss, at)
	}
	v := d.classify("vision-live", base.Add(5*time.Minute))
	if v != leakEvict {
		t.Fatalf("want leakEvict, got %v", v)
	}
}

func TestLeak_FlatRSSIsHealthy(t *testing.T) {
	d := newLeakDetector(leakConfig{
		WarnSlopeMBPerMin:  5,
		WarnSustain:        10 * time.Minute,
		EvictSlopeMBPerMin: 20,
		EvictSustain:       5 * time.Minute,
		SampleWindow:       10 * time.Minute,
	})
	base := time.Unix(0, 0)
	for i := 0; i <= 30; i++ {
		at := base.Add(time.Duration(i) * 30 * time.Second)
		d.observe("daemon", 200, at) // flat
	}
	v := d.classify("daemon", base.Add(15*time.Minute))
	if v != leakHealthy {
		t.Fatalf("want leakHealthy, got %v", v)
	}
}

func TestLeak_NotEnoughSamplesIsHealthy(t *testing.T) {
	d := newLeakDetector(leakConfig{
		WarnSlopeMBPerMin:  5,
		WarnSustain:        10 * time.Minute,
		EvictSlopeMBPerMin: 20,
		EvictSustain:       5 * time.Minute,
		SampleWindow:       10 * time.Minute,
	})
	base := time.Unix(0, 0)
	d.observe("p", 100, base)
	d.observe("p", 200, base.Add(30*time.Second))
	if v := d.classify("p", base.Add(time.Minute)); v != leakHealthy {
		t.Fatalf("short series should not classify, got %v", v)
	}
}
