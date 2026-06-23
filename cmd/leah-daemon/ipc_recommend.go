package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/learn"
)

// defaultRecommendBatch caps NextBatch when the HUD omits maxN — keeps the wire
// payload small without forcing the pane to know the §3.4 pacing window.
const defaultRecommendBatch = 5

type recommendListPayload struct {
	Surface string `json:"surface"`
	MaxN    int    `json:"maxN"`
}

type recommendApplyPayload struct {
	ID          int64 `json:"id"`
	OutcomeKind int   `json:"outcome_kind"`
	LatencyMS   int   `json:"latency_ms"`
}

type recommendDismissPayload struct {
	ID        int64 `json:"id"`
	LatencyMS int   `json:"latency_ms"`
}

type recommendAntiAddPayload struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
	Source string `json:"source"`
}

type recommendWire struct {
	ID         int64   `json:"id"`
	Kind       string  `json:"kind"`
	Body       string  `json:"body"`
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`
	ActionRef  string  `json:"action_ref"`
	SurfacedAt int64   `json:"surfaced_at"`
	ExpiresAt  int64   `json:"expires_at"`
}

type antiRuleWire struct {
	Kind    string `json:"kind"`
	Reason  string `json:"reason"`
	Source  string `json:"source"`
	AddedAt int64  `json:"added_at"`
}

// handleRecommendList drives NextBatch — pacing caps live in the recommender,
// so a saturated day legitimately returns an empty array (NOT an error frame).
func handleRecommendList(ctx context.Context, req ipc.Frame, rec learn.Recommender) (<-chan ipc.Frame, error) {
	if rec == nil {
		return errFrame(req, "recommender unavailable"), nil
	}
	var p recommendListPayload
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errFrame(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.MaxN <= 0 {
		p.MaxN = defaultRecommendBatch
	}
	surface := learn.Surface(p.Surface)
	if surface == "" {
		surface = learn.SurfaceCoachCard
	}
	batch, err := rec.NextBatch(ctx, surface, p.MaxN)
	if err != nil {
		return errFrame(req, fmt.Sprintf("recommend.list: %v", err)), nil
	}
	wire := make([]recommendWire, len(batch))
	for i, r := range batch {
		wire[i] = recommendWire{
			ID:         int64(r.ID),
			Kind:       string(r.Kind),
			Body:       r.Body,
			Score:      r.Score,
			Confidence: r.Confidence,
			ActionRef:  string(r.Action),
			SurfacedAt: r.SurfacedAt.Unix(),
			ExpiresAt:  r.ExpiresAt.Unix(),
		}
	}
	payload, _ := json.Marshal(map[string]any{"recommendations": wire})
	return oneFrame(ipc.KindRecommendList, req, payload), nil
}

func handleRecommendApply(ctx context.Context, req ipc.Frame, rec learn.Recommender) (<-chan ipc.Frame, error) {
	if rec == nil {
		return errFrame(req, "recommender unavailable"), nil
	}
	var p recommendApplyPayload
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errFrame(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.ID == 0 {
		return errFrame(req, "id required"), nil
	}
	// Default outcome = Applied so the HUD's "I did it" button doesn't need
	// to know the enum; Accepted is still reachable via explicit outcome_kind.
	kind := learn.Applied
	if p.OutcomeKind != 0 {
		kind = learn.OutcomeKind(p.OutcomeKind)
	}
	if err := rec.Record(ctx, learn.RecommendationID(p.ID), learn.Outcome{Kind: kind, LatencyMS: p.LatencyMS}); err != nil {
		return errFrame(req, fmt.Sprintf("recommend.apply: %v", err)), nil
	}
	payload, _ := json.Marshal(map[string]bool{"ok": true})
	return oneFrame(ipc.KindRecommendApply, req, payload), nil
}

func handleRecommendDismiss(ctx context.Context, req ipc.Frame, rec learn.Recommender) (<-chan ipc.Frame, error) {
	if rec == nil {
		return errFrame(req, "recommender unavailable"), nil
	}
	var p recommendDismissPayload
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errFrame(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.ID == 0 {
		return errFrame(req, "id required"), nil
	}
	if err := rec.Record(ctx, learn.RecommendationID(p.ID), learn.Outcome{Kind: learn.Dismissed, LatencyMS: p.LatencyMS}); err != nil {
		return errFrame(req, fmt.Sprintf("recommend.dismiss: %v", err)), nil
	}
	payload, _ := json.Marshal(map[string]bool{"ok": true})
	return oneFrame(ipc.KindRecommendDismiss, req, payload), nil
}

func handleRecommendAntiAdd(ctx context.Context, req ipc.Frame, rec learn.Recommender) (<-chan ipc.Frame, error) {
	if rec == nil {
		return errFrame(req, "recommender unavailable"), nil
	}
	var p recommendAntiAddPayload
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errFrame(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.Kind == "" {
		return errFrame(req, "kind required"), nil
	}
	if err := rec.AntiAdd(ctx, learn.RecommendKind(p.Kind), p.Reason); err != nil {
		return errFrame(req, fmt.Sprintf("recommend.anti.add: %v", err)), nil
	}
	payload, _ := json.Marshal(map[string]bool{"ok": true})
	return oneFrame(ipc.KindRecommendAntiAdd, req, payload), nil
}

func handleRecommendAntiList(ctx context.Context, req ipc.Frame, rec learn.Recommender) (<-chan ipc.Frame, error) {
	if rec == nil {
		return errFrame(req, "recommender unavailable"), nil
	}
	rules, err := rec.AntiList(ctx)
	if err != nil {
		return errFrame(req, fmt.Sprintf("recommend.anti.list: %v", err)), nil
	}
	wire := make([]antiRuleWire, len(rules))
	for i, r := range rules {
		wire[i] = antiRuleWire{
			Kind:    string(r.Kind),
			Reason:  r.Reason,
			Source:  string(r.Source),
			AddedAt: r.AddedAt.Unix(),
		}
	}
	payload, _ := json.Marshal(map[string]any{"rules": wire})
	return oneFrame(ipc.KindRecommendAntiList, req, payload), nil
}

func oneFrame(kind string, req ipc.Frame, payload json.RawMessage) <-chan ipc.Frame {
	out := make(chan ipc.Frame, 1)
	out <- ipc.Frame{Kind: kind, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out
}
