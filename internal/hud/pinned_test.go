package hud_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/hud"
)

// TestWatcher_DebouncesBurstWithin200ms writes the pin file 3 times within 200 ms
// and asserts the watcher coalesces them into exactly ONE PinnedChanged.
func TestWatcher_DebouncesBurstWithin200ms(t *testing.T) {
	dir := t.TempDir()
	pinPath := filepath.Join(dir, "pinned-widgets.json")
	regPath := filepath.Join(dir, "widget-registry.json")

	w, err := hud.NewWatcher(pinPath, regPath)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer func() { _ = w.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events := make(chan hud.PinnedChanged, 8)
	go func() { _ = w.Run(ctx, func(e hud.PinnedChanged) { events <- e }) }()

	// Give the watcher time to start.
	time.Sleep(50 * time.Millisecond)

	entries := []hud.PinnedEntry{{ID: "w1", Type: "market", Props: json.RawMessage(`{}`), Refresh: 60}}
	for range 3 {
		writePins(t, pinPath, entries)
		time.Sleep(50 * time.Millisecond)
	}

	// Wait long enough for the 200 ms debounce to fire.
	deadline := time.After(800 * time.Millisecond)
	var got int
loop:
	for {
		select {
		case <-events:
			got++
		case <-deadline:
			break loop
		}
	}
	if got != 1 {
		t.Fatalf("debounce: got %d PinnedChanged, want 1", got)
	}
}

// TestPinCapTwo: first two Pins succeed; third is a no-op (pin file unchanged)
// and a notification.toast frame is emitted.
func TestPinCapTwo(t *testing.T) {
	dir := t.TempDir()
	pinPath := filepath.Join(dir, "pinned-widgets.json")
	regPath := filepath.Join(dir, "widget-registry.json")

	w, err := hud.NewWatcher(pinPath, regPath)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer func() { _ = w.Close() }()

	mkEntry := func(id string) hud.PinnedEntry {
		return hud.PinnedEntry{ID: id, Type: "market", Props: json.RawMessage(`{}`), Refresh: 60}
	}
	if _, err := w.Pin(mkEntry("w1")); err != nil {
		t.Fatalf("Pin w1: %v", err)
	}
	if _, err := w.Pin(mkEntry("w2")); err != nil {
		t.Fatalf("Pin w2: %v", err)
	}
	toast, err := w.Pin(mkEntry("w3"))
	if err != nil {
		t.Fatalf("Pin w3: %v", err)
	}
	if toast == nil {
		t.Fatal("Pin w3: expected toast frame, got nil")
	}
	if toast.Kind != "notification.toast" {
		t.Fatalf("Pin w3: toast.Kind=%q, want notification.toast", toast.Kind)
	}
	var payload struct {
		Level string `json:"level"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(toast.Payload, &payload); err != nil {
		t.Fatalf("Pin w3: unmarshal toast payload: %v", err)
	}
	if payload.Level != "info" {
		t.Fatalf("Pin w3: toast level=%q, want info", payload.Level)
	}
	if payload.Text == "" {
		t.Fatalf("Pin w3: toast text empty")
	}
	list, err := w.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("after capped pin: got %d entries, want 2", len(list))
	}
}

// TestSingleWatcherForBothFiles probes fsnotify's live watch list — both
// parent dirs must appear on ONE watcher (spec § 10.2). A regression that
// constructs a second fsnotify.Watcher per file would not register both
// dirs on the single instance the Watcher exposes.
func TestSingleWatcherForBothFiles(t *testing.T) {
	pinDir := t.TempDir()
	regDir := t.TempDir()
	pinPath := filepath.Join(pinDir, "pinned-widgets.json")
	regPath := filepath.Join(regDir, "widget-registry.json")
	_ = os.WriteFile(pinPath, []byte("[]"), 0o644)
	_ = os.WriteFile(regPath, []byte("{}"), 0o644)

	w, err := hud.NewWatcher(pinPath, regPath)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer func() { _ = w.Close() }()

	got := w.WatchedPaths()
	if len(got) != 2 {
		t.Fatalf("WatchedPaths = %v (len %d), want both dirs on one watcher", got, len(got))
	}
	seen := map[string]bool{}
	for _, p := range got {
		seen[filepath.Clean(p)] = true
	}
	if !seen[filepath.Clean(pinDir)] || !seen[filepath.Clean(regDir)] {
		t.Fatalf("WatchedPaths missing pin or reg dir: got %v", got)
	}
}

// TestPinThreeFiresToastFrame asserts the toast emitted by Pin #3 carries the
// spec §13.7 frame shape (kind=notification.toast, payload.level=info, payload.text non-empty).
func TestPinThreeFiresToastFrame(t *testing.T) {
	dir := t.TempDir()
	pinPath := filepath.Join(dir, "pinned-widgets.json")
	regPath := filepath.Join(dir, "widget-registry.json")

	w, err := hud.NewWatcher(pinPath, regPath)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer func() { _ = w.Close() }()

	mk := func(id string) hud.PinnedEntry {
		return hud.PinnedEntry{ID: id, Type: "weather", Props: json.RawMessage(`{}`), Refresh: 300}
	}
	_, _ = w.Pin(mk("w1"))
	_, _ = w.Pin(mk("w2"))
	toast, err := w.Pin(mk("w3"))
	if err != nil {
		t.Fatalf("Pin w3: %v", err)
	}
	if toast == nil || toast.Kind != "notification.toast" {
		t.Fatalf("toast frame: got %+v", toast)
	}
	var p struct {
		Level string `json:"level"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(toast.Payload, &p); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if p.Level != "info" || p.Text == "" {
		t.Fatalf("toast payload: %+v", p)
	}
}

func writePins(t *testing.T, path string, entries []hud.PinnedEntry) {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename: %v", err)
	}
}
