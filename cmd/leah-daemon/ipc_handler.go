package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/knowledge"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/reasoner"
	"github.com/trilam/leah/internal/tts"
	"github.com/trilam/leah/internal/voice/duplex"
)

// errVerifyFailed is returned by pingFn when the API key is rejected.
var errVerifyFailed = errors.New("api key rejected")

// VisionIPC, SyncIPC, RecommendIPC, PluginIPC, A2AIPC are the optional
// dispatch seams. Concrete handlers live in sibling files (ipc_vision.go,
// ipc_sync.go, ipc_recommend.go, ipc_plugin.go, ipc_a2a.go) and are bound
// to these surfaces at composition time. Switch arms call methods on
// possibly-nil interfaces — the switch nil-checks so a daemon booted
// without (e.g.) sync still answers other kinds normally.
type VisionIPC interface {
	Snap(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	StreamStart(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	StreamFrame(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

type SyncIPC interface {
	PeerList(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PairStart(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PairAck(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

type RecommendIPC interface {
	List(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Apply(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Dismiss(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	AntiAdd(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	AntiList(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

type PluginIPC interface {
	List(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Install(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Enable(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Disable(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Uninstall(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	Logs(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

type A2AIPC interface {
	PeerList(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PairStart(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PeerPause(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
	PeerUnpair(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error)
}

// IPCDeps groups the optional dispatch seams so the production constructor's
// argument list does not balloon further. Nil fields are safe — the switch
// returns an error frame so the HUD sees a structured failure instead of a
// closed conn.
type IPCDeps struct {
	Vision    VisionIPC
	Sync      SyncIPC
	Recommend RecommendIPC
	Plugin    PluginIPC
	A2A       A2AIPC
}

// streamFn adapts the live reasoner (or a test stub) into ipc.Frame output.
// Each frame must be either "prose.delta" (with {"text":"..."} payload) or
// "turn.end" (with {} payload). The channel is always closed by the producer.
type streamFn func(ctx context.Context, turnID, userText string) (<-chan ipc.Frame, error)

// pingFn is called by the verify-key path; returns nil on success.
type pingFn func(ctx context.Context, key string) error

// classifyFn routes a user query to an Intent before the stream path fires.
type classifyFn func(ctx context.Context, text string) reasoner.Intent

// fetchFn retrieves top-k knowledge chunks for a query; nil return is safe.
type fetchFn func(ctx context.Context, query string, k int) ([]knowledge.Chunk, error)

// enrichFn resolves a citation URL against the KG; (nil, nil) is "no match".
// Errors degrade silently — the widget tile still mounts without enrichment.
type enrichFn func(ctx context.Context, citationURL string) (*knowledge.CitationEnrichment, error)

// newIPCHandlerWithDeps is the production constructor. Same as the pre-fold
// constructor plus the dispatch seams for sync / recommend / plugin / a2a /
// vision. Composition-root binds non-nil deps once their backing services
// boot.
func newIPCHandlerWithDeps(sonnet *reasoner.AnthropicClient, db *sql.DB, kg *knowledge.Graph, ring *obs.ErrorRing, ttsCloud, ttsLocal tts.Provider, ttsClass tts.Classifier, voiceSess duplex.DuplexSession, deps IPCDeps) ipc.Handler {
	return newIPCHandlerWithClassifyEnrichDeps(db,
		liveStreamFn(sonnet),
		liveOpusStreamFn(sonnet),
		liveClassifyFn(),
		livePingFn(),
		liveFetchFn(kg),
		liveEnrichFn(kg),
		time.Now(),
		ring,
		ttsCloud, ttsLocal, ttsClass,
		voiceSess,
		deps,
	)
}

// newIPCHandlerForTest wires a caller-supplied streamFn; used by tests that
// don't need to exercise classify or verify-key.
func newIPCHandlerForTest(db *sql.DB, s streamFn) ipc.Handler {
	noClassify := func(_ context.Context, _ string) reasoner.Intent { return reasoner.Intent{Kind: "chat"} }
	return newIPCHandlerWithClassifyEnrichDeps(db, s, s, noClassify,
		func(_ context.Context, _ string) error { return nil }, nil, nil, time.Time{}, nil, nil, nil, nil, nil, IPCDeps{})
}

// newIPCHandlerWithPingForTest is kept for existing verify-key tests.
func newIPCHandlerWithPingForTest(db *sql.DB, s streamFn, ping pingFn) ipc.Handler {
	noClassify := func(_ context.Context, _ string) reasoner.Intent { return reasoner.Intent{Kind: "chat"} }
	return newIPCHandlerWithClassifyEnrichDeps(db, s, s, noClassify, ping, nil, nil, time.Time{}, nil, nil, nil, nil, nil, IPCDeps{})
}

// newIPCHandlerWithDiag is kept for the diag_test.go test fixture.
func newIPCHandlerWithDiag(db *sql.DB, s streamFn, ping pingFn, startTime time.Time) ipc.Handler {
	noClassify := func(_ context.Context, _ string) reasoner.Intent { return reasoner.Intent{Kind: "chat"} }
	return newIPCHandlerWithClassifyEnrichDeps(db, s, s, noClassify, ping, nil, nil, startTime, nil, nil, nil, nil, nil, IPCDeps{})
}

// newIPCHandlerWithClassify is the pre-enrichment injection point — kept so
// existing tests continue to compile. Forwards to the enrichment-aware variant
// with a nil enricher (citation widgets degrade to the bare URL tile).
func newIPCHandlerWithClassify(
	db *sql.DB,
	sonnetStream, opusStream streamFn,
	classify classifyFn,
	ping pingFn,
	fetch fetchFn,
	startTime time.Time,
	ring *obs.ErrorRing,
	ttsCloud, ttsLocal tts.Provider,
	ttsClass tts.Classifier,
) ipc.Handler {
	return newIPCHandlerWithClassifyEnrichDeps(db, sonnetStream, opusStream, classify, ping, fetch, nil,
		startTime, ring, ttsCloud, ttsLocal, ttsClass, nil, IPCDeps{})
}

// newIPCHandlerWithClassifyEnrich is kept for the citation-enrichment test
// fixtures; forwards to the deps-aware variant with empty IPCDeps.
func newIPCHandlerWithClassifyEnrich(
	db *sql.DB,
	sonnetStream, opusStream streamFn,
	classify classifyFn,
	ping pingFn,
	fetch fetchFn,
	enrich enrichFn,
	startTime time.Time,
	ring *obs.ErrorRing,
	ttsCloud, ttsLocal tts.Provider,
	ttsClass tts.Classifier,
	voiceSess duplex.DuplexSession,
) ipc.Handler {
	return newIPCHandlerWithClassifyEnrichDeps(db, sonnetStream, opusStream, classify, ping, fetch, enrich,
		startTime, ring, ttsCloud, ttsLocal, ttsClass, voiceSess, IPCDeps{})
}

// newIPCHandlerWithClassifyEnrichDeps is the full injection point — all
// production wiring and dispatch tests route through here.
func newIPCHandlerWithClassifyEnrichDeps(
	db *sql.DB,
	sonnetStream, opusStream streamFn,
	classify classifyFn,
	ping pingFn,
	fetch fetchFn,
	enrich enrichFn,
	startTime time.Time,
	ring *obs.ErrorRing,
	ttsCloud, ttsLocal tts.Provider,
	ttsClass tts.Classifier,
	voiceSess duplex.DuplexSession,
	deps IPCDeps,
) ipc.Handler {
	reg := newTTSRegistry()
	return func(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
		// turn_id required for correlation across multiplexed conns.
		if req.TurnID == "" && req.Kind != ipc.KindDiag {
			return errFrame(req, "turn_id required"), nil
		}
		// seq must be non-negative — monotonic per turn.
		if req.Seq < 0 {
			return errFrame(req, fmt.Sprintf("seq must be >= 0, got %d", req.Seq)), nil
		}
		switch req.Kind {
		case ipc.KindAsk:
			return handleAsk(ctx, req, db, sonnetStream, opusStream, classify, fetch, enrich)
		case ipc.KindVerifyKey:
			return handleVerifyKey(ctx, req, ping)
		case ipc.KindDiag:
			var last string
			if ring != nil {
				last = ring.Last()
			}
			return ipc.HandleState(ctx, startTime, last, req.TurnID)
		case ipc.KindTTSSpeak:
			return handleTTSSpeak(ctx, req, ttsCloud, ttsLocal, ttsClass, reg)
		case ipc.KindTTSCancel:
			return handleTTSCancel(ctx, req, reg)
		case ipc.KindVoiceStart:
			return handleVoiceStart(ctx, req, voiceSess)
		case ipc.KindVoiceBarge:
			return handleVoiceBarge(ctx, req, voiceSess)
		case ipc.KindVoiceEnd:
			return handleVoiceEnd(ctx, req, voiceSess)

		case ipc.KindVisionSnap:
			return dispatchVision(ctx, req, deps.Vision, VisionIPC.Snap)
		case ipc.KindVisionStreamStart:
			return dispatchVision(ctx, req, deps.Vision, VisionIPC.StreamStart)
		case ipc.KindVisionStreamFrame:
			return dispatchVision(ctx, req, deps.Vision, VisionIPC.StreamFrame)

		case ipc.KindSyncPeerList:
			return dispatchSync(ctx, req, deps.Sync, SyncIPC.PeerList)
		case ipc.KindSyncPairStart:
			return dispatchSync(ctx, req, deps.Sync, SyncIPC.PairStart)
		case ipc.KindSyncPairAck:
			return dispatchSync(ctx, req, deps.Sync, SyncIPC.PairAck)

		case ipc.KindRecommendList:
			return dispatchRecommend(ctx, req, deps.Recommend, RecommendIPC.List)
		case ipc.KindRecommendApply:
			return dispatchRecommend(ctx, req, deps.Recommend, RecommendIPC.Apply)
		case ipc.KindRecommendDismiss:
			return dispatchRecommend(ctx, req, deps.Recommend, RecommendIPC.Dismiss)
		case ipc.KindRecommendAntiAdd:
			return dispatchRecommend(ctx, req, deps.Recommend, RecommendIPC.AntiAdd)
		case ipc.KindRecommendAntiList:
			return dispatchRecommend(ctx, req, deps.Recommend, RecommendIPC.AntiList)

		case ipc.KindPluginList:
			return dispatchPlugin(ctx, req, deps.Plugin, PluginIPC.List)
		case ipc.KindPluginInstall:
			return dispatchPlugin(ctx, req, deps.Plugin, PluginIPC.Install)
		case ipc.KindPluginEnable:
			return dispatchPlugin(ctx, req, deps.Plugin, PluginIPC.Enable)
		case ipc.KindPluginDisable:
			return dispatchPlugin(ctx, req, deps.Plugin, PluginIPC.Disable)
		case ipc.KindPluginUninstall:
			return dispatchPlugin(ctx, req, deps.Plugin, PluginIPC.Uninstall)
		case ipc.KindPluginLogs:
			return dispatchPlugin(ctx, req, deps.Plugin, PluginIPC.Logs)

		case ipc.KindA2APeerList:
			return dispatchA2A(ctx, req, deps.A2A, A2AIPC.PeerList)
		case ipc.KindA2APairStart:
			return dispatchA2A(ctx, req, deps.A2A, A2AIPC.PairStart)
		case ipc.KindA2APeerPause:
			return dispatchA2A(ctx, req, deps.A2A, A2AIPC.PeerPause)
		case ipc.KindA2APeerUnpair:
			return dispatchA2A(ctx, req, deps.A2A, A2AIPC.PeerUnpair)

		default:
			return errFrame(req, fmt.Sprintf("unknown kind: %q", req.Kind)), nil
		}
	}
}

// dispatchVision/Sync/Recommend/Plugin/A2A bind the per-surface nil-check.
// Method values would let us write one generic helper, but Go's type system
// can't capture "method on the interface itself" as a single signature
// across receivers — five three-line helpers beat a reflective hop.
func dispatchVision(ctx context.Context, req ipc.Frame, v VisionIPC, m func(VisionIPC, context.Context, ipc.Frame) (<-chan ipc.Frame, error)) (<-chan ipc.Frame, error) {
	if v == nil {
		return errFrame(req, fmt.Sprintf("vision unavailable for %s", req.Kind)), nil
	}
	return m(v, ctx, req)
}

func dispatchSync(ctx context.Context, req ipc.Frame, s SyncIPC, m func(SyncIPC, context.Context, ipc.Frame) (<-chan ipc.Frame, error)) (<-chan ipc.Frame, error) {
	if s == nil {
		return errFrame(req, fmt.Sprintf("sync unavailable for %s", req.Kind)), nil
	}
	return m(s, ctx, req)
}

func dispatchRecommend(ctx context.Context, req ipc.Frame, r RecommendIPC, m func(RecommendIPC, context.Context, ipc.Frame) (<-chan ipc.Frame, error)) (<-chan ipc.Frame, error) {
	if r == nil {
		return errFrame(req, fmt.Sprintf("recommend unavailable for %s", req.Kind)), nil
	}
	return m(r, ctx, req)
}

func dispatchPlugin(ctx context.Context, req ipc.Frame, p PluginIPC, m func(PluginIPC, context.Context, ipc.Frame) (<-chan ipc.Frame, error)) (<-chan ipc.Frame, error) {
	if p == nil {
		return errFrame(req, fmt.Sprintf("plugin unavailable for %s", req.Kind)), nil
	}
	return m(p, ctx, req)
}

func dispatchA2A(ctx context.Context, req ipc.Frame, a A2AIPC, m func(A2AIPC, context.Context, ipc.Frame) (<-chan ipc.Frame, error)) (<-chan ipc.Frame, error) {
	if a == nil {
		return errFrame(req, fmt.Sprintf("a2a unavailable for %s", req.Kind)), nil
	}
	return m(a, ctx, req)
}

// errFrame builds a one-shot error frame echoing the request turn_id.
func errFrame(req ipc.Frame, msg string) <-chan ipc.Frame {
	out := make(chan ipc.Frame, 1)
	payload, _ := json.Marshal(map[string]string{"error": msg})
	out <- ipc.Frame{Kind: ipc.KindError, TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out
}

// handleAsk classifies the query (Haiku router), routes widget intents to
// widget.mount, escalates to Opus when requested, and persists the turn.
// RecordTurn failure is logged but non-fatal per spec §17.16.
func handleAsk(
	ctx context.Context,
	req ipc.Frame,
	db *sql.DB,
	sonnetStream, opusStream streamFn,
	classify classifyFn,
	fetch fetchFn,
	enrich enrichFn,
) (<-chan ipc.Frame, error) {
	var p struct {
		Text         string `json:"text"`
		EscalateOpus bool   `json:"escalate_opus"`
	}
	// nil/null/missing payload coerce to empty p; non-empty must parse.
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errFrame(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	// Reject empty text BEFORE hitting reasoner — avoid burning API tokens.
	if strings.TrimSpace(p.Text) == "" {
		return errFrame(req, "text required"), nil
	}

	// Haiku classify: widget intent emits widget.mount and skips Sonnet.
	if classify != nil {
		intent := classify(ctx, p.Text)
		if intent.Kind == "widget" {
			out := make(chan ipc.Frame, 1)
			out <- ipc.Frame{Kind: "widget.mount", TurnID: req.TurnID, Seq: 1, Payload: mountPayload(ctx, intent, enrich)}
			close(out)
			return out, nil
		}
	}

	// Pick stream: Opus on escalation flag, Sonnet otherwise.
	s := sonnetStream
	if p.EscalateOpus && opusStream != nil {
		s = opusStream
	}

	if s == nil {
		// No reasoner available (API key missing at boot): emit error frame.
		out := make(chan ipc.Frame, 1)
		payload, _ := json.Marshal(map[string]string{"error": "reasoner unavailable"})
		out <- ipc.Frame{Kind: ipc.KindError, TurnID: req.TurnID, Seq: 1, Payload: payload}
		close(out)
		return out, nil
	}

	promptText := p.Text
	if fetch != nil {
		if chunks, err := fetch(ctx, p.Text, 5); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: SearchRelevant: %v\n", err)
		} else if len(chunks) > 0 {
			texts := make([]string, len(chunks))
			for i, c := range chunks {
				texts[i] = c.Text
			}
			promptText = "Context:\n" + strings.Join(texts, "\n") + "\n\nQuery: " + p.Text
		}
	}

	raw, err := s(ctx, req.TurnID, promptText)
	if err != nil {
		return nil, fmt.Errorf("ipc handler stream: %w", err)
	}

	out := make(chan ipc.Frame, 8)
	go func() {
		defer close(out)
		var assembled string
		for f := range raw {
			if f.Kind == "prose.delta" {
				var pp struct {
					Text string `json:"text"`
				}
				_ = json.Unmarshal(f.Payload, &pp)
				assembled += pp.Text
			}
			select {
			case <-ctx.Done():
				return
			case out <- f:
			}
			if f.Kind == "turn.end" {
				// Persist original user text (not the RAG-augmented prompt).
				if err := memory.RecordTurn(db, req.TurnID, p.Text, assembled); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: RecordTurn: %v\n", err)
				}
			}
		}
	}()
	return out, nil
}

// handleVerifyKey runs a 1-token ping against the Anthropic API with the
// supplied key. Returns "verify-key.result" on happy path, errFrame on
// malformed payload or missing key so Settings can distinguish wire error
// from API rejection.
func handleVerifyKey(ctx context.Context, req ipc.Frame, ping pingFn) (<-chan ipc.Frame, error) {
	var p struct {
		Key string `json:"key"`
	}
	if len(req.Payload) > 0 && string(req.Payload) != "null" {
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errFrame(req, fmt.Sprintf("bad payload: %v", err)), nil
		}
	}
	if p.Key == "" {
		return errFrame(req, "key required"), nil
	}

	err := ping(ctx, p.Key)
	ok := err == nil

	payload, _ := json.Marshal(map[string]bool{"ok": ok})
	out := make(chan ipc.Frame, 1)
	out <- ipc.Frame{Kind: "verify-key.result", TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out, nil
}

// liveStreamFn builds the production Sonnet streamFn from a live AnthropicClient.
// It calls client.Stream directly (not Reasoner.AskStream) to avoid the
// budget-charging path — the daemon HUD chat does not route through
// the per-process budget cap used by CLI commands.
func liveStreamFn(c *reasoner.AnthropicClient) streamFn {
	if c == nil {
		return nil
	}
	return func(ctx context.Context, turnID, userText string) (<-chan ipc.Frame, error) {
		deltas, err := c.Stream(ctx, "", userText)
		if err != nil {
			return nil, fmt.Errorf("anthropic stream: %w", err)
		}
		out := make(chan ipc.Frame, 16)
		go func() {
			defer close(out)
			var seq int64
			for {
				select {
				case <-ctx.Done():
					return
				case d, ok := <-deltas:
					if !ok {
						seq++
						select {
						case <-ctx.Done():
						case out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: seq, Payload: json.RawMessage(`{}`)}:
						}
						return
					}
					if d.Text == "" {
						continue
					}
					seq++
					payload, _ := json.Marshal(map[string]string{"text": d.Text})
					select {
					case <-ctx.Done():
						return
					case out <- ipc.Frame{Kind: "prose.delta", TurnID: turnID, Seq: seq, Payload: payload}:
					}
				}
			}
		}()
		return out, nil
	}
}

// liveOpusStreamFn builds the Opus streamFn for the escalate_opus path.
func liveOpusStreamFn(c *reasoner.AnthropicClient) streamFn {
	if c == nil {
		return nil
	}
	opus, err := reasoner.NewAnthropicClientWithModel("claude-opus-4-8")
	if err != nil {
		return nil
	}
	return liveStreamFn(opus)
}

// liveClassifyFn builds the Haiku-backed classify function.
// Degrades to a no-op chat classifier when ANTHROPIC_API_KEY is absent.
func liveClassifyFn() classifyFn {
	haiku, err := reasoner.NewAnthropicClientWithModel("claude-haiku-4-5")
	if err != nil {
		return func(_ context.Context, _ string) reasoner.Intent { return reasoner.Intent{Kind: "chat"} }
	}
	return func(ctx context.Context, text string) reasoner.Intent {
		intent, err := reasoner.Classify(ctx, haiku, text)
		if err != nil {
			return reasoner.Intent{Kind: "chat"}
		}
		return intent
	}
}

// livePingFn validates the wizard-supplied key by issuing a 1-token Messages
// call with a transient SDK client bound to THAT key — NOT the daemon's
// existing ANTHROPIC_API_KEY. Without this, the wizard would silently
// validate the previously-set key and false-positive bad input.
func livePingFn() pingFn {
	return reasoner.VerifyKey
}

// liveFetchFn wraps a knowledge.Graph's SearchRelevant as a fetchFn.
// Returns nil when graph is nil (no knowledge DB configured).
func liveFetchFn(kg *knowledge.Graph) fetchFn {
	if kg == nil {
		return nil
	}
	return kg.SearchRelevant
}

// liveEnrichFn curries the KG into knowledge.EnrichCitation. Returns nil when
// the graph is absent so citation widgets degrade to bare-URL tiles instead
// of failing the mount.
func liveEnrichFn(kg *knowledge.Graph) enrichFn {
	if kg == nil {
		return nil
	}
	return func(ctx context.Context, citationURL string) (*knowledge.CitationEnrichment, error) {
		return knowledge.EnrichCitation(ctx, kg, citationURL)
	}
}

// mountPayload renders the widget.mount payload, calling EnrichCitation
// exactly once for citation widgets. Enrichment failure or nil-result keeps
// the mount alive — only the enrichment field is omitted.
func mountPayload(ctx context.Context, intent reasoner.Intent, enrich enrichFn) json.RawMessage {
	if intent.Widget != "citation" {
		payload, _ := json.Marshal(map[string]string{"widget_type": intent.Widget})
		return payload
	}
	out := struct {
		WidgetType string                        `json:"widget_type"`
		URL        string                        `json:"url,omitempty"`
		Enrichment *knowledge.CitationEnrichment `json:"enrichment,omitempty"`
	}{WidgetType: "citation", URL: intent.URL}
	if enrich != nil && intent.URL != "" {
		if enr, err := enrich(ctx, intent.URL); err == nil && enr != nil {
			out.Enrichment = enr
		}
	}
	payload, _ := json.Marshal(out)
	return payload
}
