// Package plugin owns the daemon-side host: manifest validator, signed-bundle
// gate (delegated to attest.Verifier), sandbox subprocess launcher, and the
// IPC quota meter. Public SDK surface plugin authors import lives in
// pkg/leahplugin/.
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/trilam/leah/pkg/leahplugin"
)

// Manifest is the host-side alias of the SDK manifest type so callers in this package don't need a transitive import.
type Manifest = leahplugin.Manifest

// ManifestAuthor is the host-side alias for the SDK author struct.
type ManifestAuthor = leahplugin.ManifestAuthor

// Capability is the host-side alias for the SDK capability struct.
type Capability = leahplugin.Capability

// Permissions is the host-side alias for the SDK permissions struct.
type Permissions = leahplugin.Permissions

// Quota is the host-side alias for the SDK quota struct.
type Quota = leahplugin.Quota

// UISpec is the host-side alias for the SDK UI spec struct.
type UISpec = leahplugin.UISpec

// ErrManifestRequiredField fires when id/name/version are missing — these three are load-bearing for routing + display.
var ErrManifestRequiredField = errors.New("manifest: id, name, version required")

// ErrManifestUnsupportedSchema fires when schema_version != 1; future versions get their own constant rather than silent coercion.
var ErrManifestUnsupportedSchema = errors.New("manifest: unsupported schema_version")

// ParseManifest decodes + validates the raw manifest.json bytes per spec §7.3.
func ParseManifest(raw []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("manifest: %w", err)
	}
	if m.SchemaVersion != leahplugin.SchemaVersion {
		return m, ErrManifestUnsupportedSchema
	}
	if m.ID == "" || m.Name == "" || m.Version == "" {
		return m, ErrManifestRequiredField
	}
	return m, nil
}

// ReadManifest loads + validates manifest.json out of a bundle dir.
func ReadManifest(bundlePath string) (Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(bundlePath, "Contents", "manifest.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	return ParseManifest(raw)
}
