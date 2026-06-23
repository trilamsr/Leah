package inbound

import (
	"context"
	"encoding/json"
)

// FirstPartyDeps are the cross-package collaborators the §5.3 tool set needs.
// Each field is optional — when nil, the corresponding tool returns a typed
// internal error so a misconfigured composition root surfaces at first call.
type FirstPartyDeps struct {
	MemorySearch func(ctx context.Context, query string, k int) ([]MemoryHit, error)
	CalendarNext func(ctx context.Context, within string) ([]CalendarEvent, error)
	RepoCite     func(ctx context.Context, repo, question string) ([]Citation, error)
	Ask          func(ctx context.Context, prompt string) (string, error)
	WidgetRender func(ctx context.Context, widgetID string, params json.RawMessage) (any, error)
	// AskConsent gates leah.ask per §5.7 — return ConsentDeniedError to
	// reject. nil means "allow" (used by tests; production wires HUD prompt).
	AskConsent func(ctx context.Context, prompt string) error
}

// MemoryHit is the leah.memory.search result row. Mirrors the §5.3 shape.
type MemoryHit struct {
	ID    string  `json:"id"`
	Text  string  `json:"text"`
	Score float64 `json:"score"`
}

// CalendarEvent is the leah.calendar.next result row.
type CalendarEvent struct {
	Title    string `json:"title"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
	Location string `json:"location,omitempty"`
}

// Citation is one leah.repo.cite result.
type Citation struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// RegisterFirstParty mounts the §5.3 toolset onto srv. Each tool is gated by
// its declared scope; nil deps cause the call to return a stable "internal"
// error so misconfiguration is loud, not silent.
func RegisterFirstParty(srv *Server, d FirstPartyDeps) {
	srv.MustRegister(MCPTool{
		Name:           "leah.memory.search",
		RequiredScopes: []Scope{ScopeMemoryRead},
		Handler:        memorySearchHandler(d.MemorySearch),
	})
	srv.MustRegister(MCPTool{
		Name:           "leah.calendar.next",
		RequiredScopes: []Scope{ScopeCalendarRead},
		Handler:        calendarNextHandler(d.CalendarNext),
	})
	srv.MustRegister(MCPTool{
		Name:           "leah.repo.cite",
		RequiredScopes: []Scope{ScopeRepoRead},
		Handler:        repoCiteHandler(d.RepoCite),
	})
	srv.MustRegister(MCPTool{
		Name:           "leah.ask",
		RequiredScopes: []Scope{ScopeAskRun},
		Handler:        askHandler(d.Ask, d.AskConsent),
	})
	srv.MustRegister(MCPTool{
		Name:           "leah.widget.render",
		RequiredScopes: []Scope{ScopeWidgetRender},
		Handler:        widgetRenderHandler(d.WidgetRender),
	})
}

func memorySearchHandler(fn func(context.Context, string, int) ([]MemoryHit, error)) func(context.Context, json.RawMessage) (any, error) {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		if fn == nil {
			return nil, errInternal("memory.search not wired")
		}
		var in struct {
			Query string `json:"query"`
			K     int    `json:"k"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if in.K <= 0 {
			in.K = 5
		}
		hits, err := fn(ctx, in.Query, in.K)
		if err != nil {
			return nil, err
		}
		return map[string]any{"hits": hits}, nil
	}
}

func calendarNextHandler(fn func(context.Context, string) ([]CalendarEvent, error)) func(context.Context, json.RawMessage) (any, error) {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		if fn == nil {
			return nil, errInternal("calendar.next not wired")
		}
		var in struct {
			Within string `json:"within"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		evts, err := fn(ctx, in.Within)
		if err != nil {
			return nil, err
		}
		return map[string]any{"events": evts}, nil
	}
}

func repoCiteHandler(fn func(context.Context, string, string) ([]Citation, error)) func(context.Context, json.RawMessage) (any, error) {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		if fn == nil {
			return nil, errInternal("repo.cite not wired")
		}
		var in struct {
			Repo     string `json:"repo"`
			Question string `json:"question"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		cs, err := fn(ctx, in.Repo, in.Question)
		if err != nil {
			return nil, err
		}
		return map[string]any{"citations": cs}, nil
	}
}

// askHandler — §5.7 per-call consent gate runs BEFORE the upstream call so a
// deny never spends Anthropic tokens. The reasoner is wired to charge
// `cloud.llm.tokens`; budget exhaustion surfaces here via BudgetExhaustedError.
func askHandler(fn func(context.Context, string) (string, error), consent func(context.Context, string) error) func(context.Context, json.RawMessage) (any, error) {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		if fn == nil {
			return nil, errInternal("ask not wired")
		}
		var in struct {
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		if consent != nil {
			if err := consent(ctx, in.Prompt); err != nil {
				return nil, err
			}
		}
		text, err := fn(ctx, in.Prompt)
		if err != nil {
			return nil, err
		}
		return map[string]any{"text": text}, nil
	}
}

func widgetRenderHandler(fn func(context.Context, string, json.RawMessage) (any, error)) func(context.Context, json.RawMessage) (any, error) {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		if fn == nil {
			return nil, errInternal("widget.render not wired")
		}
		var in struct {
			WidgetID string          `json:"widget_id"`
			Params   json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		return fn(ctx, in.WidgetID, in.Params)
	}
}

type internalErr struct{ msg string }

func (e *internalErr) Error() string { return e.msg }

func errInternal(msg string) error { return &internalErr{msg: msg} }
