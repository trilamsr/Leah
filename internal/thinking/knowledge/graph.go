package knowledge

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

type EntityKind string

const (
	KindPerson     EntityKind = "person"
	KindProject    EntityKind = "project"
	KindEvent      EntityKind = "event"
	KindLocation   EntityKind = "location"
	KindTimeWindow EntityKind = "time_window"
)

type ItemRef struct {
	Source    string
	ID        string
	Timestamp time.Time
}

type Entity struct {
	Kind        EntityKind
	Key         string
	Display     string
	Aliases     []string
	Refs        []ItemRef
	FirstSeen   time.Time
	LastTouched time.Time
}

type Resolver interface {
	Kind() EntityKind
	Resolve(ctx context.Context, subject string) ([]Entity, error)
}

type KnowledgeQuery struct {
	Subject string
	Kind    EntityKind
	Within  time.Duration
	Limit   int
}

// Reasoner-safe projection: scalars are remote-safe; Entity/Items stay local.
type Result struct {
	Entity  Entity
	Items   []ItemRef
	Scalars map[string]int
}

type Config struct {
	Path          string
	AuditLogger   *audit.Logger
	Now           func() time.Time
	RetentionDays int
}

var (
	ErrUnknownEntity = errors.New("knowledge: unknown entity")
	ErrClosed        = errors.New("knowledge: graph closed")
)

type Graph struct {
	store    *storage
	now      func() time.Time
	audit    *audit.Logger
	retainD  int
	mu       sync.RWMutex
	resolver map[EntityKind]Resolver
}

func New(config Config) (*Graph, error) {
	if config.Path == "" {
		return nil, fmt.Errorf("knowledge: empty Path")
	}
	st, err := openStorage(config.Path)
	if err != nil {
		return nil, err
	}
	now := config.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	d := config.RetentionDays
	if d <= 0 {
		d = 90
	}
	return &Graph{
		store:    st,
		now:      now,
		audit:    config.AuditLogger,
		retainD:  d,
		resolver: make(map[EntityKind]Resolver),
	}, nil
}

func (g *Graph) Close() error { return g.store.Close() }

func (g *Graph) Register(r Resolver) {
	g.mu.Lock()
	g.resolver[r.Kind()] = r
	g.mu.Unlock()
}

func (g *Graph) Add(e Entity) error {
	if e.Key == "" {
		return fmt.Errorf("knowledge: empty Key")
	}
	if e.Kind == "" {
		return fmt.Errorf("knowledge: empty Kind")
	}
	prior, err := g.store.getEntity(e.Kind, e.Key)
	if err != nil && !errors.Is(err, ErrUnknownEntity) {
		return err
	}
	if err == nil {
		e.Aliases = unionStrings(prior.Aliases, e.Aliases)
		if prior.FirstSeen.Before(e.FirstSeen) || e.FirstSeen.IsZero() {
			e.FirstSeen = prior.FirstSeen
		}
	} else if e.FirstSeen.IsZero() {
		e.FirstSeen = g.now()
	}
	return g.store.upsertEntity(e, g.now())
}

func (g *Graph) Resolve(ctx context.Context, subject string) ([]Entity, error) {
	entities, err := g.store.matchEntities("", subject, 0)
	if err != nil {
		return nil, err
	}
	g.mu.RLock()
	resolvers := make([]Resolver, 0, len(g.resolver))
	for _, r := range g.resolver {
		resolvers = append(resolvers, r)
	}
	g.mu.RUnlock()
	for _, r := range resolvers {
		fresh, ferr := r.Resolve(ctx, subject)
		if ferr != nil {
			return nil, fmt.Errorf("resolver %s: %w", r.Kind(), ferr)
		}
		for _, e := range fresh {
			if err := g.Add(e); err != nil {
				return nil, err
			}
			entities = append(entities, e)
		}
	}
	if len(entities) == 0 {
		return nil, ErrUnknownEntity
	}
	return entities, nil
}

func (g *Graph) Query(ctx context.Context, q KnowledgeQuery) (Result, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	entities, err := g.store.matchEntities(q.Kind, q.Subject, 1)
	if err != nil {
		return Result{}, err
	}
	if len(entities) == 0 {
		return Result{}, ErrUnknownEntity
	}
	e := entities[0]
	var since time.Time
	if q.Within > 0 {
		since = g.now().Add(-q.Within)
	}
	refs, err := g.store.listRefs(e.Kind, e.Key, since, limit)
	if err != nil {
		return Result{}, err
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Timestamp.After(refs[j].Timestamp) })
	e.Refs = refs
	return Result{Entity: e, Items: refs, Scalars: g.scalars(e, refs)}, nil
}

// Integer-only projection; keys must stay PII-free (spec §5).
func (g *Graph) scalars(e Entity, refs []ItemRef) map[string]int {
	bySource := make(map[string]int)
	for _, r := range refs {
		bySource[r.Source]++
	}
	out := make(map[string]int, len(bySource)+2)
	for src, n := range bySource {
		out["source_"+src] = n
	}
	out["item_count"] = len(refs)
	ageDays := int(g.now().Sub(e.LastTouched) / (24 * time.Hour))
	if ageDays < 0 {
		ageDays = 0
	}
	out["entity_age_days"] = ageDays
	return out
}

func (g *Graph) Forget(ctx context.Context, kind EntityKind, key string) error {
	deleted, err := g.store.deleteEntity(kind, key)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrUnknownEntity
	}
	if g.audit != nil {
		_ = g.audit.Append(audit.Entry{
			Kind:     "knowledge_forget",
			ArgsHash: key,
			Outcome:  "ok",
			Detail:   string(kind),
		})
	}
	return nil
}

// SearchRelevant delegates to the underlying storage layer.
func (g *Graph) SearchRelevant(ctx context.Context, query string, k int) ([]Chunk, error) {
	return g.store.SearchRelevant(ctx, query, k)
}

func unionStrings(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(a, b...) {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
