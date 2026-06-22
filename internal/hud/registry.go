package hud

import (
	"encoding/json"
	"fmt"
	"os"
)

// RegistryEntry is one row in widget-registry.json — kind → metadata.
type RegistryEntry struct {
	Refresh int `json:"refresh"`
}

// ReadRegistry loads widget-registry.json. Missing file is a non-error
// (empty map) so first-launch HUD doesn't surface fsnotify ENOENT noise.
// The same Watcher (NewWatcher in pinned.go) covers writes to this file.
func ReadRegistry(path string) (map[string]RegistryEntry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]RegistryEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hud.Registry: read: %w", err)
	}
	if len(data) == 0 {
		return map[string]RegistryEntry{}, nil
	}
	out := map[string]RegistryEntry{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("hud.Registry: parse: %w", err)
	}
	return out, nil
}
