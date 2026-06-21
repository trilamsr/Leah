package maps

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	ProviderGoogle = "google"
	ProviderOSM    = "osm"
)

type providerConfig struct {
	Provider string `json:"provider"`
}

func DefaultProviderPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".leah-state", "maps.json"), nil
}

// SelectProvider reads maps.json; missing/unknown resolves to Google (default
// unchanged) — only an explicit "osm" opts into the sovereign backend.
func SelectProvider(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProviderGoogle, nil
		}
		return "", err
	}
	var cfg providerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", err
	}
	if cfg.Provider == ProviderOSM {
		return ProviderOSM, nil
	}
	return ProviderGoogle, nil
}
