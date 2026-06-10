//go:build darwin && cgo

package osevent

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework Contacts

#include <stdint.h>

// Implemented in bridge_darwin.m.
extern void leahOSEventStart(uintptr_t handle);
*/
import "C"

import (
	"context"
	"errors"
	"runtime"
	"runtime/cgo"
	"strings"
	"sync"
	"time"
)

// NewSource starts the macOS push pump. Exactly one pump runs per process
// (NSWorkspace observers are global); subsequent calls return the same Source.
func NewSource(cfg Config) (Source, error) {
	pumpOnce.Do(func() { pumpErr = startPump(cfg) })
	if pumpErr != nil {
		return nil, pumpErr
	}
	// Each NewSource call gets its own Blocklist projection over the singleton
	// pump — operator config is per-Source, not per-process.
	return &darwinSource{blocklist: append([]string(nil), cfg.Blocklist...)}, nil
}

var (
	pumpOnce sync.Once
	pumpErr  error
	pump     *darwinPump
)

type darwinPump struct {
	mu      sync.Mutex
	subs    map[*darwinSub]struct{}
	handle  cgo.Handle
	stopped bool
}

type darwinSub struct {
	kind      Kind
	ch        chan Event
	stop      func()
	blocklist []string
	closed    bool
}

type darwinSource struct {
	mu        sync.Mutex
	blocklist []string
	subs      []*darwinSub
	closed    bool
}

func startPump(_ Config) error {
	p := &darwinPump{subs: map[*darwinSub]struct{}{}}
	p.handle = cgo.NewHandle(p)
	ready := make(chan struct{})
	go func() {
		// Pin to one OS thread — CFRunLoop is per-thread, NSWorkspace observers
		// only fire on the thread that registered them.
		runtime.LockOSThread()
		close(ready)
		C.leahOSEventStart(C.uintptr_t(p.handle))
	}()
	<-ready
	pump = p
	return nil
}

func (s *darwinSource) Subscribe(ctx context.Context, kind Kind) (<-chan Event, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("osevent: source closed")
	}
	sctx, cancel := context.WithCancel(ctx)
	sub := &darwinSub{
		kind:      kind,
		ch:        make(chan Event, 64),
		stop:      cancel,
		blocklist: s.blocklist,
	}
	s.subs = append(s.subs, sub)
	s.mu.Unlock()

	pump.mu.Lock()
	pump.subs[sub] = struct{}{}
	pump.mu.Unlock()

	go func() {
		<-sctx.Done()
		s.removeSub(sub)
	}()
	return sub.ch, nil
}

func (s *darwinSource) removeSub(sub *darwinSub) {
	pump.mu.Lock()
	if sub.closed {
		pump.mu.Unlock()
		return
	}
	sub.closed = true
	delete(pump.subs, sub)
	close(sub.ch)
	pump.mu.Unlock()

	s.mu.Lock()
	for i, x := range s.subs {
		if x == sub {
			s.subs = append(s.subs[:i], s.subs[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
}

func (s *darwinSource) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	subs := s.subs
	s.subs = nil
	s.mu.Unlock()
	for _, sub := range subs {
		sub.stop()
		s.removeSub(sub)
	}
	return nil
}

// dispatchEvent is invoked from the cgo callback thread. Drop-on-full per
// subscriber; blocklist enforced at the source so a single bypass cannot leak.
func (p *darwinPump) dispatchEvent(kind Kind, bundleID, name string) {
	e := Event{Kind: kind, At: time.Now()}
	if bundleID != "" || name != "" {
		e.Detail = map[string]any{}
		if bundleID != "" {
			e.Detail["bundle_id"] = bundleID
		}
		if name != "" {
			e.Detail["name"] = name
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for sub := range p.subs {
		if sub.kind != kind || sub.closed {
			continue
		}
		if kind == WorkspaceActiveAppChanged && blockedBundle(sub.blocklist, bundleID) {
			continue
		}
		select {
		case sub.ch <- e:
		default:
		}
	}
}

func blockedBundle(blocklist []string, bundleID string) bool {
	for _, p := range blocklist {
		if p != "" && strings.HasPrefix(bundleID, p) {
			return true
		}
	}
	return false
}

//export leahOSEventDeliver
func leahOSEventDeliver(handle C.uintptr_t, kindC *C.char, bundleC *C.char, nameC *C.char) {
	h := cgo.Handle(handle)
	p, ok := h.Value().(*darwinPump)
	if !ok {
		return
	}
	kind := Kind(C.GoString(kindC))
	bundleID := C.GoString(bundleC)
	name := C.GoString(nameC)
	p.dispatchEvent(kind, bundleID, name)
}
