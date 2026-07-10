//go:build darwin && cgo

package sqliteopen

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework CoreServices

#include <stdint.h>
#include <stdlib.h>

extern int leahWALWatchStart(uintptr_t handle, const char *path);
extern void leahWALWatchStop(int streamID);
*/
import "C"

import (
	"context"
	"errors"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"
)

var errClosed = errors.New("sqliteopen: WALWatcher closed")

// fsWatcher is the darwin FSEvents-backed WALWatcher. One CFRunLoop pump per
// instance is fine here — each push source watches one path, and the kernel
// coalesces filesystem events under the hood. WHY a per-instance stream: a
// process-wide singleton would force every consumer to filter on path, and the
// path set is small (3) and known at construction time.
type fsWatcher struct {
	mu       sync.Mutex
	closed   bool
	subs     []*fsWatchSub
	handle   cgo.Handle
	streamID C.int
}

type fsWatchSub struct {
	ch     chan WALEvent
	stop   func()
	closed bool
}

// NewFSEventsWatcher starts an FSEvents stream rooted at path's parent dir.
// FSEvents watches directories not files; the parent-dir scope is intentional
// — SQLite WAL truncation can recreate the file inode, and a file-scoped
// watch would silently stop firing after the first checkpoint.
func NewFSEventsWatcher(path string) (WALWatcher, error) {
	w := &fsWatcher{}
	w.handle = cgo.NewHandle(w)
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	id := C.leahWALWatchStart(C.uintptr_t(w.handle), cpath)
	if id < 0 {
		w.handle.Delete()
		return nil, errors.New("sqliteopen: FSEventStreamCreate failed")
	}
	w.streamID = id
	return w, nil
}

func (w *fsWatcher) Subscribe(ctx context.Context) (<-chan WALEvent, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, errClosed
	}
	sctx, cancel := context.WithCancel(ctx)
	s := &fsWatchSub{ch: make(chan WALEvent, 16), stop: cancel}
	w.subs = append(w.subs, s)
	go func() {
		<-sctx.Done()
		w.remove(s)
	}()
	return s.ch, nil
}

func (w *fsWatcher) remove(s *fsWatchSub) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
	for i, x := range w.subs {
		if x == s {
			w.subs = append(w.subs[:i], w.subs[i+1:]...)
			return
		}
	}
}

func (w *fsWatcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	subs := w.subs
	w.subs = nil
	id := w.streamID
	w.mu.Unlock()
	C.leahWALWatchStop(id)
	w.handle.Delete()
	for _, s := range subs {
		if !s.closed {
			s.closed = true
			close(s.ch)
		}
	}
	for _, s := range subs {
		s.stop()
	}
	return nil
}

func (w *fsWatcher) deliver() {
	ev := WALEvent{At: time.Now()}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, s := range w.subs {
		if s.closed {
			continue
		}
		select {
		case s.ch <- ev:
		default:
		}
	}
}

//export leahWALWatchDeliver
func leahWALWatchDeliver(handle C.uintptr_t) {
	h := cgo.Handle(handle)
	w, ok := h.Value().(*fsWatcher)
	if !ok {
		return
	}
	w.deliver()
}
