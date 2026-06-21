//go:build !(darwin && cgo)

package sqliteopen

import "errors"

var errClosed = errors.New("sqliteopen: WALWatcher closed")

// NewFSEventsWatcher is darwin-only — FSEvents is a macOS framework. The stub
// errors so non-darwin callers fail loudly rather than silently no-op.
func NewFSEventsWatcher(path string) (WALWatcher, error) {
	return nil, errors.New("sqliteopen: FSEvents watcher requires darwin+cgo")
}
