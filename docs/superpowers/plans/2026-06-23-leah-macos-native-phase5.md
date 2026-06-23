# Leah macOS Native UI — Phase 5 Implementation Plan

> **Authored from candidate set — operator may revise once the Phase 5 design spec lands.** This plan was drafted in parallel with the spec (sibling agent on branch `phase5-design-spec-local-v2`). The candidate deliverable set was derived from (a) the Phase 4 spec executive summary "what Phase 4 is not" exclusions, (b) `memory/research_ai_capability_domains_2026.md` Leah-gap list, (c) `memory/research_ai_assistants_*.md` competitor inventories, and (d) `memory/project_residual_backlog_2026-06-23.md` survivor set. When the spec lands at `docs/superpowers/specs/2026-06-23-leah-phase5-design.md`, every section header below should be reconciled against it; mismatches are spec-wins.

> **For agentic workers:** REQUIRED SUB-SKILL — Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every Go-side dispatch MUST reference `docs/engineer/dispatch-templates/implementer-adapter.md` by path; every Swift-side dispatch MUST reference `docs/engineer/dispatch-templates/implementer.md` by path. Every reviewer dispatch MUST prepend `.claude/notes/reviewer-header.md` and return its verdict via the subagent transcript channel — NOT `gh pr comment` (same-session reviewer posting to GH inherits author identity = self-approval). Resolve the live reviewer-header path with `find . -name reviewer-header.md -path '*/notes/*' -not -path '*/.git/*'`.

**Goal:** Land Phase 5 ("Agentic operator + lifestyle integrations") from candidate spec `docs/superpowers/specs/2026-06-23-leah-phase5-design.md` (in flight). Nine deliverables, five build waves, twenty-one implementer tasks. v1.2 ship after W5.

1. **Computer-use loop** (§1) — daemon-driven on-device browser + UI automation: invoke a sandboxed WebKit browser controlled by Claude Computer Use; macOS Accessibility API tap for native-app actions; per-action consent ladder + recording.
2. **Deep-research workflow** (§2) — multi-source web fan-out + per-claim citation verifier + adversarial reviewer + cited markdown report, surfaced as a Dashboard "Research" card.
3. **Calendar smart-scheduling** (§3) — Motion/Reclaim-class meeting placement: scan EventKit + travel-time + focus-time guards + working-hours model + per-attendee timezone resolver + auto-decline rules.
4. **Subscription + bill watcher** (§4) — Mail-derived subscription detection (mailbox-only signal, never bank scrape), per-charge anomaly alert, calendar-tied renewal cards, one-tap cancel-helper URL.
5. **Location-aware reminders** (§5) — CoreLocation geofence + significant-change events → daemon → reminder lifecycle (`leah remind`-driven), Apple Reminders parity.
6. **Taste model + media DJ** (§6) — operator taste profile editable from Settings; Apple Music + Spotify + library scan → per-mood + per-context playlists; surfaced as widget + Coach card suggestions.
7. **Household profiles** (§7) — voice-ID-keyed sub-profile selector (operator-spouse-kid); per-profile memory namespace + per-profile attestation gate; Voice ID enrollment wizard.
8. **Long-context session mode** (§8) — 1M-token context window with prompt caching + Anthropic SDK cache control + per-conversation cache cursor + cold-start vs cache-hit telemetry.
9. **Mobile companion bridge** (§9) — iPhone Shortcut + push-notification action surface so iMessage / iPhone Focus / Apple Watch can ask Leah without round-tripping via the Mac UI — relies on iCloud-Drive payload drop + Sparkle-style EdDSA verify on inbound shortcuts.

**Architecture:** Phase 5 layers a seventh "agentic" surface atop the Phase 1–4 substrates. The daemon stays the trust + LLM-key boundary (predecessor §17.14). All new long-lived subsystems route through the existing Phase 4 `Supervisor` (no new supervision substrate). Single SQLite file invariant holds — every new table lives in `~/Library/Application Support/Leah/leah.db`. Default-OFF for any cross-application or cross-device side effect (computer-use, location, household voice-ID, mobile bridge).

| Subsystem | Phase 4 state | Phase 5 delta |
|---|---|---|
| `internal/agent/` (new) | none | computer-use loop runner + action ledger + consent ladder |
| `internal/agent/webkit/` (new) | none | WKWebView cgo bridge + screenshot-and-click controller |
| `internal/agent/uia/` (new) | none | macOS Accessibility API (AXUIElement) cgo bridge |
| `internal/research/` (new) | `internal/feeds/` + `internal/papers/` only | deep-research orchestrator + per-claim verifier + cited markdown emitter |
| `internal/calendar/scheduler/` (new) | EventKit read-only via `internal/macos/` | meeting placement solver + travel-time + auto-decline rules |
| `internal/subs/` (new) | none | mail-derived subscription detector + renewal cards |
| `internal/geofence/` (new) | none | CoreLocation cgo bridge + reminder lifecycle |
| `internal/taste/` (new) | none | taste profile + per-mood playlist generator |
| `internal/household/` (new) | none | voice-ID sub-profile selector + per-profile attestation |
| `internal/longctx/` (new) | none | 1M-token cache cursor + Anthropic prompt-cache wiring |
| `internal/bridge/mobile/` (new) | none | iCloud-Drive payload-drop verifier + shortcut intake |
| Swift HUD voice | Phase 4 `VoiceCoordinator` | + speaker-diarization probe wired to `internal/household/` |
| Settings panes | 12 panes locked (Phase 4 v1.1) | + Research (new), + Subscriptions (new), + Household (new), + Mobile (new); Calendar gains a Scheduling sub-pane (§3.10); About gains a "Computer-use ledger" row (§1.10) |

**Tech Stack:** Go 1.25, SwiftUI/AppKit (macOS 14+), `github.com/anthropics/anthropic-sdk-go` (Computer Use beta + prompt caching), Apple `WebKit.framework` (WKWebView), Apple `Accessibility` API (`AXUIElement`), Apple `EventKit.framework`, Apple `CoreLocation.framework`, Apple `MusicKit.framework` (Apple Music), Apple `MediaPlayer.framework` (library scan), Apple `Speech.framework` (voice-ID via `SFSpeakerRecognitionRequest`), iCloud Drive container, Sparkle EdDSA verify (mobile payload).

## Global Constraints

- Module path: `github.com/trilam/leah`
- macOS deployment target: 14.0 (no new floor; Phase 4 stayed at 14.0)
- Single SQLite file invariant: `~/Library/Application Support/Leah/leah.db`; Phase 5 migrations are additive only
- Default-OFF for any cross-application or cross-device side effect (computer-use, location, household voice-ID, mobile bridge)
- Daemon owns the Anthropic API key — HUD/agent/research/scheduler/bridge processes never see it
- Privacy budget is enforced in the daemon, not by callers — every cloud call site MUST `Charge()` before issuing; Phase 5 adds three new buckets (`computer_use`, `research_fanout`, `location_lookup`) on top of the Phase 4 seven
- No AI signatures anywhere (no `Co-Authored-By`, no "Generated with", no "written by Claude")
- WHY-not-WHAT comments. Default to no comment. Test/Fuzz/Benchmark godocs: 1 line max
- Reviewer required per PR — independent reviewer subagent (agent-id `^(a[0-9a-f]{16}|cavecrew-reviewer-[a-z0-9-]+)$`) spawned immediately after `gh pr create`, verdict via transcript channel only
- Author posting own APPROVE = self-approval regardless of channel — never `gh pr comment` from same-session reviewer
- Deletion default: every PR states what got smaller. Phase 5 deletes the two superseded sketches in W5-T21:
  - `docs/engineer/specs/2026-06-10-computer-use-sketch.md` (placeholder file; create-if-absent before delete)
  - `docs/engineer/specs/2026-06-10-mobile-bridge-sketch.md` (placeholder file; create-if-absent before delete)
- Pre-PR verify gate (Go): `gofmt -l .` empty AND `go vet ./...` clean AND `golangci-lint run ./internal/<pkg> ./cmd/<pkg> 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5` empty AND `go test ./internal/<pkg> ./cmd/<pkg> 2>&1 | tail -5` PASS — `golangci-lint` is non-negotiable per Phase 3 lesson
- Pre-PR verify gate (Swift): `cd app/Leah && swift build 2>&1 | tail -5` clean AND `swift test --filter <module>Tests 2>&1 | tail -5` PASS
- Resource bundles in Swift packages: use `.copy("Fixtures")` not `.process` when call sites pass `subdirectory:` (Phase 2 lesson)
- Shared-seam files: when ≥ 2 tasks register defaults into the same shared table (e.g. `subscription_kind` rows, `taste_seed` rows, `geofence_class` rows) — land a solo `registerXxxDefaults()` seam helper PR BEFORE fan-out (Phase 3 lesson)
- Orphan-scan-before-tag: every wave-exit gate AND the v1.2 ship gate (T21) MUST run `scripts/dev/orphan-scan.sh` (or equivalent `grep -RIn "<pkg>\." cmd/ internal/ | grep -v _test.go`) over every Phase-5 `internal/<pkg>/` and assert ZERO Phase-5 packages have zero non-test callers — v3.3.0 shipped with 3 wiring gaps (TTS provider, KG citation route, MCP composition) because providers existed but the composition root never instantiated them. Catching this post-tag is too late (Phase 3 lesson)
- Composition-root wiring is its own task, never implicit: providers/runtimes/handlers added in earlier waves do NOT self-register into `cmd/leah-daemon/main.go`. T19 (Wave 5) is the explicit composition-root wiring task — it must land BEFORE T20 (E2E smoke), because the smoke is meaningless if the daemon boot path never instantiates the surfaces under test (Phase 3 lesson)
- Cavecrew-builder dispatch has NO `Bash` tool — only `Read`, `Edit`, `Write`, `Grep`, `Glob`. Any fix that requires running tests, `go vet`, `golangci-lint`, `git` (status/diff/commit/push), `gh`, or `make` MUST dispatch to `general-purpose` (or `claude`) instead. Builder is correct for typo fixes, single-function rewrites, mechanical renames, comment removal — not for anything that has to run the verify gate or push (Phase 3 dispatch lesson)
- Spec parity guard: `scripts/check-spec-parity.sh` MUST stay green; forbidden phrases (renamed terms, killed cosmetics) cannot enter code or tests. Phase 5 adds these forbidden phrases on top of Phase 4: "browser agent" (use "computer-use loop"), "smart calendar" (use "smart-scheduling"), "DJ mode" (use "taste model"), "kid mode" (use "household profile")
- Existing IPC frame: `struct Frame { Kind, TurnID string; Seq uint64; Payload json.RawMessage }` — do NOT reuse Kind strings reserved by Phase 1/2/3/4 (see Phase 4 plan for the locked list). Phase 5 adds: `agent.step`, `agent.consent`, `agent.halt`, `research.progress`, `research.cite`, `sched.place`, `sched.conflict`, `subs.detect`, `subs.renew`, `geofence.enter`, `geofence.exit`, `taste.update`, `household.switch`, `longctx.cache.hit`, `longctx.cache.miss`, `bridge.shortcut`
- Settings IA: the 12-pane lock from Phase 4 v1.1 RELAXES in Phase 5 — four NEW panes (Research, Subscriptions, Household, Mobile) are explicitly authorized by §2.9, §4.10, §7.10, §9.10; Calendar gains a Scheduling sub-pane (§3.10); About gains a "Computer-use ledger" row (§1.10)
- Computer-use action ledger is append-only and load-bearing: every agent step writes `agent_action` BEFORE the action executes (intent-recorded) and writes a second row AFTER (outcome-recorded). The two-row pattern means a kill-9 of the daemon mid-step leaves a discoverable orphan-intent row — supervisor reconciles on boot.
- Voice-ID enrollment is operator-gated: household sub-profiles can be created only via the wizard (`leah household enroll`) and only when the operator is the authenticated user (Touch ID gate per §17.13).
- Long-context cache cursor is per-conversation: a switch of conversation purges the cache cursor and re-warms; cache-hit rate is exposed in About → Diagnostics so the operator can observe drift.
- Mobile bridge payloads are EdDSA-signed by the iPhone Shortcut and verified by the daemon BEFORE intake; unverified payloads land in a quarantine bucket the operator can manually clear from Settings → Mobile.
- Cross-cutting invariants from §0.2 of the Phase 5 spec are binding for every task — implementers should not rederive them

---

## File structure decisions

### Go-side new package boundaries

| Package | Responsibility | Key files |
|---|---|---|
| `internal/agent/` | computer-use loop runner + action ledger + consent ladder | `loop.go`, `ledger.go`, `consent.go` |
| `internal/agent/webkit/` | WKWebView cgo bridge + screenshot-and-click controller | `webkit_darwin.go`, `webkit_stub.go`, `screenshot.go`, `tap.go` |
| `internal/agent/uia/` | macOS Accessibility API (AXUIElement) cgo bridge | `uia_darwin.go`, `uia_stub.go`, `query.go`, `act.go` |
| `internal/research/` | deep-research orchestrator + verifier + cited emitter | `orch.go`, `verify.go`, `cite.go`, `report.go` |
| `internal/calendar/scheduler/` | meeting placement solver + travel-time + auto-decline | `solver.go`, `travel.go`, `decline.go` |
| `internal/subs/` | mail-derived subscription detector + renewal cards | `detect.go`, `renew.go`, `cancel_url.go` |
| `internal/geofence/` | CoreLocation cgo bridge + reminder lifecycle | `geofence_darwin.go`, `geofence_stub.go`, `reminder.go` |
| `internal/taste/` | taste profile + per-mood playlist generator | `profile.go`, `mood.go`, `playlist.go` |
| `internal/taste/musickit/` | MusicKit cgo bridge for Apple Music | `musickit_darwin.go`, `musickit_stub.go` |
| `internal/household/` | voice-ID sub-profile selector + per-profile attestation | `profile.go`, `voiceid.go`, `enroll.go` |
| `internal/household/diarize/` | speaker-diarization probe (cgo SFSpeakerRecognitionRequest) | `diarize_darwin.go`, `diarize_stub.go` |
| `internal/longctx/` | 1M-token cache cursor + Anthropic prompt-cache wiring | `cursor.go`, `cache.go`, `telemetry.go` |
| `internal/bridge/mobile/` | iCloud-Drive payload-drop verifier + shortcut intake | `intake.go`, `verify.go`, `quarantine.go` |

### Swift-side new module boundaries

| Module | Responsibility | Key files |
|---|---|---|
| `LeahHUD/Agent/` | computer-use overlay + per-action consent prompt + Halt button | `AgentOverlay.swift`, `ConsentPrompt.swift`, `HaltButton.swift` |
| `LeahHUD/Research/` | research card + per-source citation popover + cancel link | `ResearchCard.swift`, `CitationPopover.swift` |
| `LeahHUD/Household/` | profile switcher chrome + active-profile indicator on menubar | `ProfileSwitcher.swift`, `MenuBarBadge.swift` |
| `LeahUI/Settings/ResearchPane.swift` | per-domain allowlist + max-fanout + cache TTL | `ResearchPane.swift` |
| `LeahUI/Settings/SubscriptionsPane.swift` | detected subscriptions + per-row mute + manual-add | `SubscriptionsPane.swift` |
| `LeahUI/Settings/HouseholdPane.swift` | sub-profiles + voice-ID enrollment wizard + per-profile attestation toggle | `HouseholdPane.swift`, `EnrollWizardView.swift` |
| `LeahUI/Settings/MobilePane.swift` | iCloud-Drive payload drop status + quarantine bucket clear | `MobilePane.swift` |
| `LeahUI/Settings/CalendarPane.swift` (modify) | + Scheduling sub-pane (working hours / focus blocks / auto-decline rules) | `CalendarPane.swift` |
| `LeahUI/Settings/AboutPane.swift` (modify) | + "Computer-use ledger" row that opens the action ledger viewer | `AboutPane.swift` |
| `LeahUI/Dashboard/ResearchCard.swift` | last-3 research runs + click-through to full report | `ResearchCard.swift` |
| `LeahUI/Dashboard/SubscriptionCard.swift` | renewals in next 30 days + total monthly | `SubscriptionCard.swift` |
| `LeahUI/Dashboard/TasteCard.swift` | current taste seed + last-played + suggested playlist | `TasteCard.swift` |

### Migrations

All in `internal/sqlstore/migrations/`; chronologically ordered:

- `2026-06-23-001-agent.sql` — `agent_action`, `agent_consent`, `agent_session`
- `2026-06-23-002-research.sql` — `research_run`, `research_claim`, `research_source`
- `2026-06-23-003-scheduler.sql` — `sched_attempt`, `sched_rule`, `sched_decline`
- `2026-06-23-004-subs.sql` — `subscription`, `subscription_charge`, `subscription_rule`
- `2026-06-23-005-geofence.sql` — `geofence`, `geofence_event`, `reminder`
- `2026-06-23-006-taste.sql` — `taste_profile`, `taste_seed`, `taste_play`
- `2026-06-23-007-household.sql` — `household_profile`, `household_voice_sample`, `household_attest`
- `2026-06-23-008-longctx.sql` — `longctx_cursor`, `longctx_telemetry`
- `2026-06-23-009-bridge.sql` — `bridge_shortcut`, `bridge_quarantine`

W1-T05 lands all nine migrations as a single PR (single-owner per CLAUDE.md frozen-enum-files rule); subsequent tasks reference but do not author migration files.

---

## Wave dependency matrix (21 tasks)

- **Wave 1** (perception + ledger substrate, parallel ≤ 5 — file-disjoint Go-side except T05 single-owner): T01 Computer-use loop runner + action ledger, T02 WKWebView cgo bridge + screenshot/tap controller, T03 macOS Accessibility API (AXUIElement) bridge, T04 deep-research orchestrator + verifier scaffold, T05 nine migrations (single-owner, lands FIRST).
- **Wave 2** (control plane, parallel ≤ 4 — file-disjoint): T06 calendar smart-scheduler + travel-time + auto-decline, T07 subscription detector + renewal cards, T08 location geofence + reminder lifecycle, T09 long-context cache cursor + Anthropic prompt-cache wiring. Wave 2 starts after T05 merged.
- **Wave 3** (taste + household, parallel ≤ 3 — file-disjoint): T10 taste profile + MusicKit bridge, T11 per-mood + per-context playlist generator, T12 household voice-ID enrollment wizard + per-profile attestation. Wave 3 starts after T09 merged.
- **Wave 4** (agentic surfaces, parallel ≤ 3 — file-disjoint): T13 Computer-use HUD overlay + consent prompt + Halt button, T14 deep-research dashboard card + citation popover, T15 mobile bridge intake + iCloud-Drive payload verify + quarantine, T16 sample first-party Shortcut + signing key custody doc.
- **Wave 5** (supervision wire-in + ship, ≤ 3 parallel then serialized composition-root → E2E → ship): T17 supervisor registration for all Phase-5 long-lived subsystems (agent loop, research orchestrator, geofence listener, mobile-bridge intake), T18 Dashboard cards (Research + Subscription + Taste) wire-in, T19 composition-root wiring of every Phase-5 surface into `cmd/leah-daemon/main.go` (single-owner serialized, lands BEFORE T20), T20 Phase 5 E2E smoke + dispatch-template referenced harness, T21 Phase 5 ship checklist + spec-parity + orphan-scan + deletion of two superseded sketches + reviewer-and-merge pass. Wave 5 starts after W1–W4 land; T19 → T20 → T21 strictly serialized.

---

## Wave 1 — Perception + ledger substrate (parallel ≤ 5)

---

### Task 1: Computer-use loop runner + action ledger (`internal/agent/`)

**Files:**
- Create: `internal/agent/loop.go`
- Create: `internal/agent/loop_test.go`
- Create: `internal/agent/ledger.go`
- Create: `internal/agent/ledger_test.go`
- Create: `internal/agent/consent.go`
- Create: `internal/agent/consent_test.go`
- Modify: `go.mod` — bump `github.com/anthropics/anthropic-sdk-go` to pull in Computer Use beta types

**Why this exists:** §1 mandates a daemon-driven computer-use loop with per-action consent and an append-only action ledger. Without the ledger every step is unaudited; without the consent ladder the loop can sidestep operator authority. Both are load-bearing for the Phase-5 trust story — the operator must be able to read back exactly what the loop did, and stop it at any step.

**Interfaces:**
- Produces:
  - `package agent` at `internal/agent/`
  - `type Step struct { ID string; Intent string; Tool string; Args map[string]any; Ts time.Time }`
  - `type Outcome struct { StepID string; OK bool; Err string; Screenshot []byte; Ts time.Time }`
  - `type Loop interface { Start(ctx context.Context, goal string) (<-chan Event, error); Halt() error; Status() Status }`
  - `type Event struct { Kind EventKind; Step Step; Outcome Outcome; Need Consent }`
  - `type Consent struct { StepID string; Question string; Default ConsentDecision }`
  - `type Ledger interface { Append(s Step) error; Resolve(stepID string, o Outcome) error; List(since time.Time) ([]Row, error) }`

- [ ] **Step 1: Write failing tests**

`internal/agent/ledger_test.go`:
```go
package agent

import (
	"testing"
	"time"
)

func TestLedger_AppendThenResolveIsTwoRows(t *testing.T) {
	l := newTestLedger(t)
	step := Step{ID: "s1", Intent: "click", Tool: "webkit.tap", Ts: time.Now()}
	if err := l.Append(step); err != nil {
		t.Fatal(err)
	}
	if err := l.Resolve("s1", Outcome{StepID: "s1", OK: true}); err != nil {
		t.Fatal(err)
	}
	rows, err := l.List(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("ledger must persist intent + outcome as two rows, got %d", len(rows))
	}
}

func TestLedger_OrphanIntentSurvivesCrash(t *testing.T) {
	l := newTestLedger(t)
	step := Step{ID: "s2", Intent: "type", Tool: "webkit.type", Ts: time.Now()}
	if err := l.Append(step); err != nil {
		t.Fatal(err)
	}
	rows, err := l.List(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("intent row must persist before outcome (kill-9 recovery), got %d", len(rows))
	}
	if rows[0].Resolved {
		t.Fatal("intent-only row must be unresolved until Resolve()")
	}
}
```

`internal/agent/consent_test.go`:
```go
package agent

import "testing"

func TestConsent_AlwaysAskForKnownDangerous(t *testing.T) {
	c := NewConsent(ConsentDefaults{})
	if got := c.PolicyFor("webkit.submit_form"); got != ConsentAlwaysAsk {
		t.Fatalf("submit_form must require ask, got %v", got)
	}
	if got := c.PolicyFor("webkit.screenshot"); got != ConsentSilent {
		t.Fatalf("screenshot is read-only, must be silent, got %v", got)
	}
}
```

`internal/agent/loop_test.go`:
```go
package agent

import (
	"context"
	"testing"
	"time"
)

func TestLoop_HaltStopsWithin80ms(t *testing.T) {
	l := newTestLoop(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ev, err := l.Start(ctx, "open google")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := l.Halt(); err != nil {
		t.Fatal(err)
	}
	for range ev {
	}
	if d := time.Since(start); d > 80*time.Millisecond {
		t.Fatalf("Halt must stop loop in <80ms, took %v", d)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/agent/... 2>&1 | tail -10
```

Expected: FAIL — `undefined: Step`, `undefined: NewConsent`, `undefined: newTestLedger`.

- [ ] **Step 3: Implement `internal/agent/ledger.go`**

```go
package agent

import (
	"context"
	"database/sql"
	"time"
)

type Row struct {
	StepID    string
	Kind      string
	Intent    string
	Tool      string
	Args      string
	OK        sql.NullBool
	Err       sql.NullString
	Ts        time.Time
	Resolved  bool
}

type sqlLedger struct{ db *sql.DB }

func NewLedger(db *sql.DB) Ledger { return &sqlLedger{db: db} }

func (l *sqlLedger) Append(s Step) error {
	_, err := l.db.ExecContext(context.Background(),
		`INSERT INTO agent_action (step_id, kind, intent, tool, args_json, ts)
		 VALUES (?, 'intent', ?, ?, ?, ?)`,
		s.ID, s.Intent, s.Tool, mustJSON(s.Args), s.Ts)
	return err
}

func (l *sqlLedger) Resolve(stepID string, o Outcome) error {
	_, err := l.db.ExecContext(context.Background(),
		`INSERT INTO agent_action (step_id, kind, ok, err, ts)
		 VALUES (?, 'outcome', ?, ?, ?)`,
		stepID, o.OK, nullStr(o.Err), o.Ts)
	return err
}

func (l *sqlLedger) List(since time.Time) ([]Row, error) {
	rows, err := l.db.QueryContext(context.Background(),
		`SELECT step_id, kind, COALESCE(intent,''), COALESCE(tool,''), COALESCE(args_json,''),
		        ok, err, ts FROM agent_action WHERE ts >= ? ORDER BY ts`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.StepID, &r.Kind, &r.Intent, &r.Tool, &r.Args, &r.OK, &r.Err, &r.Ts); err != nil {
			return nil, err
		}
		r.Resolved = r.Kind == "outcome"
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Implement consent ladder**

```go
package agent

type ConsentPolicy int

const (
	ConsentSilent ConsentPolicy = iota
	ConsentAlwaysAsk
	ConsentRememberSession
	ConsentBlock
)

var defaultPolicies = map[string]ConsentPolicy{
	"webkit.screenshot":  ConsentSilent,
	"webkit.read_dom":    ConsentSilent,
	"webkit.click":       ConsentRememberSession,
	"webkit.type":        ConsentRememberSession,
	"webkit.submit_form": ConsentAlwaysAsk,
	"webkit.navigate":    ConsentRememberSession,
	"uia.read":           ConsentSilent,
	"uia.click":          ConsentRememberSession,
	"uia.write":          ConsentAlwaysAsk,
}

type Consent struct {
	overrides map[string]ConsentPolicy
}

func NewConsent(d ConsentDefaults) *Consent {
	return &Consent{overrides: d.Overrides}
}

func (c *Consent) PolicyFor(tool string) ConsentPolicy {
	if p, ok := c.overrides[tool]; ok {
		return p
	}
	return defaultPolicies[tool]
}
```

- [ ] **Step 5: Implement loop runner**

The runner wraps the Anthropic Computer-Use beta client. Each tool call is appended to the ledger BEFORE the tool executes (intent row); the outcome is appended AFTER (outcome row). Halt closes the cancel channel and aborts within 80 ms.

- [ ] **Step 6: Re-run tests + verify green**

```bash
go test ./internal/agent/... -count=1 2>&1 | tail -10
```

- [ ] **Step 7: Commit + PR + reviewer**

Dispatch reviewer per `docs/engineer/dispatch-templates/reviewer.md`.

---

### Task 2: WKWebView cgo bridge + screenshot/tap controller (`internal/agent/webkit/`)

**Files:**
- Create: `internal/agent/webkit/webkit_darwin.go`
- Create: `internal/agent/webkit/webkit_darwin_test.go`
- Create: `internal/agent/webkit/webkit_stub.go`
- Create: `internal/agent/webkit/screenshot.go`
- Create: `internal/agent/webkit/tap.go`
- Create: `internal/agent/webkit/webkit.h`
- Create: `internal/agent/webkit/webkit.m`

**Why this exists:** §1.4 mandates a sandboxed WKWebView the computer-use loop drives — this is the substitute for shelling out to a real browser, and it keeps the browser process inside the daemon's trust boundary. Apple's WebKit is the only macOS-native option that exposes off-screen rendering + programmatic input.

**Interfaces:**
- Produces:
  - `package webkit`
  - `type Browser interface { Navigate(url string) error; Screenshot() ([]byte, error); Tap(x, y int) error; Type(text string) error; ReadDOM() (string, error); Close() error }`
  - `func New(opts Opts) (Browser, error)` — opts include user-agent, content-blocker list, on-disk cache dir
  - `type Opts struct { UserAgent string; ContentBlockerJSON string; CacheDir string; Viewport image.Point }`

- [ ] **Step 1: Failing tests**

`internal/agent/webkit/webkit_darwin_test.go`:
```go
//go:build darwin

package webkit

import (
	"image"
	"testing"
)

func TestBrowser_ScreenshotReturnsNonEmptyPNG(t *testing.T) {
	b, err := New(Opts{Viewport: image.Pt(1024, 768)})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.Navigate("about:blank"); err != nil {
		t.Fatal(err)
	}
	png, err := b.Screenshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(png) < 1024 {
		t.Fatalf("screenshot len = %d, want ≥1024", len(png))
	}
	if string(png[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatal("screenshot missing PNG header")
	}
}

func TestBrowser_TapAfterNavigateIsNoError(t *testing.T) {
	b, _ := New(Opts{Viewport: image.Pt(800, 600)})
	defer b.Close()
	_ = b.Navigate("about:blank")
	if err := b.Tap(100, 100); err != nil {
		t.Fatalf("Tap = %v, want nil", err)
	}
}

func TestBrowser_CloseIsIdempotent(t *testing.T) {
	b, _ := New(Opts{Viewport: image.Pt(800, 600)})
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil (idempotent)", err)
	}
}
```

- [ ] **Step 2: Author `webkit.h` + `webkit.m`** — Objective-C shim exposing `wk_new`, `wk_navigate`, `wk_screenshot`, `wk_tap`, `wk_type`, `wk_close`. Use `WKWebView` with off-screen `NSWindow`.

```objc
// webkit.h
#ifndef LEAH_WEBKIT_H
#define LEAH_WEBKIT_H
#include <stdint.h>
typedef struct wk_handle_t* wk_handle;
wk_handle wk_new(int viewport_w, int viewport_h);
int wk_navigate(wk_handle h, const char* url);
int wk_screenshot(wk_handle h, uint8_t** out_png, int* out_n);
int wk_tap(wk_handle h, int x, int y);
int wk_type(wk_handle h, const char* text);
const char* wk_read_dom(wk_handle h);
void wk_close(wk_handle h);
#endif
```

- [ ] **Step 3: Implement `webkit_darwin.go`** — cgo wrapper; serialize calls via a single goroutine pinned to the main thread (WKWebView requires main thread). The pinned-goroutine pattern uses `runtime.LockOSThread()` at startup and routes every `Browser` method through a channel.

```go
//go:build darwin

package webkit

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework WebKit -framework AppKit -framework Foundation
#include "webkit.h"
*/
import "C"

import (
	"errors"
	"image"
	"runtime"
	"sync"
	"unsafe"
)

type darwinBrowser struct {
	h      C.wk_handle
	mu     sync.Mutex
	closed bool
}

var mainLoop = make(chan func(), 8)

func init() {
	go func() {
		runtime.LockOSThread()
		for f := range mainLoop {
			f()
		}
	}()
}

func New(opts Opts) (Browser, error) {
	if opts.Viewport.X == 0 || opts.Viewport.Y == 0 {
		opts.Viewport = image.Pt(1024, 768)
	}
	resp := make(chan C.wk_handle, 1)
	mainLoop <- func() {
		resp <- C.wk_new(C.int(opts.Viewport.X), C.int(opts.Viewport.Y))
	}
	h := <-resp
	if h == nil {
		return nil, errors.New("webkit: wk_new returned nil")
	}
	return &darwinBrowser{h: h}, nil
}

func (b *darwinBrowser) Navigate(url string) error {
	cu := C.CString(url)
	defer C.free(unsafe.Pointer(cu))
	resp := make(chan C.int, 1)
	mainLoop <- func() { resp <- C.wk_navigate(b.h, cu) }
	if rc := <-resp; rc != 0 {
		return errors.New("webkit: navigate failed")
	}
	return nil
}
```

- [ ] **Step 4: Implement `webkit_stub.go`** — `//go:build !darwin` returns `errors.New("webkit: darwin only")` for every Browser method, so CI lanes on linux compile cleanly.

- [ ] **Step 5: Verify + commit + PR + reviewer**

---

### Task 3: macOS Accessibility API (AXUIElement) bridge (`internal/agent/uia/`)

**Files:**
- Create: `internal/agent/uia/uia_darwin.go`
- Create: `internal/agent/uia/uia_darwin_test.go`
- Create: `internal/agent/uia/uia_stub.go`
- Create: `internal/agent/uia/query.go`
- Create: `internal/agent/uia/act.go`
- Create: `internal/agent/uia/uia.h`
- Create: `internal/agent/uia/uia.m`

**Why this exists:** §1.5 mandates native-app automation (Mail, Calendar, Reminders, Notes) via Accessibility. WebKit handles browser-only flows; AXUIElement handles everything else. Both share the same Loop/Consent gate.

**Interfaces:**
- Produces:
  - `package uia`
  - `type Element struct { Role string; Title string; Value string; Children []Element }`
  - `func QueryFrontmost() (Element, error)`
  - `func FindByTitle(root Element, title string) (Element, error)`
  - `func Press(e Element) error` — `AXPress` action
  - `func Set(e Element, value string) error` — `AXValue` set

- [ ] **Step 1: Failing tests**

`internal/agent/uia/uia_darwin_test.go`:
```go
//go:build darwin

package uia

import "testing"

func TestQueryFrontmost_ReturnsNonZeroRole(t *testing.T) {
	e, err := QueryFrontmost()
	if err == ErrPermissionDenied {
		t.Skip("Accessibility not granted; runtime-gated test")
	}
	if err != nil {
		t.Fatal(err)
	}
	if e.Role == "" {
		t.Fatal("frontmost element must have non-empty Role")
	}
}

func TestPress_ReturnsNoSuchActionOnNonPressable(t *testing.T) {
	e := Element{Role: "AXStaticText", Title: "hello"}
	if err := Press(e); err != ErrNoSuchAction {
		t.Fatalf("Press on static text = %v, want ErrNoSuchAction", err)
	}
}

func TestSet_ReturnsReadOnlyOnReadOnlyValue(t *testing.T) {
	e := Element{Role: "AXLabel", Value: "x", isReadOnly: true}
	if err := Set(e, "y"); err != ErrReadOnly {
		t.Fatalf("Set on read-only label = %v, want ErrReadOnly", err)
	}
}
```

- [ ] **Step 2: Author `uia.h` + `uia.m`** — Objective-C shim calling `AXUIElementCopyAttributeValue`, `AXUIElementPerformAction`, `AXUIElementSetAttributeValue`.

```objc
// uia.h
#ifndef LEAH_UIA_H
#define LEAH_UIA_H
#include <stdbool.h>
typedef struct ax_element_t* ax_element;
bool ax_trusted(void);
ax_element ax_frontmost(void);
const char* ax_role(ax_element e);
const char* ax_title(ax_element e);
const char* ax_value(ax_element e);
int ax_press(ax_element e);
int ax_set_value(ax_element e, const char* value);
void ax_release(ax_element e);
#endif
```

- [ ] **Step 3: Implement `uia_darwin.go`** — cgo wrapper. Cache root element by bundle ID. Check `AXIsProcessTrustedWithOptions` on first call; return `ErrPermissionDenied` if false (operator must grant Accessibility in System Settings).

```go
//go:build darwin

package uia

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework ApplicationServices -framework Foundation
#include "uia.h"
*/
import "C"

import "errors"

var (
	ErrPermissionDenied = errors.New("uia: Accessibility permission not granted")
	ErrNoSuchAction     = errors.New("uia: element does not support AXPress")
	ErrReadOnly         = errors.New("uia: AXValue is read-only")
)

func QueryFrontmost() (Element, error) {
	if !bool(C.ax_trusted()) {
		return Element{}, ErrPermissionDenied
	}
	h := C.ax_frontmost()
	if h == nil {
		return Element{}, errors.New("uia: no frontmost element")
	}
	defer C.ax_release(h)
	return Element{
		Role:  C.GoString(C.ax_role(h)),
		Title: C.GoString(C.ax_title(h)),
		Value: C.GoString(C.ax_value(h)),
	}, nil
}
```

- [ ] **Step 4: Implement `uia_stub.go`** — `//go:build !darwin` returns ErrUnsupported on every entry point.

- [ ] **Step 5: Verify + commit + PR + reviewer**

---

### Task 4: Deep-research orchestrator + verifier scaffold (`internal/research/`)

**Files:**
- Create: `internal/research/orch.go`
- Create: `internal/research/orch_test.go`
- Create: `internal/research/verify.go`
- Create: `internal/research/verify_test.go`
- Create: `internal/research/cite.go`
- Create: `internal/research/report.go`

**Why this exists:** §2 mandates a deep-research workflow with per-claim citation verification. The orchestrator fan-outs web queries via the existing `internal/feeds/` + a new `internal/web/search.go` adapter, the verifier confirms each claim against the cited source, and the emitter writes a cited markdown report. Without verification this is just "web search summary" — every other AI assistant ships that; the differentiation is adversarial verification.

**Interfaces:**
- Produces:
  - `package research`
  - `type Run struct { ID string; Question string; Started time.Time }`
  - `type Claim struct { Text string; SourceURL string; Verified VerifyStatus; Confidence float64 }`
  - `type Orchestrator interface { Start(ctx context.Context, q string) (<-chan Progress, error); Get(id string) (Run, []Claim, error) }`
  - `type Verifier interface { Verify(ctx context.Context, claim string, sourceURL string) (VerifyStatus, float64, error) }`
  - `type VerifyStatus int` (`Unverified`, `Supported`, `Refuted`, `Inconclusive`)

- [ ] **Step 1: Failing tests**

`internal/research/orch_test.go`:
```go
package research

import (
	"context"
	"testing"
)

func TestOrchestrator_ReturnsAtLeastThreeDistinctSources(t *testing.T) {
	o := newTestOrch(t, fixtureSearch())
	_, err := o.Start(context.Background(), "what is Brewster's angle")
	if err != nil {
		t.Fatal(err)
	}
	_, claims, err := o.Get("r1")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range claims {
		seen[hostname(c.SourceURL)] = true
	}
	if len(seen) < 3 {
		t.Fatalf("orchestrator returned %d distinct sources, want ≥3", len(seen))
	}
}
```

`internal/research/verify_test.go`:
```go
package research

import (
	"context"
	"testing"
)

func TestVerifier_RefutesWhenSourceContradictsClaim(t *testing.T) {
	v := newTestVerifier(t, fixtureLLM(contradicts))
	status, conf, err := v.Verify(context.Background(), "Earth is flat", "https://nasa.gov/earth-shape")
	if err != nil {
		t.Fatal(err)
	}
	if status != Refuted {
		t.Fatalf("Verify status = %v, want Refuted", status)
	}
	if conf < 0.7 {
		t.Fatalf("Verify confidence = %v, want ≥0.7", conf)
	}
}
```

- [ ] **Step 2: Implement orchestrator** — fan-out via `internal/feeds/` + new `internal/web/search.go` (DuckDuckGo HTML scrape, no API key required); honor `research_fanout` privacy budget bucket; respect per-domain allowlist from Settings → Research.

```go
package research

import (
	"context"
	"sync"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/web"
)

type orchestrator struct {
	search web.Searcher
	verify Verifier
	budget *budget.Bucket
	store  Store
}

func (o *orchestrator) Start(ctx context.Context, q string) (<-chan Progress, error) {
	if err := o.budget.Charge(1); err != nil {
		return nil, err
	}
	hits, err := o.search.Query(ctx, q, 8)
	if err != nil {
		return nil, err
	}
	prog := make(chan Progress, 8)
	go func() {
		defer close(prog)
		var wg sync.WaitGroup
		for _, h := range hits {
			h := h
			wg.Add(1)
			go func() {
				defer wg.Done()
				claim, err := o.extractClaim(ctx, h)
				if err != nil {
					return
				}
				status, conf, _ := o.verify.Verify(ctx, claim.Text, h.URL)
				claim.Verified = status
				claim.Confidence = conf
				_ = o.store.AddClaim(claim)
				prog <- Progress{Claim: claim}
			}()
		}
		wg.Wait()
	}()
	return prog, nil
}
```

- [ ] **Step 3: Implement verifier** — independent Anthropic call per claim with the cited source page as input; adversarial framing in the system prompt ("does this source support the claim? cite the exact paragraph or refute"). Charge `research_fanout` budget per verify call.

```go
package research

import (
	"context"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

const verifySystemPrompt = `You are an adversarial fact-checker. The user provides a claim and a source page. Decide if the source SUPPORTS, REFUTES, or is INCONCLUSIVE about the claim. Quote the exact paragraph that drives your verdict. Be harsh — if the source only tangentially mentions the topic, return INCONCLUSIVE.`

type llmVerifier struct {
	client *anthropic.Client
	budget *budget.Bucket
}

func (v *llmVerifier) Verify(ctx context.Context, claim string, sourceURL string) (VerifyStatus, float64, error) {
	if err := v.budget.Charge(1); err != nil {
		return Unverified, 0, err
	}
	body, err := fetchAndExtract(ctx, sourceURL)
	if err != nil {
		return Unverified, 0, err
	}
	prompt := "Claim: " + claim + "\n\nSource page:\n" + body
	resp, err := v.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.ModelClaudeSonnet45Latest),
		MaxTokens: anthropic.F(int64(512)),
		System:    anthropic.F([]anthropic.TextBlockParam{{Text: anthropic.F(verifySystemPrompt)}}),
		Messages:  anthropic.F([]anthropic.MessageParam{{Role: anthropic.F(anthropic.MessageParamRoleUser), Content: anthropic.F([]anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(prompt)})}}),
	})
	if err != nil {
		return Unverified, 0, err
	}
	out := joinText(resp.Content)
	switch {
	case strings.Contains(out, "SUPPORTS"):
		return Supported, 0.9, nil
	case strings.Contains(out, "REFUTES"):
		return Refuted, 0.85, nil
	default:
		return Inconclusive, 0.5, nil
	}
}
```

- [ ] **Step 4: Implement cited markdown emitter** — claim + footnote + source URL + verify status; report path `~/Library/Application Support/Leah/research/<run_id>.md`.

```go
package research

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteReport(dir string, run Run, claims []Claim) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n_Started %s_\n\n", run.Question, run.Started.Format("2006-01-02 15:04"))
	for i, c := range claims {
		fmt.Fprintf(&b, "%s[^%d] — %s\n\n", c.Text, i+1, c.Verified)
	}
	b.WriteString("\n---\n\n")
	for i, c := range claims {
		fmt.Fprintf(&b, "[^%d]: <%s>\n", i+1, c.SourceURL)
	}
	path := filepath.Join(dir, run.ID+".md")
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}
```

- [ ] **Step 5: Verify + commit + PR + reviewer**

---

### Task 5: Phase 5 migrations (`internal/sqlstore/migrations/2026-06-23-*.sql`) — SINGLE OWNER

**Files:**
- Create: `internal/sqlstore/migrations/2026-06-23-001-agent.sql`
- Create: `internal/sqlstore/migrations/2026-06-23-002-research.sql`
- Create: `internal/sqlstore/migrations/2026-06-23-003-scheduler.sql`
- Create: `internal/sqlstore/migrations/2026-06-23-004-subs.sql`
- Create: `internal/sqlstore/migrations/2026-06-23-005-geofence.sql`
- Create: `internal/sqlstore/migrations/2026-06-23-006-taste.sql`
- Create: `internal/sqlstore/migrations/2026-06-23-007-household.sql`
- Create: `internal/sqlstore/migrations/2026-06-23-008-longctx.sql`
- Create: `internal/sqlstore/migrations/2026-06-23-009-bridge.sql`
- Modify: `internal/sqlstore/migrations.go` — register new files in chronological order
- Create: `internal/sqlstore/migrations_phase5_test.go`

**Why this exists:** All nine Phase 5 migrations land in one PR because the migration registry is a frozen-enum file (per CLAUDE.md). Splitting across tasks invites stale-base regressions when two PRs branched off the same main both edit the registry.

- [ ] **Step 1: Write failing test** — `migrations_phase5_test.go` runs each migration against a fresh sqlite DB, then issues a representative INSERT into every new table. Fails if any migration is missing or any INSERT errors.

```go
package sqlstore

import (
	"database/sql"
	"testing"
	_ "modernc.org/sqlite"
)

func TestPhase5Migrations_AllApply(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	for _, ins := range phase5InsertSmoke {
		if _, err := db.Exec(ins); err != nil {
			t.Fatalf("phase5 smoke insert failed: %v\nSQL: %s", err, ins)
		}
	}
}

var phase5InsertSmoke = []string{
	`INSERT INTO agent_action (step_id, kind, intent, tool, ts) VALUES ('s1', 'intent', 'click', 'webkit.tap', '2026-06-23')`,
	`INSERT INTO research_run (id, question, started_ts) VALUES ('r1', 'q', '2026-06-23')`,
	`INSERT INTO sched_attempt (id, question, ts) VALUES ('a1', 'q', '2026-06-23')`,
	`INSERT INTO subscription (id, vendor, monthly_cents) VALUES ('s1', 'x', 999)`,
	`INSERT INTO geofence (id, label, lat, lon, radius_m) VALUES ('g1', 'home', 37.7, -122.4, 100)`,
	`INSERT INTO taste_profile (id, label) VALUES ('t1', 'work')`,
	`INSERT INTO household_profile (id, label) VALUES ('h1', 'operator')`,
	`INSERT INTO longctx_cursor (conv_id, cache_key, ts) VALUES ('c1', 'k1', '2026-06-23')`,
	`INSERT INTO bridge_shortcut (id, label, ts) VALUES ('b1', 'x', '2026-06-23')`,
}
```

- [ ] **Step 2: Run, FAIL** — expected `no such table` for every new table.

- [ ] **Step 3: Author migration 001-agent.sql**

```sql
CREATE TABLE agent_action (
	step_id TEXT NOT NULL,
	kind    TEXT NOT NULL CHECK (kind IN ('intent', 'outcome')),
	intent  TEXT,
	tool    TEXT,
	args_json TEXT,
	ok      INTEGER,
	err     TEXT,
	ts      TEXT NOT NULL,
	PRIMARY KEY (step_id, kind)
);
CREATE INDEX idx_agent_action_ts ON agent_action(ts);

CREATE TABLE agent_consent (
	tool         TEXT PRIMARY KEY,
	policy       TEXT NOT NULL,
	updated_ts   TEXT NOT NULL
);

CREATE TABLE agent_session (
	id           TEXT PRIMARY KEY,
	goal         TEXT NOT NULL,
	started_ts   TEXT NOT NULL,
	ended_ts     TEXT,
	halted       INTEGER NOT NULL DEFAULT 0
);
```

- [ ] **Step 4: Author migrations 002–009** — schemas mirror the table list in the file-structure-decisions migrations table above. Every table includes a `ts` column or equivalent for retention sweeps.

- [ ] **Step 5: Register migrations** — append the nine filenames to the migration registry in chronological order.

- [ ] **Step 6: Run tests** — must PASS.

- [ ] **Step 7: Commit + PR + reviewer**

---

## Wave 2 — Control plane (parallel ≤ 4)

---

### Task 6: Calendar smart-scheduler + travel-time + auto-decline (`internal/calendar/scheduler/`)

**Files:**
- Create: `internal/calendar/scheduler/solver.go`
- Create: `internal/calendar/scheduler/solver_test.go`
- Create: `internal/calendar/scheduler/travel.go`
- Create: `internal/calendar/scheduler/travel_test.go`
- Create: `internal/calendar/scheduler/decline.go`
- Create: `internal/calendar/scheduler/decline_test.go`

**Why this exists:** §3 mandates Motion/Reclaim-class meeting placement: scan EventKit + travel-time + focus-time guards + working-hours model + per-attendee timezone resolver + auto-decline rules. The operator's calendar is the single load-bearing surface for time — without scheduling Leah is a memo, not a chief-of-staff.

**Interfaces:**
- Produces:
  - `package scheduler`
  - `type Request struct { Title string; Duration time.Duration; Attendees []Attendee; Preferences Prefs }`
  - `type Slot struct { Start time.Time; End time.Time; Score float64; Reasons []string }`
  - `type Solver interface { Propose(ctx context.Context, r Request) ([]Slot, error); Place(ctx context.Context, s Slot, r Request) (string, error) }`
  - `type AutoDecline struct { Rule string; Match Matcher }`
  - `func ApplyDecline(ctx context.Context, evt EventKitEvent, rules []AutoDecline) (declined bool, reason string)`

- [ ] **Step 1: Failing tests**

`internal/calendar/scheduler/solver_test.go`:
```go
package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestSolver_ProposeReturnsAtLeastThreeSlots(t *testing.T) {
	s := newTestSolver(t, fixtureBusy())
	req := Request{
		Title:    "test",
		Duration: 30 * time.Minute,
		Preferences: Prefs{
			WorkingHoursStart: 9, WorkingHoursEnd: 17,
		},
	}
	slots, err := s.Propose(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) < 3 {
		t.Fatalf("Propose must return ≥3 slots, got %d", len(slots))
	}
}

func TestSolver_AvoidsFocusBlocks(t *testing.T) {
	focus := time.Date(2026, 7, 1, 14, 0, 0, 0, time.UTC)
	s := newTestSolver(t, []Event{{Title: "Focus", Start: focus, End: focus.Add(2 * time.Hour), IsFocus: true}})
	req := Request{Duration: 30 * time.Minute, Preferences: Prefs{WorkingHoursStart: 9, WorkingHoursEnd: 17}}
	slots, _ := s.Propose(context.Background(), req)
	for _, sl := range slots {
		if sl.Start.Before(focus.Add(2*time.Hour)) && sl.End.After(focus) {
			t.Fatalf("slot %v overlaps focus block %v", sl, focus)
		}
	}
}

func TestSolver_TravelTimePadding(t *testing.T) {
	prev := Event{Title: "Lunch", Start: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 1, 13, 0, 0, 0, time.UTC), Location: "Cafe Grumpy NYC"}
	s := newTestSolver(t, []Event{prev})
	req := Request{Duration: 30 * time.Minute, Location: "Bryant Park NYC"}
	slots, _ := s.Propose(context.Background(), req)
	for _, sl := range slots {
		if sl.Start.Sub(prev.End) < 15*time.Minute && sl.Start.After(prev.End) {
			t.Fatalf("slot %v lacks 15-min travel padding after %v", sl.Start, prev.End)
		}
	}
}
```

`internal/calendar/scheduler/decline_test.go`:
```go
package scheduler

import "testing"

func TestApplyDecline_MatchesByTitleRegex(t *testing.T) {
	rules := []AutoDecline{{Rule: `(?i)recruiter`, Match: regexMatcher(`(?i)recruiter`)}}
	evt := Event{Title: "Recruiter intro chat"}
	ok, reason := ApplyDecline(nil, evt, rules)
	if !ok || reason == "" {
		t.Fatalf("must decline recruiter, got ok=%v reason=%q", ok, reason)
	}
}
```

- [ ] **Step 2: Run, FAIL** — `undefined: newTestSolver`, `undefined: ApplyDecline`.

- [ ] **Step 3: Implement solver** — read EventKit via existing `internal/macos/eventkit.go`; score slots by (a) operator working hours, (b) focus-block avoidance, (c) attendee timezone overlap, (d) travel-time gap, (e) preferred-day-of-week. Score is in `[0, 1]`; lowest-friction slot wins.

```go
package scheduler

import (
	"context"
	"sort"
	"time"
)

type solver struct {
	cal    CalendarReader
	travel TravelEstimator
}

func New(cal CalendarReader, travel TravelEstimator) Solver { return &solver{cal: cal, travel: travel} }

func (s *solver) Propose(ctx context.Context, r Request) ([]Slot, error) {
	now := time.Now()
	horizon := now.Add(7 * 24 * time.Hour)
	busy, err := s.cal.Busy(ctx, now, horizon)
	if err != nil {
		return nil, err
	}
	candidates := s.enumerateCandidates(now, horizon, r)
	candidates = s.filterBusy(candidates, busy, r)
	candidates = s.filterTravel(ctx, candidates, busy, r)
	scored := s.score(candidates, r)
	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > 10 {
		scored = scored[:10]
	}
	return scored, nil
}
```

- [ ] **Step 4: Implement travel-time estimator** — reuse `internal/maps/` for distance + estimated drive/transit time; cache per origin-destination pair for 24 h.

```go
package scheduler

import (
	"sync"
	"time"
)

type travelCache struct {
	mu sync.RWMutex
	v  map[string]travelEntry
}

type travelEntry struct {
	Dur time.Duration
	Ts  time.Time
}

func (t *travelCache) Get(origin, dest string) (time.Duration, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	e, ok := t.v[origin+"|"+dest]
	if !ok || time.Since(e.Ts) > 24*time.Hour {
		return 0, false
	}
	return e.Dur, true
}
```

- [ ] **Step 5: Implement auto-decline rules** — rules persisted in `sched_rule`; declines logged to `sched_decline` with reason. Rule format: `(?i)<regex>` on `Title`, OR `from:<sender>` on `Organizer`.

- [ ] **Step 6: Run tests, verify green**

- [ ] **Step 7: Verify + commit + PR + reviewer**

---

### Task 7: Subscription detector + renewal cards (`internal/subs/`)

**Files:**
- Create: `internal/subs/detect.go`
- Create: `internal/subs/detect_test.go`
- Create: `internal/subs/renew.go`
- Create: `internal/subs/renew_test.go`
- Create: `internal/subs/cancel_url.go`
- Create: `internal/subs/cancel_url_test.go`
- Create: `internal/subs/testdata/receipts/` (fixtures)

**Why this exists:** §4 mandates Mail-derived subscription detection (mailbox-only signal, never bank scrape — privacy invariant). Per-charge anomaly alerts catch silent price hikes; renewal cards land on the calendar; cancel-helper URLs let the operator one-tap to the unsubscribe page.

**Interfaces:**
- Produces:
  - `package subs`
  - `type Subscription struct { ID string; Vendor string; MonthlyCents int; FirstSeen time.Time; LastCharge time.Time; CancelURL string }`
  - `type Detector interface { Scan(ctx context.Context, since time.Time) ([]Subscription, error) }`
  - `type RenewCardEmitter interface { Emit(ctx context.Context, s Subscription, next time.Time) error }`
  - `func ResolveCancelURL(vendor string) (string, bool)` — table of 50+ known vendors → cancel pages

- [ ] **Step 1: Failing tests**

`internal/subs/detect_test.go`:
```go
package subs

import (
	"context"
	"testing"
	"time"
)

func TestDetect_FindsKnownVendorsInFixtureCorpus(t *testing.T) {
	d := newTestDetector(t, "testdata/receipts")
	subs, err := d.Scan(context.Background(), time.Now().Add(-90*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Netflix", "Spotify", "iCloud+"}
	got := map[string]bool{}
	for _, s := range subs {
		got[s.Vendor] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("expected vendor %q in subs, missing", w)
		}
	}
}

func TestDetect_AnomalyFiresOnPriceHike(t *testing.T) {
	d := newTestDetector(t, "testdata/receipts/pricehike")
	subs, _ := d.Scan(context.Background(), time.Now().Add(-365*24*time.Hour))
	for _, s := range subs {
		if s.Vendor == "Netflix" && !s.AnomalyFired {
			t.Fatal("Netflix +14% MoM must fire anomaly")
		}
	}
}
```

`internal/subs/cancel_url_test.go`:
```go
package subs

import "testing"

func TestCancelURL_ResolvesTopVendors(t *testing.T) {
	want := map[string]string{
		"Netflix":   "https://www.netflix.com/cancelplan",
		"Spotify":   "https://www.spotify.com/account/subscription/cancel/",
		"NYTimes":   "https://www.nytimes.com/subscription/cancel",
		"iCloud+":   "https://support.apple.com/HT207692",
		"YouTubePremium": "https://support.google.com/youtube/answer/6308278",
	}
	for v, u := range want {
		got, ok := ResolveCancelURL(v)
		if !ok || got != u {
			t.Errorf("ResolveCancelURL(%q) = (%q, %v), want (%q, true)", v, got, ok, u)
		}
	}
}
```

- [ ] **Step 2: Implement detector** — parse Mail.app sqlite via `internal/macos/mail.go`; regex-match receipt phrases per vendor; aggregate by (vendor, monthly_cents). Honor mail-content privacy invariant — only subject + sender are scanned by default; body scan requires `subs.body_ok` consent toggle.

```go
package subs

import (
	"context"
	"regexp"
	"time"
)

var vendorPatterns = map[string]*regexp.Regexp{
	"Netflix":        regexp.MustCompile(`(?i)from:.*netflix.*subject:.*(receipt|invoice|payment)`),
	"Spotify":        regexp.MustCompile(`(?i)from:.*spotify.*subject:.*(receipt|invoice|payment)`),
	"iCloud+":        regexp.MustCompile(`(?i)from:.*apple.*subject:.*icloud.*(receipt|invoice)`),
	"NYTimes":        regexp.MustCompile(`(?i)from:.*nytimes.*subject:.*(receipt|invoice)`),
	"YouTubePremium": regexp.MustCompile(`(?i)from:.*youtube.*subject:.*premium.*(receipt|invoice)`),
}

type detector struct {
	mail MailReader
	body bool
}

func (d *detector) Scan(ctx context.Context, since time.Time) ([]Subscription, error) {
	msgs, err := d.mail.HeadersOnly(ctx, since)
	if err != nil {
		return nil, err
	}
	by := map[string][]Charge{}
	for _, m := range msgs {
		composite := "from:" + m.From + "subject:" + m.Subject
		for v, re := range vendorPatterns {
			if re.MatchString(composite) {
				cents := parseCharge(m.Subject)
				if cents > 0 {
					by[v] = append(by[v], Charge{Cents: cents, Ts: m.Date})
				}
			}
		}
	}
	return aggregate(by), nil
}
```

- [ ] **Step 3: Implement renew emitter** — write a calendar event 3 days before next charge via EventKit write API (Phase 5 first calendar-write use); emit `subs.renew` IPC for Dashboard.

- [ ] **Step 4: Implement cancel URL table** — hard-coded map in `cancel_url.go`; ≥20 vendors at land time, +5/quarter via PR.

```go
package subs

var cancelURLs = map[string]string{
	"Netflix":        "https://www.netflix.com/cancelplan",
	"Spotify":        "https://www.spotify.com/account/subscription/cancel/",
	"iCloud+":        "https://support.apple.com/HT207692",
	"NYTimes":        "https://www.nytimes.com/subscription/cancel",
	"YouTubePremium": "https://support.google.com/youtube/answer/6308278",
	"AppleMusic":     "https://support.apple.com/HT202039",
	"HuluPlus":       "https://www.hulu.com/account/cancel",
	"Disney+":        "https://www.disneyplus.com/account/subscription",
	"HBOMax":         "https://help.max.com/Answer/Detail/000001188",
	"AmazonPrime":    "https://www.amazon.com/gp/help/customer/display.html?nodeId=GMUUNV2NCT35MEZF",
	"Dropbox":        "https://www.dropbox.com/account/plan",
	"GoogleOne":      "https://one.google.com/storage/management",
	"Github":         "https://github.com/settings/billing",
	"OpenAI":         "https://chat.openai.com/account/manage-account",
	"AnthropicPro":   "https://console.anthropic.com/account/billing",
	"Notion":         "https://www.notion.so/my-account",
	"1Password":      "https://my.1password.com/billing",
	"Linear":         "https://linear.app/settings/billing",
	"Figma":          "https://www.figma.com/files/settings/billing",
	"Adobe":          "https://account.adobe.com/plans",
	"Substack":       "https://substack.com/account",
}

func ResolveCancelURL(vendor string) (string, bool) {
	u, ok := cancelURLs[vendor]
	return u, ok
}
```

- [ ] **Step 5: Verify + commit + PR + reviewer**

---

### Task 8: Location geofence + reminder lifecycle (`internal/geofence/`)

**Files:**
- Create: `internal/geofence/geofence_darwin.go`
- Create: `internal/geofence/geofence_darwin_test.go`
- Create: `internal/geofence/geofence_stub.go`
- Create: `internal/geofence/reminder.go`
- Create: `internal/geofence/reminder_test.go`
- Create: `internal/geofence/geofence.h`
- Create: `internal/geofence/geofence.m`

**Why this exists:** §5 mandates CoreLocation geofence + significant-change events → reminder lifecycle. Apple Reminders parity is table stakes for any macOS personal AI; without it, location-conditioned tasks die in the operator's head.

**Interfaces:**
- Produces:
  - `package geofence`
  - `type Fence struct { ID string; Label string; Lat, Lon float64; RadiusM int; OnEnter bool; OnExit bool }`
  - `type Event struct { FenceID string; Kind EventKind; Ts time.Time }`
  - `type Listener interface { Add(f Fence) error; Remove(id string) error; Stream(ctx context.Context) <-chan Event }`
  - `type Reminder struct { ID string; Text string; FenceID string; Done bool }`
  - `func Lifecycle(ctx context.Context, l Listener, store ReminderStore) error`

- [ ] **Step 1: Failing tests**

`internal/geofence/reminder_test.go`:
```go
package geofence

import (
	"context"
	"testing"
	"time"
)

func TestLifecycle_FiresReminderOnEnter(t *testing.T) {
	l := newStubListener()
	store := newTestReminderStore(t)
	_ = store.Put(Reminder{ID: "r1", Text: "grocery list", FenceID: "home"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	fired := make(chan string, 1)
	go func() {
		_ = Lifecycle(ctx, l, store, func(text string) { fired <- text })
	}()
	l.emit(Event{FenceID: "home", Kind: Enter, Ts: time.Now()})
	select {
	case got := <-fired:
		if got != "grocery list" {
			t.Fatalf("fired text = %q, want %q", got, "grocery list")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("reminder did not fire on enter")
	}
}

func TestListener_AddRemoveIdempotent(t *testing.T) {
	l := newStubListener()
	f := Fence{ID: "home", Label: "home", Lat: 37.7, Lon: -122.4, RadiusM: 100, OnEnter: true}
	if err := l.Add(f); err != nil {
		t.Fatal(err)
	}
	if err := l.Add(f); err != nil {
		t.Fatalf("second Add must be idempotent, got %v", err)
	}
	if err := l.Remove("home"); err != nil {
		t.Fatal(err)
	}
	if err := l.Remove("home"); err != nil {
		t.Fatalf("second Remove must be idempotent, got %v", err)
	}
}
```

- [ ] **Step 2: Author `geofence.h` + `geofence.m`** — Objective-C shim around `CLLocationManager` + `CLCircularRegion` + significant-change events.

```objc
// geofence.h
#ifndef LEAH_GEOFENCE_H
#define LEAH_GEOFENCE_H
#include <stdint.h>
typedef void (*geofence_event_cb)(const char* fence_id, int kind, int64_t ts);
int gf_init(geofence_event_cb cb);
int gf_add(const char* id, double lat, double lon, int radius_m, int on_enter, int on_exit);
int gf_remove(const char* id);
int gf_shutdown(void);
#endif
```

- [ ] **Step 3: Implement `geofence_darwin.go`** — cgo wrapper; request `kCLAuthorizationStatusAlways` only when operator explicitly enables in Settings → Privacy → Location. Authorization is a runtime gate, not a build-time one.

```go
//go:build darwin
package geofence

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework CoreLocation -framework Foundation
#include "geofence.h"
*/
import "C"

import (
	"errors"
	"sync"
	"time"
	"unsafe"
)

type darwinListener struct {
	mu     sync.Mutex
	fences map[string]Fence
	out    chan Event
}

func (d *darwinListener) Add(f Fence) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fences[f.ID] = f
	cid := C.CString(f.ID)
	defer C.free(unsafe.Pointer(cid))
	on := func(b bool) C.int { if b { return 1 }; return 0 }
	if rc := C.gf_add(cid, C.double(f.Lat), C.double(f.Lon), C.int(f.RadiusM), on(f.OnEnter), on(f.OnExit)); rc != 0 {
		return errors.New("gf_add failed")
	}
	return nil
}
```

- [ ] **Step 4: Implement reminder lifecycle** — on `Event.Kind == Enter` look up reminders bound to fence; emit `notification.toast` IPC; mark `done = 1` if operator confirms.

- [ ] **Step 5: Verify + commit + PR + reviewer**

---

### Task 9: Long-context cache cursor + Anthropic prompt-cache wiring (`internal/longctx/`)

**Files:**
- Create: `internal/longctx/cursor.go`
- Create: `internal/longctx/cursor_test.go`
- Create: `internal/longctx/cache.go`
- Create: `internal/longctx/cache_test.go`
- Create: `internal/longctx/telemetry.go`
- Modify: `internal/reasoner/reasoner.go` — call `longctx.Cursor.ApplyTo(req)` before issuing the Anthropic call

**Why this exists:** §8 mandates 1M-token context with prompt caching. Without per-conversation cache cursors every long convo pays cold-start cost on every turn — at 1M tokens that's prohibitive. Anthropic's prompt-cache API has explicit cache_control markers; cursor tracks the longest prefix worth caching.

**Interfaces:**
- Produces:
  - `package longctx`
  - `type Cursor struct { ConvID string; CacheKey string; LastApplied time.Time; HitCount int; MissCount int }`
  - `type Store interface { Get(convID string) (Cursor, bool, error); Put(c Cursor) error; Purge(convID string) error }`
  - `func ApplyTo(req *anthropic.MessageNewParams, c Cursor) *anthropic.MessageNewParams` — mutates the request to mark cache breakpoints
  - `type HitRate struct { Hits, Misses int; Ratio float64 }`
  - `func RateFor(store Store, since time.Time) (HitRate, error)`

- [ ] **Step 1: Failing tests**

`internal/longctx/cursor_test.go`:
```go
package longctx

import (
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestApplyTo_MarksMessagesUpToCursorAsCached(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Messages: anthropic.F([]anthropic.MessageParam{
			{Role: anthropic.F(anthropic.MessageParamRoleUser), Content: anthropic.F([]anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("turn 1")})},
			{Role: anthropic.F(anthropic.MessageParamRoleAssistant), Content: anthropic.F([]anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("reply 1")})},
			{Role: anthropic.F(anthropic.MessageParamRoleUser), Content: anthropic.F([]anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("turn 2")})},
		}),
	}
	c := Cursor{ConvID: "c1", CacheKey: "k1", LastApplied: time.Now()}
	got := ApplyTo(req, c)
	if !hasCacheControl(got, 1) {
		t.Fatal("ApplyTo must mark cache breakpoint at cursor index")
	}
}

func TestPurge_ClearsCursor(t *testing.T) {
	s := newTestStore(t)
	_ = s.Put(Cursor{ConvID: "c1", CacheKey: "k1"})
	if err := s.Purge("c1"); err != nil {
		t.Fatal(err)
	}
	_, ok, _ := s.Get("c1")
	if ok {
		t.Fatal("Purge must remove cursor")
	}
}

func TestHitRate_ComputesRatioOverFixtureRows(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 7; i++ {
		_ = s.RecordHit("c1")
	}
	for i := 0; i < 3; i++ {
		_ = s.RecordMiss("c1")
	}
	rate, _ := RateFor(s, time.Now().Add(-time.Hour))
	if rate.Hits != 7 || rate.Misses != 3 {
		t.Fatalf("rate = %+v, want hits=7 misses=3", rate)
	}
	if want := 0.7; rate.Ratio < want-1e-6 || rate.Ratio > want+1e-6 {
		t.Fatalf("ratio = %v, want %v", rate.Ratio, want)
	}
}
```

- [ ] **Step 2: Implement cursor** — `cursor.go` records per-conversation cache markers; conversation switch in `internal/reasoner/` calls `Purge` and re-warms.

```go
package longctx

import (
	"github.com/anthropics/anthropic-sdk-go"
)

func ApplyTo(req *anthropic.MessageNewParams, c Cursor) *anthropic.MessageNewParams {
	if c.CacheKey == "" {
		return req
	}
	msgs := req.Messages.Value
	cut := len(msgs)
	if cut > 4 {
		cut = cut - 2
	}
	for i := 0; i < cut; i++ {
		blocks := msgs[i].Content.Value
		for j, b := range blocks {
			if tb, ok := b.(anthropic.TextBlockParam); ok {
				tb.CacheControl = anthropic.F(anthropic.CacheControlEphemeralParam{
					Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral),
				})
				blocks[j] = tb
			}
		}
	}
	return req
}
```

- [ ] **Step 3: Implement telemetry** — every Anthropic response carries `usage.cache_read_input_tokens` and `usage.cache_creation_input_tokens`; record both into `longctx_telemetry` so About → Diagnostics surfaces cache-hit rate.

- [ ] **Step 4: Wire reasoner** — `internal/reasoner/reasoner.go` calls `longctx.ApplyTo` before issuing the call; on response, calls `longctx.RecordUsage`. Wire is a single load-bearing edit per the composition-root discipline; this task ships the modification + a test that reasoner does the call.

```go
// internal/reasoner/reasoner.go diff
@@
-	resp, err := r.client.Messages.New(ctx, req)
+	cursor, _, _ := r.longctx.Get(convID)
+	req = longctx.ApplyTo(req, cursor)
+	resp, err := r.client.Messages.New(ctx, req)
+	if err == nil {
+		r.longctx.RecordUsage(convID, resp.Usage)
+	}
```

- [ ] **Step 5: Verify + commit + PR + reviewer**

---

## Wave 3 — Taste + household (parallel ≤ 3)

---

### Task 10: Taste profile + MusicKit bridge (`internal/taste/` + `internal/taste/musickit/`)

**Files:**
- Create: `internal/taste/profile.go`
- Create: `internal/taste/profile_test.go`
- Create: `internal/taste/mood.go`
- Create: `internal/taste/mood_test.go`
- Create: `internal/taste/musickit/musickit_darwin.go`
- Create: `internal/taste/musickit/musickit_stub.go`
- Create: `internal/taste/musickit/musickit.h`
- Create: `internal/taste/musickit/musickit.m`

**Why this exists:** §6 mandates an operator-editable taste profile + per-mood playlists. Spotify's Large Taste Model normalized taste as a primitive — Leah needs a parallel surface or it's tone-deaf about the operator's actual listening context.

**Interfaces:**
- Produces:
  - `package taste`
  - `type Profile struct { ID string; Label string; Seeds []Seed; UpdatedTs time.Time }`
  - `type Seed struct { ArtistID string; GenreTag string; MoodTag string; Weight float64 }`
  - `type Mood string` (`Focus`, `Energize`, `WindDown`, `Travel`, `Workout`, `Background`)
  - `type Picker interface { Pick(ctx context.Context, m Mood, p Profile) (Playlist, error) }`
  - `package musickit`
  - `func PlayPlaylist(ctx context.Context, id string) error`
  - `func RecentlyPlayed(ctx context.Context, limit int) ([]Track, error)`

- [ ] **Step 1: Failing tests**

`internal/taste/profile_test.go`:
```go
package taste

import (
	"context"
	"testing"
	"time"
)

func TestProfile_RoundTripPreservesSeeds(t *testing.T) {
	s := newTestStore(t)
	in := Profile{
		ID:    "p1",
		Label: "work",
		Seeds: []Seed{
			{ArtistID: "a1", GenreTag: "ambient", MoodTag: "focus", Weight: 0.7},
			{ArtistID: "a2", GenreTag: "lo-fi",   MoodTag: "focus", Weight: 0.3},
		},
		UpdatedTs: time.Now(),
	}
	if err := s.Put(in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Seeds) != 2 {
		t.Fatalf("got %d seeds, want 2", len(got.Seeds))
	}
	if got.Seeds[0].Weight != 0.7 {
		t.Fatalf("seed[0].Weight = %v, want 0.7", got.Seeds[0].Weight)
	}
}

func TestPicker_ReturnsAtLeastTenTracks(t *testing.T) {
	p := newTestPicker(t)
	pl, err := p.Pick(context.Background(), Focus, Profile{Seeds: []Seed{{GenreTag: "ambient", Weight: 1.0}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Tracks) < 10 {
		t.Fatalf("Pick returned %d tracks, want ≥10", len(pl.Tracks))
	}
}
```

- [ ] **Step 2: Author musickit cgo shim** — Objective-C wrapper around `MPMusicPlayerController` + `MKMusicSearchRequest`. The shim is darwin-only; the stub returns ErrUnsupported on linux so CI lanes compile cleanly.

```objc
// musickit.h
#ifndef LEAH_MUSICKIT_H
#define LEAH_MUSICKIT_H
int mk_init(void);
int mk_play_playlist(const char* id);
const char* mk_recently_played_json(int limit);
void mk_free(const char* p);
#endif
```

- [ ] **Step 3: Implement profile + mood** — seed-weighted ranking over a Track corpus; profile editable from Settings → Taste. Mood is a discrete enum; Picker takes (Mood, Profile) and produces a Playlist by weighted random walk over the seed graph (artist → genre → similar artists).

```go
package taste

import "context"

type seedPicker struct {
	corpus TrackCorpus
}

func (p *seedPicker) Pick(ctx context.Context, m Mood, prof Profile) (Playlist, error) {
	candidates := []Track{}
	for _, seed := range prof.Seeds {
		batch, err := p.corpus.MatchingSeed(ctx, seed, m, 30)
		if err != nil {
			return Playlist{}, err
		}
		candidates = append(candidates, batch...)
	}
	scored := weightedSort(candidates, prof.Seeds, m)
	if len(scored) > 30 {
		scored = scored[:30]
	}
	return Playlist{ID: newID(), Tracks: scored, Mood: m, Seed: prof.Seeds[0]}, nil
}
```

- [ ] **Step 4: Verify + commit + PR + reviewer**

---

### Task 11: Per-mood + per-context playlist generator (`internal/taste/playlist.go`)

**Files:**
- Create: `internal/taste/playlist.go`
- Create: `internal/taste/playlist_test.go`
- Create: `internal/taste/context.go`
- Create: `internal/taste/context_test.go`

**Why this exists:** §6.3 mandates context detection (calendar focus block → Focus mood; CarPlay session → Travel mood; gym geofence → Workout mood). Without context detection the operator manually picks moods, which defeats the surface.

**Interfaces:**
- Produces:
  - `type Playlist struct { ID string; Tracks []Track; Mood Mood; Seed Seed }`
  - `type ContextDetector interface { Detect(ctx context.Context) (Mood, bool, error) }`
  - `func NewContextDetector(cal CalendarReader, geo GeofenceReader, car CarPlayReader) ContextDetector`

- [ ] **Step 1: Failing tests**

`internal/taste/context_test.go`:
```go
package taste

import (
	"context"
	"testing"
	"time"
)

func TestContextDetector_FocusBlockReturnsFocus(t *testing.T) {
	cal := stubCal{focusNow: true}
	d := NewContextDetector(cal, stubGeo{}, stubCar{})
	m, ok, err := d.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || m != Focus {
		t.Fatalf("Detect = (%v,%v), want (Focus,true)", m, ok)
	}
}

func TestContextDetector_CarPlayReturnsTravel(t *testing.T) {
	d := NewContextDetector(stubCal{}, stubGeo{}, stubCar{attached: true})
	m, ok, _ := d.Detect(context.Background())
	if !ok || m != Travel {
		t.Fatalf("Detect = (%v,%v), want (Travel,true)", m, ok)
	}
}

func TestContextDetector_GymGeofenceReturnsWorkout(t *testing.T) {
	d := NewContextDetector(stubCal{}, stubGeo{inside: "gym"}, stubCar{})
	m, ok, _ := d.Detect(context.Background())
	if !ok || m != Workout {
		t.Fatalf("Detect = (%v,%v), want (Workout,true)", m, ok)
	}
}

func TestContextDetector_FallbackIsBackground(t *testing.T) {
	d := NewContextDetector(stubCal{}, stubGeo{}, stubCar{})
	m, ok, _ := d.Detect(context.Background())
	if !ok || m != Background {
		t.Fatalf("Detect = (%v,%v), want (Background,true)", m, ok)
	}
	_ = time.Now
}
```

- [ ] **Step 2: Implement context detector** — consult EventKit (focus blocks), `internal/geofence/` (gym label), `internal/macos/carplay.go` (new — minimal probe via `IOServiceMatching`). Order matters: CarPlay wins over geofence wins over focus-block wins over fallback (most-specific first).

```go
package taste

import "context"

type detector struct {
	cal CalendarReader
	geo GeofenceReader
	car CarPlayReader
}

func (d *detector) Detect(ctx context.Context) (Mood, bool, error) {
	if d.car.Attached() {
		return Travel, true, nil
	}
	for _, label := range []string{"gym", "studio"} {
		if d.geo.InsideLabel(label) {
			return Workout, true, nil
		}
	}
	if d.cal.HasActiveFocusBlock(ctx) {
		return Focus, true, nil
	}
	return Background, true, nil
}
```

- [ ] **Step 3: Implement playlist generator** — seed-weighted; rotates last-played for variety; persists `taste_play` row per emit. Rotation ensures the same playlist isn't re-emitted within 24 h for the same (mood, profile) pair.

- [ ] **Step 4: Verify + commit + PR + reviewer**

---

### Task 12: Household voice-ID enrollment wizard + per-profile attestation (`internal/household/` + `internal/household/diarize/`)

**Files:**
- Create: `internal/household/profile.go`
- Create: `internal/household/profile_test.go`
- Create: `internal/household/voiceid.go`
- Create: `internal/household/voiceid_test.go`
- Create: `internal/household/enroll.go`
- Create: `internal/household/enroll_test.go`
- Create: `internal/household/diarize/diarize_darwin.go`
- Create: `internal/household/diarize/diarize_stub.go`
- Create: `internal/household/diarize/diarize.h`
- Create: `internal/household/diarize/diarize.m`
- Create: `cmd/leah/household.go` — `leah household enroll <label>` CLI

**Why this exists:** §7 mandates voice-ID-keyed sub-profiles. Without this, household members share the operator's memory namespace and attestation gate — privacy disaster. With it, each profile has its own memory shard, own attestation policy, own taste profile.

**Interfaces:**
- Produces:
  - `package household`
  - `type Profile struct { ID string; Label string; VoicePrintHash string; Attest AttestPolicy }`
  - `type Selector interface { Select(ctx context.Context, sample []int16) (Profile, float64, error) }`
  - `func Enroll(ctx context.Context, label string, samples [][]int16) (Profile, error)`
  - `package diarize`
  - `func Embed(pcm []int16) ([]float32, error)` — 192-dim speaker embedding
  - `func Similarity(a, b []float32) float64` — cosine

- [ ] **Step 1: Failing tests**

`internal/household/enroll_test.go`:
```go
package household

import (
	"context"
	"testing"
)

func TestEnroll_PersistsProfileWithVoicePrint(t *testing.T) {
	samples := [][]int16{loadFixture(t, "alice-1.pcm"), loadFixture(t, "alice-2.pcm"), loadFixture(t, "alice-3.pcm")}
	p, err := Enroll(context.Background(), "alice", samples)
	if err != nil {
		t.Fatal(err)
	}
	if p.VoicePrintHash == "" {
		t.Fatal("Enroll must populate VoicePrintHash")
	}
}

func TestSelector_ReturnsEnrolledOnHeldOut(t *testing.T) {
	samples := [][]int16{loadFixture(t, "alice-1.pcm"), loadFixture(t, "alice-2.pcm"), loadFixture(t, "alice-3.pcm")}
	p, _ := Enroll(context.Background(), "alice", samples)
	holdout := loadFixture(t, "alice-heldout.pcm")
	sel := NewSelector(testProfiles(t, p))
	got, sim, err := sel.Select(context.Background(), holdout)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID {
		t.Fatalf("Select = %q, want %q", got.ID, p.ID)
	}
	if sim < 0.85 {
		t.Fatalf("similarity = %v, want ≥0.85", sim)
	}
}

func TestEnroll_BlocksWithoutTouchID(t *testing.T) {
	if err := EnrollWithGate(context.Background(), "alice", nil, denyGate{}); err == nil {
		t.Fatal("Enroll must fail when Touch ID gate denies")
	}
}
```

- [ ] **Step 2: Author diarize cgo shim** — Objective-C wrapper around `SFSpeakerRecognitionRequest` (macOS 14+). Returns a 192-dim float embedding; cosine similarity in Go.

```objc
// diarize.h
#ifndef LEAH_DIARIZE_H
#define LEAH_DIARIZE_H
#include <stdint.h>
int dz_embed(const int16_t* pcm, int n_samples, int sample_rate, float* out_192);
#endif
```

- [ ] **Step 3: Implement Enroll** — record 30 s of speech via existing voice pipeline, embed via `diarize.Embed`, persist hashed embedding into `household_voice_sample`. Hash = SHA-256 over the embedding bytes; the raw embedding stays in `household_voice_sample` but only the hash leaves the daemon.

```go
package household

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/trilam/leah/internal/household/diarize"
)

func Enroll(ctx context.Context, label string, samples [][]int16) (Profile, error) {
	if len(samples) < 3 {
		return Profile{}, errors.New("Enroll: need ≥3 samples")
	}
	var emb []float32
	for _, s := range samples {
		v, err := diarize.Embed(s)
		if err != nil {
			return Profile{}, err
		}
		if emb == nil {
			emb = v
		} else {
			for i := range emb {
				emb[i] = (emb[i] + v[i]) / 2
			}
		}
	}
	h := sha256.Sum256(float32sToBytes(emb))
	return Profile{
		ID:             newID(),
		Label:          label,
		VoicePrintHash: hex.EncodeToString(h[:]),
		Attest:         DefaultAttestPolicy,
	}, nil
}
```

- [ ] **Step 4: Implement Selector** — every voice turn calls `Selector.Select` before reaching the reasoner; mismatch (similarity <0.7) raises an `household.switch` IPC and gates the turn until operator confirms or denies the profile switch.

```go
package household

import "context"

type selector struct {
	profiles []Profile
}

func (s *selector) Select(ctx context.Context, sample []int16) (Profile, float64, error) {
	v, err := diarize.Embed(sample)
	if err != nil {
		return Profile{}, 0, err
	}
	var best Profile
	var bestSim float64 = -1
	for _, p := range s.profiles {
		stored, err := loadEmbedding(p.ID)
		if err != nil {
			continue
		}
		sim := diarize.Similarity(v, stored)
		if sim > bestSim {
			bestSim = sim
			best = p
		}
	}
	return best, bestSim, nil
}
```

- [ ] **Step 5: Implement CLI** — `leah household enroll <label>` walks the operator through the 30-second recording. Mic capture via existing voice pipeline; Touch-ID gate via `internal/macos/touchid.go`.

- [ ] **Step 6: Verify + commit + PR + reviewer**

---

## Wave 4 — Agentic surfaces (parallel ≤ 3)

---

### Task 13: Computer-use HUD overlay + consent prompt + Halt button (`LeahHUD/Agent/`)

**Files:**
- Create: `app/Leah/Sources/LeahHUD/Agent/AgentOverlay.swift`
- Create: `app/Leah/Sources/LeahHUD/Agent/AgentOverlayTests.swift`
- Create: `app/Leah/Sources/LeahHUD/Agent/ConsentPrompt.swift`
- Create: `app/Leah/Sources/LeahHUD/Agent/ConsentPromptTests.swift`
- Create: `app/Leah/Sources/LeahHUD/Agent/HaltButton.swift`
- Create: `app/Leah/Sources/LeahHUD/Agent/HaltButtonTests.swift`
- Modify: `app/Leah/Sources/LeahIPC/Frame.swift` — add `agent.step`, `agent.consent`, `agent.halt` decoders

**Why this exists:** §1.7 mandates a visible per-step overlay (operator sees what the loop is about to click) + a one-key Halt + a modal consent prompt for `webkit.submit_form` and `uia.write`. Without the overlay the loop is invisible — operator can't supervise.

**Interfaces:**
- Produces:
  - `LeahHUD.Agent.AgentOverlay` — SwiftUI overlay rendering the current step's screenshot + tool + args
  - `LeahHUD.Agent.ConsentPrompt(.always | .session | .deny)` — modal prompt for AlwaysAsk tools
  - `LeahHUD.Agent.HaltButton` — single ⌘. binding (Cmd-period)

- [ ] **Step 1: Failing tests**

`app/Leah/Sources/LeahHUD/Agent/AgentOverlayTests.swift`:
```swift
import XCTest
import SwiftUI
@testable import LeahHUD

final class AgentOverlayTests: XCTestCase {
	func testOverlayRendersThreeStatSlots() {
		let step = AgentStep(id: "s1", tool: "webkit.tap", target: "submit-btn", args: "x=200,y=400")
		let overlay = AgentOverlay(step: step)
		XCTAssertEqual(overlay.statSlotCount, 3)
		XCTAssertEqual(overlay.toolText, "webkit.tap")
		XCTAssertEqual(overlay.targetText, "submit-btn")
	}

	func testConsentPromptStableButtonOrder() {
		let p = ConsentPrompt(question: "Submit form?", tool: "webkit.submit_form")
		XCTAssertEqual(p.buttons.map(\.label), ["Always for this site", "Just this session", "Deny"])
	}

	func testHaltButtonEmitsHaltOnTap() {
		let bus = TestFrameBus()
		let halt = HaltButton(bus: bus)
		halt.simulateTap()
		XCTAssertEqual(bus.lastSent?.kind, "agent.halt")
	}
}
```

- [ ] **Step 2: Implement overlay + consent + halt** — match HUD chrome from predecessor §4.7.

```swift
struct AgentOverlay: View {
	let step: AgentStep

	var body: some View {
		VStack(alignment: .leading, spacing: 8) {
			HStack {
				StatSlot(label: "TOOL",   value: step.tool)
				StatSlot(label: "TARGET", value: step.target)
				StatSlot(label: "ARGS",   value: step.args)
			}
			HaltButton(bus: FrameBus.shared)
		}
		.padding(12)
		.background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
	}
}

struct ConsentPrompt: View {
	let question: String
	let tool: String
	let buttons: [ConsentButton]

	init(question: String, tool: String) {
		self.question = question
		self.tool = tool
		self.buttons = [
			ConsentButton(label: "Always for this site", decision: .always),
			ConsentButton(label: "Just this session",    decision: .session),
			ConsentButton(label: "Deny",                 decision: .deny),
		]
	}
}
```

- [ ] **Step 3: Wire IPC** — overlay subscribes to `agent.step` frames; ConsentPrompt is summoned by `agent.consent` frames; HaltButton publishes `agent.halt`. The ⌘. (Cmd-period) global binding is registered in `LeahApp/HotkeyManager.swift` so the operator can Halt without focus on the overlay.

- [ ] **Step 4: Verify + commit + PR + reviewer**

---

### Task 14: Deep-research dashboard card + citation popover (`LeahHUD/Research/` + `LeahUI/Dashboard/ResearchCard.swift`)

**Files:**
- Create: `app/Leah/Sources/LeahHUD/Research/ResearchCard.swift`
- Create: `app/Leah/Sources/LeahHUD/Research/CitationPopover.swift`
- Create: `app/Leah/Sources/LeahUI/Dashboard/ResearchCard.swift` (Dashboard surface)
- Create: `app/Leah/Sources/LeahHUD/Research/ResearchCardTests.swift`

**Why this exists:** §2.7 mandates a Dashboard card surfacing the last 3 research runs with per-claim citation popovers. The popover is the differentiation surface — every other AI shows "sources" as a flat link list; Leah shows verify status (`Supported`/`Refuted`/`Inconclusive`) per claim.

- [ ] **Step 1: Failing tests**

`app/Leah/Sources/LeahHUD/Research/ResearchCardTests.swift`:
```swift
import XCTest
@testable import LeahHUD

final class ResearchCardTests: XCTestCase {
	func testCardListsLastThreeRunsOrderedByStartedTs() {
		let runs = [
			ResearchRun(id: "r1", question: "old",   startedTs: .init(timeIntervalSince1970: 100)),
			ResearchRun(id: "r2", question: "newer", startedTs: .init(timeIntervalSince1970: 200)),
			ResearchRun(id: "r3", question: "newest", startedTs: .init(timeIntervalSince1970: 300)),
			ResearchRun(id: "r4", question: "evicted", startedTs: .init(timeIntervalSince1970: 50)),
		]
		let card = ResearchCard(runs: runs)
		XCTAssertEqual(card.visibleRuns.count, 3)
		XCTAssertEqual(card.visibleRuns[0].id, "r3")
		XCTAssertEqual(card.visibleRuns[2].id, "r1")
	}

	func testPopoverRendersVerifyStatusBadgePerClaim() {
		let claims = [
			Claim(text: "Paris is the capital", sourceURL: "https://wiki", verified: .supported),
			Claim(text: "Mars is closer than Venus", sourceURL: "https://nasa", verified: .refuted),
		]
		let pop = CitationPopover(claims: claims)
		XCTAssertEqual(pop.badge(for: claims[0]), .supported)
		XCTAssertEqual(pop.badge(for: claims[1]), .refuted)
	}
}
```

- [ ] **Step 2: Implement card + popover** — SwiftUI; match Dashboard chrome from predecessor §4.7. The badge styling differentiates `Supported` (green) / `Refuted` (red) / `Inconclusive` (amber) / `Unverified` (gray); colorblind-safe palette.

```swift
struct ResearchCard: View {
	let runs: [ResearchRun]

	var visibleRuns: [ResearchRun] {
		runs.sorted(by: { $0.startedTs > $1.startedTs }).prefix(3).map { $0 }
	}

	var body: some View {
		VStack(alignment: .leading, spacing: 8) {
			Text("Research").font(.headline)
			ForEach(visibleRuns) { r in
				ResearchRow(run: r)
			}
		}
	}
}

struct CitationPopover: View {
	let claims: [Claim]

	func badge(for c: Claim) -> VerifyStatus { c.verified }

	var body: some View {
		VStack(alignment: .leading) {
			ForEach(claims) { c in
				HStack {
					VerifyBadge(status: c.verified)
					Text(c.text).font(.callout)
					Link("source", destination: URL(string: c.sourceURL)!)
				}
			}
		}
	}
}
```

- [ ] **Step 3: Verify + commit + PR + reviewer**

---

### Task 15: Mobile bridge intake + iCloud-Drive payload verify + quarantine (`internal/bridge/mobile/`)

**Files:**
- Create: `internal/bridge/mobile/intake.go`
- Create: `internal/bridge/mobile/intake_test.go`
- Create: `internal/bridge/mobile/verify.go`
- Create: `internal/bridge/mobile/verify_test.go`
- Create: `internal/bridge/mobile/quarantine.go`
- Create: `internal/bridge/mobile/quarantine_test.go`
- Create: `internal/bridge/mobile/testdata/payloads/valid.json`
- Create: `internal/bridge/mobile/testdata/payloads/forged.json`

**Why this exists:** §9 mandates an iPhone Shortcut-driven bridge so the operator can ask Leah from iOS without round-tripping via the Mac UI. EdDSA-signing the Shortcut output + verifying in the daemon keeps the trust boundary intact even though the payload lands via iCloud Drive (a userspace file).

**Interfaces:**
- Produces:
  - `package mobile`
  - `type Payload struct { ID string; Kind string; Body json.RawMessage; SigEd25519 []byte; Ts time.Time }`
  - `type Intake interface { Watch(ctx context.Context, dir string) (<-chan Payload, error); Verify(p Payload) (ok bool, err error); Quarantine(p Payload, reason string) error }`
  - `func KeyFromKeychain() (ed25519.PublicKey, error)` — pulls the operator's mobile-pair public key from Keychain

- [ ] **Step 1: Failing tests**

`internal/bridge/mobile/verify_test.go`:
```go
package mobile

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVerify_AcceptsSignedPayload(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	body, _ := json.Marshal(map[string]string{"question": "what time is it"})
	sig := ed25519.Sign(priv, body)
	p := Payload{ID: "p1", Kind: "ask", Body: body, SigEd25519: sig}
	v := newVerifier(pub)
	if ok, err := v.Verify(p); !ok || err != nil {
		t.Fatalf("Verify legit = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestVerify_RejectsForgedPayload(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	body, _ := json.Marshal(map[string]string{"question": "evil"})
	bad := make([]byte, 64)
	p := Payload{ID: "p2", Kind: "ask", Body: body, SigEd25519: bad}
	v := newVerifier(pub)
	if ok, _ := v.Verify(p); ok {
		t.Fatal("Verify forged must reject")
	}
}

func TestQuarantine_MovesFileWithReason(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "Inbox", "p3.json")
	_ = os.MkdirAll(filepath.Dir(src), 0o755)
	_ = os.WriteFile(src, []byte(`{}`), 0o644)
	q := newQuarantine(dir)
	if err := q.Quarantine(Payload{ID: "p3"}, "bad sig"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dir, "Inbox", "Quarantine", "p3.json")
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("expected file at %s, got %v", moved, err)
	}
}
```

- [ ] **Step 2: Implement Watch** — `fsnotify` over the iCloud-Drive inbox; on new file, parse + verify + dispatch. The watch only fires for `.json` files; other extensions land directly in quarantine.

```go
package mobile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

func (i *intake) Watch(ctx context.Context, dir string) (<-chan Payload, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	out := make(chan Payload, 4)
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return nil, err
	}
	go func() {
		defer w.Close()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&fsnotify.Create == 0 || !strings.HasSuffix(ev.Name, ".json") {
					continue
				}
				b, err := os.ReadFile(ev.Name)
				if err != nil {
					continue
				}
				var p Payload
				if err := json.Unmarshal(b, &p); err != nil {
					_ = i.Quarantine(p, "malformed JSON: "+err.Error())
					continue
				}
				p.Path = filepath.Base(ev.Name)
				out <- p
			}
		}
	}()
	return out, nil
}
```

- [ ] **Step 3: Implement Verify** — `ed25519.Verify` against Keychain-stored public key; on success emit `bridge.shortcut` IPC; on failure call Quarantine.

```go
package mobile

import (
	"crypto/ed25519"
	"errors"
)

type verifier struct{ pub ed25519.PublicKey }

func (v *verifier) Verify(p Payload) (bool, error) {
	if len(p.SigEd25519) != ed25519.SignatureSize {
		return false, errors.New("invalid signature size")
	}
	if !ed25519.Verify(v.pub, p.Body, p.SigEd25519) {
		return false, errors.New("signature mismatch")
	}
	return true, nil
}
```

- [ ] **Step 4: Verify + commit + PR + reviewer**

---

### Task 16: Sample first-party Shortcut + signing key custody doc (`shortcuts/leah-ask.shortcut` + `docs/specs/2026-06-23-mobile-pair-key.md`)

**Files:**
- Create: `shortcuts/leah-ask.shortcut` (binary, PR-uploaded)
- Create: `shortcuts/README.md`
- Create: `docs/specs/2026-06-23-mobile-pair-key.md`
- Create: `cmd/leah/mobile.go` — `leah mobile pair` CLI

**Why this exists:** §9.4 mandates a first-party Shortcut that operators can install in one tap. Without the sample, the bridge is theoretical. Signing-key custody doc lives next to the Shortcut so operator setup is self-contained.

- [ ] **Step 1: Author Shortcut** — 4 actions: `Ask for Input → Sign with Ed25519 → Write to iCloud Drive Inbox → Show Result`. Test on a physical iPhone before commit.

- [ ] **Step 2: Author custody doc** — `2026-06-23-mobile-pair-key.md` covers: key-pair generation on first `leah mobile pair`, public-key stored in Mac Keychain + private-key wrapped in iCloud Keychain on the iPhone (per-device, never leaves the device), rotation procedure, lost-device revocation.

- [ ] **Step 3: Implement `leah mobile pair` CLI** — generates Ed25519 keypair, prints QR for iPhone Shortcut to scan, writes public key to Mac Keychain.

- [ ] **Step 4: Verify + commit + PR + reviewer**

---

## Wave 5 — Supervision + ship

---

### Task 17: Supervisor registration for Phase-5 long-lived subsystems (`internal/supervisor/registrations_phase5.go`)

**Files:**
- Create: `internal/supervisor/registrations_phase5.go`
- Create: `internal/supervisor/registrations_phase5_test.go`

**Why this exists:** §0 ship gate requires every long-lived Phase-5 subsystem to register with the existing Phase-4 supervisor — agent loop, research orchestrator, geofence listener, mobile-bridge intake, household selector. Without registration a kill-9 of any of these leaks until daemon restart.

- [ ] **Step 1: Failing tests**

`internal/supervisor/registrations_phase5_test.go`:
```go
package supervisor

import "testing"

func TestPhase5_RegistrationsCoverAllLongLivedSubsystems(t *testing.T) {
	s := New()
	RegisterPhase5(s, fakeDeps())
	want := []string{
		"agent.loop",
		"research.orch",
		"geofence.listener",
		"bridge.mobile.intake",
		"household.selector",
	}
	for _, name := range want {
		if !s.HasEntry(name) {
			t.Errorf("supervisor missing registration: %s", name)
		}
	}
}

func TestPhase5_RestartUsesBackoffLadder(t *testing.T) {
	s := New()
	deps := failOnceDeps()
	RegisterPhase5(s, deps)
	s.RestartAll()
	if deps.restartLatency() > 250*time.Millisecond {
		t.Fatalf("restart did not respect 200ms backoff floor")
	}
}
```

- [ ] **Step 2: Implement registrations** — pass each subsystem's `Start`/`Stop`/`Restart` into `supervisor.Register(name, lifecycle)`.

```go
package supervisor

import "context"

func RegisterPhase5(s *Supervisor, deps Phase5Deps) {
	s.Register("agent.loop", Lifecycle{
		Start: func(ctx context.Context) error { return deps.AgentLoop.Start(ctx) },
		Stop:  func() error { return deps.AgentLoop.Halt() },
	})
	s.Register("research.orch", Lifecycle{
		Start: func(ctx context.Context) error { return deps.Research.Run(ctx) },
		Stop:  func() error { return deps.Research.Close() },
	})
	s.Register("geofence.listener", Lifecycle{
		Start: func(ctx context.Context) error { return deps.Geofence.Listen(ctx) },
		Stop:  func() error { return deps.Geofence.Close() },
	})
	s.Register("bridge.mobile.intake", Lifecycle{
		Start: func(ctx context.Context) error { return deps.MobileIntake.Watch(ctx, deps.MobileIntakeDir) },
		Stop:  func() error { return deps.MobileIntake.Close() },
	})
	s.Register("household.selector", Lifecycle{
		Start: func(ctx context.Context) error { return deps.HouseholdSelector.Run(ctx) },
		Stop:  func() error { return deps.HouseholdSelector.Close() },
	})
}
```

- [ ] **Step 3: Verify + commit + PR + reviewer**

---

### Task 18: Dashboard cards (Research + Subscription + Taste) wire-in (`LeahUI/Dashboard/`)

**Files:**
- Create: `app/Leah/Sources/LeahUI/Dashboard/ResearchCard.swift`
- Create: `app/Leah/Sources/LeahUI/Dashboard/SubscriptionCard.swift`
- Create: `app/Leah/Sources/LeahUI/Dashboard/TasteCard.swift`
- Modify: `app/Leah/Sources/LeahUI/Dashboard/Dashboard.swift` — register the three new cards

**Why this exists:** §2.7, §4.10, §6.7 each spec a card on the Dashboard; this task gathers them so the Dashboard module is touched once (not 3×).

- [ ] **Step 1: Failing tests** — snapshot tests assert each card has 3 stat slots; ResearchCard lists last 3 runs; SubscriptionCard lists next 30 days of renewals + monthly total; TasteCard shows current seed + last-played + suggested.

```swift
import XCTest
@testable import LeahUI

final class DashboardCardsPhase5Tests: XCTestCase {
	func testResearchCardThreeStatSlots() {
		let card = ResearchCard(runs: fixtureRuns(count: 3))
		XCTAssertEqual(card.statSlotCount, 3)
	}

	func testSubscriptionCardListsNext30DaysRenewals() {
		let subs = fixtureSubs(renewIn: [5, 12, 28, 45])
		let card = SubscriptionCard(subs: subs)
		XCTAssertEqual(card.upcomingRenewals.count, 3) // 5, 12, 28 in next 30 days
	}

	func testTasteCardShowsCurrentSeedAndSuggested() {
		let card = TasteCard(profile: fixtureProfile(), lastPlayed: fixturePlayed(), suggested: fixtureSuggested())
		XCTAssertEqual(card.statSlotCount, 3)
		XCTAssertFalse(card.suggestedTracks.isEmpty)
	}
}
```

- [ ] **Step 2: Implement cards** — match Dashboard chrome from predecessor §4.7. Each card consumes a daemon-side push stream (subscription, research, taste) so updates land without a poll loop.

- [ ] **Step 3: Verify + commit + PR + reviewer**

---

### Task 19: Wire all Phase 5 surfaces into composition root (`cmd/leah-daemon/main.go` + `scripts/dev/orphan-scan.sh`)

**Files:**
- Modify: `cmd/leah-daemon/main.go` — add `bootPhase5(ctx, deps)` invocation
- Create: `cmd/leah-daemon/boot_phase5.go`
- Modify: `scripts/dev/orphan-scan.sh` — add the Phase-5 internal packages to the scan list
- Create: `cmd/leah-daemon/boot_phase5_test.go`

**Why this exists:** v3.3.0 shipped with three wiring gaps (TTS providers, KG citation route, MCP composition) because Wave 1 producer PRs added `NewX()` constructors but the daemon boot path never instantiated them — composition-root wiring is its own task, not implicit. Phase 5 has more new packages (13) than Phase 4 (12); without an explicit wiring task at least 3 packages will be orphaned at tag time.

- [ ] **Step 1: Failing test — orphan-scan asserts ZERO Phase-5 packages have zero non-test callers**

```bash
# scripts/dev/orphan-scan.sh
PHASE5_PKGS=(
	"agent"
	"agent/webkit"
	"agent/uia"
	"research"
	"calendar/scheduler"
	"subs"
	"geofence"
	"taste"
	"taste/musickit"
	"household"
	"household/diarize"
	"longctx"
	"bridge/mobile"
)
fail=0
for pkg in "${PHASE5_PKGS[@]}"; do
	callers=$(grep -RIn "${pkg##*/}\." cmd/ internal/ 2>/dev/null | grep -v _test.go | grep -v "internal/${pkg}/" | head -1)
	if [ -z "$callers" ]; then
		echo "ORPHAN: internal/${pkg} has zero non-test callers"
		fail=1
	fi
done
exit $fail
```

- [ ] **Step 2: Implement `bootPhase5`** — extend the existing `main()` boot path (do NOT fork it). Each subsystem's constructor is called in dependency order: ledger → consent → webkit → uia → agent.Loop; orchestrator → verifier → cite; scheduler → travel → decline; subs → renew; geofence → reminder; taste → musickit → playlist; household → diarize → selector; longctx → reasoner-wire; mobile.Intake → quarantine. Each `Start` is registered with the supervisor (Task 17).

- [ ] **Step 3: IPC frame registration** — register every Phase-5 frame kind from the Global Constraints list (`agent.step`, `agent.consent`, `agent.halt`, `research.progress`, `research.cite`, `sched.place`, `sched.conflict`, `subs.detect`, `subs.renew`, `geofence.enter`, `geofence.exit`, `taste.update`, `household.switch`, `longctx.cache.hit`, `longctx.cache.miss`, `bridge.shortcut`) into `internal/ipc/frame.go`'s reserved-kind table.

- [ ] **Step 4: Verify gate**

```bash
bash scripts/dev/orphan-scan.sh
go vet ./...
golangci-lint run ./cmd/leah-daemon 2>&1 | grep -E 'errcheck|govet|staticcheck' | head -5
go test ./cmd/leah-daemon 2>&1 | tail -5
```

All must pass — `orphan-scan.sh` exits 0, lint clean, tests PASS.

- [ ] **Step 5: Commit + PR + reviewer**

---

### Task 20: Phase 5 E2E smoke + dispatch-template harness (`scripts/dev/phase5-e2e.sh` + `internal/eval/phase5.go`)

**Files:**
- Create: `scripts/dev/phase5-e2e.sh`
- Create: `internal/eval/phase5.go`
- Create: `internal/eval/phase5_test.go`

**Why this exists:** §0 ship gate requires every wave's deliverable to demonstrate end-to-end on `make dev`. T20 builds an automated smoke that drives each Phase 5 surface and asserts a load-bearing observable per deliverable.

- [ ] **Step 1: Author `phase5-e2e.sh`** — one section per deliverable:
  1. `agent` — invoke `leah agent run "open example.com and take screenshot"` → assert one screenshot landed under `~/Library/Application Support/Leah/agent/<session>/` AND `agent_action` table has ≥2 rows
  2. `research` — `leah research "what is the capital of France"` → assert report at `~/Library/Application Support/Leah/research/<run>.md` AND ≥1 claim row has `verify_status='Supported'`
  3. `scheduler` — `leah sched propose --title "test" --duration 30m` → assert ≥3 slots in JSON
  4. `subs` — `leah subs scan --since 30d` → assert no error (empty mailbox OK)
  5. `geofence` — `leah geofence list` → assert no error
  6. `taste` — `leah taste mood Focus` → assert ≥10 tracks returned
  7. `household` — `leah household list` → assert ≥1 row (operator profile)
  8. `longctx` — `leah ask --conversation c1 "hi"` then again → second call's response has `cache_read_input_tokens > 0`
  9. `bridge` — drop a signed payload into iCloud-inbox fixture dir → assert `bridge.shortcut` IPC emitted

- [ ] **Step 2: Author `phase5_test.go`** with one subtest per surface; each subtest gated on the prior wave's package being importable.

- [ ] **Step 3: Run + commit + PR + reviewer**

---

### Task 21: Phase 5 ship checklist + spec-parity + orphan-scan + deletion of superseded sketches + reviewer-and-merge pass

**Files:**
- Create: `docs/superpowers/plans/2026-06-23-leah-macos-native-phase5-ship-checklist.md`
- Delete: `docs/engineer/specs/2026-06-10-computer-use-sketch.md` (create-if-absent first)
- Delete: `docs/engineer/specs/2026-06-10-mobile-bridge-sketch.md` (create-if-absent first)
- Modify: `scripts/check-spec-parity.sh` — add Phase-5 forbidden phrases
- Modify: `CHANGELOG.md` — append `## v1.2.0 — 2026-XX-XX` block
- Modify: `docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md` (predecessor §19 update — sequence Phases 5 + 6)

**Why this exists:** Phase 5 spec §14 mandates the deletion of the two thin sketches and §0 sets the v1.2 ship line as the wave-5 exit gate. T19 (composition-root) and T21 (this) are the two non-skippable Wave-5 gates.

- [ ] **Step 1: Author ship checklist** — copy the Phase-4 ship-checklist scaffold; substitute Phase-5 deliverables 1–9; require operator sign-off per deliverable before tag.

- [ ] **Step 2: Delete superseded sketches** — touch the two files if they don't exist (so the delete is clean in the PR diff), then delete.

- [ ] **Step 3: Update spec parity**

Add to `scripts/check-spec-parity.sh`:

```bash
PHASE5_FORBIDDEN=(
	"browser agent"
	"smart calendar"
	"DJ mode"
	"kid mode"
	"sub-account"
)
for phrase in "${PHASE5_FORBIDDEN[@]}"; do
	if grep -RIn "$phrase" internal/ cmd/ app/ docs/superpowers/ 2>/dev/null | grep -v _test.go | grep -v "historical-anchor"; then
		echo "FORBIDDEN PHRASE: $phrase"; exit 1
	fi
done
```

- [ ] **Step 4: CHANGELOG**

```markdown
## v1.2.0 — 2026-XX-XX

Phase 5 ship criterion met. Computer-use loop landed (sandboxed WKWebView + AXUIElement bridge, per-action consent ladder, append-only action ledger). Deep-research workflow shipped (multi-source fan-out + per-claim citation verifier + cited markdown report). Calendar smart-scheduling landed (EventKit-driven slot solver + travel-time + auto-decline rules). Mail-derived subscription detector + renewal cards shipped (mailbox-only signal — never bank scrape). CoreLocation geofence + reminder lifecycle landed (Apple Reminders parity). Taste profile + MusicKit-driven mood playlists shipped (Spotify Large-Taste-Model parallel). Household voice-ID sub-profiles landed (per-profile memory namespace + attestation gate; Touch-ID-gated enrollment wizard). 1M-token long-context cache cursor wired into reasoner (Anthropic prompt-cache; per-conversation cache markers; cache-hit rate exposed in About → Diagnostics). iPhone Shortcut → iCloud-Drive bridge landed (EdDSA-signed payloads + verify + quarantine; sample `leah-ask.shortcut` shipped).
```

- [ ] **Step 5: Predecessor §19 update** — extend the predecessor design spec's §19 sequencing table with Phase 5 ship line and a forward stub for Phase 6.

- [ ] **Step 6: Final reviewer pass** — dispatch reviewer with `docs/engineer/dispatch-templates/reviewer.md`; reviewer asserts (a) every deliverable has a Dashboard or Settings surface, (b) every long-lived subsystem is supervised, (c) orphan-scan passes, (d) spec-parity passes, (e) E2E smoke passes, (f) two superseded sketches deleted, (g) CHANGELOG written.

- [ ] **Step 7: Tag + release** — operator-only step. `git tag v1.2.0 && git push origin v1.2.0` after reviewer APPROVE on the ship PR.

---

## Self-review notes (post-author)

- **Spec-not-found risk.** This plan was authored against a candidate set, not the live Phase 5 spec. Every section header should be reconciled against the spec when it lands; mismatches are spec-wins. The candidate set covers the nine highest-leverage gaps identified in `memory/research_ai_capability_domains_2026.md` plus a long-context surface motivated by the operator's "long sessions" friction line. If the spec changes the count, drop tasks from the bottom (T15/T16 are most movable — mobile bridge can defer to Phase 6).
- **Composition-root wiring is non-skippable.** T19 mirrors the Phase-4 lesson — without it, at least 3 packages will be orphaned at tag time. T19 → T20 → T21 must serialize.
- **Frozen-enum files single-owner.** T05 (migrations) is single-owner; the IPC frame kind additions in T19 land in T19 alone — no other Wave-5 task touches `internal/ipc/frame.go`.
- **Phase-5 buckets on top of Phase-4 seven.** Privacy budget gains `computer_use`, `research_fanout`, `location_lookup` — total 10 buckets at v1.2 ship line. Update the budget pane labels in T18.
- **macOS API surface load.** Phase 5 introduces five new cgo bridges (WKWebView, AXUIElement, CoreLocation, MusicKit, SFSpeakerRecognitionRequest). Stub-on-linux must compile cleanly for every one; CI lane needs a darwin-only build matrix.
- **Touch ID gating.** Voice-ID enrollment is operator-only (Touch ID gate). Computer-use Halt is operator-only (no other process can publish `agent.halt`). Mobile-bridge pair is operator-only.
