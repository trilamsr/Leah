// Package widget is the daemon-side widget catalog: envelope schema
// (spec §10.7) plus per-kind adapters. The 256 KB cap on Validate()
// mirrors ipc.MaxFrameBytes so an envelope that fits here is guaranteed
// to fit in one IPC frame.
package widget

import (
	"encoding/json"
	"fmt"
	"regexp"
)

const MaxEnvelopeBytes = 256 * 1024

var (
	idPattern       = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
	callbackPattern = regexp.MustCompile(`^leah://action/[a-z_]+(\?.*)?$`)
)

type Action struct {
	Label    string `json:"label"`
	Callback string `json:"callback"`
	Icon     string `json:"icon,omitempty"`
}

type Envelope struct {
	Widget  string          `json:"widget"`
	ID      string          `json:"id"`
	Size    string          `json:"size"`
	Refresh *int            `json:"refresh,omitempty"`
	Actions []Action        `json:"actions,omitempty"`
	Props   json.RawMessage `json:"props,omitempty"`
}

var validKinds = map[string]struct{}{
	"market": {}, "flights": {}, "calendar": {}, "weather": {},
	"maps": {}, "table": {}, "chart": {}, "image": {},
	"code": {}, "citation": {}, "stat": {}, "list": {}, "diff": {},
}

var validSizes = map[string]struct{}{
	"small": {}, "medium": {}, "large": {}, "hero": {},
}

func (e *Envelope) Validate() error {
	if e.Widget == "" {
		return fmt.Errorf("widget kind required")
	}
	if _, ok := validKinds[e.Widget]; !ok {
		return fmt.Errorf("unknown widget kind %q", e.Widget)
	}
	if e.ID == "" {
		return fmt.Errorf("id required")
	}
	if !idPattern.MatchString(e.ID) {
		return fmt.Errorf("id %q must match %s", e.ID, idPattern)
	}
	if e.Size == "" {
		return fmt.Errorf("size required")
	}
	if _, ok := validSizes[e.Size]; !ok {
		return fmt.Errorf("unknown size %q", e.Size)
	}
	if e.Refresh != nil && *e.Refresh < 5 {
		return fmt.Errorf("refresh %d below minimum 5s", *e.Refresh)
	}
	if len(e.Actions) > 4 {
		return fmt.Errorf("actions %d exceeds max 4", len(e.Actions))
	}
	for i, a := range e.Actions {
		if a.Label == "" || a.Callback == "" {
			return fmt.Errorf("action[%d] missing label or callback", i)
		}
		if len(a.Label) > 32 {
			return fmt.Errorf("action[%d] label %d chars exceeds max 32", i, len(a.Label))
		}
		if !callbackPattern.MatchString(a.Callback) {
			return fmt.Errorf("action[%d] callback %q must match %s", i, a.Callback, callbackPattern)
		}
	}
	if len(e.Props) > MaxEnvelopeBytes {
		return fmt.Errorf("props %d bytes exceeds 256 KB cap", len(e.Props))
	}
	return nil
}
