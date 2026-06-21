package gcal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPServiceListToday(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"items":[
			{"id":"e1","summary":"standup","location":"Zoom",
			 "start":{"dateTime":"2026-06-20T09:00:00-07:00"},
			 "end":{"dateTime":"2026-06-20T09:30:00-07:00"}},
			{"id":"e2","summary":"all-day offsite",
			 "start":{"date":"2026-06-20"},
			 "end":{"date":"2026-06-21"}}
		]}`))
	}))
	defer srv.Close()

	svc := newHTTPService(srv.Client(), srv.URL, "primary")
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.Local)
	evs, err := svc.ListToday(context.Background(), "tok-xyz", now)
	if err != nil {
		t.Fatalf("ListToday err = %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("len = %d, want 2", len(evs))
	}
	e := evs[0]
	if e.ID != "e1" || e.Summary != "standup" || e.Location != "Zoom" {
		t.Fatalf("event0 = %+v", e)
	}
	if e.Start.IsZero() || e.End.IsZero() {
		t.Fatalf("event0 timestamps not parsed: %+v", e)
	}
	if !e.Start.Equal(time.Date(2026, 6, 20, 9, 0, 0, 0, e.Start.Location())) {
		t.Fatalf("event0 start = %v", e.Start)
	}

	// all-day event falls back to start.date
	ad := evs[1]
	if ad.ID != "e2" || ad.Summary != "all-day offsite" {
		t.Fatalf("event1 = %+v", ad)
	}
	if ad.Start.IsZero() {
		t.Fatalf("all-day start not parsed from date field: %+v", ad)
	}
	if ad.Start.Year() != 2026 || ad.Start.Month() != time.June || ad.Start.Day() != 20 {
		t.Fatalf("all-day start = %v, want 2026-06-20", ad.Start)
	}

	if gotAuth != "Bearer tok-xyz" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	for _, want := range []string{"singleEvents=true", "orderBy=startTime", "timeMin=", "timeMax="} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestHTTPServiceListTodayAuthRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	svc := newHTTPService(srv.Client(), srv.URL, "primary")
	_, err := svc.ListToday(context.Background(), "stale", time.Now())
	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("err = %v, want ErrAuthRequired", err)
	}
}
