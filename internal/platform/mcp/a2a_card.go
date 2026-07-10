package mcp

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/trilam/leah/internal/attest"
)

// A2A protocol version pinned to spec §14; bumping = wave-spec, not silent.
const A2AProtocolVersion = "1.0"

// AgentCardWellKnownPath is the A2A 1.0 §8.2 IANA-registered URI.
const AgentCardWellKnownPath = "/.well-known/agent-card.json"

// canonicalCard builds the on-the-wire card. Gate fields (skills[*].x-leah.*,
// securitySchemes) are regenerated from code on every read; operator-edits to
// non-gate fields (name, description, version) are preserved from disk.
func canonicalCard(addr string, operatorOverrides map[string]any) map[string]any {
	card := map[string]any{
		"name":        "leah",
		"description": "Operator's local agent. Audit, memory, SelfBuild.",
		"version":     "0.1.0",
		"supportedInterfaces": []any{
			map[string]any{
				"url":             "http://" + addr + "/a2a",
				"protocolBinding": "JSONRPC",
				"protocolVersion": A2AProtocolVersion,
			},
		},
		"capabilities": map[string]any{
			"streaming":         false,
			"extendedAgentCard": false,
		},
		"defaultInputModes":  []any{"text/plain"},
		"defaultOutputModes": []any{"text/plain"},
		"securitySchemes": map[string]any{
			"bearer": map[string]any{"type": "http", "scheme": "bearer"},
		},
		"securityRequirements": []any{
			map[string]any{"bearer": []any{}},
		},
		"skills": []any{
			map[string]any{
				"id":          "self_build",
				"name":        "Self-build",
				"description": "Delegate spec → PR. Per-call operator attestation.",
				"tags":        []any{"write", "operator-attested"},
				"x-leah": map[string]any{
					"auth_scope":           attest.ScopeSelfBuildA2A,
					"blast_radius":         4,
					"requires_attestation": true,
				},
			},
		},
	}
	for _, k := range []string{"name", "description", "version"} {
		if v, ok := operatorOverrides[k]; ok {
			card[k] = v
		}
	}
	return card
}

// loadCardOverrides reads agent_card.json from disk and returns the
// name/description/version values + whether any gate field was tampered with.
// Tamper = securitySchemes / securityRequirements / skills[*].x-leah edited
// (operator may not downgrade the gate via file edit; spec §4).
func loadCardOverrides(path string) (overrides map[string]any, tampered bool, tamperedField string) {
	overrides = map[string]any{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return overrides, false, ""
	}
	var disk map[string]any
	if err := json.Unmarshal(raw, &disk); err != nil {
		return overrides, false, ""
	}
	for _, k := range []string{"name", "description", "version"} {
		if v, ok := disk[k]; ok {
			overrides[k] = v
		}
	}
	canonical := canonicalCard("placeholder", nil)
	for _, gate := range []string{"securitySchemes", "securityRequirements"} {
		if !reflect.DeepEqual(disk[gate], canonical[gate]) {
			return overrides, true, gate
		}
	}
	diskSkills, _ := disk["skills"].([]any)
	canSkills, _ := canonical["skills"].([]any)
	for i := range canSkills {
		if i >= len(diskSkills) {
			return overrides, true, "skills"
		}
		dx := diskSkills[i].(map[string]any)["x-leah"]
		cx := canSkills[i].(map[string]any)["x-leah"]
		if !reflect.DeepEqual(dx, cx) {
			return overrides, true, "skills[" + strconv.Itoa(i) + "].x-leah"
		}
	}
	return overrides, false, ""
}

// cardTamperSuppressWindow throttles identical-tamper mcp_card_reset emissions
// (one row per field per 60s) so a card-polling peer cannot flood the audit log.
const cardTamperSuppressWindow = 60 * time.Second

type tamperThrottle struct {
	mu       sync.Mutex
	lastEmit map[string]time.Time
}

func (t *tamperThrottle) allow(field string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lastEmit == nil {
		t.lastEmit = map[string]time.Time{}
	}
	if prev, ok := t.lastEmit[field]; ok && now.Sub(prev) < cardTamperSuppressWindow {
		return false
	}
	t.lastEmit[field] = now
	return true
}

// serveAgentCard emits the canonical card; emits mcp_card_reset audit row
// when operator-edits to gate fields are detected. Identical-field tamper rows
// are suppressed within cardTamperSuppressWindow to prevent poll-driven flood.
func (a *A2AHandler) serveAgentCard(addr string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		overrides, tampered, field := loadCardOverrides(a.AgentCardPath)
		if tampered && a.AuditLogger != nil && a.tamper.allow(field, a.now()) {
			_ = a.AuditLogger.Append(auditEntry("mcp_card_reset", "field="+field, 0, "ok"))
		}
		card := canonicalCard(addr, overrides)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(card)
	}
}
