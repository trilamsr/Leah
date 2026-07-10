package plugin

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/trilam/leah/pkg/leahplugin"
)

// DefaultQuota is the per-minute IPC ceiling when a manifest omits ipc_quota.
var DefaultQuota = leahplugin.Quota{RPCPerMinute: 60, StreamBytesPerMinute: 1 << 20}

// ErrOverQuota fires when an RPC or byte charge would breach the per-minute window.
var ErrOverQuota = errors.New("plugin: over quota")

// QuotaMeter enforces per-plugin RPC + stream-byte ceilings on a 60-second rolling window.
type QuotaMeter struct {
	mu      sync.Mutex
	clock   func() time.Time
	wins    map[leahplugin.PluginID]*window
	configs map[leahplugin.PluginID]leahplugin.Quota
}

type window struct {
	startUnix int64
	rpcs      int
	bytes     int64
	pausedTo  int64
}

// NewQuotaMeter builds a meter with the supplied clock (test injection) — pass time.Now for production.
func NewQuotaMeter(clock func() time.Time) *QuotaMeter {
	if clock == nil {
		clock = time.Now
	}
	return &QuotaMeter{
		clock:   clock,
		wins:    map[leahplugin.PluginID]*window{},
		configs: map[leahplugin.PluginID]leahplugin.Quota{},
	}
}

// Configure sets the per-plugin quota — quota with non-positive fields falls back to DefaultQuota's matching field.
func (q *QuotaMeter) Configure(id leahplugin.PluginID, cfg leahplugin.Quota) {
	if cfg.RPCPerMinute <= 0 {
		cfg.RPCPerMinute = DefaultQuota.RPCPerMinute
	}
	if cfg.StreamBytesPerMinute <= 0 {
		cfg.StreamBytesPerMinute = DefaultQuota.StreamBytesPerMinute
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.configs[id] = cfg
}

// ChargeRPC bills one RPC against the current window; returns ErrOverQuota on breach + pauses the plugin for the rest of the window + 60s.
func (q *QuotaMeter) ChargeRPC(id leahplugin.PluginID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	cfg, w, now := q.windowLocked(id)
	if now < w.pausedTo {
		return ErrOverQuota
	}
	if w.rpcs+1 < 0 { // overflow guard for the int counter
		w.rpcs = math.MaxInt
		w.pausedTo = now + 60
		return ErrOverQuota
	}
	if w.rpcs+1 > cfg.RPCPerMinute {
		w.pausedTo = now + 60
		return ErrOverQuota
	}
	w.rpcs++
	return nil
}

// ChargeBytes bills n stream bytes; returns ErrOverQuota when the new total breaches the stream cap.
func (q *QuotaMeter) ChargeBytes(id leahplugin.PluginID, n int64) error {
	if n <= 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	cfg, w, now := q.windowLocked(id)
	if now < w.pausedTo {
		return ErrOverQuota
	}
	if n > math.MaxInt64-w.bytes {
		w.bytes = math.MaxInt64
		w.pausedTo = now + 60
		return ErrOverQuota
	}
	if w.bytes+n > cfg.StreamBytesPerMinute {
		w.pausedTo = now + 60
		return ErrOverQuota
	}
	w.bytes += n
	return nil
}

// PausedUntil returns the unix-second timestamp this plugin is paused until — 0 means not paused.
func (q *QuotaMeter) PausedUntil(id leahplugin.PluginID) int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	w, ok := q.wins[id]
	if !ok {
		return 0
	}
	return w.pausedTo
}

// windowLocked returns the active 60s window for id, rolling over when the current second exceeds the window start by 60.
func (q *QuotaMeter) windowLocked(id leahplugin.PluginID) (leahplugin.Quota, *window, int64) {
	now := q.clock().Unix()
	w, ok := q.wins[id]
	if !ok || now-w.startUnix >= 60 {
		w = &window{startUnix: now}
		q.wins[id] = w
	}
	cfg, ok := q.configs[id]
	if !ok {
		cfg = DefaultQuota
	}
	return cfg, w, now
}
