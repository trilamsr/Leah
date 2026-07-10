package telemetry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/obs"
)

func newTestTracer(t *testing.T, rate float64) telemetry.Tracer {
	t.Helper()
	tr, err := telemetry.New(telemetry.Config{SampleRate: rate})
	if err != nil {
		t.Fatalf("obs.New: %v", err)
	}
	t.Cleanup(func() { _ = tr.Shutdown(context.Background()) })
	return tr
}

func TestStartSpan_RecordsSpan(t *testing.T) {
	tr := newTestTracer(t, 1.0)
	ctx := context.Background()
	sp, _ := tr.StartSpan(ctx, "unit.start")
	sp.SetAttribute(telemetry.RefIDAttr, "ref-start")
	sp.End()
	if err := tr.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	got, err := tr.Query(ctx, "ref-start")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 || got[0].Name != "unit.start" {
		t.Fatalf("want 1 span named unit.start, got %#v", got)
	}
}

func TestSpan_SetAttribute_StoresKV(t *testing.T) {
	tr := newTestTracer(t, 1.0)
	ctx := context.Background()
	sp, _ := tr.StartSpan(ctx, "unit.attr")
	sp.SetAttribute(telemetry.RefIDAttr, "ref-attr")
	sp.SetAttribute("leah.outcome", "ok")
	sp.SetAttribute("leah.count", 7)
	sp.End()
	_ = tr.Flush(ctx)
	got, _ := tr.Query(ctx, "ref-attr")
	if len(got) != 1 {
		t.Fatalf("want 1 span, got %d", len(got))
	}
	if got[0].Attributes["leah.outcome"] != "ok" {
		t.Fatalf("outcome attr lost: %#v", got[0].Attributes)
	}
	if got[0].Attributes["leah.count"] != "7" {
		t.Fatalf("count attr lost: %#v", got[0].Attributes)
	}
}

func TestSpan_End_FlushesToCollector(t *testing.T) {
	tr := newTestTracer(t, 1.0)
	ctx := context.Background()
	sp, _ := tr.StartSpan(ctx, "unit.flush")
	sp.SetAttribute(telemetry.RefIDAttr, "ref-flush")
	if pre, _ := tr.Query(ctx, "ref-flush"); len(pre) != 0 {
		t.Fatalf("collector saw span before End: %#v", pre)
	}
	sp.End()
	_ = tr.Flush(ctx)
	got, _ := tr.Query(ctx, "ref-flush")
	if len(got) != 1 {
		t.Fatalf("want 1 span after End, got %d", len(got))
	}
}

func TestCollector_QueryByRefID_ReturnsSpanTree(t *testing.T) {
	tr := newTestTracer(t, 1.0)
	ctx := context.Background()
	parent, childCtx := tr.StartSpan(ctx, "tree.parent")
	parent.SetAttribute(telemetry.RefIDAttr, "ref-tree")
	child, grandCtx := tr.StartSpan(childCtx, "tree.child")
	grand, _ := tr.StartSpan(grandCtx, "tree.grand")
	grand.End()
	child.End()
	parent.End()
	_ = tr.Flush(ctx)
	got, err := tr.Query(ctx, "ref-tree")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 spans, got %d", len(got))
	}
	names := map[string]bool{}
	traceID := got[0].TraceID
	for _, s := range got {
		names[s.Name] = true
		if s.TraceID != traceID {
			t.Fatalf("span %q on different trace %q vs %q", s.Name, s.TraceID, traceID)
		}
	}
	for _, want := range []string{"tree.parent", "tree.child", "tree.grand"} {
		if !names[want] {
			t.Fatalf("missing %q in tree: %v", want, names)
		}
	}
}

func TestSpan_RecordError_SetsErrorStatus(t *testing.T) {
	tr := newTestTracer(t, 1.0)
	ctx := context.Background()
	sp, _ := tr.StartSpan(ctx, "unit.err")
	sp.SetAttribute(telemetry.RefIDAttr, "ref-err")
	sp.RecordError(errors.New("boom"))
	sp.End()
	_ = tr.Flush(ctx)
	got, _ := tr.Query(ctx, "ref-err")
	if len(got) != 1 || got[0].Status != "Error" || got[0].StatusMsg != "boom" {
		t.Fatalf("error status lost: %#v", got)
	}
}

func TestSampleRate_50pct_HalfSampled(t *testing.T) {
	tr := newTestTracer(t, 0.5)
	ctx := context.Background()
	const n = 400
	sampled := 0
	for i := 0; i < n; i++ {
		sp, _ := tr.StartSpan(ctx, "unit.rate")
		refID := refIDFor(i)
		sp.SetAttribute(telemetry.RefIDAttr, refID)
		sp.End()
		_ = tr.Flush(ctx)
		if got, _ := tr.Query(ctx, refID); len(got) == 1 {
			sampled++
		}
	}
	if sampled < n/4 || sampled > 3*n/4 {
		t.Fatalf("0.5 rate produced %d / %d sampled — outside [25%%, 75%%] band", sampled, n)
	}
}

func TestSampleRate_0_NoSample(t *testing.T) {
	tr := newTestTracer(t, 0.0)
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		sp, _ := tr.StartSpan(ctx, "unit.zero")
		sp.SetAttribute(telemetry.RefIDAttr, "ref-zero")
		sp.End()
	}
	_ = tr.Flush(ctx)
	if got, _ := tr.Query(ctx, "ref-zero"); len(got) != 0 {
		t.Fatalf("rate=0 still recorded %d spans", len(got))
	}
}

func TestNew_RejectsOutOfRangeRate(t *testing.T) {
	if _, err := telemetry.New(telemetry.Config{SampleRate: -0.1}); err == nil {
		t.Fatalf("want error for negative rate")
	}
	if _, err := telemetry.New(telemetry.Config{SampleRate: 1.5}); err == nil {
		t.Fatalf("want error for rate > 1")
	}
}

func TestCollector_MaxTraces_EvictsLRU(t *testing.T) {
	tr, err := telemetry.New(telemetry.Config{SampleRate: 1.0, MaxTraces: 3})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	t.Cleanup(func() { _ = tr.Shutdown(context.Background()) })
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		sp, _ := tr.StartSpan(ctx, "lru")
		sp.SetAttribute(telemetry.RefIDAttr, refIDFor(i))
		sp.End()
	}
	_ = tr.Flush(ctx)
	for i := 0; i < 2; i++ {
		if got, _ := tr.Query(ctx, refIDFor(i)); len(got) != 0 {
			t.Fatalf("ref %s should have been evicted, got %d spans", refIDFor(i), len(got))
		}
	}
	for i := 2; i < 5; i++ {
		if got, _ := tr.Query(ctx, refIDFor(i)); len(got) != 1 {
			t.Fatalf("ref %s should be present, got %d spans", refIDFor(i), len(got))
		}
	}
}

func refIDFor(i int) string {
	return "ref-" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	neg := i < 0
	if neg {
		i = -i
	}
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	s := string(buf[pos:])
	if neg {
		s = "-" + s
	}
	return s
}
