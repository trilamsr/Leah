package tmdb

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/trilam/leah/internal/platform/contracts"
)

var (
	ErrAttestationDenied = errors.New("tmdb: attestation denied")
	ErrAuthFailed        = errors.New("tmdb: auth failed")
	ErrNotFound          = errors.New("tmdb: not found")
	ErrRateLimited       = errors.New("tmdb: rate limited")
	ErrInvalidRegion     = errors.New("tmdb: invalid region code")
)

const ScopeFind = "tmdb:find"

// regionRe is the [A-Z]{2} ISO-3166 allowlist that gates the providers RPC.
// Anything else short-circuits before the network call to keep operator
// typos out of the upstream's URL.
var regionRe = regexp.MustCompile(`^[A-Z]{2}$`)

type SearchHit struct {
	ID        int
	MediaType string
	Title     string
	Year      int
}

type Provider struct {
	Name     string
	LogoPath string
}

type ProviderSet struct {
	Flatrate []Provider
	Rent     []Provider
	Buy      []Provider
}

type Result struct {
	Title    string
	Year     int
	Flatrate []Provider
	Rent     []Provider
	Buy      []Provider
}

type Transport interface {
	Search(ctx context.Context, key, query string) ([]SearchHit, error)
	WatchProviders(ctx context.Context, key, mediaType string, id int, region string) (ProviderSet, error)
}

type Config struct {
	Attestor    contracts.Attestor
	TokenSource contracts.TokenSource
	Transport   Transport
	Region      string
}

type Adapter struct {
	att    contracts.Attestor
	ts     contracts.TokenSource
	tr     Transport
	region string
}

func New(config Config) (*Adapter, error) {
	if config.Attestor == nil {
		return nil, errors.New("tmdb: Config.Attestor required")
	}
	if config.TokenSource == nil {
		return nil, errors.New("tmdb: Config.TokenSource required")
	}
	if config.Transport == nil {
		return nil, errors.New("tmdb: Config.Transport required")
	}
	region := config.Region
	if region == "" {
		region = "US"
	}
	return &Adapter{att: config.Attestor, ts: config.TokenSource, tr: config.Transport, region: region}, nil
}

func (a *Adapter) Find(ctx context.Context, query string) (Result, error) {
	if !regionRe.MatchString(a.region) {
		return Result{}, fmt.Errorf("%w: %q", ErrInvalidRegion, a.region)
	}
	if err := a.att.Attest(ctx, ScopeFind); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("tmdb: token: %w", err)
	}
	hits, err := a.tr.Search(ctx, tok, query)
	if err != nil {
		return Result{}, err
	}
	if len(hits) == 0 {
		return Result{}, ErrNotFound
	}
	top := hits[0]
	set, err := a.tr.WatchProviders(ctx, tok, top.MediaType, top.ID, a.region)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Title:    top.Title,
		Year:     top.Year,
		Flatrate: set.Flatrate,
		Rent:     set.Rent,
		Buy:      set.Buy,
	}, nil
}
