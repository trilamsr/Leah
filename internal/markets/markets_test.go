package markets

import (
	"encoding/json"
	"testing"
)

func TestQuoteJSONRoundTrip(t *testing.T) {
	q := Quote{Symbol: "AAPL", Price: "213.40", Change: "1.20", ChangePct: "0.5655%", AsOf: "2026-06-22"}
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Quote
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != q {
		t.Fatalf("round-trip mismatch: got %+v want %+v", out, q)
	}
}

func TestOptionsZeroValueDefaults(t *testing.T) {
	var o Options
	if o.Endpoint != "" || o.APIKey != "" {
		t.Fatalf("zero Options expected empty, got %+v", o)
	}
}
