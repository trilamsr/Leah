package markets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/trilam/leah/internal/ratelimit"
	"github.com/trilam/leah/internal/widget"
)

const (
	defaultEndpoint  = "https://www.alphavantage.co/query"
	defaultRateLimit = 5
	staleAfter       = 60 * time.Second
)

// Adapter implements widget.WidgetAdapter for kind "market".
type Adapter struct {
	endpoint string
	apiKey   string
	client   *http.Client
	rl       *ratelimit.Window
}

// NewAdapter wires an Alpha Vantage GLOBAL_QUOTE adapter. A blank Endpoint
// falls back to prod; a zero RateLimit applies the 5 req/min free-tier cap.
func NewAdapter(opts Options) *Adapter {
	ep := opts.Endpoint
	if ep == "" {
		ep = defaultEndpoint
	}
	budget := opts.RateLimit
	if budget <= 0 {
		budget = defaultRateLimit
	}
	return &Adapter{
		endpoint: ep,
		apiKey:   opts.APIKey,
		client:   &http.Client{Timeout: 15 * time.Second},
		rl:       ratelimit.NewWindow(time.Minute, budget, nil),
	}
}

func (a *Adapter) Type() string { return "market" }

type props struct {
	Symbols []string `json:"symbols"`
}

func (a *Adapter) Validate(raw json.RawMessage) error {
	p, err := decodeProps(raw)
	if err != nil {
		return err
	}
	if len(p.Symbols) == 0 {
		return errors.New("markets: props.symbols must be non-empty")
	}
	return nil
}

func decodeProps(raw json.RawMessage) (props, error) {
	var p props
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("markets: invalid props: %w", err)
	}
	return p, nil
}

func (a *Adapter) Fetch(ctx context.Context, raw json.RawMessage) (widget.Payload, error) {
	if err := a.Validate(raw); err != nil {
		return widget.Payload{}, err
	}
	p, _ := decodeProps(raw)
	// Rate-limit gate runs AFTER Validate so malformed props never burn budget.
	if !a.rl.Allow("alpha_vantage") {
		return widget.Payload{}, errors.New("markets: rate limit exceeded (5/min)")
	}
	quotes := make([]Quote, 0, len(p.Symbols))
	for _, sym := range p.Symbols {
		q, err := a.fetchOne(ctx, sym)
		if err != nil {
			return widget.Payload{}, err
		}
		quotes = append(quotes, q)
	}
	data, err := json.Marshal(struct {
		Quotes []Quote `json:"quotes"`
	}{quotes})
	if err != nil {
		return widget.Payload{}, fmt.Errorf("markets: encode payload: %w", err)
	}
	now := time.Now().UTC()
	return widget.Payload{
		Data:       data,
		FetchedAt:  now,
		StaleAfter: staleAfter,
		Source:     "alpha_vantage",
		Etag:       etag(p.Symbols, now),
	}, nil
}

// Refresh skips the network when the prev payload's etag matches the current
// minute bucket — AV has no conditional-GET, but the etag (sorted symbols +
// UTC minute) means an identical request inside the same minute can reuse the
// cached payload. Falls through to Fetch on stale/missing prev.
func (a *Adapter) Refresh(ctx context.Context, _ string, raw json.RawMessage, prev *widget.Payload) (widget.Payload, error) {
	if prev != nil && prev.Etag != "" {
		p, err := decodeProps(raw)
		if err == nil && len(p.Symbols) > 0 && etag(p.Symbols, time.Now().UTC()) == prev.Etag {
			return *prev, nil
		}
	}
	return a.Fetch(ctx, raw)
}

func (a *Adapter) fetchOne(ctx context.Context, sym string) (Quote, error) {
	u, err := url.Parse(a.endpoint)
	if err != nil {
		return Quote{}, fmt.Errorf("markets: bad endpoint: %w", err)
	}
	q := u.Query()
	q.Set("function", "GLOBAL_QUOTE")
	q.Set("symbol", sym)
	q.Set("apikey", a.apiKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Quote{}, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return Quote{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Quote{}, fmt.Errorf("markets: upstream %s: %d", sym, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Quote{}, err
	}
	var wire struct {
		GQ map[string]string `json:"Global Quote"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return Quote{}, fmt.Errorf("markets: decode %s: %w", sym, err)
	}
	if len(wire.GQ) == 0 {
		return Quote{}, fmt.Errorf("markets: unknown symbol %q", sym)
	}
	return Quote{
		Symbol:    wire.GQ["01. symbol"],
		Price:     wire.GQ["05. price"],
		Change:    wire.GQ["09. change"],
		ChangePct: wire.GQ["10. change percent"],
		AsOf:      wire.GQ["07. latest trading day"],
	}, nil
}

// etag = sha256(sorted symbols + UTC minute bucket); same symbols within the
// same minute return the same etag regardless of input ordering.
func etag(syms []string, t time.Time) string {
	cp := append([]string(nil), syms...)
	sort.Strings(cp)
	h := sha256.New()
	io.WriteString(h, strings.Join(cp, ","))
	io.WriteString(h, "|")
	io.WriteString(h, t.Truncate(time.Minute).Format(time.RFC3339))
	return hex.EncodeToString(h.Sum(nil))
}
