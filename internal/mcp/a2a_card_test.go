package mcp

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/attestation"
	"github.com/trilam/leah/internal/audit"
)

// Spec §4 wire-format pin. AgentCard JSON must include protocolVersion,
// protocolBinding, securityRequirements, and skills.self_build with the
// x-leah extension (auth_scope=self-build-a2a, blast_radius=4,
// requires_attestation=true). All field names verbatim from A2A 1.0 + spec.
func TestA2ACard_MatchesSpec(t *testing.T) {
	s, _, dir := newTestServer(t)
	s.A2A = &A2AHandler{
		AgentCardPath:  filepath.Join(dir, "agent_card.json"),
		AuditLogger:    &audit.Logger{Path: filepath.Join(dir, "audit.jsonl")},
		AttestationTTY: &stubTTY{ok: true},
	}
	cancel := startListener(t, s)
	defer cancel()

	resp, err := http.Get("http://" + s.Addr + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	var card map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Top-level required fields.
	for _, k := range []string{"name", "description", "version",
		"supportedInterfaces", "capabilities", "defaultInputModes",
		"defaultOutputModes", "securitySchemes", "securityRequirements", "skills"} {
		if _, ok := card[k]; !ok {
			t.Errorf("missing top-level field %q", k)
		}
	}

	// supportedInterfaces[0].protocolBinding + protocolVersion (A2A 1.0 §4.4.1).
	ifaces, ok := card["supportedInterfaces"].([]any)
	if !ok || len(ifaces) == 0 {
		t.Fatalf("supportedInterfaces: %T %v", card["supportedInterfaces"], card["supportedInterfaces"])
	}
	iface := ifaces[0].(map[string]any)
	if iface["protocolBinding"] != "JSONRPC" {
		t.Errorf("protocolBinding = %v, want JSONRPC", iface["protocolBinding"])
	}
	if iface["protocolVersion"] != "1.0" {
		t.Errorf("protocolVersion = %v, want 1.0", iface["protocolVersion"])
	}

	// securityRequirements[0].bearer present.
	secReqs, ok := card["securityRequirements"].([]any)
	if !ok || len(secReqs) == 0 {
		t.Fatalf("securityRequirements missing or empty")
	}
	if _, ok := secReqs[0].(map[string]any)["bearer"]; !ok {
		t.Errorf("securityRequirements[0].bearer missing")
	}

	// skills[0] = self_build with x-leah extension.
	skills, ok := card["skills"].([]any)
	if !ok || len(skills) == 0 {
		t.Fatalf("skills missing")
	}
	sk := skills[0].(map[string]any)
	if sk["id"] != "self_build" {
		t.Errorf("skills[0].id = %v, want self_build", sk["id"])
	}
	x, ok := sk["x-leah"].(map[string]any)
	if !ok {
		t.Fatalf("skills[0].x-leah missing")
	}
	if x["auth_scope"] != attestation.ScopeSelfBuildA2A {
		t.Errorf("x-leah.auth_scope = %v, want %s", x["auth_scope"], attestation.ScopeSelfBuildA2A)
	}
	if br, _ := x["blast_radius"].(float64); br != 4 {
		t.Errorf("x-leah.blast_radius = %v, want 4", x["blast_radius"])
	}
	if x["requires_attestation"] != true {
		t.Errorf("x-leah.requires_attestation = %v, want true", x["requires_attestation"])
	}
}

// Operator may edit name/description/version on disk; gate fields
// (skills[*].x-leah.*, securitySchemes) regenerated from code +
// mcp_card_reset audit row appended.
func TestA2ACard_OperatorEditsToGateFieldsAreOverwritten(t *testing.T) {
	s, logger, dir := newTestServer(t)
	cardPath := filepath.Join(dir, "agent_card.json")
	// Operator-tampered card on disk: description edited (allowed) AND
	// requires_attestation=false (forbidden gate edit).
	tampered := `{
	  "name": "leah-custom",
	  "description": "operator description",
	  "version": "9.9.9",
	  "supportedInterfaces": [{"url":"x","protocolBinding":"JSONRPC","protocolVersion":"1.0"}],
	  "capabilities": {},
	  "defaultInputModes": [], "defaultOutputModes": [],
	  "securitySchemes": {}, "securityRequirements": [],
	  "skills": [{"id":"self_build","name":"sb","x-leah":{"auth_scope":"none","blast_radius":0,"requires_attestation":false}}]
	}`
	if err := writeFile(cardPath, tampered); err != nil {
		t.Fatal(err)
	}
	s.A2A = &A2AHandler{
		AgentCardPath:  cardPath,
		AuditLogger:    logger,
		AttestationTTY: &stubTTY{ok: true},
	}
	cancel := startListener(t, s)
	defer cancel()

	resp, err := http.Get("http://" + s.Addr + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var card map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatal(err)
	}
	if card["description"] != "operator description" {
		t.Errorf("operator description was overwritten: %v", card["description"])
	}
	skills := card["skills"].([]any)
	x := skills[0].(map[string]any)["x-leah"].(map[string]any)
	if x["requires_attestation"] != true {
		t.Errorf("gate field requires_attestation NOT regenerated: %v", x["requires_attestation"])
	}
	if x["auth_scope"] != attestation.ScopeSelfBuildA2A {
		t.Errorf("gate field auth_scope NOT regenerated: %v", x["auth_scope"])
	}
	raw := readFile(t, logger.Path)
	if !strings.Contains(raw, `"kind":"mcp_card_reset"`) {
		t.Errorf("want mcp_card_reset audit row, got %s", raw)
	}
}
