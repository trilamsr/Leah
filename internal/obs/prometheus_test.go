package obs

import (
	"bytes"
	"strings"
	"testing"
)

// TestPrometheus_CounterEmission: counter with labels emits TYPE+series.
func TestPrometheus_CounterEmission(t *testing.T) {
	r := NewRegistry()
	r.Counter("leah_requests_total").Add(map[string]string{"path": "/api"}, 7)
	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "# TYPE leah_requests_total counter") {
		t.Errorf("missing TYPE line: %s", out)
	}
	if !strings.Contains(out, `leah_requests_total{path="/api"} 7`) {
		t.Errorf("missing series: %s", out)
	}
}

// TestPrometheus_CounterNoLabels: counter without labels emits bare series.
func TestPrometheus_CounterNoLabels(t *testing.T) {
	r := NewRegistry()
	r.Counter("leah_starts_total").Add(nil, 3)
	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	if !strings.Contains(buf.String(), "leah_starts_total 3") {
		t.Errorf("missing bare series: %s", buf.String())
	}
}

// TestPrometheus_GaugeEmission: gauge emits TYPE gauge + float value.
func TestPrometheus_GaugeEmission(t *testing.T) {
	r := NewRegistry()
	r.Gauge("leah_queue_depth").Set(map[string]string{"q": "audit"}, 12.5)
	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "# TYPE leah_queue_depth gauge") {
		t.Errorf("missing TYPE gauge: %s", out)
	}
	if !strings.Contains(out, `leah_queue_depth{q="audit"} 12.5`) {
		t.Errorf("missing gauge series: %s", out)
	}
}

// TestPrometheus_HistogramEmission: emits bucket, sum, count lines.
func TestPrometheus_HistogramEmission(t *testing.T) {
	r := NewRegistry()
	h := r.Histogram("leah_latency_seconds", []float64{0.1, 1, 10})
	h.Observe(nil, 0.05)
	h.Observe(nil, 5)
	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"# TYPE leah_latency_seconds histogram",
		`leah_latency_seconds_bucket{le="0.1"} 1`,
		`leah_latency_seconds_bucket{le="1"} 1`,
		`leah_latency_seconds_bucket{le="10"} 2`,
		`leah_latency_seconds_bucket{le="+Inf"} 2`,
		"leah_latency_seconds_sum ",
		"leah_latency_seconds_count 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestPrometheus_NameEscaping: dots/dashes in names become underscores.
func TestPrometheus_NameEscaping(t *testing.T) {
	r := NewRegistry()
	r.Counter("leah.foo-bar").Add(nil, 1)
	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	if !strings.Contains(buf.String(), "leah_foo_bar 1") {
		t.Errorf("name not escaped: %s", buf.String())
	}
}

// TestPrometheus_LabelEscaping: quote and backslash in label values get escaped.
func TestPrometheus_LabelEscaping(t *testing.T) {
	r := NewRegistry()
	r.Counter("leah_msgs").Add(map[string]string{"msg": `he said "hi"`}, 1)
	var buf bytes.Buffer
	if err := r.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	if !strings.Contains(buf.String(), `msg="he said \"hi\""`) {
		t.Errorf("label not escaped: %s", buf.String())
	}
}

// TestPrometheus_Deterministic: same registry → identical output across calls.
func TestPrometheus_Deterministic(t *testing.T) {
	r := NewRegistry()
	r.Counter("a_total").Add(map[string]string{"x": "1"}, 1)
	r.Counter("b_total").Add(map[string]string{"y": "2"}, 1)
	r.Gauge("g").Set(nil, 3)
	var b1, b2 bytes.Buffer
	_ = r.WritePrometheus(&b1)
	_ = r.WritePrometheus(&b2)
	if b1.String() != b2.String() {
		t.Errorf("non-deterministic output:\n%s\n---\n%s", b1.String(), b2.String())
	}
}
