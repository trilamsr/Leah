package hud

import (
	"encoding/json"
	"fmt"
	"os"
)

type RegistryEntry struct {
	Refresh int `json:"refresh"`
}

// ReadRegistry treats missing file as empty map so first-launch HUD doesn't
// surface fsnotify ENOENT noise. The § 10.2 Watcher covers this file too.
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
