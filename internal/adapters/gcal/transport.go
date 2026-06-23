package gcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://www.googleapis.com/calendar/v3"

var errNotImplemented = errors.New("gcal: service method not implemented")

type httpService struct {
	hc         *http.Client
	baseURL    string
	calendarID string
}

func newHTTPService(hc *http.Client, baseURL, calendarID string) *httpService {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if calendarID == "" {
		calendarID = "primary"
	}
	return &httpService{hc: hc, baseURL: baseURL, calendarID: calendarID}
}

func (s *httpService) ListToday(ctx context.Context, token string, now time.Time) ([]Event, error) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 1)
	q := url.Values{
		"timeMin":      {start.Format(time.RFC3339)},
		"timeMax":      {end.Format(time.RFC3339)},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
	}
	u := fmt.Sprintf("%s/calendars/%s/events?%s", s.baseURL, url.PathEscape(s.calendarID), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("gcal: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gcal: list_today: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ErrAuthRequired
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("gcal: list_today: provider %s", resp.Status)
	}

	var out struct {
		Items []struct {
			ID       string `json:"id"`
			Summary  string `json:"summary"`
			Location string `json:"location"`
			Start    eventTime `json:"start"`
			End      eventTime `json:"end"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("gcal: decode: %w", err)
	}
	evs := make([]Event, 0, len(out.Items))
	for _, it := range out.Items {
		evs = append(evs, Event{
			ID:       it.ID,
			Summary:  it.Summary,
			Location: it.Location,
			Start:    it.Start.time(),
			End:      it.End.time(),
		})
	}
	return evs, nil
}

func (s *httpService) Create(context.Context, string, Event) (*Event, error) {
	return nil, errNotImplemented
}

type eventTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

// time falls back to the date-only field so all-day events still carry a Start.

func (e eventTime) time() time.Time {
	if e.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, e.DateTime); err == nil {
			return t
		}
	}
	if e.Date != "" {
		if t, err := time.ParseInLocation("2006-01-02", e.Date, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}
