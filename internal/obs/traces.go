package obs

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// RefIDAttr — /traces/{id} resolves id against this span attribute.
const RefIDAttr = "leah.ref_id"

type Tracer interface {
	StartSpan(ctx context.Context, name string) (Span, context.Context)
	Query(ctx context.Context, refID string) ([]SpanRecord, error)
	Flush(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

type Span interface {
	End()
	SetAttribute(k string, v any)
	RecordError(err error)
}

// SpanRecord shape is JSON-stable: renaming a field breaks /traces/{id}.
type SpanRecord struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Name         string            `json:"name"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	DurationMS   int64             `json:"duration_ms"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Status       string            `json:"status,omitempty"`
	StatusMsg    string            `json:"status_msg,omitempty"`
}

type Config struct {
	SampleRate float64
	MaxTraces  int
}

func New(cfg Config) (Tracer, error) {
	if cfg.SampleRate < 0 || cfg.SampleRate > 1 {
		return nil, fmt.Errorf("obs: sample rate %v out of [0,1]", cfg.SampleRate)
	}
	if cfg.MaxTraces <= 0 {
		cfg.MaxTraces = 4096
	}
	coll := newCollector(cfg.MaxTraces)
	var sampler sdktrace.Sampler
	switch {
	case cfg.SampleRate <= 0:
		sampler = sdktrace.NeverSample()
	case cfg.SampleRate >= 1:
		sampler = sdktrace.AlwaysSample()
	default:
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sampler)),
		sdktrace.WithSyncer(coll),
	)
	return &tracer{
		tp:    tp,
		inner: tp.Tracer("github.com/trilam/leah"),
		coll:  coll,
	}, nil
}

type tracer struct {
	tp    *sdktrace.TracerProvider
	inner oteltrace.Tracer
	coll  *collector
}

func (t *tracer) StartSpan(ctx context.Context, name string) (Span, context.Context) {
	newCtx, sp := t.inner.Start(ctx, name)
	return &span{inner: sp}, newCtx
}

func (t *tracer) Query(_ context.Context, refID string) ([]SpanRecord, error) {
	return t.coll.queryByRefID(refID), nil
}

func (t *tracer) Flush(ctx context.Context) error { return t.tp.ForceFlush(ctx) }

func (t *tracer) Shutdown(ctx context.Context) error { return t.tp.Shutdown(ctx) }

type span struct{ inner oteltrace.Span }

func (s *span) End() { s.inner.End() }

func (s *span) SetAttribute(k string, v any) {
	switch x := v.(type) {
	case string:
		s.inner.SetAttributes(attribute.String(k, x))
	case bool:
		s.inner.SetAttributes(attribute.Bool(k, x))
	case int:
		s.inner.SetAttributes(attribute.Int(k, x))
	case int64:
		s.inner.SetAttributes(attribute.Int64(k, x))
	case float64:
		s.inner.SetAttributes(attribute.Float64(k, x))
	default:
		s.inner.SetAttributes(attribute.String(k, fmt.Sprintf("%v", v)))
	}
}

func (s *span) RecordError(err error) {
	if err == nil {
		return
	}
	s.inner.RecordError(err)
	s.inner.SetStatus(codes.Error, err.Error())
}

type collector struct {
	mu     sync.Mutex
	max    int
	traces map[string]*list.Element
	order  *list.List
	closed bool
}

type traceEntry struct {
	traceID string
	spans   []SpanRecord
}

func newCollector(max int) *collector {
	return &collector{
		max:    max,
		traces: map[string]*list.Element{},
		order:  list.New(),
	}
}

func (c *collector) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("obs: collector closed")
	}
	for _, s := range spans {
		rec := toRecord(s)
		el, ok := c.traces[rec.TraceID]
		if !ok {
			entry := &traceEntry{traceID: rec.TraceID}
			el = c.order.PushFront(entry)
			c.traces[rec.TraceID] = el
		} else {
			c.order.MoveToFront(el)
		}
		entry := el.Value.(*traceEntry)
		entry.spans = append(entry.spans, rec)
		for c.order.Len() > c.max {
			last := c.order.Back()
			if last == nil {
				break
			}
			old := last.Value.(*traceEntry)
			c.order.Remove(last)
			delete(c.traces, old.traceID)
		}
	}
	return nil
}

func (c *collector) Shutdown(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

// queryByRefID returns the WHOLE trace any matching span belongs to —
// caller wants the causal tree, not the lone tagged span.
func (c *collector) queryByRefID(refID string) []SpanRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []SpanRecord
	for _, el := range c.traces {
		entry := el.Value.(*traceEntry)
		match := false
		for _, sp := range entry.spans {
			if sp.Attributes[RefIDAttr] == refID {
				match = true
				break
			}
		}
		if match {
			out = append(out, entry.spans...)
		}
	}
	return out
}

func toRecord(s sdktrace.ReadOnlySpan) SpanRecord {
	sc := s.SpanContext()
	parent := s.Parent()
	attrs := map[string]string{}
	for _, kv := range s.Attributes() {
		attrs[string(kv.Key)] = kv.Value.String()
	}
	rec := SpanRecord{
		TraceID:    sc.TraceID().String(),
		SpanID:     sc.SpanID().String(),
		Name:       s.Name(),
		StartTime:  s.StartTime(),
		EndTime:    s.EndTime(),
		DurationMS: s.EndTime().Sub(s.StartTime()).Milliseconds(),
		Attributes: attrs,
		Status:     s.Status().Code.String(),
		StatusMsg:  s.Status().Description,
	}
	if parent.HasSpanID() {
		rec.ParentSpanID = parent.SpanID().String()
	}
	return rec
}
