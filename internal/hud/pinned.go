package hud

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/trilam/leah/internal/ipc"
)

// PinnedEntry is one slot in ~/Library/Application Support/Leah/pinned-widgets.json.
type PinnedEntry struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Props   json.RawMessage `json:"props"`
	Refresh int             `json:"refresh"`
}

// PinnedChanged is emitted by Watcher after the 200 ms debounce settles
// following any pinned-widgets.json or widget-registry.json write.
type PinnedChanged struct {
	Entries []PinnedEntry
}

const (
	maxPinned        = 2
	debounceInterval = 200 * time.Millisecond
)

// Watcher wraps a SINGLE fsnotify.Watcher across pinned-widgets.json and
// widget-registry.json. Spec § 10.2 mandates this single-watcher invariant
// — debounceInterval coalesces APFS atomic-rename's 2-3 events per write
// into one PinnedChanged, so subscribers don't redraw the HUD three times
// per pin/unpin.
type Watcher struct {
	pinPath string
	regPath string

	mu sync.Mutex
	fs *fsnotify.Watcher
	// fsWatchers makes the §10.2 single-watcher invariant structurally
	// testable — timing-based goroutine counts proved tautological.
	fsWatchers int
}

// NumFSWatchers returns the count of underlying fsnotify.Watcher instances.
// Spec § 10.2 requires exactly 1 across both files.
func (w *Watcher) NumFSWatchers() int { return w.fsWatchers }

// NewWatcher registers the parent directories of both files. fsnotify watches
// dirs (not files) so atomic-rename writes land as Create on the target name.
func NewWatcher(pinPath, regPath string) (*Watcher, error) {
	f, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("hud.Watcher: fsnotify: %w", err)
	}
	w := &Watcher{pinPath: pinPath, regPath: regPath, fs: f, fsWatchers: 1}
	for _, p := range []string{pinPath, regPath} {
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("hud.Watcher: mkdir %s: %w", dir, err)
		}
		if err := f.Add(dir); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("hud.Watcher: watch %s: %w", dir, err)
		}
	}
	return w, nil
}

func (w *Watcher) Close() error { return w.fs.Close() }

// Run blocks until ctx is cancelled. Source of truth for onChange is the
// file content read after debounce — fsnotify self-fires on local save() so
// Pin/Unpin do not need to call onChange directly. List() takes w.mu, so
// a Pin holding the lock briefly delays fire(); the emitted snapshot then
// reflects the post-Pin file (which is what the operator wants anyway).
func (w *Watcher) Run(ctx context.Context, onChange func(PinnedChanged)) error {
	var debounce *time.Timer
	stopDebounce := func() {
		if debounce != nil {
			debounce.Stop()
			debounce = nil
		}
	}
	defer stopDebounce()

	fire := func() {
		entries, err := w.List()
		if err != nil {
			log.Printf("hud.Watcher: list after change: %v", err)
			return
		}
		onChange(PinnedChanged{Entries: entries})
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.fs.Events:
			if !ok {
				return nil
			}
			name := filepath.Clean(ev.Name)
			if name != filepath.Clean(w.pinPath) && name != filepath.Clean(w.regPath) {
				continue
			}
			stopDebounce()
			debounce = time.AfterFunc(debounceInterval, fire)
		case err, ok := <-w.fs.Errors:
			if !ok {
				return nil
			}
			if err != nil {
				log.Printf("hud.Watcher: fsnotify error: %v", err)
			}
		}
	}
}

// Pin appends e, enforcing the § 10.3 2-slot cap. Idempotent on same ID.
// Returns a non-nil § 13.7 toast frame when the cap is hit and leaves the
// pin file untouched — never silently evict (§ 10.3 explicit).
func (w *Watcher) Pin(e PinnedEntry) (*ipc.Frame, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, err := w.load()
	if err != nil {
		return nil, err
	}
	for i, ex := range entries {
		if ex.ID == e.ID {
			entries[i] = e
			return nil, w.save(entries)
		}
	}
	if len(entries) >= maxPinned {
		payload, _ := json.Marshal(struct {
			Level string `json:"level"`
			Text  string `json:"text"`
		}{
			Level: "info",
			Text:  "Ambient is full · unpin one to add this one.",
		})
		return &ipc.Frame{Kind: ipc.KindNotificationToast, Payload: payload}, nil
	}
	return nil, w.save(append(entries, e))
}

func (w *Watcher) Unpin(id string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, err := w.load()
	if err != nil {
		return err
	}
	out := entries[:0]
	for _, e := range entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	return w.save(out)
}

func (w *Watcher) List() ([]PinnedEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.load()
}

func (w *Watcher) load() ([]PinnedEntry, error) {
	data, err := os.ReadFile(w.pinPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hud.Watcher: read pinned: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var entries []PinnedEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("hud.Watcher: parse pinned: %w", err)
	}
	return entries, nil
}

func (w *Watcher) save(entries []PinnedEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("hud.Watcher: marshal pinned: %w", err)
	}
	tmp := w.pinPath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(w.pinPath), 0o755); err != nil {
		return fmt.Errorf("hud.Watcher: mkdir: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("hud.Watcher: write tmp: %w", err)
	}
	if err := os.Rename(tmp, w.pinPath); err != nil {
		return fmt.Errorf("hud.Watcher: rename: %w", err)
	}
	return nil
}
