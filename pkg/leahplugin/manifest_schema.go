package leahplugin

// SchemaVersion is the manifest schema version this SDK targets.
const SchemaVersion = 1

// Manifest is the on-disk declaration the host parses at install + load.
type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	MinLeah       string         `json:"min_leah"`
	Author        ManifestAuthor `json:"author"`
	Capabilities  []Capability   `json:"capabilities"`
	Permissions   Permissions    `json:"permissions"`
	IPCQuota      Quota          `json:"ipc_quota"`
	UI            UISpec         `json:"ui"`
}

// ManifestAuthor identifies the plugin author for the operator UI.
type ManifestAuthor struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Capability is one entry in the manifest capabilities array.
type Capability struct {
	Kind     string   `json:"kind"`
	Type     string   `json:"type,omitempty"`
	Name     string   `json:"name,omitempty"`
	Renderer string   `json:"renderer,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
}

// Permissions is the daemon-enforced ceiling on what the plugin may touch.
type Permissions struct {
	Network  []string `json:"network"`
	FSRead   []string `json:"fs.read"`
	FSWrite  []string `json:"fs.write"`
	Keychain []string `json:"keychain"`
}

// Quota is the per-minute IPC budget enforced by the daemon.
type Quota struct {
	RPCPerMinute         int   `json:"rpc_per_minute"`
	StreamBytesPerMinute int64 `json:"stream_bytes_per_minute"`
}

// UISpec points the operator UI at the plugin's icon + settings pane assets.
type UISpec struct {
	Icon         string `json:"icon"`
	SettingsPane string `json:"settings_pane"`
}
