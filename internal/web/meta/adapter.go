package meta

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/trilam/leah/internal/widget"
)

const (
	maxCitationBytes = 1 * 1024 * 1024
	maxImageBytes    = 10 * 1024 * 1024
	inlineThreshold  = 32 * 1024
	connectTimeout   = 5 * time.Second
	maxRedirects     = 3
)

var allowedImageMIME = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
	"image/gif":  {},
}

type urlProps struct {
	URL string `json:"url"`
}

func validateURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("url required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("url parse: %w", err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("scheme %q: only https supported", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("host required")
	}
	return u, nil
}

func parseURLProps(raw json.RawMessage) (*url.URL, error) {
	var p urlProps
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("props: %w", err)
	}
	return validateURL(p.URL)
}

func isPrivateAddr(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsPrivate() || ip.IsMulticast() {
		return true
	}
	return false
}

func safeDialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: connectTimeout}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
		for _, ia := range ips {
			if isPrivateAddr(ia.IP) {
				return nil, fmt.Errorf("private address blocked: %s", ia.IP)
			}
		}
		return d.DialContext(ctx, network, net.JoinHostPort(host, port))
	}
}

func newSafeClient() *http.Client {
	tr := &http.Transport{
		DialContext:           safeDialContext(),
		TLSHandshakeTimeout:   connectTimeout,
		ResponseHeaderTimeout: connectTimeout,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("too many redirects (>%d)", maxRedirects)
			}
			if _, err := validateURL(req.URL.String()); err != nil {
				return fmt.Errorf("redirect %w", err)
			}
			return nil
		},
	}
}

func readCapped(body io.ReadCloser, max int64) ([]byte, error) {
	defer body.Close()
	buf, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > max {
		return nil, fmt.Errorf("body size > %d bytes", max)
	}
	return buf, nil
}

type CitationAdapter struct {
	client           *http.Client
	skipPrivateCheck bool
}

func NewCitationAdapter() *CitationAdapter {
	return &CitationAdapter{client: newSafeClient()}
}

func newCitationAdapterWithClient(c *http.Client) *CitationAdapter {
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return fmt.Errorf("too many redirects (>%d)", maxRedirects)
		}
		return nil
	}
	return &CitationAdapter{client: c, skipPrivateCheck: true}
}

func (c *CitationAdapter) Type() string { return "citation" }

func (c *CitationAdapter) Validate(props json.RawMessage) error {
	_, err := parseURLProps(props)
	return err
}

func (c *CitationAdapter) Fetch(ctx context.Context, props json.RawMessage) (widget.Payload, error) {
	u, err := parseURLProps(props)
	if err != nil {
		return widget.Payload{}, err
	}
	if !c.skipPrivateCheck {
		if host := u.Hostname(); host != "" {
			if ip := net.ParseIP(host); ip != nil && isPrivateAddr(ip) {
				return widget.Payload{}, fmt.Errorf("private address blocked: %s", ip)
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return widget.Payload{}, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := c.client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "private address") {
			return widget.Payload{}, fmt.Errorf("citation: private address blocked")
		}
		return widget.Payload{}, fmt.Errorf("citation fetch: %w", err)
	}
	body, err := readCapped(resp.Body, maxCitationBytes)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("citation: %w", err)
	}
	m, err := ParseMeta(strings.NewReader(string(body)), u.String())
	if err != nil {
		return widget.Payload{}, fmt.Errorf("citation parse: %w", err)
	}
	data, err := json.Marshal(m)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("citation marshal: %w", err)
	}
	return widget.Payload{
		Data:       data,
		FetchedAt:  time.Now().UTC(),
		StaleAfter: 24 * time.Hour,
		Source:     "web/meta",
	}, nil
}

func (c *CitationAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, _ *widget.Payload) (widget.Payload, error) {
	return c.Fetch(ctx, props)
}

type ImageAdapter struct {
	client           *http.Client
	cache            *ImageCache
	skipPrivateCheck bool
}

func NewImageAdapter(cacheDir string) *ImageAdapter {
	cache, err := NewImageCache(cacheDir, DefaultCacheCapBytes)
	if err != nil {
		cache = &ImageCache{dir: cacheDir, cap: DefaultCacheCapBytes}
	}
	return &ImageAdapter{client: newSafeClient(), cache: cache}
}

func newImageAdapterWithClient(cacheDir string, c *http.Client) *ImageAdapter {
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return fmt.Errorf("too many redirects (>%d)", maxRedirects)
		}
		return nil
	}
	cache, _ := NewImageCache(cacheDir, DefaultCacheCapBytes)
	return &ImageAdapter{client: c, cache: cache, skipPrivateCheck: true}
}

func (a *ImageAdapter) Type() string { return "image" }

func (a *ImageAdapter) Validate(props json.RawMessage) error {
	_, err := parseURLProps(props)
	return err
}

type imagePayload struct {
	Path         string `json:"path,omitempty"`
	InlineBase64 string `json:"inline_base64,omitempty"`
	Mime         string `json:"mime"`
	Size         int    `json:"size"`
}

func (a *ImageAdapter) Fetch(ctx context.Context, props json.RawMessage) (widget.Payload, error) {
	u, err := parseURLProps(props)
	if err != nil {
		return widget.Payload{}, err
	}
	if !a.skipPrivateCheck {
		if host := u.Hostname(); host != "" {
			if ip := net.ParseIP(host); ip != nil && isPrivateAddr(ip) {
				return widget.Payload{}, fmt.Errorf("private address blocked: %s", ip)
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return widget.Payload{}, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "private address") {
			return widget.Payload{}, fmt.Errorf("image: private address blocked")
		}
		return widget.Payload{}, fmt.Errorf("image fetch: %w", err)
	}
	mime := strings.ToLower(strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]))
	if _, ok := allowedImageMIME[mime]; !ok {
		resp.Body.Close()
		return widget.Payload{}, fmt.Errorf("image: mime %q not allowed", mime)
	}
	body, err := readCapped(resp.Body, maxImageBytes)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("image: %w", err)
	}
	out := imagePayload{Mime: mime, Size: len(body)}
	if len(body) < inlineThreshold {
		out.InlineBase64 = base64.StdEncoding.EncodeToString(body)
	} else {
		path, err := a.cache.Put(u.String(), mime, body)
		if err != nil {
			return widget.Payload{}, fmt.Errorf("image cache: %w", err)
		}
		out.Path = path
	}
	data, err := json.Marshal(out)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("image marshal: %w", err)
	}
	return widget.Payload{
		Data:       data,
		FetchedAt:  time.Now().UTC(),
		StaleAfter: 7 * 24 * time.Hour,
		Source:     "web/meta",
	}, nil
}

func (a *ImageAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, prev *widget.Payload) (widget.Payload, error) {
	if prev != nil {
		return *prev, nil
	}
	return a.Fetch(ctx, props)
}
