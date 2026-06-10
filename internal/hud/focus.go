package hud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const FocusIdleTimeout = 30 * time.Second

var ErrEmptyQuery = errors.New("hud: empty query")

type FocusResponse struct {
	Text        string   `json:"text"`
	Sources     []Source `json:"sources,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

type Source struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Focus struct {
	Machine   *Machine
	DaemonURL string
	HTTP      *http.Client

	mu              sync.Mutex
	lastInteraction time.Time
}

func NewFocus(m *Machine, daemonURL string) *Focus {
	return &Focus{
		Machine:   m,
		DaemonURL: strings.TrimRight(daemonURL, "/"),
		HTTP:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (f *Focus) SummonHotkey() { f.SummonAt(time.Now()) }

func (f *Focus) SummonAt(now time.Time) {
	f.mu.Lock()
	f.lastInteraction = now
	f.mu.Unlock()
	f.Machine.Summon()
}

func (f *Focus) TickIdle(now time.Time) {
	f.mu.Lock()
	gap := now.Sub(f.lastInteraction)
	f.mu.Unlock()
	if gap >= FocusIdleTimeout {
		f.Machine.OnIdle()
	}
}

func (f *Focus) Query(ctx context.Context, prompt string) (FocusResponse, error) {
	if strings.TrimSpace(prompt) == "" {
		return FocusResponse{}, ErrEmptyQuery
	}
	f.mu.Lock()
	f.lastInteraction = time.Now()
	f.mu.Unlock()

	body, _ := json.Marshal(struct {
		Query string `json:"query"`
	}{Query: prompt})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.DaemonURL+"/reasoner/query", bytes.NewReader(body))
	if err != nil {
		return fallback(err), err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return fallback(err), err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		e := fmt.Errorf("hud: /reasoner/query status %d", resp.StatusCode)
		return fallback(e), e
	}
	var out FocusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fallback(err), err
	}
	return out, nil
}

func fallback(cause error) FocusResponse {
	return FocusResponse{Text: fmt.Sprintf("Reasoner unavailable: %v", cause)}
}
