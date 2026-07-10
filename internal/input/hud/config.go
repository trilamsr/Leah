package hud

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Position string

const (
	PositionTopRight    Position = "top-right"
	PositionTopLeft     Position = "top-left"
	PositionBottomRight Position = "bottom-right"
	PositionBottomLeft  Position = "bottom-left"
)

// Config is operator-tunable HUD state persisted at ~/.leah-state/hud-config.json.
type Config struct {
	AccentColor   string
	Hotkey        string
	NewsAllowlist []string
	BodyBlocklist []string
	Position      Position
}

func DefaultConfig() Config {
	return Config{
		AccentColor:   "#5af2ff",
		Hotkey:        "cmd+shift+l",
		NewsAllowlist: []string{},
		BodyBlocklist: []string{"password", "banking", "ssn", "mfa", "otp", "recovery", "secret", "token"},
		Position:      PositionTopRight,
	}
}

var hexColorRE = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

func ValidateAccentColor(s string) error {
	if !hexColorRE.MatchString(s) {
		return fmt.Errorf("invalid accent color %q: want #rgb or #rrggbb", s)
	}
	return nil
}

func validPosition(p Position) bool {
	switch p {
	case PositionTopRight, PositionTopLeft, PositionBottomRight, PositionBottomLeft:
		return true
	}
	return false
}

func LoadConfig(path string) (Config, error) {
	c := DefaultConfig()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read hud config: %w", err)
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return DefaultConfig(), fmt.Errorf("parse hud config: %w", err)
	}
	if c.NewsAllowlist == nil {
		c.NewsAllowlist = []string{}
	}
	if c.BodyBlocklist == nil {
		c.BodyBlocklist = DefaultConfig().BodyBlocklist
	}
	if !validPosition(c.Position) {
		c.Position = PositionTopRight
	}
	return c, nil
}

func (c Config) Save(path string) error {
	if err := ValidateAccentColor(c.AccentColor); err != nil {
		return err
	}
	if !validPosition(c.Position) {
		return fmt.Errorf("invalid position %q", c.Position)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir hud config: %w", err)
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hud config: %w", err)
	}
	return os.WriteFile(path, raw, 0o600)
}

// RedactBody replaces case-insensitive blocklist matches with [REDACTED];
// the HUD overlays this with `filter: blur(8px)` per spec § Privacy.
func (c Config) RedactBody(s string) string {
	out := s
	for _, w := range c.BodyBlocklist {
		if w == "" {
			continue
		}
		re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(w) + `\b\S*`)
		if err != nil {
			continue
		}
		out = re.ReplaceAllString(out, "[REDACTED]")
	}
	return out
}

// ConfigPath resolves ~/.leah-state/hud-config.json (LEAH_STATE_DIR honored).
func ConfigPath() string {
	d := os.Getenv("LEAH_STATE_DIR")
	if d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".leah-state")
	}
	return filepath.Join(d, "hud-config.json")
}
