package eval

import "testing"

func TestBootstrap_ReasonerJSONL_Loads(t *testing.T) {
	tr, err := loadTraces("../../evals/reasoner.jsonl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tr) < 5 {
		t.Fatalf("want ≥5 bootstrap traces, got %d", len(tr))
	}
	for _, x := range tr {
		if x.Feature != "reasoner" {
			t.Errorf("%s: feature=%q want reasoner", x.ID, x.Feature)
		}
	}
}
