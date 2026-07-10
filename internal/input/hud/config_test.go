package hud

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
)

func TestLoadConfig_FileMissing_ReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hud-config.json")

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig missing file: %v", err)
	}
	want := DefaultConfig()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing-file config mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestLoadConfig_HappyPath_ParsesAccentHotkey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hud-config.json")
	raw := `{
        "AccentColor": "#ff8800",
        "Hotkey": "cmd+shift+k",
        "NewsAllowlist": ["nytimes.com", "bbc.co.uk"],
        "BodyBlocklist": ["password"],
        "Position": "top-left"
    }`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.AccentColor != "#ff8800" {
		t.Errorf("AccentColor=%q want #ff8800", got.AccentColor)
	}
	if got.Hotkey != "cmd+shift+k" {
		t.Errorf("Hotkey=%q want cmd+shift+k", got.Hotkey)
	}
	if got.Position != PositionTopLeft {
		t.Errorf("Position=%q want top-left", got.Position)
	}
	if !reflect.DeepEqual(got.NewsAllowlist, []string{"nytimes.com", "bbc.co.uk"}) {
		t.Errorf("NewsAllowlist=%v", got.NewsAllowlist)
	}
}

func TestSaveConfig_0600Mode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix perms")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "hud-config.json")

	c := DefaultConfig()
	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode=%v want 0600", mode)
	}
}

func TestSaveConfig_PreservesAllowlist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hud-config.json")

	c := DefaultConfig()
	c.NewsAllowlist = []string{"apnews.com", "reuters.com"}
	c.BodyBlocklist = append(c.BodyBlocklist, "diagnosis")
	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	sort.Strings(got.NewsAllowlist)
	if !reflect.DeepEqual(got.NewsAllowlist, []string{"apnews.com", "reuters.com"}) {
		t.Errorf("NewsAllowlist round-trip: %v", got.NewsAllowlist)
	}
	found := false
	for _, w := range got.BodyBlocklist {
		if w == "diagnosis" {
			found = true
		}
	}
	if !found {
		t.Errorf("blocklist lost 'diagnosis': %v", got.BodyBlocklist)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("on-disk file is not JSON: %v", err)
	}
}

func TestConfig_AccentColor_RejectsInvalidHex(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		ok   bool
	}{
		{"6-digit", "#5af2ff", true},
		{"uppercase", "#5AF2FF", true},
		{"3-digit", "#abc", true},
		{"no-hash", "5af2ff", false},
		{"too-short", "#12345", false},
		{"non-hex", "#zzzzzz", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAccentColor(tc.hex)
			if tc.ok && err != nil {
				t.Errorf("want valid, got err=%v", err)
			}
			if !tc.ok && err == nil {
				t.Errorf("want error for %q, got nil", tc.hex)
			}
		})
	}

	c := DefaultConfig()
	c.AccentColor = "not-a-color"
	dir := t.TempDir()
	if err := c.Save(filepath.Join(dir, "x.json")); err == nil {
		t.Errorf("Save accepted invalid AccentColor")
	}
}

func TestConfig_BlocklistFilters_RedactsBody(t *testing.T) {
	c := DefaultConfig()
	c.BodyBlocklist = []string{"password", "ssn"}

	in := "Your password is hunter2 and the SSN field is empty."
	out := c.RedactBody(in)
	if out == in {
		t.Fatalf("RedactBody no-op on blocklisted body")
	}
	if containsCase(out, "hunter2") && !containsCase(out, "[REDACTED]") {
		t.Errorf("expected redaction marker, got %q", out)
	}
	// Case-insensitive: "SSN" matches blocklist entry "ssn".
	if containsCase(out, "SSN field") {
		t.Errorf("expected case-insensitive redaction, got %q", out)
	}
	// Non-blocklisted text survives.
	clean := "weather is clear"
	if c.RedactBody(clean) != clean {
		t.Errorf("clean body was modified: %q", c.RedactBody(clean))
	}
}

func containsCase(s, sub string) bool {
	return len(s) >= len(sub) && indexFold(s, sub) >= 0
}

func indexFold(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFold(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ai, bi := a[i], b[i]
		if ai >= 'A' && ai <= 'Z' {
			ai += 'a' - 'A'
		}
		if bi >= 'A' && bi <= 'Z' {
			bi += 'a' - 'A'
		}
		if ai != bi {
			return false
		}
	}
	return true
}
