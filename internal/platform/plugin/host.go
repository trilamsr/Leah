package plugin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/trilam/leah/internal/platform/attest"
	"github.com/trilam/leah/pkg/leahplugin"
)

// PluginID is the host-side alias for the SDK's PluginID.
type PluginID = leahplugin.PluginID

// PluginInfo is the host-side alias for the SDK's PluginInfo.
type PluginInfo = leahplugin.PluginInfo

// LogLine is the host-side alias for the SDK's LogLine.
type LogLine = leahplugin.LogLine

// ErrPluginNotFound fires when a host op references a plugin id that isn't in the plugin table.
var ErrPluginNotFound = errors.New("plugin: not found")

// ErrPluginAttestFailed fires when attest.Verifier returns Failed on install or reload — load is blocked.
var ErrPluginAttestFailed = errors.New("plugin: attestation failed")

// Host is the daemon-side facade the operator UI + leah CLI drive.
type Host interface {
	Install(ctx context.Context, bundlePath string) (PluginID, error)
	Uninstall(ctx context.Context, id PluginID) error
	Enable(ctx context.Context, id PluginID) error
	Disable(ctx context.Context, id PluginID) error
	Reload(ctx context.Context, id PluginID) error
	List() []PluginInfo
	Logs(ctx context.Context, id PluginID, tail int) ([]LogLine, error)
}

// HostConfig wires the host's dependencies — caller owns lifecycle of db + verifier + sandbox.
type HostConfig struct {
	DB       *sql.DB
	Verifier attest.Verifier
	Sandbox  Sandbox
	Quota    *QuotaMeter
	Clock    func() time.Time
}

type host struct {
	cfg HostConfig
	mu  sync.Mutex
}

// NewHost builds the daemon's plugin host. Verifier MUST be non-nil; unsigned bundles cannot be loaded otherwise (security ceiling §7.7).
func NewHost(cfg HostConfig) (Host, error) {
	if cfg.DB == nil {
		return nil, errors.New("plugin: HostConfig.DB required")
	}
	if cfg.Verifier == nil {
		return nil, errors.New("plugin: HostConfig.Verifier required")
	}
	if cfg.Sandbox == nil {
		cfg.Sandbox = NewSandbox(nil)
	}
	if cfg.Quota == nil {
		cfg.Quota = NewQuotaMeter(cfg.Clock)
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &host{cfg: cfg}, nil
}

// Install verifies + records a bundle. Returns the manifest id. Attest.Failed blocks the insert; Stale warns but proceeds (matches verifier semantics).
func (h *host) Install(ctx context.Context, bundlePath string) (PluginID, error) {
	abs, err := filepath.Abs(bundlePath)
	if err != nil {
		return "", fmt.Errorf("plugin: resolve bundle path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("plugin: bundle not a directory: %s", abs)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(abs, "Contents", "manifest.json"))
	if err != nil {
		return "", fmt.Errorf("plugin: read manifest: %w", err)
	}
	mf, err := ParseManifest(manifestBytes)
	if err != nil {
		return "", err
	}
	att, err := h.cfg.Verifier.VerifyPlugin(ctx, abs)
	if err != nil {
		return "", err
	}
	if att.State == attest.Failed {
		return "", fmt.Errorf("%w: %s", ErrPluginAttestFailed, att.Reason)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, err := h.cfg.DB.ExecContext(ctx,
		`INSERT INTO plugin(id,name,version,installed_at,enabled,manifest,bundle_path,attest_state)
		 VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
		   name=excluded.name,
		   version=excluded.version,
		   manifest=excluded.manifest,
		   bundle_path=excluded.bundle_path,
		   attest_state=excluded.attest_state`,
		mf.ID, mf.Name, mf.Version, h.cfg.Clock().Unix(), 1, manifestBytes, abs, string(att.State),
	); err != nil {
		return "", fmt.Errorf("plugin: insert: %w", err)
	}
	h.cfg.Quota.Configure(PluginID(mf.ID), mf.IPCQuota)
	return PluginID(mf.ID), nil
}

// Uninstall removes the row + cascades plugin_log rows via the ON DELETE CASCADE in 008-plugin.sql.
func (h *host) Uninstall(ctx context.Context, id PluginID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	res, err := h.cfg.DB.ExecContext(ctx, `DELETE FROM plugin WHERE id=?`, string(id))
	if err != nil {
		return fmt.Errorf("plugin: delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// Enable flips enabled=1; reload of the sandbox process is the daemon's responsibility (lifecycle, not state).
func (h *host) Enable(ctx context.Context, id PluginID) error {
	return h.setEnabled(ctx, id, true)
}

// Disable flips enabled=0; daemon stops the subprocess separately.
func (h *host) Disable(ctx context.Context, id PluginID) error {
	return h.setEnabled(ctx, id, false)
}

func (h *host) setEnabled(ctx context.Context, id PluginID, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	res, err := h.cfg.DB.ExecContext(ctx, `UPDATE plugin SET enabled=? WHERE id=?`, v, string(id))
	if err != nil {
		return fmt.Errorf("plugin: update enabled: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// Reload re-verifies the bundle and updates attest_state — Failed flips enabled=0 + returns ErrPluginAttestFailed (§7.7 sig invalid at runtime recheck).
func (h *host) Reload(ctx context.Context, id PluginID) error {
	h.mu.Lock()
	var bundlePath string
	row := h.cfg.DB.QueryRowContext(ctx, `SELECT bundle_path FROM plugin WHERE id=?`, string(id))
	if err := row.Scan(&bundlePath); err != nil {
		h.mu.Unlock()
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPluginNotFound
		}
		return err
	}
	h.mu.Unlock()
	att, err := h.cfg.Verifier.VerifyPlugin(ctx, bundlePath)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	enabled := 1
	if att.State == attest.Failed {
		enabled = 0
	}
	if _, err := h.cfg.DB.ExecContext(ctx, `UPDATE plugin SET attest_state=?, enabled=? WHERE id=?`, string(att.State), enabled, string(id)); err != nil {
		return fmt.Errorf("plugin: update attest_state: %w", err)
	}
	if att.State == attest.Failed {
		return fmt.Errorf("%w: %s", ErrPluginAttestFailed, att.Reason)
	}
	return nil
}

// List snapshots installed plugins for the Settings pane.
func (h *host) List() []PluginInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	rows, err := h.cfg.DB.Query(`SELECT id,name,version,enabled,attest_state FROM plugin ORDER BY name`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var out []PluginInfo
	for rows.Next() {
		var pi PluginInfo
		var enabled int
		if err := rows.Scan(&pi.ID, &pi.Name, &pi.Version, &enabled, &pi.AttestState); err != nil {
			return out
		}
		pi.Enabled = enabled == 1
		out = append(out, pi)
	}
	return out
}

// Logs returns the most recent tail rows from plugin_log for id, newest first.
func (h *host) Logs(ctx context.Context, id PluginID, tail int) ([]LogLine, error) {
	if tail <= 0 {
		tail = 100
	}
	rows, err := h.cfg.DB.QueryContext(ctx,
		`SELECT at, level, msg, kv FROM plugin_log WHERE plugin_id=? ORDER BY id DESC LIMIT ?`,
		string(id), tail)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []LogLine
	for rows.Next() {
		var ll LogLine
		var level int
		if err := rows.Scan(&ll.At, &level, &ll.Msg, &ll.KV); err != nil {
			return out, err
		}
		ll.Level = leahplugin.LogLevel(level)
		out = append(out, ll)
	}
	return out, nil
}

// AppendLog persists one log line — daemon-internal helper plugin subprocesses route their slog handler through.
func (h *host) AppendLog(ctx context.Context, id PluginID, level leahplugin.LogLevel, msg string, kv []byte) error {
	_, err := h.cfg.DB.ExecContext(ctx,
		`INSERT INTO plugin_log(plugin_id, at, level, msg, kv) VALUES(?,?,?,?,?)`,
		string(id), h.cfg.Clock().Unix(), int(level), msg, kv)
	return err
}
