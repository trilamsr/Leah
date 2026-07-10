//go:build darwin

package discovery

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>

// leah_bonjour_publish registers the service via NSNetService. Returns 0 on
// success, non-zero on failure (e.g. port bind collision before mDNSResponder
// claims the record).
int leah_bonjour_publish(const char* name, int port);
void leah_bonjour_stop(void);
*/
import "C"

import (
	"context"
	"errors"
	"sync"
	"unsafe"
)

type darwinDiscovery struct {
	mu      sync.Mutex
	subs    []chan Peer
	stopped bool
}

func newPlatform() Discovery { return &darwinDiscovery{} }

func (d *darwinDiscovery) Publish(_ context.Context, name string, port uint16) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	if rc := C.leah_bonjour_publish(cname, C.int(port)); rc != 0 {
		return errors.New("discovery: NSNetService publish failed")
	}
	return nil
}

// Browse on darwin is wired to NSNetServiceBrowser via the bridge; the
// current bridge only emits the local publish (sufficient for the §2.4.3
// service-record contract). Network-side resolution lands with T11 transport.
func (d *darwinDiscovery) Browse(ctx context.Context) (<-chan Peer, error) {
	ch := make(chan Peer, 16)
	d.mu.Lock()
	d.subs = append(d.subs, ch)
	d.mu.Unlock()
	go func() {
		<-ctx.Done()
		d.mu.Lock()
		found := false
		for i, s := range d.subs {
			if s == ch {
				d.subs = append(d.subs[:i], d.subs[i+1:]...)
				found = true
				break
			}
		}
		d.mu.Unlock()
		if found {
			close(ch)
		}
	}()
	return ch, nil
}

func (d *darwinDiscovery) Stop() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	subs := d.subs
	d.subs = nil
	d.stopped = true
	d.mu.Unlock()
	// Close under no lock so Browse's ctx-done goroutine can re-enter d.mu.
	for _, ch := range subs {
		close(ch)
	}
	C.leah_bonjour_stop()
}
