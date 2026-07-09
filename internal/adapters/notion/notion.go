package notion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/obs/connectadapter"
)

// Sentinel errors are exported so callers switch on failure mode without
// string-matching wrapped messages.
var (
	ErrAttestationDenied = errors.New("notion: attestation denied")
	ErrAuthFailed        = errors.New("notion: auth failed")
	ErrNotFound          = errors.New("notion: not found")
	ErrRateLimited       = errors.New("notion: rate limited")
	ErrInvalidPage       = errors.New("notion: invalid page payload")
)

// Scopes the Attestor sees. Distinct per RPC so the operator-attestation log
// attributes consent at the per-action grain (list vs. query vs. read vs.
// mutate).
const (
	ScopeListDatabases = "notion:list_dbs"
	ScopeQueryDatabase = "notion:query"
	ScopeGetPage       = "notion:get_page"
	ScopeCreatePage    = "notion:create_page"
)

// DB is the minimum database envelope MVP exposes; the typed block-tree
// surface is deferred per spec § 10.
type DB struct {
	ID    string
	Title string
	URL   string
}

// Page is the minimum page envelope returned by Query / Get / Create. DB
// is the parent database ID; empty when the page is workspace-rooted.
type Page struct {
	ID    string
	Title string
	URL   string
	DB    string
}

// Filter is the simplest single-property equality filter the MVP supports;
// richer Notion filter grammar (compound, date, relation) defers.
type Filter struct {
	PropertyName  string
	PropertyValue string
}

// Props carries the title + simple string properties for CreatePage. Block
// content beyond the title defers to a follow-up wave (spec § 10).
type Props struct {
	Title  string
	Fields map[string]string
}

// Transport is the seam between the adapter's policy layer (attestation,
// validation, error mapping) and the api.notion.com/v1 REST calls. Keeping
// it an interface lets the wiring wave plug in either hand-rolled net/http
// or github.com/jomei/notionapi without touching the policy surface.
type Transport interface {
	ListDatabases(ctx context.Context, bearer string) ([]DB, error)
	QueryDatabase(ctx context.Context, bearer, dbID string, f Filter) ([]Page, error)
	GetPage(ctx context.Context, bearer, id string) (Page, error)
	CreatePage(ctx context.Context, bearer, dbID string, p Props) (Page, error)
}

// Config carries the three collaborators Adapter needs. All required; New
// refuses construction without an Attestor because that would silently
// bypass the operator-consent gate.
type Config struct {
	Attestor    contracts.Attestor
	TokenSource contracts.TokenSource
	Transport   Transport
	// Metrics is optional — nil is a no-op (connectadapter contract), so
	// existing callers keep working without a registry.
	Metrics *connectadapter.Metrics
}

// Adapter is the Notion adapter. No background goroutines; lifecycle is
// owned by the caller.
type Adapter struct {
	att contracts.Attestor
	ts  contracts.TokenSource
	tr  Transport
	m   *connectadapter.Metrics
}

// New validates the wiring contract and returns a ready Adapter; no I/O.
func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("notion: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.TokenSource == nil {
		return nil, errors.New("notion: Config.TokenSource required")
	}
	if cfg.Transport == nil {
		return nil, errors.New("notion: Config.Transport required")
	}
	return &Adapter{att: cfg.Attestor, ts: cfg.TokenSource, tr: cfg.Transport, m: cfg.Metrics}, nil
}

// ListDatabases returns the databases the operator's integration can see.
// Notion's per-DB share model means this returns only the explicitly-shared
// set; intentional, prevents cross-workspace discovery.
func (a *Adapter) ListDatabases(ctx context.Context) ([]DB, error) {
	tok, err := a.gateAndToken(ctx, ScopeListDatabases)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	dbs, err := a.tr.ListDatabases(ctx, tok)
	a.m.ObserveAPI("list_databases", time.Since(start).Seconds())
	return dbs, err
}

// QueryDatabase returns pages from dbID matching f. ID-validation runs
// before attestation so a typo never consumes a consent prompt.
func (a *Adapter) QueryDatabase(ctx context.Context, dbID string, f Filter) ([]Page, error) {
	if dbID == "" {
		return nil, fmt.Errorf("%w: empty database id", ErrInvalidPage)
	}
	tok, err := a.gateAndToken(ctx, ScopeQueryDatabase)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	pages, err := a.tr.QueryDatabase(ctx, tok, dbID, f)
	a.m.ObserveAPI("query_database", time.Since(start).Seconds())
	return pages, err
}

// GetPage fetches a single page by id.
func (a *Adapter) GetPage(ctx context.Context, id string) (Page, error) {
	if id == "" {
		return Page{}, fmt.Errorf("%w: empty page id", ErrInvalidPage)
	}
	tok, err := a.gateAndToken(ctx, ScopeGetPage)
	if err != nil {
		return Page{}, err
	}
	start := time.Now()
	page, err := a.tr.GetPage(ctx, tok, id)
	a.m.ObserveAPI("get_page", time.Since(start).Seconds())
	return page, err
}

// CreatePage creates a page in dbID with p as its title + simple-string
// properties. Validation runs BEFORE attestation so an empty title or
// missing parent never burns an operator consent prompt.
func (a *Adapter) CreatePage(ctx context.Context, dbID string, p Props) (Page, error) {
	if dbID == "" {
		return Page{}, fmt.Errorf("%w: empty database id", ErrInvalidPage)
	}
	if p.Title == "" {
		return Page{}, fmt.Errorf("%w: empty title", ErrInvalidPage)
	}
	tok, err := a.gateAndToken(ctx, ScopeCreatePage)
	if err != nil {
		return Page{}, err
	}
	start := time.Now()
	page, err := a.tr.CreatePage(ctx, tok, dbID, p)
	a.m.ObserveAPI("create_page", time.Since(start).Seconds())
	return page, err
}

// gateAndToken runs the attestation gate first; only on consent does the
// integration secret leave the TokenSource. Token-load-before-attestation
// would leak the secret into a logger / panic trace even on a denial path.
func (a *Adapter) gateAndToken(ctx context.Context, scope string) (string, error) {
	if err := a.att.Attest(ctx, scope); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("notion: token load (%s): %w", scope, err)
	}
	return tok, nil
}
