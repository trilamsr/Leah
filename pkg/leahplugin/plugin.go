package leahplugin

// PluginID is the canonical id (typically reverse-DNS) the host uses to address a plugin.
type PluginID string

// PluginInfo is the operator-facing snapshot the host's List returns.
type PluginInfo struct {
	ID          PluginID
	Name        string
	Version     string
	Enabled     bool
	AttestState string
}

// LogLine is one persisted log row surfaced by Host.Logs.
type LogLine struct {
	At    int64
	Level LogLevel
	Msg   string
	KV    []byte
}
