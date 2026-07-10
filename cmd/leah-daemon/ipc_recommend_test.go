package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/ipc"
	"github.com/trilam/leah/internal/thinking/learn"
)

type stubRecommender struct {
	nextBatch     []learn.Recommendation
	nextErr       error
	recordErr     error
	antiAddErr    error
	antiListRules []learn.AntiRule
	antiListErr   error

	gotSurface    learn.Surface
	gotMaxN       int
	gotRecordID   learn.RecommendationID
	gotOutcome    learn.Outcome
	gotAntiKind   learn.RecommendKind
	gotAntiReason string
}

func (s *stubRecommender) Observe(_ context.Context, _ learn.Observation) error { return nil }

func (s *stubRecommender) NextBatch(_ context.Context, surface learn.Surface, maxN int) ([]learn.Recommendation, error) {
	s.gotSurface, s.gotMaxN = surface, maxN
	return s.nextBatch, s.nextErr
}

func (s *stubRecommender) Record(_ context.Context, id learn.RecommendationID, out learn.Outcome) error {
	s.gotRecordID, s.gotOutcome = id, out
	return s.recordErr
}

func (s *stubRecommender) AntiAdd(_ context.Context, kind learn.RecommendKind, reason string) error {
	s.gotAntiKind, s.gotAntiReason = kind, reason
	return s.antiAddErr
}

func (s *stubRecommender) AntiList(_ context.Context) ([]learn.AntiRule, error) {
	return s.antiListRules, s.antiListErr
}

func recommendDrain(ch <-chan ipc.Frame) []ipc.Frame {
	var out []ipc.Frame
	for f := range ch {
		out = append(out, f)
	}
	return out
}

func TestHandleRecommendList_ReturnsBatchAsSingleFrame(t *testing.T) {
	now := time.Unix(1700000000, 0)
	sr := &stubRecommender{nextBatch: []learn.Recommendation{
		{ID: 7, Kind: "wake_volume_down", Body: "lower wake mic gain", Score: 0.82, Confidence: 0.61, Action: "wake.gain.down", SurfacedAt: now, ExpiresAt: now.Add(24 * time.Hour)},
		{ID: 8, Kind: "tts_voice_swap", Body: "try Apple Ava", Score: 0.55, Confidence: 0.40},
	}}
	body, _ := json.Marshal(map[string]any{"surface": "coach", "maxN": 5})
	req := ipc.Frame{Kind: ipc.KindRecommendList, TurnID: "t1", Seq: 1, Payload: body}

	ch, err := handleRecommendList(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendList: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindRecommendList {
		t.Fatalf("expected single recommend.list frame; got %+v", got)
	}
	if sr.gotSurface != learn.SurfaceCoachCard || sr.gotMaxN != 5 {
		t.Fatalf("bad inputs: surface=%q maxN=%d", sr.gotSurface, sr.gotMaxN)
	}
	var resp struct {
		Recommendations []struct {
			ID         int64   `json:"id"`
			Kind       string  `json:"kind"`
			Body       string  `json:"body"`
			Score      float64 `json:"score"`
			Confidence float64 `json:"confidence"`
			ActionRef  string  `json:"action_ref"`
			SurfacedAt int64   `json:"surfaced_at"`
			ExpiresAt  int64   `json:"expires_at"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(got[0].Payload, &resp); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(resp.Recommendations) != 2 {
		t.Fatalf("expected 2 recommendations, got %d", len(resp.Recommendations))
	}
	r0 := resp.Recommendations[0]
	if r0.ID != 7 || r0.Kind != "wake_volume_down" || r0.Body != "lower wake mic gain" || r0.ActionRef != "wake.gain.down" {
		t.Fatalf("rec0 fields mismatch: %+v", r0)
	}
	if r0.SurfacedAt != now.Unix() || r0.ExpiresAt != now.Add(24*time.Hour).Unix() {
		t.Fatalf("rec0 timestamps mismatch: surfaced=%d expires=%d", r0.SurfacedAt, r0.ExpiresAt)
	}
}

func TestHandleRecommendList_NilPayloadDefaults(t *testing.T) {
	sr := &stubRecommender{}
	req := ipc.Frame{Kind: ipc.KindRecommendList, TurnID: "t1", Seq: 1}
	ch, err := handleRecommendList(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendList: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindRecommendList {
		t.Fatalf("expected single frame; got %+v", got)
	}
	if sr.gotMaxN <= 0 {
		t.Fatalf("expected default maxN > 0, got %d", sr.gotMaxN)
	}
}

func TestHandleRecommendList_BadPayloadEmitsError(t *testing.T) {
	req := ipc.Frame{Kind: ipc.KindRecommendList, TurnID: "t1", Seq: 1, Payload: json.RawMessage(`{not json`)}
	ch, err := handleRecommendList(context.Background(), req, &stubRecommender{})
	if err != nil {
		t.Fatalf("handleRecommendList: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected error frame; got %+v", got)
	}
}

func TestHandleRecommendList_RecommenderErrorIsErrFrame(t *testing.T) {
	sr := &stubRecommender{nextErr: errors.New("db boom")}
	body, _ := json.Marshal(map[string]any{"surface": "notification", "maxN": 3})
	req := ipc.Frame{Kind: ipc.KindRecommendList, TurnID: "t1", Seq: 1, Payload: body}
	ch, err := handleRecommendList(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendList: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected error frame; got %+v", got)
	}
}

func TestHandleRecommendList_NilRecommenderIsErrFrame(t *testing.T) {
	req := ipc.Frame{Kind: ipc.KindRecommendList, TurnID: "t1", Seq: 1}
	ch, err := handleRecommendList(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("handleRecommendList: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected error frame; got %+v", got)
	}
}

func TestHandleRecommendApply_RecordsAppliedOutcome(t *testing.T) {
	sr := &stubRecommender{}
	body, _ := json.Marshal(map[string]any{"id": 42, "outcome_kind": int(learn.Applied), "latency_ms": 1234})
	req := ipc.Frame{Kind: ipc.KindRecommendApply, TurnID: "t1", Seq: 1, Payload: body}
	ch, err := handleRecommendApply(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendApply: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindRecommendApply {
		t.Fatalf("expected recommend.apply frame; got %+v", got)
	}
	if sr.gotRecordID != 42 || sr.gotOutcome.Kind != learn.Applied || sr.gotOutcome.LatencyMS != 1234 {
		t.Fatalf("record args mismatch: id=%d kind=%v latency=%d", sr.gotRecordID, sr.gotOutcome.Kind, sr.gotOutcome.LatencyMS)
	}
}

func TestHandleRecommendApply_DefaultsToAppliedKind(t *testing.T) {
	sr := &stubRecommender{}
	body, _ := json.Marshal(map[string]any{"id": 9})
	req := ipc.Frame{Kind: ipc.KindRecommendApply, TurnID: "t1", Seq: 1, Payload: body}
	ch, err := handleRecommendApply(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendApply: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindRecommendApply {
		t.Fatalf("expected recommend.apply frame; got %+v", got)
	}
	if sr.gotOutcome.Kind != learn.Applied {
		t.Fatalf("default outcome kind should be Applied, got %v", sr.gotOutcome.Kind)
	}
}

func TestHandleRecommendApply_RecordErrorEmitsError(t *testing.T) {
	sr := &stubRecommender{recordErr: errors.New("write fail")}
	body, _ := json.Marshal(map[string]any{"id": 1})
	req := ipc.Frame{Kind: ipc.KindRecommendApply, TurnID: "t1", Seq: 1, Payload: body}
	ch, err := handleRecommendApply(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendApply: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected error frame; got %+v", got)
	}
}

func TestHandleRecommendDismiss_RecordsDismissed(t *testing.T) {
	sr := &stubRecommender{}
	body, _ := json.Marshal(map[string]any{"id": 11, "latency_ms": 800})
	req := ipc.Frame{Kind: ipc.KindRecommendDismiss, TurnID: "t1", Seq: 1, Payload: body}
	ch, err := handleRecommendDismiss(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendDismiss: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindRecommendDismiss {
		t.Fatalf("expected recommend.dismiss frame; got %+v", got)
	}
	if sr.gotRecordID != 11 || sr.gotOutcome.Kind != learn.Dismissed || sr.gotOutcome.LatencyMS != 800 {
		t.Fatalf("record args mismatch: id=%d kind=%v latency=%d", sr.gotRecordID, sr.gotOutcome.Kind, sr.gotOutcome.LatencyMS)
	}
}

func TestHandleRecommendAntiAdd_PropagatesKindAndReason(t *testing.T) {
	sr := &stubRecommender{}
	body, _ := json.Marshal(map[string]any{"kind": "tts_voice_swap", "reason": "user vetoed"})
	req := ipc.Frame{Kind: ipc.KindRecommendAntiAdd, TurnID: "t1", Seq: 1, Payload: body}
	ch, err := handleRecommendAntiAdd(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendAntiAdd: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindRecommendAntiAdd {
		t.Fatalf("expected recommend.anti.add frame; got %+v", got)
	}
	if sr.gotAntiKind != "tts_voice_swap" || sr.gotAntiReason != "user vetoed" {
		t.Fatalf("anti args mismatch: kind=%q reason=%q", sr.gotAntiKind, sr.gotAntiReason)
	}
}

func TestHandleRecommendAntiAdd_EmptyKindIsErrFrame(t *testing.T) {
	sr := &stubRecommender{}
	body, _ := json.Marshal(map[string]any{"kind": "", "reason": "x"})
	req := ipc.Frame{Kind: ipc.KindRecommendAntiAdd, TurnID: "t1", Seq: 1, Payload: body}
	ch, err := handleRecommendAntiAdd(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendAntiAdd: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindError {
		t.Fatalf("expected error frame for empty kind; got %+v", got)
	}
}

func TestHandleRecommendAntiList_ReturnsRules(t *testing.T) {
	now := time.Unix(1700000000, 0)
	sr := &stubRecommender{antiListRules: []learn.AntiRule{
		{Kind: "wake_volume_down", Reason: "spec", Source: learn.AntiSpec, AddedAt: now},
		{Kind: "tts_voice_swap", Reason: "user vetoed", Source: learn.AntiOperator, AddedAt: now.Add(time.Hour)},
	}}
	req := ipc.Frame{Kind: ipc.KindRecommendAntiList, TurnID: "t1", Seq: 1}
	ch, err := handleRecommendAntiList(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendAntiList: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindRecommendAntiList {
		t.Fatalf("expected recommend.anti.list frame; got %+v", got)
	}
	var resp struct {
		Rules []struct {
			Kind    string `json:"kind"`
			Reason  string `json:"reason"`
			Source  string `json:"source"`
			AddedAt int64  `json:"added_at"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(got[0].Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(resp.Rules))
	}
	if resp.Rules[0].Source != "spec" || resp.Rules[1].Source != "operator" {
		t.Fatalf("source mismatch: %+v", resp.Rules)
	}
	if resp.Rules[0].AddedAt != now.Unix() {
		t.Fatalf("added_at mismatch: %d vs %d", resp.Rules[0].AddedAt, now.Unix())
	}
}

func TestHandleRecommendAntiList_EmptyRulesReturnsEmptyArray(t *testing.T) {
	sr := &stubRecommender{}
	req := ipc.Frame{Kind: ipc.KindRecommendAntiList, TurnID: "t1", Seq: 1}
	ch, err := handleRecommendAntiList(context.Background(), req, sr)
	if err != nil {
		t.Fatalf("handleRecommendAntiList: %v", err)
	}
	got := recommendDrain(ch)
	if len(got) != 1 || got[0].Kind != ipc.KindRecommendAntiList {
		t.Fatalf("expected anti.list frame; got %+v", got)
	}
	// Payload should parse as {"rules":[]} or {"rules":null}; both acceptable.
	var resp struct {
		Rules []learn.AntiRule `json:"rules"`
	}
	if err := json.Unmarshal(got[0].Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Rules) != 0 {
		t.Fatalf("expected empty rules, got %d", len(resp.Rules))
	}
}
