package main

// §17.9 push-source substrate: fan internal/macos/{mail,contacts,focus,
// activeapp}/push.go runners into a single IPC frame stream so HUD widgets
// subscribe to a stable `push.*` namespace instead of each producer's
// internal obs.Event kind. translatePushKind is the seam — rename a producer
// kind and only this file needs to change.

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/trilam/leah/internal/macos/activeapp"
	"github.com/trilam/leah/internal/macos/contacts"
	"github.com/trilam/leah/internal/macos/focus"
	"github.com/trilam/leah/internal/macos/mail"
	"github.com/trilam/leah/internal/macos/sqliteopen"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/osevent"
)

// runPushSourcesFromChan is the test seam. Production runPushSources composes
// each producer's ObsEmit into a buffered channel and forwards here.
func runPushSourcesFromChan(ctx context.Context, in <-chan obs.Event, emit func(kind string, payload []byte)) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-in:
			if !ok {
				return
			}
			kind := translatePushKind(ev.Kind)
			if kind == "" {
				continue
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			emit(kind, payload)
		}
	}
}

// translatePushKind maps producer-internal obs.Event kinds onto the daemon's
// public `push.*` IPC namespace. Empty return = drop (unknown producer kind).
func translatePushKind(obsKind string) string {
	switch obsKind {
	case "mail_changed":
		return "push.mail"
	case "contact_store_changed":
		return "push.contacts"
	case "focus.state_changed":
		return "push.focus"
	case "workspace.active_app_changed":
		return "push.activeapp"
	}
	return ""
}

// runPushSources is the production entry. It constructs the four macOS push
// sources, fans their ObsEmit calls into a single buffered channel, and
// forwards through runPushSourcesFromChan → emit. Each producer that fails
// to construct (e.g. focus on a non-darwin build, mail with no
// Envelope Index) is logged and skipped; remaining sources keep running.
func runPushSources(ctx context.Context, lg *slog.Logger, registry *obs.Registry, mailWatcher sqliteopen.WALWatcher, emit func(kind string, payload []byte)) {
	if emit == nil {
		return
	}
	fan := make(chan obs.Event, 64)
	obsEmit := func(e obs.Event) {
		select {
		case fan <- e:
		case <-ctx.Done():
		default:
			// Drop on full queue rather than block a producer goroutine —
			// debounce + HUD subscribers should keep up; backpressure into
			// the producer would stall the os-event pump.
		}
	}

	var wg sync.WaitGroup

	// activeapp + contacts + focus all share the osevent.Source pump.
	if src, err := osevent.NewSource(osevent.Config{Blocklist: activeapp.DefaultBlocklist}); err == nil {
		p := &activeapp.PushSource{Source: src, ObsEmit: obsEmit, Debounce: activeapp.DefaultDebounce}
		wg.Add(1)
		obs.SafeGo(lg, registry, "pushsource-activeapp", func() {
			defer wg.Done()
			if err := p.Run(ctx); err != nil && ctx.Err() == nil {
				lg.Warn("activeapp push exit", "err", err)
			}
			_ = src.Close()
		})
	} else {
		lg.Warn("activeapp push disabled", "err", err)
	}

	if src, err := osevent.NewSource(osevent.Config{}); err == nil {
		p := &contacts.PushSource{Source: src, ObsEmit: obsEmit, Debounce: 250 * time.Millisecond}
		wg.Add(1)
		obs.SafeGo(lg, registry, "pushsource-contacts", func() {
			defer wg.Done()
			if err := p.Run(ctx); err != nil && ctx.Err() == nil {
				lg.Warn("contacts push exit", "err", err)
			}
			_ = src.Close()
		})
	} else {
		lg.Warn("contacts push disabled", "err", err)
	}

	if src, err := osevent.NewSource(osevent.Config{}); err == nil {
		p := &focus.PushSource{Source: src, ObsEmit: obsEmit, Debounce: 250 * time.Millisecond}
		wg.Add(1)
		obs.SafeGo(lg, registry, "pushsource-focus", func() {
			defer wg.Done()
			if err := p.Run(ctx); err != nil && ctx.Err() == nil {
				lg.Warn("focus push exit", "err", err)
			}
			_ = src.Close()
		})
	} else {
		lg.Warn("focus push disabled", "err", err)
	}

	if mailWatcher != nil {
		p := &mail.PushSource{Watcher: mailWatcher, ObsEmit: obsEmit, Debounce: 500 * time.Millisecond}
		wg.Add(1)
		obs.SafeGo(lg, registry, "pushsource-mail", func() {
			defer wg.Done()
			if err := p.Run(ctx); err != nil && ctx.Err() == nil {
				lg.Warn("mail push exit", "err", err)
			}
		})
	}

	// Fan-out goroutine. Closes the channel once all producers exit so the
	// forwarder returns naturally instead of blocking the daemon shutdown.
	go func() {
		wg.Wait()
		close(fan)
	}()

	runPushSourcesFromChan(ctx, fan, emit)
}
