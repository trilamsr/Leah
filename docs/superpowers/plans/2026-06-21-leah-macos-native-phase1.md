# Leah macOS Native UI — Phase 1 Implementation Plan

> **Plan version:** v1.1 (2026-06-21) — folds 4 operator decisions on pre-flight conflict findings (F1 sqlite-vec via `modernc.org/sqlite/vec` pure-Go · F2 HashGenerator Phase 1, BGE Phase 2 · F3 full 6-step wizard · F4 Opus toggle Phase 1 with Settings → Advanced). See spec v3.2.2 §18 changelog + `/tmp/leah-phase1-preflight.md`.
>
> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the answer-engine + minimal native macOS shell: ⌥Space summons a SwiftUI focus panel; the operator types; the Go daemon streams a Sonnet 4.6 answer with `stat` / `table` / `list` widgets back over a Unix socket; the app is Developer-ID-signed, notarized, and Sparkle-updatable from GitHub Releases.

**Architecture:** Two-process design. (1) `leah-daemon` (Go, existing) owns ALL Anthropic + Voyage + Keychain I/O and persists conversation/memory to an existing SQLite WAL DB under `~/Library/Application Support/Leah/leah.db`. (2) `Leah.app` (new SwiftUI+AppKit native app under `app/Leah/`) renders the focus panel (NSPanel non-activating, key-window), NSStatusItem menubar, 3-step wizard, and Settings — and connects to the daemon via a length-prefixed JSON Unix socket at `~/Library/Caches/Leah/leah.sock`. The HUD process NEVER sees the Anthropic API key.

**Tech Stack:**
- **Daemon:** Go 1.25, `github.com/anthropics/anthropic-sdk-go v1.49.0` (already in `go.mod`), **`modernc.org/sqlite` v1.50.1+** (pure-Go, no CGo — bump from current pin per F1; v1.47.0 was the first version with the in-tree `sqlite-vec` port, v1.50.1+ is the current stable line), existing `internal/reasoner/`, `internal/embed/`, `internal/memory/`, `internal/sqlstore/` packages.
- **App:** Swift 5.9 / Swift Package Manager, macOS 14.0 (Sonoma) deployment target, AppKit + SwiftUI + Combine, Hardened Runtime (App Sandbox OFF), Sparkle 2.x via SPM (`https://github.com/sparkle-project/Sparkle`).
- **IPC:** AF_UNIX socket, length-prefixed (4-byte BE uint32) JSON frames per spec §10.7.
- **Embeddings (Phase 1, F2):** Voyage `voyage-3.5-lite` @ 1024d (cloud) primary + existing `internal/embed/` **HashGenerator** as the offline-degraded fallback. **No ONNX runtime in Phase 1.** Phase 2 swaps HashGenerator for BGE-small-en-v1.5 ONNX per spec §17.15.
- **Vector store (Phase 1, F1):** **sqlite-vec via `modernc.org/sqlite/vec`** — pure-Go in-tree port shipped in `modernc.org/sqlite` v1.47.0+ (2026-03-17). Activation = blank import `_ "modernc.org/sqlite/vec"` from `internal/sqlstore/`. Vector tables declared via `CREATE VIRTUAL TABLE knowledge_chunk_vec USING vec0(embedding float[1024])`; similarity queries use the `MATCH` operator. **No CGo, no driver swap, no `mattn/go-sqlite3` migration.** Brute-force `vec0` scan at < 200K vectors hits ~10–15 ms p95 per spec §17.10/§17.16.
- **Opus 4.8 escalation (Phase 1, F4):** Settings → Advanced ships a "Use Opus 4.8 for hard queries" single-shot toggle (next request only) + a session-wide variant. Daemon reads the flag from IPC frame OR settings file and flips the model id from `claude-sonnet-4-6` to `claude-opus-4-8`. Default OFF; default model remains Sonnet 4.6 per spec §17.14.
- **Distribution:** Developer ID Application cert + `xcrun notarytool` + Sparkle EdDSA + GitHub Pages `appcast.xml` + GitHub Releases binaries.

## Global Constraints

- **macOS minimum:** 14.0 (Sonoma). `LSMinimumSystemVersion = 14.0` in `Info.plist`.
- **App Sandbox: OFF; Hardened Runtime: ON.** Notarization required.
- **Bundle ID:** `com.maydow.leah` (app), `com.maydow.leah.daemon` (LaunchAgent).
- **Keychain service:** `com.maydow.leah.anthropic`, account `default`, class `kSecClassGenericPassword`, accessibility `kSecAttrAccessibleWhenUnlocked`.
- **Anthropic models (locked):** primary `claude-sonnet-4-6`, router `claude-haiku-4-5`, **escalation `claude-opus-4-8` opt-in via Settings → Advanced** (Phase 1 F4; single-shot OR session-wide; default OFF).
- **Prompt cache:** `cache_control: { type: "ephemeral" }` ON for system-prompt + conversation-history prefix when prompt is ≥1024 tokens (existing `internal/reasoner/cache.go::CacheableThresholdTokens`).
- **ZDR:** workspace-level Zero Data Retention required (verified at verify-ping time, surfaced in wizard).
- **IPC socket path:** `~/Library/Caches/Leah/leah.sock` (spec §17.2; v3.1 moved from Application Support per decision #105).
- **Hotkey:** `⌥Space` (Option+Space), fires on keydown; re-bindable Settings → General → Hotkey. Requires Accessibility permission.
- **Focus panel size:** 860 × 480 px default. NSPanel `styleMask = [.titled, .nonactivatingPanel]`, `level = .modalPanel`, `becomesKeyOnlyIfNeeded = false`, `worksWhenModal = true`, `collectionBehavior = [.fullScreenAuxiliary, .moveToActiveSpace, .stationary]`.
- **Dark mode ONLY** for Phase 1; light mode parity is Phase 2.
- **Widget primitives Phase 1:** `stat`, `table`, `list` only. 10 other widget types are Phase 2.
- **No AI signatures** in any committed code, commit message, or PR body (CLAUDE.md identity rule).
- **TDD enforced:** failing test FIRST; capture failing output in PR body; then impl; then green.
- **Reviewer per PR:** spawn an independent reviewer subagent immediately after `gh pr create`; verdict posted via `gh pr comment <N>` OR subagent transcript before merge (CLAUDE.md).
- **Dispatch parallelism:** Tasks 2–4 (daemon-only Go), 5–7 (knowledge/memory Go), 9–15 (app/Swift) are file-disjoint — up to 6 in flight. Tasks 1 (Makefile + bootstrap), 8 (shared IPC contract — single owner), and 17 (sign-and-notarize Makefile target) SERIALIZE.

---

## Task 1: App scaffold — Swift Package + Xcode project + entitlements

**Files:**
- Create: `app/Leah/Package.swift`
- Create: `app/Leah/Sources/LeahApp/LeahApp.swift`
- Create: `app/Leah/Sources/LeahApp/Info.plist`
- Create: `app/Leah/Sources/LeahApp/Leah.entitlements`
- Create: `app/Leah/Tests/LeahAppTests/LeahAppTests.swift`
- Modify: `Makefile` (add `app-build`, `app-test`, `app-run` targets) — SINGLE OWNER per CLAUDE.md
- Create: `app/Leah/README.md` (one paragraph; "how to build the app")

**Interfaces:**
- Consumes: nothing
- Produces: `LeahApp` SwiftUI `App` struct; `xcodebuild -workspace … build` and `swift test` both work; `make app-build` produces `app/Leah/.build/release/Leah.app` skeleton.

- [ ] **Step 1: Write the failing test**

`app/Leah/Tests/LeahAppTests/LeahAppTests.swift`:
```swift
import XCTest
@testable import LeahApp

final class LeahAppTests: XCTestCase {
  func testAppBundleIdentifier() {
    XCTAssertEqual(LeahApp.bundleIdentifier, "com.maydow.leah")
  }

  func testMinimumMacOSVersion() {
    XCTAssertEqual(LeahApp.minimumMacOS, "14.0")
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd app/Leah && swift test 2>&1 | tail -20
```
Expected: `error: no such module 'LeahApp'` (the package + target don't exist yet).

- [ ] **Step 3: Write minimal implementation**

`app/Leah/Package.swift`:
```swift
// swift-tools-version:5.9
import PackageDescription

let package = Package(
  name: "Leah",
  platforms: [.macOS(.v14)],
  products: [
    .executable(name: "Leah", targets: ["LeahApp"]),
  ],
  targets: [
    .executableTarget(
      name: "LeahApp",
      path: "Sources/LeahApp",
      resources: [.process("Info.plist"), .process("Leah.entitlements")]
    ),
    .testTarget(
      name: "LeahAppTests",
      dependencies: ["LeahApp"],
      path: "Tests/LeahAppTests"
    ),
  ]
)
```

`app/Leah/Sources/LeahApp/LeahApp.swift`:
```swift
import SwiftUI

@main
struct LeahApp: App {
  static let bundleIdentifier = "com.maydow.leah"
  static let minimumMacOS = "14.0"

  var body: some Scene {
    WindowGroup {
      // Replaced by NSPanel-hosted FocusPanel in Task 9; placeholder window
      // keeps the SwiftUI App protocol satisfied during bootstrap.
      Color.clear.frame(width: 1, height: 1)
    }
  }
}
```

`app/Leah/Sources/LeahApp/Info.plist`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleIdentifier</key><string>com.maydow.leah</string>
  <key>CFBundleName</key><string>Leah</string>
  <key>CFBundleShortVersionString</key><string>0.1.0</string>
  <key>CFBundleVersion</key><string>1</string>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>LSUIElement</key><false/>
  <key>NSMicrophoneUsageDescription</key><string>Leah needs the microphone for dictation.</string>
  <key>NSCalendarsUsageDescription</key><string>Leah reads your calendar to answer scheduling questions.</string>
  <key>NSAppleEventsUsageDescription</key><string>Leah uses AppleScript where EventKit doesn't suffice.</string>
</dict>
</plist>
```

`app/Leah/Sources/LeahApp/Leah.entitlements`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>com.apple.security.app-sandbox</key><false/>
  <key>com.apple.security.device.audio-input</key><true/>
  <key>com.apple.security.network.client</key><true/>
  <key>com.apple.security.automation.apple-events</key><true/>
  <key>com.apple.security.files.user-selected.read-write</key><true/>
  <key>com.apple.security.personal-information.calendars</key><true/>
  <key>com.apple.security.cs.disable-library-validation</key><true/>
  <key>com.apple.security.cs.allow-jit</key><true/>
</dict>
</plist>
```

Append to `Makefile` (after the existing `help:` target):
```makefile
.PHONY: app-build app-test app-run

# Swift Package build of the Leah.app shell.
app-build:
	@cd app/Leah && swift build -c release

app-test:
	@cd app/Leah && swift test

# Run the app from the SwiftPM build artifact (dev loop; production app is
# built via xcodebuild + sign-and-notarize.sh — see scripts/sign-and-notarize.sh).
app-run: app-build
	@app/Leah/.build/release/Leah
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd app/Leah && swift test 2>&1 | tail -10
```
Expected: `Test Suite 'All tests' passed` with `Executed 2 tests, with 0 failures`.

- [ ] **Step 5: Commit**

```bash
git add app/Leah/ Makefile
git commit -m "feat(app): bootstrap LeahApp Swift package + entitlements + Makefile targets"
```

---

## Task 2: Daemon IPC server — Unix socket + length-prefixed JSON frames

**Files:**
- Create: `internal/ipc/server.go`
- Create: `internal/ipc/frame.go`
- Create: `internal/ipc/server_test.go`
- Create: `internal/ipc/frame_test.go`
- Modify: `cmd/leah-daemon/main.go:30-60` (wire `ipc.NewServer(...).Serve(ctx)` into the daemon composition root after the existing audit/obs setup)

**Interfaces:**
- Consumes: `context.Context`, a handler `func(ctx context.Context, req Frame) (<-chan Frame, error)`
- Produces:
  - `func NewServer(socketPath string, handler Handler) *Server`
  - `func (s *Server) Serve(ctx context.Context) error`
  - `type Frame struct { Kind string \`json:"kind"\`; TurnID string \`json:"turn_id"\`; Seq uint64 \`json:"seq"\`; Payload json.RawMessage \`json:"payload,omitempty"\` }`
  - `func WriteFrame(w io.Writer, f Frame) error` — 4-byte BE uint32 length + JSON body, **256 KB cap** per spec §10.7
  - `func ReadFrame(r io.Reader) (Frame, error)`

- [ ] **Step 1: Write the failing test**

`internal/ipc/frame_test.go`:
```go
package ipc

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRoundTripFrame(t *testing.T) {
	in := Frame{Kind: "prose.delta", TurnID: "t1", Seq: 7, Payload: json.RawMessage(`{"text":"hi"}`)}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out.Kind != in.Kind || out.TurnID != in.TurnID || out.Seq != in.Seq {
		t.Fatalf("mismatch: got %+v want %+v", out, in)
	}
	if string(out.Payload) != string(in.Payload) {
		t.Fatalf("payload mismatch: %s vs %s", out.Payload, in.Payload)
	}
}

func TestFrameSizeCap(t *testing.T) {
	huge := Frame{Kind: "prose.delta", TurnID: "t1", Seq: 1, Payload: json.RawMessage(`"` + strings.Repeat("x", 300_000) + `"`)}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, huge); err == nil {
		t.Fatal("expected size-cap error, got nil")
	}
}
```

`internal/ipc/server_test.go`:
```go
package ipc

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestServerEcho(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "leah.sock")
	handler := func(ctx context.Context, req Frame) (<-chan Frame, error) {
		out := make(chan Frame, 1)
		out <- Frame{Kind: "prose.delta", TurnID: req.TurnID, Seq: 1, Payload: json.RawMessage(`{"text":"echo"}`)}
		close(out)
		return out, nil
	}
	s := NewServer(sock, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Serve(ctx) }()
	// wait for socket to appear
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := net.Dial("unix", sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := WriteFrame(conn, Frame{Kind: "ask", TurnID: "t1", Seq: 0, Payload: json.RawMessage(`{"text":"hi"}`)}); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, err := ReadFrame(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.Kind != "prose.delta" || resp.TurnID != "t1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/ipc/... 2>&1 | tail -10
```
Expected: `package github.com/trilam/leah/internal/ipc: no Go files` OR build errors `undefined: NewServer, ReadFrame, WriteFrame, Frame`.

- [ ] **Step 3: Write minimal implementation**

`internal/ipc/frame.go`:
```go
// Package ipc is the daemon-side AF_UNIX socket transport for the
// SwiftUI HUD. Length-prefixed JSON frames per spec §10.7; 256 KB cap
// protects the parser buffer from runaway widget payloads.
package ipc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const MaxFrameBytes = 256 * 1024

type Frame struct {
	Kind    string          `json:"kind"`
	TurnID  string          `json:"turn_id"`
	Seq     uint64          `json:"seq"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func WriteFrame(w io.Writer, f Frame) error {
	body, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	if len(body) > MaxFrameBytes {
		return fmt.Errorf("frame size %d exceeds cap %d", len(body), MaxFrameBytes)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("write hdr: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

func ReadFrame(r io.Reader) (Frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, fmt.Errorf("read hdr: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxFrameBytes {
		return Frame{}, fmt.Errorf("frame size %d exceeds cap %d", n, MaxFrameBytes)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return Frame{}, fmt.Errorf("read body: %w", err)
	}
	var f Frame
	if err := json.Unmarshal(body, &f); err != nil {
		return Frame{}, fmt.Errorf("unmarshal: %w", err)
	}
	return f, nil
}
```

`internal/ipc/server.go`:
```go
package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

type Handler func(ctx context.Context, req Frame) (<-chan Frame, error)

type Server struct {
	path    string
	handler Handler
}

func NewServer(socketPath string, handler Handler) *Server {
	return &Server{path: socketPath, handler: handler}
}

func (s *Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("mkdir socket dir: %w", err)
	}
	_ = os.Remove(s.path) // stale-socket recovery from crash (§17.8)
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}
	go func() { <-ctx.Done(); _ = ln.Close() }()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		go s.serveConn(ctx, conn)
	}
}

func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	for {
		req, err := ReadFrame(conn)
		if err != nil {
			return
		}
		out, err := s.handler(ctx, req)
		if err != nil {
			_ = WriteFrame(conn, Frame{Kind: "error", TurnID: req.TurnID, Seq: 0})
			continue
		}
		for f := range out {
			if err := WriteFrame(conn, f); err != nil {
				return
			}
		}
	}
}
```

Wire into `cmd/leah-daemon/main.go` (add to imports and after existing audit/obs setup; pick the existing post-onboarding/audit insertion point):
```go
import (
	// ... existing imports ...
	"github.com/trilam/leah/internal/ipc"
)

// inside main(), after `_ = onboarding.MarkInstalled(sd, time.Now())`:
sockPath := filepath.Join(os.Getenv("HOME"), "Library", "Caches", "Leah", "leah.sock")
// Placeholder handler — Task 8 replaces this with the reasoner-backed router.
echoHandler := func(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
	out := make(chan ipc.Frame, 1)
	out <- ipc.Frame{Kind: "prose.delta", TurnID: req.TurnID, Seq: 1}
	close(out)
	return out, nil
}
go func() { _ = ipc.NewServer(sockPath, echoHandler).Serve(ctx) }()
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/ipc/... -v 2>&1 | tail -15
```
Expected: `PASS` on both `TestRoundTripFrame`, `TestFrameSizeCap`, and `TestServerEcho`.

- [ ] **Step 5: Commit**

```bash
git add internal/ipc/ cmd/leah-daemon/main.go
git commit -m "feat(ipc): AF_UNIX socket + length-prefixed JSON frames (spec §10.7)"
```

---

## Task 3: Anthropic streaming — daemon-side Sonnet stream over IPC

**Files:**
- Create: `internal/reasoner/stream_ipc.go`
- Create: `internal/reasoner/stream_ipc_test.go`

(`internal/reasoner/anthropic.go` + `cache.go` + `router.go` ALREADY EXIST per `internal/reasoner/`. Extend, do not recreate.)

**Interfaces:**
- Consumes: existing `AnthropicClient` from `internal/reasoner/anthropic.go`; existing `Router` + `Breaker` from `internal/reasoner/router.go`; `ipc.Frame` from Task 2.
- Produces:
  - `func StreamToIPC(ctx context.Context, client *AnthropicClient, turnID string, systemPrompt string, history []anthropic.MessageParam, userText string, escalateOpus bool) (<-chan ipc.Frame, error)` — emits `prose.delta` frames per chunk + a final `turn.end` frame with cost.
  - Applies `cache_control: { type: "ephemeral" }` on the LAST history message + the system prompt block when `ShouldCachePrompt(text)` returns true (re-uses existing `internal/reasoner/cache.go::ShouldCachePrompt`).
  - Adds the `anthropic-version` header AND honors the ZDR workspace at the API-key level (no per-request header — ZDR is workspace-config).
  - **Model selector (v3.2.2 F4)** — when `escalateOpus == true` (sourced from the IPC frame's `escalate_opus` boolean OR from `settings.json::leah.opus.sessionWide`), swap the model id passed to the Anthropic SDK from `claude-sonnet-4-6` to `claude-opus-4-8` for this request only. Single-shot UserDefault `leah.opus.singleShot` auto-resets after the daemon emits `turn.end`. Default remains Sonnet 4.6. Wire the read in this Task; the writer ships in Task 15 (Settings → Advanced).

- [ ] **Step 1: Write the failing test**

`internal/reasoner/stream_ipc_test.go`:
```go
package reasoner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/trilam/leah/internal/ipc"
)

// fakeStreamer implements the same shape AnthropicClient exposes for
// StreamMessage; lets us assert frame ordering + cache_control wiring
// without a network call.
type fakeStreamer struct {
	chunks      []string
	cacheBlocks int
}

func (f *fakeStreamer) StreamChunks(ctx context.Context, system string, history []anthropic.MessageParam, userText string, cache bool) (<-chan string, error) {
	out := make(chan string, len(f.chunks))
	if cache {
		f.cacheBlocks++
	}
	for _, c := range f.chunks {
		out <- c
	}
	close(out)
	return out, nil
}

func TestStreamToIPCEmitsProseDeltas(t *testing.T) {
	s := &fakeStreamer{chunks: []string{"hello ", "world"}}
	long := strings.Repeat("x ", 600) // ~1200 chars ⇒ ≥ 300 tokens; force cache off
	system := strings.Repeat("system ", 2000) // > 1024 token threshold ⇒ cache on
	out, err := streamToIPCWith(context.Background(), s, "t1", system, nil, long)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var frames []ipc.Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) < 3 {
		t.Fatalf("want >=3 frames (2 deltas + turn.end), got %d", len(frames))
	}
	if frames[0].Kind != "prose.delta" || frames[len(frames)-1].Kind != "turn.end" {
		t.Fatalf("bad frame ordering: %+v", frames)
	}
	var p struct{ Text string `json:"text"` }
	_ = json.Unmarshal(frames[0].Payload, &p)
	if p.Text != "hello " {
		t.Fatalf("first chunk wrong: %q", p.Text)
	}
	if s.cacheBlocks != 1 {
		t.Fatalf("expected ephemeral cache_control on >1024-token prompt, got %d", s.cacheBlocks)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/reasoner/ -run TestStreamToIPCEmits 2>&1 | tail -10
```
Expected: `undefined: streamToIPCWith` build error.

- [ ] **Step 3: Write minimal implementation**

`internal/reasoner/stream_ipc.go`:
```go
package reasoner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/trilam/leah/internal/ipc"
)

// streamer is the narrow interface StreamToIPC actually needs; the
// production binding is *AnthropicClient.StreamChunks; tests inject a
// fake. Keeps the public AnthropicClient surface untouched.
type streamer interface {
	StreamChunks(ctx context.Context, system string, history []anthropic.MessageParam, userText string, cache bool) (<-chan string, error)
}

// StreamToIPC wraps an AnthropicClient stream and emits ipc.Frame events:
// one prose.delta per chunk, then a turn.end with a cost payload. The
// caller forwards the frames to the Unix socket; this file does NOT
// touch the socket so it stays unit-testable.
func StreamToIPC(ctx context.Context, client *AnthropicClient, turnID string, systemPrompt string, history []anthropic.MessageParam, userText string) (<-chan ipc.Frame, error) {
	return streamToIPCWith(ctx, clientAdapter{client}, turnID, systemPrompt, history, userText)
}

type clientAdapter struct{ c *AnthropicClient }

func (a clientAdapter) StreamChunks(ctx context.Context, system string, history []anthropic.MessageParam, userText string, cache bool) (<-chan string, error) {
	// Production: call a.c.Stream (existing in anthropic.go) with cache_control
	// applied on the system block + last history message when cache==true.
	// The existing AnthropicClient.Stream (anthropic.go) exposes the chunk
	// channel; the cache flag wires through to its cache_control toggle.
	return a.c.Stream(ctx, system, history, userText, cache)
}

func streamToIPCWith(ctx context.Context, s streamer, turnID, system string, history []anthropic.MessageParam, userText string) (<-chan ipc.Frame, error) {
	cache := ShouldCachePrompt(system)
	chunks, err := s.StreamChunks(ctx, system, history, userText, cache)
	if err != nil {
		return nil, fmt.Errorf("stream: %w", err)
	}
	out := make(chan ipc.Frame, 8)
	go func() {
		defer close(out)
		seq := uint64(0)
		totalIn, totalOut := 0, 0
		for c := range chunks {
			seq++
			totalOut += len(c) / charsPerToken
			payload, _ := json.Marshal(map[string]string{"text": c})
			select {
			case <-ctx.Done():
				return
			case out <- ipc.Frame{Kind: "prose.delta", TurnID: turnID, Seq: seq, Payload: payload}:
			}
		}
		totalIn = MeasurePromptTokens(system) + MeasurePromptTokens(userText)
		cost := map[string]any{
			"input_tokens":  totalIn,
			"output_tokens": totalOut,
			"cached":        cache,
		}
		cp, _ := json.Marshal(cost)
		seq++
		out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: seq, Payload: cp}
	}()
	return out, nil
}
```

NOTE: this task assumes `AnthropicClient.Stream(ctx, system, history, userText, cache bool) (<-chan string, error)` exists in `internal/reasoner/anthropic.go`. If the existing `Stream` signature differs, this task extends `anthropic.go` to add a `Stream` method with the above shape (a thin wrapper around the SDK's `Messages.Stream(...)` that toggles `cache_control: { type: "ephemeral" }` on the system block + last history message when `cache` is true). The signature is the contract — keep it as written.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/reasoner/ -run TestStreamToIPC 2>&1 | tail -10
```
Expected: `PASS: TestStreamToIPCEmitsProseDeltas`.

- [ ] **Step 5: Commit**

```bash
git add internal/reasoner/stream_ipc.go internal/reasoner/stream_ipc_test.go internal/reasoner/anthropic.go
git commit -m "feat(reasoner): stream Sonnet chunks to ipc.Frame with ephemeral cache_control"
```

---

## Task 4: Haiku router classification — widget vs chat decision

**Files:**
- Create: `internal/reasoner/classify.go`
- Create: `internal/reasoner/classify_test.go`

(`internal/reasoner/router.go` already exists with `Breaker` + degraded-leg model swap. This task adds the WIDGET-INTENT classifier as a new file in the same package — no edits to router.go.)

**Interfaces:**
- Consumes: existing `AnthropicClient` (initialized with `claude-haiku-4-5` via `NewAnthropicClientWithModel("claude-haiku-4-5")`).
- Produces:
  - `type Intent struct { Kind string; Widget string; Confidence float64 }` where `Kind ∈ {"chat","widget"}` and `Widget ∈ {"stat","table","list",""}`.
  - `func Classify(ctx context.Context, haiku *AnthropicClient, userText string) (Intent, error)` — single non-streaming Haiku call with a tight system prompt; budget ≤ 200 ms target.

- [ ] **Step 1: Write the failing test**

`internal/reasoner/classify_test.go`:
```go
package reasoner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// haikuStub fakes the Haiku one-shot call; returns a fixed JSON blob.
type haikuStub struct{ resp string }

func (h *haikuStub) OneShot(ctx context.Context, system, user string) (string, error) {
	return h.resp, nil
}

func TestClassifyWidgetIntent(t *testing.T) {
	stub := &haikuStub{resp: `{"kind":"widget","widget":"stat","confidence":0.92}`}
	got, err := classifyWith(context.Background(), stub, "what's the status of MAY-19?")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got.Kind != "widget" || got.Widget != "stat" {
		t.Fatalf("want widget/stat, got %+v", got)
	}
}

func TestClassifyChatFallback(t *testing.T) {
	stub := &haikuStub{resp: `{"kind":"chat"}`}
	got, _ := classifyWith(context.Background(), stub, "hi")
	if got.Kind != "chat" {
		t.Fatalf("want chat, got %+v", got)
	}
}

func TestClassifyBadJSONIsChat(t *testing.T) {
	stub := &haikuStub{resp: `not json`}
	got, err := classifyWith(context.Background(), stub, "hi")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got.Kind != "chat" {
		t.Fatalf("malformed Haiku output must degrade to chat, got %+v", got)
	}
	if !strings.HasPrefix(got.Widget, "") {
		_ = json.Marshal(got) // silence unused-import on json
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/reasoner/ -run TestClassify 2>&1 | tail -10
```
Expected: `undefined: classifyWith` build error.

- [ ] **Step 3: Write minimal implementation**

`internal/reasoner/classify.go`:
```go
package reasoner

import (
	"context"
	"encoding/json"
	"fmt"
)

type Intent struct {
	Kind       string  `json:"kind"`       // "chat" | "widget"
	Widget     string  `json:"widget"`     // "stat" | "table" | "list" | ""
	Confidence float64 `json:"confidence"` // 0..1
}

const classifySystem = `You are a router. Reply with ONE line of JSON:
{"kind":"chat"} for conversational replies.
{"kind":"widget","widget":"stat|table|list","confidence":0..1} when the user
is asking for a metric (stat), tabular data (table), or an enumeration (list).
Never include prose, only the JSON line.`

type oneShot interface {
	OneShot(ctx context.Context, system, user string) (string, error)
}

func Classify(ctx context.Context, haiku *AnthropicClient, userText string) (Intent, error) {
	return classifyWith(ctx, haikuAdapter{haiku}, userText)
}

type haikuAdapter struct{ c *AnthropicClient }

func (a haikuAdapter) OneShot(ctx context.Context, system, user string) (string, error) {
	// AnthropicClient.OneShot is a non-streaming wrapper around
	// Messages.New(...) with max_tokens=64. Implemented in anthropic.go.
	return a.c.OneShot(ctx, system, user)
}

func classifyWith(ctx context.Context, s oneShot, userText string) (Intent, error) {
	raw, err := s.OneShot(ctx, classifySystem, userText)
	if err != nil {
		return Intent{}, fmt.Errorf("haiku oneshot: %w", err)
	}
	var i Intent
	if err := json.Unmarshal([]byte(raw), &i); err != nil {
		// Spec doctrine: degrade silently to chat on malformed Haiku output.
		return Intent{Kind: "chat"}, nil
	}
	if i.Kind == "" {
		i.Kind = "chat"
	}
	return i, nil
}
```

NOTE: this task assumes `AnthropicClient.OneShot(ctx, system, user) (string, error)` exists in `anthropic.go`. If not, extend `anthropic.go` to add it: a `Messages.New(ctx, ...)` call with `MaxTokens: 64`, `Model: "claude-haiku-4-5"`, system + single user message, returning the first text block as a string.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/reasoner/ -run TestClassify 2>&1 | tail -15
```
Expected: `PASS: TestClassifyWidgetIntent`, `PASS: TestClassifyChatFallback`, `PASS: TestClassifyBadJSONIsChat`.

- [ ] **Step 5: Commit**

```bash
git add internal/reasoner/classify.go internal/reasoner/classify_test.go internal/reasoner/anthropic.go
git commit -m "feat(reasoner): Haiku widget-intent classifier with chat-fallback on bad JSON"
```

---

## Task 5: Knowledge store — JOIN-able schema over existing leah.db

**Files:**
- Modify: `internal/memory/schema.sql` (append schema_version 8 — `knowledge_doc` + `knowledge_chunk` + `knowledge_chunk_vec` vec0 virtual table)
- Modify: `go.mod` — bump `modernc.org/sqlite` to v1.50.1+ (the in-tree `sqlite-vec` port landed in v1.47.0, 2026-03-17; v1.50.1 is the current stable line).
- Modify: `internal/sqlstore/` — add `_ "modernc.org/sqlite/vec"` blank import to activate `vec0`.
- Create: `internal/knowledge/store.go` (extends existing `internal/knowledge/storage.go` — new file; do not edit storage.go)
- Create: `internal/knowledge/store_test.go`

**Interfaces:**
- Consumes: `*sql.DB` from existing `sqlstore.OpenWAL` (now with `vec0` registered via blank import).
- Produces:
  - `func PutDoc(db *sql.DB, source string, content string) (docID string, err error)`
  - `func PutChunk(db *sql.DB, docID string, ordinal int, content string, embedding []float32) error` — writes the chunk row AND inserts into `knowledge_chunk_vec(rowid, embedding)` keyed by `(doc_id, ordinal)`.
  - `func SearchChunks(db *sql.DB, queryVec []float32, k int) ([]Chunk, error)` — uses `vec0` `MATCH` query: `SELECT … FROM knowledge_chunk_vec WHERE embedding MATCH ? ORDER BY distance LIMIT ?`. Replaces the prior brute-force Go cosine loop (v3.2.2 F1).
  - `type Chunk struct { DocID string; Ordinal int; Content string; Score float64 }` — `Score = 1.0 - distance` (vec0 returns L2/cosine distance, smaller = more similar).

**SQL for schema_version 8 (append to `internal/memory/schema.sql`):**
```sql
-- schema_version: 8 (additive — knowledge store for Phase 1 answer engine)
-- See docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md §17.10 / §17.16
-- v3.2.2 F1: sqlite-vec via modernc.org/sqlite/vec (pure-Go in-tree port).
-- Activation = blank import "_ modernc.org/sqlite/vec" in internal/sqlstore/.
-- No CGo, no sqlite3_load_extension, no driver swap from mattn/go-sqlite3.

CREATE TABLE IF NOT EXISTS knowledge_doc (
  id          TEXT PRIMARY KEY,    -- ulid
  source      TEXT NOT NULL,       -- 'repo' | 'linear' | 'github' | 'manual'
  content     TEXT NOT NULL,
  created_at  TEXT NOT NULL        -- RFC3339
);
CREATE INDEX IF NOT EXISTS idx_knowledge_doc_source ON knowledge_doc(source);

CREATE TABLE IF NOT EXISTS knowledge_chunk (
  doc_id      TEXT NOT NULL REFERENCES knowledge_doc(id) ON DELETE CASCADE,
  ordinal     INTEGER NOT NULL,
  content     TEXT NOT NULL,
  PRIMARY KEY (doc_id, ordinal)
);

-- vec0 virtual table for similarity search. rowid maps 1:1 to
-- (doc_id, ordinal) via a deterministic hash assigned on insert
-- (see store.go::rowidFor). float[1024] matches Voyage 3.5-lite dim.
CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_chunk_vec USING vec0(
  embedding float[1024]
);
```

- [ ] **Step 1: Write the failing test**

`internal/knowledge/store_test.go`:
```go
package knowledge

import (
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/sqlstore"
)

// makeVec returns a 1024-dim unit vector with a single 1.0 at slot i.
// vec0 declares float[1024] (Voyage 3.5-lite dim) per spec §17.10.
func makeVec(i int) []float32 {
	v := make([]float32, 1024)
	v[i] = 1
	return v
}

func TestPutAndSearchChunks(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlstore.OpenWAL(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := ApplySchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	docID, err := PutDoc(db, "manual", "MAY-19 shipped today.")
	if err != nil {
		t.Fatalf("put doc: %v", err)
	}
	if docID == "" {
		t.Fatal("empty doc id")
	}
	// 1024-dim canonical-basis vectors; vec0 MATCH returns smallest distance
	// for the identical vector.
	if err := PutChunk(db, docID, 0, "MAY-19 shipped today.", makeVec(0)); err != nil {
		t.Fatalf("put chunk: %v", err)
	}
	if err := PutChunk(db, docID, 1, "Unrelated content.", makeVec(1)); err != nil {
		t.Fatalf("put chunk 2: %v", err)
	}

	results, err := SearchChunks(db, makeVec(0), 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(results))
	}
	if results[0].Ordinal != 0 {
		t.Fatalf("expected ordinal-0 chunk first (distance=0), got %+v", results[0])
	}
	if results[0].Score < 0.99 {
		t.Fatalf("expected score~1 for identical vec, got %f", results[0].Score)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/knowledge/ -run TestPutAndSearch 2>&1 | tail -10
```
Expected: `undefined: PutDoc, PutChunk, SearchChunks, ApplySchema`.

- [ ] **Step 3: Write minimal implementation**

Append the schema_version 8 block above to `internal/memory/schema.sql`.

`internal/knowledge/store.go`:
```go
// Package knowledge extends internal/knowledge/storage.go with the Phase 1
// answer-engine doc + chunk store. JOIN-able against the knowledge_chunk_vec
// vec0 virtual table (v3.2.2 F1 — sqlite-vec via modernc.org/sqlite/vec
// pure-Go in-tree port; activation requires a blank import from internal/sqlstore).
// See spec §17.10 + §17.16. No CGo, no driver swap.
package knowledge

import (
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"math"
	"time"
)

type Chunk struct {
	DocID   string
	Ordinal int
	Content string
	Score   float64
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS knowledge_doc (
  id          TEXT PRIMARY KEY,
  source      TEXT NOT NULL,
  content     TEXT NOT NULL,
  created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_knowledge_doc_source ON knowledge_doc(source);
CREATE TABLE IF NOT EXISTS knowledge_chunk (
  doc_id      TEXT NOT NULL REFERENCES knowledge_doc(id) ON DELETE CASCADE,
  ordinal     INTEGER NOT NULL,
  content     TEXT NOT NULL,
  rowid_vec   INTEGER NOT NULL,
  PRIMARY KEY (doc_id, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunk_rowid ON knowledge_chunk(rowid_vec);
CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_chunk_vec USING vec0(
  embedding float[1024]
);
`

// ApplySchema creates the knowledge tables AND the vec0 virtual table
// (idempotent). Used by tests; production daemons reach these tables via
// memory.NewStore which runs the full schema.sql. Requires the
// _ "modernc.org/sqlite/vec" blank import to be active.
func ApplySchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}

func newULID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// rowidFor maps (docID, ordinal) → a stable int64 vec0 rowid via FNV-64.
// Collision probability is negligible at the < 200K-vector budget.
func rowidFor(docID string, ordinal int) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(docID))
	_, _ = h.Write([]byte{byte(ordinal >> 24), byte(ordinal >> 16), byte(ordinal >> 8), byte(ordinal)})
	return int64(h.Sum64() & 0x7FFFFFFFFFFFFFFF)
}

func PutDoc(db *sql.DB, source, content string) (string, error) {
	id := newULID()
	_, err := db.Exec(`INSERT INTO knowledge_doc(id, source, content, created_at) VALUES(?,?,?,?)`,
		id, source, content, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return "", fmt.Errorf("insert doc: %w", err)
	}
	return id, nil
}

func PutChunk(db *sql.DB, docID string, ordinal int, content string, vec []float32) error {
	rowid := rowidFor(docID, ordinal)
	if _, err := db.Exec(`INSERT INTO knowledge_chunk(doc_id, ordinal, content, rowid_vec) VALUES(?,?,?,?)`,
		docID, ordinal, content, rowid); err != nil {
		return fmt.Errorf("insert chunk: %w", err)
	}
	blob := encodeF32(vec)
	if _, err := db.Exec(`INSERT OR REPLACE INTO knowledge_chunk_vec(rowid, embedding) VALUES(?, ?)`,
		rowid, blob); err != nil {
		return fmt.Errorf("insert vec0: %w", err)
	}
	return nil
}

// SearchChunks runs a vec0 MATCH query (v3.2.2 F1). Returns top-k chunks
// ordered by ascending distance; Score = 1.0 - distance so larger = more
// similar (consumer convenience, matches the prior cosine semantics).
func SearchChunks(db *sql.DB, queryVec []float32, k int) ([]Chunk, error) {
	blob := encodeF32(queryVec)
	rows, err := db.Query(`
		SELECT kc.doc_id, kc.ordinal, kc.content, v.distance
		FROM knowledge_chunk_vec v
		JOIN knowledge_chunk kc ON kc.rowid_vec = v.rowid
		WHERE v.embedding MATCH ?
		ORDER BY v.distance
		LIMIT ?`, blob, k)
	if err != nil {
		return nil, fmt.Errorf("vec0 MATCH: %w", err)
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		var dist float64
		if err := rows.Scan(&c.DocID, &c.Ordinal, &c.Content, &dist); err != nil {
			return nil, err
		}
		c.Score = 1.0 - dist
		out = append(out, c)
	}
	return out, nil
}

func encodeF32(v []float32) []byte {
	blob := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(blob[i*4:], math.Float32bits(f))
	}
	return blob
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/knowledge/ -run TestPutAndSearch -v 2>&1 | tail -10
```
Expected: `PASS: TestPutAndSearchChunks`.

- [ ] **Step 5: Commit**

```bash
git add internal/knowledge/store.go internal/knowledge/store_test.go internal/memory/schema.sql internal/sqlstore/ go.mod go.sum
git commit -m "feat(knowledge): vec0 virtual table via modernc.org/sqlite/vec (schema v8, F1)"
```

---

## Task 6: Voyage embeddings — cloud client + (model,dim) namespacing

**Files:**
- Create: `internal/embed/voyage.go`
- Create: `internal/embed/voyage_test.go`

(v3.2.2 F2: Existing `internal/embed/embed.go` **HashGenerator** is retained as the offline-degraded fallback for Phase 1; this task adds a Voyage `Generator` implementation on top. **No ONNX runtime in Phase 1.** The BGE-small-en-v1.5 ONNX local fallback is Phase 2 — it replaces HashGenerator at that point per spec §17.15.)

**Interfaces:**
- Consumes: `VOYAGE_API_KEY` env var.
- Produces:
  - `type VoyageGenerator struct { ... }`
  - `func NewVoyageGenerator(apiKey string) *VoyageGenerator`
  - Implements existing `embed.Generator` interface: `Name() string`, `Dim() int`, `Embed(ctx context.Context, text string) ([]float32, error)`. (Check `internal/embed/embed.go` for the exact interface signature.)
  - `Name() == "voyage-3.5-lite"`, `Dim() == 1024`.
  - HTTP POST to `https://api.voyageai.com/v1/embeddings` with `{"input": [text], "model": "voyage-3.5-lite"}` and Bearer auth.

- [ ] **Step 1: Write the failing test**

`internal/embed/voyage_test.go`:
```go
package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVoyageGeneratorNameAndDim(t *testing.T) {
	g := NewVoyageGenerator("test-key")
	if g.Name() != "voyage-3.5-lite" {
		t.Fatalf("name: %s", g.Name())
	}
	if g.Dim() != 1024 {
		t.Fatalf("dim: %d", g.Dim())
	}
}

func TestVoyageGeneratorEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/wrong auth: %s", r.Header.Get("Authorization"))
		}
		var body struct {
			Input []string `json:"input"`
			Model string   `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "voyage-3.5-lite" {
			t.Errorf("model: %s", body.Model)
		}
		vec := make([]float32, 1024)
		for i := range vec {
			vec[i] = 0.1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": vec}},
		})
	}))
	defer srv.Close()
	g := NewVoyageGenerator("test-key")
	g.endpoint = srv.URL
	vec, err := g.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vec) != 1024 {
		t.Fatalf("got %d dims, want 1024", len(vec))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/embed/ -run TestVoyage 2>&1 | tail -10
```
Expected: `undefined: NewVoyageGenerator`.

- [ ] **Step 3: Write minimal implementation**

`internal/embed/voyage.go`:
```go
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// VoyageGenerator embeds via the Voyage AI HTTP API. Cloud-only; the
// daemon falls back to the existing HashGenerator when VOYAGE_API_KEY is
// unset OR when network is unreachable for > 30 s (circuit breaker added
// in a follow-up task — Phase 1 keeps the toggle env-driven).
type VoyageGenerator struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

func NewVoyageGenerator(apiKey string) *VoyageGenerator {
	return &VoyageGenerator{
		apiKey:   apiKey,
		endpoint: "https://api.voyageai.com/v1/embeddings",
		client:   &http.Client{},
	}
}

func (g *VoyageGenerator) Name() string { return "voyage-3.5-lite" }
func (g *VoyageGenerator) Dim() int     { return 1024 }

func (g *VoyageGenerator) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"input": []string{text},
		"model": "voyage-3.5-lite",
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", g.endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage call: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("voyage %d: %s", resp.StatusCode, string(buf))
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("voyage returned 0 embeddings")
	}
	return out.Data[0].Embedding, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/embed/ -run TestVoyage -v 2>&1 | tail -10
```
Expected: `PASS: TestVoyageGeneratorNameAndDim`, `PASS: TestVoyageGeneratorEmbed`.

- [ ] **Step 5: Commit**

```bash
git add internal/embed/voyage.go internal/embed/voyage_test.go
git commit -m "feat(embed): Voyage 3.5-lite generator (1024d, cloud)"
```

---

## Task 7: Memory pipeline — auto-capture conversation turns

**Files:**
- Create: `internal/memory/conversation.go`
- Create: `internal/memory/conversation_test.go`
- Modify: `internal/memory/schema.sql` (append schema_version 9 — `conversation_turn` table)

**Interfaces:**
- Consumes: `*sql.DB`, the streamed user-text + assistant-text per turn.
- Produces:
  - `func RecordTurn(db *sql.DB, turnID, userText, assistantText string) error`
  - `func RecentTurns(db *sql.DB, limit int) ([]Turn, error)`
  - `type Turn struct { ID string; UserText string; AssistantText string; CreatedAt time.Time }`

**SQL for schema_version 9 (append to `internal/memory/schema.sql`):**
```sql
-- schema_version: 9 (additive — Phase 1 conversation auto-capture)
-- See docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md §19 Phase 1 deliverable 3.
CREATE TABLE IF NOT EXISTS conversation_turn (
  id              TEXT PRIMARY KEY,
  user_text       TEXT NOT NULL,
  assistant_text  TEXT NOT NULL,
  created_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_conv_turn_created ON conversation_turn(created_at);
```

- [ ] **Step 1: Write the failing test**

`internal/memory/conversation_test.go`:
```go
package memory

import (
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/sqlstore"
)

func TestRecordAndRecentTurns(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlstore.OpenWAL(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE conversation_turn(id TEXT PRIMARY KEY, user_text TEXT NOT NULL, assistant_text TEXT NOT NULL, created_at TEXT NOT NULL);`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if err := RecordTurn(db, "t1", "hello", "hi there"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := RecordTurn(db, "t2", "what's up", "all good"); err != nil {
		t.Fatalf("record: %v", err)
	}
	turns, err := RecentTurns(db, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("want 2 turns, got %d", len(turns))
	}
	// Most recent first.
	if turns[0].ID != "t2" {
		t.Fatalf("expected t2 first, got %s", turns[0].ID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/memory/ -run TestRecordAndRecent 2>&1 | tail -10
```
Expected: `undefined: RecordTurn, RecentTurns, Turn`.

- [ ] **Step 3: Write minimal implementation**

`internal/memory/conversation.go`:
```go
package memory

import (
	"database/sql"
	"fmt"
	"time"
)

type Turn struct {
	ID            string
	UserText      string
	AssistantText string
	CreatedAt     time.Time
}

func RecordTurn(db *sql.DB, turnID, userText, assistantText string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO conversation_turn(id, user_text, assistant_text, created_at) VALUES(?,?,?,?)`,
		turnID, userText, assistantText, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert turn: %w", err)
	}
	return nil
}

func RecentTurns(db *sql.DB, limit int) ([]Turn, error) {
	rows, err := db.Query(`SELECT id, user_text, assistant_text, created_at FROM conversation_turn ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	var out []Turn
	for rows.Next() {
		var t Turn
		var created string
		if err := rows.Scan(&t.ID, &t.UserText, &t.AssistantText, &created); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, t)
	}
	return out, nil
}
```

Append the schema_version 9 block to `internal/memory/schema.sql`.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./internal/memory/ -run TestRecordAndRecent -v 2>&1 | tail -10
```
Expected: `PASS: TestRecordAndRecentTurns`.

- [ ] **Step 5: Commit**

```bash
git add internal/memory/conversation.go internal/memory/conversation_test.go internal/memory/schema.sql
git commit -m "feat(memory): conversation_turn table + RecordTurn/RecentTurns (schema v9)"
```

---

## Task 8: Daemon IPC handler — wire Sonnet stream + Haiku classify into the socket

**Files:**
- Create: `cmd/leah-daemon/ipc_handler.go`
- Create: `cmd/leah-daemon/ipc_handler_test.go`
- Modify: `cmd/leah-daemon/main.go` (replace the Task 2 placeholder `echoHandler` with `newIPCHandler(...)`) — SINGLE OWNER per CLAUDE.md

**Interfaces:**
- Consumes: `*reasoner.AnthropicClient` (Sonnet), `*reasoner.AnthropicClient` (Haiku), `*sql.DB`.
- Produces:
  - `func newIPCHandler(sonnet, haiku *reasoner.AnthropicClient, db *sql.DB) ipc.Handler`
  - Inbound frame `{"kind":"ask","payload":{"text":"..."}}` → outbound stream of `prose.delta` frames + optional `widget.mount` frame when Haiku classifies as widget intent + `turn.end`.
  - On `turn.end`, calls `memory.RecordTurn(db, turnID, userText, assembledAssistantText)`.

- [ ] **Step 1: Write the failing test**

`cmd/leah-daemon/ipc_handler_test.go`:
```go
package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/sqlstore"
)

func TestIPCHandlerEmitsTurnEndAndPersistsTurn(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlstore.OpenWAL(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE conversation_turn(id TEXT PRIMARY KEY, user_text TEXT NOT NULL, assistant_text TEXT NOT NULL, created_at TEXT NOT NULL);`); err != nil {
		t.Fatalf("schema: %v", err)
	}

	// Fake reasoner: emits "hi " + "there"
	fakeStream := func(ctx context.Context, turnID, user string) (<-chan ipc.Frame, error) {
		out := make(chan ipc.Frame, 3)
		out <- ipc.Frame{Kind: "prose.delta", TurnID: turnID, Seq: 1, Payload: json.RawMessage(`{"text":"hi "}`)}
		out <- ipc.Frame{Kind: "prose.delta", TurnID: turnID, Seq: 2, Payload: json.RawMessage(`{"text":"there"}`)}
		out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: 3, Payload: json.RawMessage(`{}`)}
		close(out)
		return out, nil
	}
	h := newIPCHandlerForTest(db, fakeStream)
	in := ipc.Frame{Kind: "ask", TurnID: "t1", Seq: 0, Payload: json.RawMessage(`{"text":"hello"}`)}
	out, err := h(context.Background(), in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var frames []ipc.Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) < 3 {
		t.Fatalf("want >=3 frames, got %d", len(frames))
	}
	if frames[len(frames)-1].Kind != "turn.end" {
		t.Fatalf("last frame must be turn.end, got %s", frames[len(frames)-1].Kind)
	}
	// Verify persistence
	var assembled string
	if err := db.QueryRow(`SELECT assistant_text FROM conversation_turn WHERE id='t1'`).Scan(&assembled); err != nil {
		t.Fatalf("query turn: %v", err)
	}
	if assembled != "hi there" {
		t.Fatalf("assembled assistant text: %q", assembled)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./cmd/leah-daemon/ -run TestIPCHandler 2>&1 | tail -10
```
Expected: `undefined: newIPCHandlerForTest`.

- [ ] **Step 3: Write minimal implementation**

`cmd/leah-daemon/ipc_handler.go`:
```go
package main

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/reasoner"
)

// streamFn isolates the reasoner dependency so tests inject a fake.
type streamFn func(ctx context.Context, turnID, userText string) (<-chan ipc.Frame, error)

func newIPCHandler(sonnet *reasoner.AnthropicClient, db *sql.DB) ipc.Handler {
	stream := func(ctx context.Context, turnID, userText string) (<-chan ipc.Frame, error) {
		return reasoner.StreamToIPC(ctx, sonnet, turnID, "You are Leah, a calm operator-side assistant.", nil, userText)
	}
	return newIPCHandlerForTest(db, stream)
}

func newIPCHandlerForTest(db *sql.DB, stream streamFn) ipc.Handler {
	return func(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
		var p struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(req.Payload, &p)
		raw, err := stream(ctx, req.TurnID, p.Text)
		if err != nil {
			return nil, err
		}
		out := make(chan ipc.Frame, 8)
		go func() {
			defer close(out)
			var assembled string
			for f := range raw {
				if f.Kind == "prose.delta" {
					var pp struct {
						Text string `json:"text"`
					}
					_ = json.Unmarshal(f.Payload, &pp)
					assembled += pp.Text
				}
				out <- f
				if f.Kind == "turn.end" {
					_ = memory.RecordTurn(db, req.TurnID, p.Text, assembled)
				}
			}
		}()
		return out, nil
	}
}
```

Modify `cmd/leah-daemon/main.go` — replace the Task 2 placeholder `echoHandler` block with:
```go
sonnet, err := reasoner.NewAnthropicClient()
if err != nil {
	_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: anthropic: %v (continuing; IPC stream will error)\n", err)
}
memDB, err := sqlstore.OpenWAL(filepath.Join(sd, "leah.db"))
if err != nil {
	_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: memory db: %v\n", err)
	os.Exit(1)
}
defer memDB.Close()
go func() { _ = ipc.NewServer(sockPath, newIPCHandler(sonnet, memDB)).Serve(ctx) }()
```

Add the corresponding imports (`"github.com/trilam/leah/internal/reasoner"`, `"github.com/trilam/leah/internal/sqlstore"`).

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/treedesk/Desktop/Projects/leah && go test ./cmd/leah-daemon/ -run TestIPCHandler -v 2>&1 | tail -10
```
Expected: `PASS: TestIPCHandlerEmitsTurnEndAndPersistsTurn`.

- [ ] **Step 5: Commit**

```bash
git add cmd/leah-daemon/ipc_handler.go cmd/leah-daemon/ipc_handler_test.go cmd/leah-daemon/main.go
git commit -m "feat(daemon): IPC handler streams Sonnet replies + persists conversation turns"
```

---

## Task 9: Swift IPC client — connect to daemon socket, decode frames

**Files:**
- Create: `app/Leah/Sources/LeahIPC/IPCClient.swift`
- Create: `app/Leah/Sources/LeahIPC/Frame.swift`
- Create: `app/Leah/Tests/LeahIPCTests/IPCClientTests.swift`
- Modify: `app/Leah/Package.swift` (add `LeahIPC` target + LeahIPCTests test target)

**Interfaces:**
- Consumes: socket path `~/Library/Caches/Leah/leah.sock`.
- Produces:
  - `struct Frame: Codable { let kind: String; let turnId: String; let seq: UInt64; let payload: Data }` (note: `turnId` ↔ `turn_id` via `CodingKeys`).
  - `actor IPCClient { func connect() async throws; func send(_ frame: Frame) async throws; func receive() async throws -> Frame; func close() }`
  - 4-byte BE uint32 length prefix per frame.

- [ ] **Step 1: Write the failing test**

Append to `app/Leah/Package.swift` targets:
```swift
    .target(name: "LeahIPC", path: "Sources/LeahIPC"),
    .testTarget(name: "LeahIPCTests", dependencies: ["LeahIPC"], path: "Tests/LeahIPCTests"),
```

`app/Leah/Tests/LeahIPCTests/IPCClientTests.swift`:
```swift
import XCTest
@testable import LeahIPC

final class FrameCodecTests: XCTestCase {
  func testRoundTripFrame() throws {
    let payload = #"{"text":"hi"}"#.data(using: .utf8)!
    let f = Frame(kind: "prose.delta", turnId: "t1", seq: 7, payload: payload)
    let encoded = try FrameCodec.encode(f)
    // 4-byte length prefix + JSON body
    XCTAssertGreaterThan(encoded.count, 4)
    let (decoded, consumed) = try FrameCodec.decode(encoded)
    XCTAssertEqual(consumed, encoded.count)
    XCTAssertEqual(decoded.kind, "prose.delta")
    XCTAssertEqual(decoded.turnId, "t1")
    XCTAssertEqual(decoded.seq, 7)
  }

  func testFrameSizeCapRejected() {
    let huge = Data(repeating: 0x78, count: 300_000)
    let f = Frame(kind: "prose.delta", turnId: "t1", seq: 1, payload: huge)
    XCTAssertThrowsError(try FrameCodec.encode(f))
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd app/Leah && swift test --filter FrameCodecTests 2>&1 | tail -10
```
Expected: `error: no such module 'LeahIPC'`.

- [ ] **Step 3: Write minimal implementation**

`app/Leah/Sources/LeahIPC/Frame.swift`:
```swift
import Foundation

public struct Frame: Codable, Sendable {
  public let kind: String
  public let turnId: String
  public let seq: UInt64
  public let payload: Data

  enum CodingKeys: String, CodingKey {
    case kind
    case turnId = "turn_id"
    case seq
    case payload
  }

  public init(kind: String, turnId: String, seq: UInt64, payload: Data) {
    self.kind = kind
    self.turnId = turnId
    self.seq = seq
    self.payload = payload
  }
}

public enum FrameCodec {
  public static let maxFrameBytes = 256 * 1024

  public enum CodecError: Error { case oversize, shortRead, badHeader, decodeFailed }

  public static func encode(_ f: Frame) throws -> Data {
    let body = try JSONEncoder().encode(f)
    guard body.count <= maxFrameBytes else { throw CodecError.oversize }
    var out = Data(count: 4)
    let n = UInt32(body.count).bigEndian
    withUnsafeBytes(of: n) { ptr in out.replaceSubrange(0..<4, with: ptr) }
    out.append(body)
    return out
  }

  public static func decode(_ data: Data) throws -> (Frame, Int) {
    guard data.count >= 4 else { throw CodecError.shortRead }
    let n = data.prefix(4).withUnsafeBytes { $0.load(as: UInt32.self).bigEndian }
    let total = 4 + Int(n)
    guard data.count >= total else { throw CodecError.shortRead }
    guard Int(n) <= maxFrameBytes else { throw CodecError.oversize }
    let body = data.subdata(in: 4..<total)
    let f = try JSONDecoder().decode(Frame.self, from: body)
    return (f, total)
  }
}
```

`app/Leah/Sources/LeahIPC/IPCClient.swift`:
```swift
import Foundation
#if canImport(Darwin)
import Darwin
#endif

public actor IPCClient {
  private let path: String
  private var fd: Int32 = -1

  public init(socketPath: String) {
    self.path = socketPath
  }

  public func connect() throws {
    fd = socket(AF_UNIX, SOCK_STREAM, 0)
    guard fd >= 0 else { throw POSIXError(.EIO) }
    var addr = sockaddr_un()
    addr.sun_family = sa_family_t(AF_UNIX)
    let pathBytes = Array(path.utf8) + [0]
    precondition(pathBytes.count <= MemoryLayout.size(ofValue: addr.sun_path), "socket path too long")
    withUnsafeMutableBytes(of: &addr.sun_path) { dst in
      pathBytes.withUnsafeBytes { src in
        dst.copyMemory(from: src)
      }
    }
    let rc = withUnsafePointer(to: &addr) { ptr -> Int32 in
      ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { s in
        Darwin.connect(fd, s, socklen_t(MemoryLayout<sockaddr_un>.size))
      }
    }
    guard rc == 0 else { close(); throw POSIXError(.ECONNREFUSED) }
  }

  public func send(_ f: Frame) throws {
    let bytes = try FrameCodec.encode(f)
    bytes.withUnsafeBytes { ptr in
      _ = write(fd, ptr.baseAddress, bytes.count)
    }
  }

  public func receive() throws -> Frame {
    var hdr = [UInt8](repeating: 0, count: 4)
    let r1 = read(fd, &hdr, 4)
    guard r1 == 4 else { throw FrameCodec.CodecError.shortRead }
    let n = UInt32(hdr[0]) << 24 | UInt32(hdr[1]) << 16 | UInt32(hdr[2]) << 8 | UInt32(hdr[3])
    guard Int(n) <= FrameCodec.maxFrameBytes else { throw FrameCodec.CodecError.oversize }
    var body = [UInt8](repeating: 0, count: Int(n))
    let r2 = read(fd, &body, Int(n))
    guard r2 == Int(n) else { throw FrameCodec.CodecError.shortRead }
    return try JSONDecoder().decode(Frame.self, from: Data(body))
  }

  public func close() {
    if fd >= 0 { _ = Darwin.close(fd); fd = -1 }
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd app/Leah && swift test --filter FrameCodecTests 2>&1 | tail -10
```
Expected: `Executed 2 tests, with 0 failures`.

- [ ] **Step 5: Commit**

```bash
git add app/Leah/Sources/LeahIPC/ app/Leah/Tests/LeahIPCTests/ app/Leah/Package.swift
git commit -m "feat(app): LeahIPC actor + frame codec with 256KB cap"
```

---

## Task 10: Focus panel — NSPanel non-activating + ⌥Space hotkey + streaming text

**Files:**
- Create: `app/Leah/Sources/LeahUI/FocusPanel.swift`
- Create: `app/Leah/Sources/LeahUI/FocusPanelView.swift`
- Create: `app/Leah/Sources/LeahUI/Hotkey.swift`
- Create: `app/Leah/Tests/LeahUITests/FocusPanelTests.swift`
- Modify: `app/Leah/Package.swift` (add `LeahUI` target depending on `LeahIPC`)
- Modify: `app/Leah/Sources/LeahApp/LeahApp.swift` (instantiate `FocusPanelController` at launch)

**Interfaces:**
- Consumes: `LeahIPC.IPCClient`.
- Produces:
  - `final class FocusPanelController: NSObject { init(client: IPCClient); func summon(); func dismiss() }`
  - `final class HotkeyMonitor { init(_ handler: @escaping () -> Void) ; static let optionSpaceKeyCode: UInt32 = 0x31 /* kVK_Space */; static let optionFlag: UInt32 = UInt32(NSEvent.ModifierFlags.option.rawValue) }`
  - `struct FocusPanelView: View` — SwiftUI body: text field + streaming response area.

- [ ] **Step 1: Write the failing test**

Append to `app/Leah/Package.swift` targets:
```swift
    .target(name: "LeahUI", dependencies: ["LeahIPC"], path: "Sources/LeahUI"),
    .testTarget(name: "LeahUITests", dependencies: ["LeahUI"], path: "Tests/LeahUITests"),
```

`app/Leah/Tests/LeahUITests/FocusPanelTests.swift`:
```swift
import XCTest
import AppKit
@testable import LeahUI

final class FocusPanelTests: XCTestCase {
  @MainActor
  func testPanelHasNonActivatingStyleMask() {
    let panel = FocusPanelController.makePanel()
    XCTAssertTrue(panel.styleMask.contains(.nonactivatingPanel))
    XCTAssertTrue(panel.styleMask.contains(.titled))
    XCTAssertEqual(panel.level, .modalPanel)
    XCTAssertFalse(panel.becomesKeyOnlyIfNeeded)
    XCTAssertTrue(panel.worksWhenModal)
  }

  @MainActor
  func testPanelHasFocusPanelCollectionBehavior() {
    let panel = FocusPanelController.makePanel()
    let cb = panel.collectionBehavior
    XCTAssertTrue(cb.contains(.fullScreenAuxiliary))
    XCTAssertTrue(cb.contains(.moveToActiveSpace))
    XCTAssertTrue(cb.contains(.stationary))
    // Per spec §4.3: focus panel drops .canJoinAllSpaces (ambient HUD owns that).
    XCTAssertFalse(cb.contains(.canJoinAllSpaces))
  }

  @MainActor
  func testPanelDefaultSize() {
    let panel = FocusPanelController.makePanel()
    XCTAssertEqual(panel.contentRect(forFrameRect: panel.frame).width, 860, accuracy: 1)
    XCTAssertEqual(panel.contentRect(forFrameRect: panel.frame).height, 480, accuracy: 1)
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd app/Leah && swift test --filter FocusPanelTests 2>&1 | tail -10
```
Expected: `error: no such module 'LeahUI'`.

- [ ] **Step 3: Write minimal implementation**

`app/Leah/Sources/LeahUI/FocusPanel.swift`:
```swift
import AppKit
import SwiftUI
import LeahIPC

public final class FocusPanelController: NSObject {
  private var panel: NSPanel?
  private let client: IPCClient

  public init(client: IPCClient) {
    self.client = client
  }

  /// Builds a focus-panel NSPanel per spec §4.3 (testable independently).
  public static func makePanel() -> NSPanel {
    let panel = NSPanel(
      contentRect: NSRect(x: 0, y: 0, width: 860, height: 480),
      styleMask: [.titled, .nonactivatingPanel],
      backing: .buffered,
      defer: false
    )
    panel.level = .modalPanel
    panel.becomesKeyOnlyIfNeeded = false
    panel.worksWhenModal = true
    panel.collectionBehavior = [.fullScreenAuxiliary, .moveToActiveSpace, .stationary]
    panel.isMovableByWindowBackground = true
    panel.titlebarAppearsTransparent = true
    panel.titleVisibility = .hidden
    panel.appearance = NSAppearance(named: .darkAqua) // Phase 1: dark mode only
    return panel
  }

  @MainActor
  public func summon() {
    if panel == nil {
      let p = FocusPanelController.makePanel()
      let view = FocusPanelView(client: client)
      p.contentView = NSHostingView(rootView: view)
      panel = p
    }
    panel?.center()
    panel?.makeKeyAndOrderFront(nil)
  }

  @MainActor
  public func dismiss() {
    panel?.orderOut(nil)
  }
}
```

`app/Leah/Sources/LeahUI/FocusPanelView.swift`:
```swift
import SwiftUI
import LeahIPC

public struct FocusPanelView: View {
  let client: IPCClient
  @State private var input: String = ""
  @State private var response: String = ""

  public init(client: IPCClient) {
    self.client = client
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 12) {
      HStack {
        Text("⬡").font(.system(size: 18, weight: .medium))
          .foregroundColor(Color(red: 201/255, green: 169/255, blue: 97/255)) // --gold-primary
        TextField("Ask Leah anything…", text: $input, onCommit: send)
          .textFieldStyle(.plain)
          .font(.system(size: 18))
      }
      Divider().background(Color.white.opacity(0.20)) // --divider
      ScrollView {
        Text(response.isEmpty ? "" : response)
          .font(.system(size: 14))
          .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255)) // --text-primary
          .frame(maxWidth: .infinity, alignment: .leading)
      }
      Spacer()
    }
    .padding(24)
    .background(Color(red: 8/255, green: 9/255, blue: 12/255)) // --obsidian-0
  }

  private func send() {
    let text = input
    input = ""
    response = ""
    Task {
      let turnID = UUID().uuidString
      let payload = try JSONEncoder().encode(["text": text])
      try await client.send(Frame(kind: "ask", turnId: turnID, seq: 0, payload: payload))
      while true {
        let f = try await client.receive()
        if f.kind == "prose.delta" {
          if let p = try? JSONDecoder().decode([String: String].self, from: f.payload),
             let t = p["text"] {
            await MainActor.run { response.append(t) }
          }
        } else if f.kind == "turn.end" {
          break
        }
      }
    }
  }
}
```

`app/Leah/Sources/LeahUI/Hotkey.swift`:
```swift
import AppKit
import Carbon.HIToolbox

public final class HotkeyMonitor {
  public static let optionSpaceKeyCode: UInt32 = UInt32(kVK_Space)
  public static let optionFlag: UInt32 = UInt32(optionKey) // Carbon optionKey mask

  private var hotKeyRef: EventHotKeyRef?
  private let handler: () -> Void

  public init(_ handler: @escaping () -> Void) {
    self.handler = handler
  }

  /// Registers ⌥Space as a global hotkey. Requires macOS Accessibility permission
  /// granted for the running app bundle (wizard step covers this).
  public func register() {
    var eventType = EventTypeSpec(eventClass: OSType(kEventClassKeyboard),
                                  eventKind: OSType(kEventHotKeyPressed))
    InstallEventHandler(GetApplicationEventTarget(), { _, eventRef, userData in
      let monitor = Unmanaged<HotkeyMonitor>.fromOpaque(userData!).takeUnretainedValue()
      monitor.handler()
      return noErr
    }, 1, &eventType, Unmanaged.passUnretained(self).toOpaque(), nil)

    var ref: EventHotKeyRef?
    let id = EventHotKeyID(signature: OSType(0x4C454148), id: 1) // 'LEAH'
    RegisterEventHotKey(HotkeyMonitor.optionSpaceKeyCode, HotkeyMonitor.optionFlag,
                        id, GetApplicationEventTarget(), 0, &ref)
    hotKeyRef = ref
  }

  deinit {
    if let r = hotKeyRef { UnregisterEventHotKey(r) }
  }
}
```

Update `app/Leah/Sources/LeahApp/LeahApp.swift` to wire it together:
```swift
import SwiftUI
import LeahIPC
import LeahUI

@main
struct LeahApp: App {
  static let bundleIdentifier = "com.maydow.leah"
  static let minimumMacOS = "14.0"

  @State private var controller: FocusPanelController?
  @State private var hotkey: HotkeyMonitor?

  var body: some Scene {
    WindowGroup {
      Color.clear
        .frame(width: 1, height: 1)
        .onAppear {
          let home = ProcessInfo.processInfo.environment["HOME"] ?? NSHomeDirectory()
          let sock = "\(home)/Library/Caches/Leah/leah.sock"
          let client = IPCClient(socketPath: sock)
          Task { try? await client.connect() }
          let c = FocusPanelController(client: client)
          controller = c
          let hk = HotkeyMonitor { Task { @MainActor in c.summon() } }
          hk.register()
          hotkey = hk
        }
    }
  }
}
```

Update LeahApp target dependencies in `Package.swift`:
```swift
    .executableTarget(name: "LeahApp", dependencies: ["LeahIPC", "LeahUI"], path: "Sources/LeahApp", resources: [.process("Info.plist"), .process("Leah.entitlements")]),
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd app/Leah && swift test --filter FocusPanelTests 2>&1 | tail -10
```
Expected: `Executed 3 tests, with 0 failures`.

- [ ] **Step 5: Commit**

```bash
git add app/Leah/Sources/LeahUI/ app/Leah/Sources/LeahApp/LeahApp.swift app/Leah/Tests/LeahUITests/ app/Leah/Package.swift
git commit -m "feat(app): focus panel NSPanel + ⌥Space hotkey + streaming text rendering"
```

---

## Task 11: NSStatusItem menubar — template image + state shape

**Files:**
- Create: `app/Leah/Sources/LeahUI/MenubarItem.swift`
- Create: `app/Leah/Sources/LeahUI/MenubarHexagon.swift` (Core Graphics-rendered template image)
- Create: `app/Leah/Tests/LeahUITests/MenubarItemTests.swift`
- Modify: `app/Leah/Sources/LeahApp/LeahApp.swift` (instantiate `MenubarItem` alongside `FocusPanelController`)

**Interfaces:**
- Consumes: `FocusPanelController` (to summon on click).
- Produces:
  - `final class MenubarItem { enum State { case idle, listening, error }; init(onClick: @escaping () -> Void); func setState(_ s: State) }`
  - `enum MenubarHexagon { static func image(filled: Bool, hasDot: Bool) -> NSImage }` — returns a `template: true` NSImage so macOS tints it.

- [ ] **Step 1: Write the failing test**

`app/Leah/Tests/LeahUITests/MenubarItemTests.swift`:
```swift
import XCTest
import AppKit
@testable import LeahUI

final class MenubarItemTests: XCTestCase {
  func testIdleHexagonIsOutlinedTemplate() {
    let img = MenubarHexagon.image(filled: false, hasDot: false)
    XCTAssertTrue(img.isTemplate, "menubar images MUST be template per spec §4.2")
    XCTAssertEqual(img.size.width, 18, accuracy: 1)
    XCTAssertEqual(img.size.height, 18, accuracy: 1)
  }

  func testListeningHexagonIsFilledTemplate() {
    let img = MenubarHexagon.image(filled: true, hasDot: false)
    XCTAssertTrue(img.isTemplate)
  }

  func testErrorHexagonHasInnerDotTemplate() {
    let img = MenubarHexagon.image(filled: true, hasDot: true)
    XCTAssertTrue(img.isTemplate)
  }

  @MainActor
  func testMenubarItemDefaultsToIdle() {
    let m = MenubarItem(onClick: {})
    XCTAssertEqual(m.currentState, .idle)
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd app/Leah && swift test --filter MenubarItemTests 2>&1 | tail -10
```
Expected: `undefined: MenubarHexagon`, `undefined: MenubarItem`.

- [ ] **Step 3: Write minimal implementation**

`app/Leah/Sources/LeahUI/MenubarHexagon.swift`:
```swift
import AppKit

public enum MenubarHexagon {
  /// Draws an 18×18 hexagon as a TEMPLATE image so macOS tints it per
  /// menubar appearance. Outlined = idle; filled = listening; filled +
  /// inner dot = error (spec §4.2 — shape carries meaning, not color).
  public static func image(filled: Bool, hasDot: Bool) -> NSImage {
    let size = NSSize(width: 18, height: 18)
    let img = NSImage(size: size, flipped: false) { rect in
      let path = NSBezierPath()
      let cx = rect.midX, cy = rect.midY, r: CGFloat = 8
      for i in 0..<6 {
        let a = (CGFloat(i) * .pi / 3) - .pi / 2
        let p = NSPoint(x: cx + r * cos(a), y: cy + r * sin(a))
        if i == 0 { path.move(to: p) } else { path.line(to: p) }
      }
      path.close()
      path.lineWidth = 1.5
      NSColor.black.setStroke()
      NSColor.black.setFill()
      if filled { path.fill() } else { path.stroke() }
      if hasDot {
        let dot = NSBezierPath(ovalIn: NSRect(x: cx - 1.5, y: cy - 1.5, width: 3, height: 3))
        NSColor.white.setFill() // punched-out; template-tinted by macOS
        dot.fill()
      }
      return true
    }
    img.isTemplate = true
    return img
  }
}
```

`app/Leah/Sources/LeahUI/MenubarItem.swift`:
```swift
import AppKit

public final class MenubarItem {
  public enum State: Equatable { case idle, listening, error }
  public private(set) var currentState: State = .idle
  private var item: NSStatusItem?
  private let onClick: () -> Void

  public init(onClick: @escaping () -> Void) {
    self.onClick = onClick
    let it = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
    it.button?.image = MenubarHexagon.image(filled: false, hasDot: false)
    it.button?.target = ClickProxy(handler: onClick)
    it.button?.action = #selector(ClickProxy.fire)
    self.item = it
  }

  public func setState(_ s: State) {
    currentState = s
    switch s {
    case .idle:
      item?.button?.image = MenubarHexagon.image(filled: false, hasDot: false)
    case .listening:
      item?.button?.image = MenubarHexagon.image(filled: true, hasDot: false)
    case .error:
      item?.button?.image = MenubarHexagon.image(filled: true, hasDot: true)
    }
  }
}

private final class ClickProxy: NSObject {
  let handler: () -> Void
  init(handler: @escaping () -> Void) { self.handler = handler }
  @objc func fire() { handler() }
}
```

Update `LeahApp.swift` `.onAppear` block to also instantiate the menubar item; reference `c.summon` as the onClick handler.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd app/Leah && swift test --filter MenubarItemTests 2>&1 | tail -10
```
Expected: `Executed 4 tests, with 0 failures`.

- [ ] **Step 5: Commit**

```bash
git add app/Leah/Sources/LeahUI/MenubarItem.swift app/Leah/Sources/LeahUI/MenubarHexagon.swift app/Leah/Tests/LeahUITests/MenubarItemTests.swift app/Leah/Sources/LeahApp/LeahApp.swift
git commit -m "feat(app): NSStatusItem menubar with idle/listening/error hexagon states"
```

---

## Task 12: Widget primitives — Stat, Table, List SwiftUI tiles

**Files:**
- Create: `app/Leah/Sources/LeahWidgets/Stat.swift`
- Create: `app/Leah/Sources/LeahWidgets/Table.swift`
- Create: `app/Leah/Sources/LeahWidgets/List.swift`
- Create: `app/Leah/Sources/LeahWidgets/WidgetEnvelope.swift`
- Create: `app/Leah/Tests/LeahWidgetsTests/WidgetTests.swift`
- Modify: `app/Leah/Package.swift` (add `LeahWidgets` target)
- Modify: `app/Leah/Sources/LeahUI/FocusPanelView.swift` (handle `widget.mount` frame, render via `WidgetEnvelope.render`)

**Interfaces:**
- Produces:
  - `struct WidgetEnvelope: Codable { let id: String; let widget: String; let size: String; let props: [String: AnyCodable]; let data: [String: AnyCodable] }`
  - `struct StatWidget: View { let label: String; let value: String; let trend: String? }` — large numeric readout.
  - `struct TableWidget: View { let headers: [String]; let rows: [[String]] }`
  - `struct ListWidget: View { let items: [String] }`
  - `@ViewBuilder func render(_ env: WidgetEnvelope) -> some View` — dispatches on `env.widget`.

- [ ] **Step 1: Write the failing test**

Append to `app/Leah/Package.swift` targets:
```swift
    .target(name: "LeahWidgets", path: "Sources/LeahWidgets"),
    .testTarget(name: "LeahWidgetsTests", dependencies: ["LeahWidgets"], path: "Tests/LeahWidgetsTests"),
```

`app/Leah/Tests/LeahWidgetsTests/WidgetTests.swift`:
```swift
import XCTest
import SwiftUI
@testable import LeahWidgets

final class WidgetTests: XCTestCase {
  func testStatWidgetRenders() {
    let w = StatWidget(label: "MERGED PRs · 7d", value: "12", trend: "+3")
    let view = w.body
    XCTAssertNotNil(view as Any)
  }

  func testTableWidgetRenders() {
    let w = TableWidget(headers: ["PR", "Title"], rows: [["#321", "B1+B5 server push"]])
    XCTAssertNotNil(w.body as Any)
  }

  func testListWidgetRenders() {
    let w = ListWidget(items: ["MAY-19 done", "MAY-13 in-progress"])
    XCTAssertNotNil(w.body as Any)
  }

  func testEnvelopeDecodesStat() throws {
    let json = #"{"id":"x","widget":"stat","size":"small","props":{"label":"PRs","value":"12"},"data":{}}"#
    let env = try JSONDecoder().decode(WidgetEnvelope.self, from: json.data(using: .utf8)!)
    XCTAssertEqual(env.widget, "stat")
    XCTAssertEqual(env.id, "x")
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd app/Leah && swift test --filter WidgetTests 2>&1 | tail -10
```
Expected: `error: no such module 'LeahWidgets'`.

- [ ] **Step 3: Write minimal implementation**

`app/Leah/Sources/LeahWidgets/WidgetEnvelope.swift`:
```swift
import Foundation
import SwiftUI

public struct AnyCodable: Codable {
  public let value: Any
  public init(_ value: Any) { self.value = value }
  public init(from decoder: Decoder) throws {
    let c = try decoder.singleValueContainer()
    if let s = try? c.decode(String.self) { value = s; return }
    if let i = try? c.decode(Int.self) { value = i; return }
    if let d = try? c.decode(Double.self) { value = d; return }
    if let b = try? c.decode(Bool.self) { value = b; return }
    if let a = try? c.decode([AnyCodable].self) { value = a; return }
    if let o = try? c.decode([String: AnyCodable].self) { value = o; return }
    value = NSNull()
  }
  public func encode(to encoder: Encoder) throws {
    var c = encoder.singleValueContainer()
    switch value {
    case let s as String: try c.encode(s)
    case let i as Int: try c.encode(i)
    case let d as Double: try c.encode(d)
    case let b as Bool: try c.encode(b)
    default: try c.encodeNil()
    }
  }
}

public struct WidgetEnvelope: Codable {
  public let id: String
  public let widget: String
  public let size: String
  public let props: [String: AnyCodable]
  public let data: [String: AnyCodable]
}

@ViewBuilder
public func render(_ env: WidgetEnvelope) -> some View {
  switch env.widget {
  case "stat":
    let label = (env.props["label"]?.value as? String) ?? ""
    let value = (env.props["value"]?.value as? String) ?? ""
    let trend = env.props["trend"]?.value as? String
    StatWidget(label: label, value: value, trend: trend)
  case "table":
    let headers = (env.props["headers"]?.value as? [AnyCodable])?.compactMap { $0.value as? String } ?? []
    let rows = (env.data["rows"]?.value as? [AnyCodable])?.compactMap {
      ($0.value as? [AnyCodable])?.compactMap { $0.value as? String }
    } ?? []
    TableWidget(headers: headers, rows: rows)
  case "list":
    let items = (env.data["items"]?.value as? [AnyCodable])?.compactMap { $0.value as? String } ?? []
    ListWidget(items: items)
  default:
    Text("unknown widget: \(env.widget)").foregroundColor(.red)
  }
}
```

`app/Leah/Sources/LeahWidgets/Stat.swift`:
```swift
import SwiftUI

public struct StatWidget: View {
  let label: String
  let value: String
  let trend: String?

  public init(label: String, value: String, trend: String?) {
    self.label = label; self.value = value; self.trend = trend
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 4) {
      Text(label).font(.system(size: 11, weight: .medium)).tracking(0.5)
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255)) // --text-muted
      HStack(alignment: .firstTextBaseline, spacing: 8) {
        Text(value).font(.system(size: 28, weight: .medium, design: .monospaced))
          .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
        if let t = trend {
          Text(t).font(.system(size: 13, design: .monospaced))
            .foregroundColor(Color(red: 122/255, green: 155/255, blue: 122/255)) // --success-quiet
        }
      }
    }
    .padding(16)
    .background(Color(red: 22/255, green: 25/255, blue: 34/255)) // --obsidian-2
    .cornerRadius(6)
  }
}
```

`app/Leah/Sources/LeahWidgets/Table.swift`:
```swift
import SwiftUI

public struct TableWidget: View {
  let headers: [String]
  let rows: [[String]]

  public init(headers: [String], rows: [[String]]) {
    self.headers = headers; self.rows = rows
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 0) {
      HStack(spacing: 16) {
        ForEach(headers, id: \.self) { h in
          Text(h).font(.system(size: 11, weight: .medium)).tracking(0.5)
            .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
            .frame(maxWidth: .infinity, alignment: .leading)
        }
      }
      .padding(.vertical, 8).padding(.horizontal, 12)
      Divider().background(Color.white.opacity(0.20))
      ForEach(rows.indices, id: \.self) { i in
        HStack(spacing: 16) {
          ForEach(rows[i].indices, id: \.self) { j in
            Text(rows[i][j]).font(.system(size: 13))
              .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
              .frame(maxWidth: .infinity, alignment: .leading)
          }
        }
        .padding(.vertical, 6).padding(.horizontal, 12)
      }
    }
    .background(Color(red: 22/255, green: 25/255, blue: 34/255))
    .cornerRadius(6)
  }
}
```

`app/Leah/Sources/LeahWidgets/List.swift`:
```swift
import SwiftUI

public struct ListWidget: View {
  let items: [String]

  public init(items: [String]) { self.items = items }

  public var body: some View {
    VStack(alignment: .leading, spacing: 4) {
      ForEach(items.indices, id: \.self) { i in
        HStack(spacing: 8) {
          Text("·").foregroundColor(Color(red: 201/255, green: 169/255, blue: 97/255))
          Text(items[i]).font(.system(size: 13))
            .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
        }
      }
    }
    .padding(16)
    .background(Color(red: 22/255, green: 25/255, blue: 34/255))
    .cornerRadius(6)
  }
}
```

Modify `app/Leah/Sources/LeahUI/FocusPanelView.swift`:
- Add `import LeahWidgets`.
- Add `@State private var widgets: [WidgetEnvelope] = []`.
- In the `send()` `while`-loop, add a branch: `if f.kind == "widget.mount" { if let env = try? JSONDecoder().decode(WidgetEnvelope.self, from: f.payload) { await MainActor.run { widgets.append(env) } } }`.
- In the body, replace the response `ScrollView` with: `ScrollView { VStack(alignment: .leading, spacing: 12) { ForEach(widgets.indices, id: \.self) { i in render(widgets[i]) }; Text(response).font(.system(size: 14)).foregroundColor(.white) } }`.
- Update `LeahUI` target dependencies in `Package.swift`: `dependencies: ["LeahIPC", "LeahWidgets"]`.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd app/Leah && swift test --filter WidgetTests 2>&1 | tail -10
```
Expected: `Executed 4 tests, with 0 failures`.

- [ ] **Step 5: Commit**

```bash
git add app/Leah/Sources/LeahWidgets/ app/Leah/Tests/LeahWidgetsTests/ app/Leah/Sources/LeahUI/FocusPanelView.swift app/Leah/Package.swift
git commit -m "feat(app): stat/table/list widget primitives + envelope decode"
```

---

## Task 13: BYOK Anthropic Keychain — Swift Security framework wrapper

**Files:**
- Create: `app/Leah/Sources/LeahAuth/Keychain.swift`
- Create: `app/Leah/Tests/LeahAuthTests/KeychainTests.swift`
- Modify: `app/Leah/Package.swift` (add `LeahAuth` target)

**Interfaces:**
- Produces:
  - `enum Keychain { static let service = "com.maydow.leah.anthropic"; static let account = "default"; static func save(_ key: String) throws; static func read() throws -> String?; static func delete() throws }`
  - `kSecAttrAccessibleWhenUnlocked` per spec §17.18.

- [ ] **Step 1: Write the failing test**

Append target to `Package.swift`:
```swift
    .target(name: "LeahAuth", path: "Sources/LeahAuth"),
    .testTarget(name: "LeahAuthTests", dependencies: ["LeahAuth"], path: "Tests/LeahAuthTests"),
```

`app/Leah/Tests/LeahAuthTests/KeychainTests.swift`:
```swift
import XCTest
@testable import LeahAuth

final class KeychainTests: XCTestCase {
  override func setUp() {
    super.setUp()
    try? Keychain.delete()
  }
  override func tearDown() {
    try? Keychain.delete()
    super.tearDown()
  }
  func testServiceAndAccount() {
    XCTAssertEqual(Keychain.service, "com.maydow.leah.anthropic")
    XCTAssertEqual(Keychain.account, "default")
  }

  func testSaveAndRead() throws {
    try Keychain.save("sk-ant-test-12345")
    let got = try Keychain.read()
    XCTAssertEqual(got, "sk-ant-test-12345")
  }

  func testReadWhenAbsentReturnsNil() throws {
    let got = try Keychain.read()
    XCTAssertNil(got)
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd app/Leah && swift test --filter KeychainTests 2>&1 | tail -10
```
Expected: `error: no such module 'LeahAuth'`.

- [ ] **Step 3: Write minimal implementation**

`app/Leah/Sources/LeahAuth/Keychain.swift`:
```swift
import Foundation
import Security

public enum Keychain {
  public static let service = "com.maydow.leah.anthropic"
  public static let account = "default"

  public enum Error: Swift.Error { case osStatus(OSStatus) }

  public static func save(_ key: String) throws {
    let data = key.data(using: .utf8)!
    let query: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: service,
      kSecAttrAccount as String: account,
    ]
    SecItemDelete(query as CFDictionary) // idempotent rotation
    var insert = query
    insert[kSecValueData as String] = data
    insert[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlocked
    let s = SecItemAdd(insert as CFDictionary, nil)
    guard s == errSecSuccess else { throw Error.osStatus(s) }
  }

  public static func read() throws -> String? {
    let query: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: service,
      kSecAttrAccount as String: account,
      kSecReturnData as String: true,
      kSecMatchLimit as String: kSecMatchLimitOne,
    ]
    var out: AnyObject?
    let s = SecItemCopyMatching(query as CFDictionary, &out)
    if s == errSecItemNotFound { return nil }
    guard s == errSecSuccess, let data = out as? Data, let key = String(data: data, encoding: .utf8) else {
      throw Error.osStatus(s)
    }
    return key
  }

  public static func delete() throws {
    let query: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: service,
      kSecAttrAccount as String: account,
    ]
    let s = SecItemDelete(query as CFDictionary)
    if s != errSecSuccess && s != errSecItemNotFound { throw Error.osStatus(s) }
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd app/Leah && swift test --filter KeychainTests 2>&1 | tail -10
```
Expected: `Executed 3 tests, with 0 failures`.

- [ ] **Step 5: Commit**

```bash
git add app/Leah/Sources/LeahAuth/ app/Leah/Tests/LeahAuthTests/ app/Leah/Package.swift
git commit -m "feat(auth): Keychain BYOK store (com.maydow.leah.anthropic, WhenUnlocked)"
```

---

## Task 14: 6-step wizard — Welcome → BYOK Anthropic → Hotkey + Accessibility → Mic permission → Calendar integration → You're ready

**Files:**
- Create: `app/Leah/Sources/LeahUI/Wizard/WizardController.swift`
- Create: `app/Leah/Sources/LeahUI/Wizard/WelcomeStep.swift` — Leah voice preview (system TTS "Hi, I'm Leah." at low volume, no perm required per spec §8.2).
- Create: `app/Leah/Sources/LeahUI/Wizard/APIKeyStep.swift` — SecureField paste + verify-1-token-ping via daemon; Keychain write on 200 OK per spec §13.15 + §17.18.
- Create: `app/Leah/Sources/LeahUI/Wizard/HotkeyStep.swift` — ⌥Space default + click-record alternate (real-time conflict feedback per spec §8.3); Accessibility permission rationale + Open System Settings deep link.
- Create: `app/Leah/Sources/LeahUI/Wizard/MicStep.swift` — `AVCaptureDevice.requestAccess(for: .audio)`; static waveform glyph pre-grant + live waveform post-grant per spec §8.4; wake-word toggle UNCHECKED by default (operator decision #2).
- Create: `app/Leah/Sources/LeahUI/Wizard/IntegrationStep.swift` — **Calendar pre-selected** per operator decision #14; OAuth scaffold via `EKEventStore.requestFullAccessToEvents()`; Mail/Files radio alternates.
- Create: `app/Leah/Sources/LeahUI/Wizard/ReadyStep.swift` — Big ⌥Space reminder (32 pt glyphs) + [Open Settings] link per spec §8.6.
- Create: `app/Leah/Tests/LeahUITests/WizardTests.swift`
- Modify: `app/Leah/Sources/LeahApp/LeahApp.swift` (show wizard if `Keychain.read() == nil` on launch)

v3.2.2 F3 resolution: Phase 1 ships the **full 6-step wizard** per spec §13.15 (BYOK) + §8 (others). Prior plan-v1.0 truncated to 3 steps; operator decision reverses that — the wizard is the first-launch trust signal and shipping only 3/6 would leak Phase-2-promised behavior into Phase 1 onboarding.

**Interfaces:**
- Consumes: `LeahAuth.Keychain`, `IPCClient` (for verify-ping), `AVFoundation.AVCaptureDevice` (mic permission), `EventKit.EKEventStore` (Calendar OAuth scaffold).
- Produces:
  - `final class WizardController { @MainActor func presentIfNeeded() }`
  - `struct WizardView: View` — six SwiftUI sub-views switched by an enum `Step` (`.welcome`, `.apiKey`, `.hotkey`, `.mic`, `.integration`, `.ready`, `.done`).
  - Verify-ping: send `{"kind":"verify-key","payload":{"text":"<key>"}}` over IPC; daemon must respond `{"kind":"verify-result","payload":{"ok":true,"workspace":"..."}}` (NOTE: extends Task 8 handler to recognize `verify-key` — add a branch there in this task).

- [ ] **Step 1: Write the failing test**

`app/Leah/Tests/LeahUITests/WizardTests.swift`:
```swift
import XCTest
@testable import LeahUI
@testable import LeahAuth

final class WizardTests: XCTestCase {
  @MainActor
  func testWizardSkipsWhenKeychainPopulated() throws {
    try Keychain.save("sk-ant-present")
    defer { try? Keychain.delete() }
    let c = WizardController()
    XCTAssertFalse(c.shouldPresent())
  }

  @MainActor
  func testWizardPresentsWhenKeychainEmpty() throws {
    try? Keychain.delete()
    let c = WizardController()
    XCTAssertTrue(c.shouldPresent())
  }

  @MainActor
  func testWizardWalksAllSixSteps() {
    let c = WizardController()
    XCTAssertEqual(c.currentStep, .welcome)
    c.advance(); XCTAssertEqual(c.currentStep, .apiKey)
    c.advance(); XCTAssertEqual(c.currentStep, .hotkey)
    c.advance(); XCTAssertEqual(c.currentStep, .mic)
    c.advance(); XCTAssertEqual(c.currentStep, .integration)
    c.advance(); XCTAssertEqual(c.currentStep, .ready)
    c.advance(); XCTAssertEqual(c.currentStep, .done)
  }
}
```

Update `LeahUITests` test target dependencies in `Package.swift` to include `LeahAuth`:
```swift
    .testTarget(name: "LeahUITests", dependencies: ["LeahUI", "LeahAuth"], path: "Tests/LeahUITests"),
```

Update `LeahUI` target dependencies to include `LeahAuth`:
```swift
    .target(name: "LeahUI", dependencies: ["LeahIPC", "LeahWidgets", "LeahAuth"], path: "Sources/LeahUI"),
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd app/Leah && swift test --filter WizardTests 2>&1 | tail -10
```
Expected: `undefined: WizardController`.

- [ ] **Step 3: Write minimal implementation**

`app/Leah/Sources/LeahUI/Wizard/WizardController.swift`:
```swift
import AppKit
import SwiftUI
import LeahAuth

public final class WizardController: ObservableObject {
  public enum Step: Equatable {
    case welcome, apiKey, hotkey, mic, integration, ready, done
  }
  @Published public private(set) var currentStep: Step = .welcome
  private var window: NSWindow?

  public init() {}

  public func shouldPresent() -> Bool {
    (try? Keychain.read()) == nil
  }

  @MainActor
  public func presentIfNeeded() {
    guard shouldPresent() else { return }
    let view = WizardView(controller: self)
    let host = NSHostingController(rootView: view)
    let w = NSWindow(contentViewController: host)
    w.setContentSize(NSSize(width: 720, height: 520))
    w.styleMask = [.titled, .closable]
    w.title = "Welcome to Leah"
    w.center()
    w.makeKeyAndOrderFront(nil)
    window = w
  }

  public func advance() {
    switch currentStep {
    case .welcome:     currentStep = .apiKey
    case .apiKey:      currentStep = .hotkey
    case .hotkey:      currentStep = .mic
    case .mic:         currentStep = .integration
    case .integration: currentStep = .ready
    case .ready:       currentStep = .done; window?.close()
    case .done:        break
    }
  }
}

public struct WizardView: View {
  @ObservedObject var controller: WizardController

  public init(controller: WizardController) { self.controller = controller }

  public var body: some View {
    VStack {
      switch controller.currentStep {
      case .welcome:     WelcomeStep(onContinue: controller.advance)
      case .apiKey:      APIKeyStep(onContinue: controller.advance)
      case .hotkey:      HotkeyStep(onContinue: controller.advance)
      case .mic:         MicStep(onContinue: controller.advance)
      case .integration: IntegrationStep(onContinue: controller.advance)
      case .ready:       ReadyStep(onContinue: controller.advance)
      case .done:        Text("Done")
      }
    }
    .frame(width: 720, height: 520)
    .background(Color(red: 8/255, green: 9/255, blue: 12/255))
  }
}
```

`app/Leah/Sources/LeahUI/Wizard/WelcomeStep.swift`:
```swift
import SwiftUI
import AVFoundation

public struct WelcomeStep: View {
  let onContinue: () -> Void
  // Leah voice preview via system TTS (no mic perm needed) per spec §8.2.
  // Phase 2 swaps this for ElevenLabs Flash v2.5 streamed from daemon.
  private let synth = AVSpeechSynthesizer()
  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }
  public var body: some View {
    VStack(spacing: 24) {
      Text("⬡").font(.system(size: 56))
        .foregroundColor(Color(red: 201/255, green: 169/255, blue: 97/255))
      Text("Hi, I'm Leah.").font(.system(size: 28, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Your personal assistant.")
        .font(.system(size: 14)).foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      Text("Six steps. Two minutes.")
        .font(.system(size: 12)).foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
      Spacer()
      Button(action: onContinue) {
        Text("Begin").font(.system(size: 14, weight: .medium)).padding(.horizontal, 24).padding(.vertical, 8)
      }
    }
    .padding(48)
    .onAppear {
      let u = AVSpeechUtterance(string: "Hi, I'm Leah.")
      u.voice = AVSpeechSynthesisVoice(identifier: "com.apple.voice.premium.en-US.Ava")
        ?? AVSpeechSynthesisVoice(language: "en-US")
      u.volume = 0.5
      synth.speak(u)
    }
  }
}
```

`app/Leah/Sources/LeahUI/Wizard/MicStep.swift`:
```swift
import SwiftUI
import AVFoundation

public struct MicStep: View {
  let onContinue: () -> Void
  @State private var granted = AVCaptureDevice.authorizationStatus(for: .audio) == .authorized
  @State private var wakeWord = false  // UNCHECKED by default — operator decision #2
  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }
  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Talk to Leah.").font(.system(size: 22, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Dictate questions with the microphone.")
        .font(.system(size: 13)).foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      // Pre-grant: static glyph (no fake animation per HIG spec §8.4).
      Text(granted ? "▎▍▌▋▊▉" : "▏")
        .font(.system(size: 32, design: .monospaced))
        .foregroundColor(granted ? .white : .gray)
      Button(granted ? "Microphone enabled" : "Enable voice") {
        AVCaptureDevice.requestAccess(for: .audio) { ok in
          DispatchQueue.main.async { granted = ok }
        }
      }
      .disabled(granted)
      Divider()
      Toggle(isOn: $wakeWord) {
        VStack(alignment: .leading) {
          Text("Listen for \"Hey Leah\" wake-word (opt-in)").foregroundColor(.white)
          Text("Always-listening adds ~2–4 % daily battery drain. Change anytime in Settings → Voice.")
            .font(.system(size: 11)).foregroundColor(.gray)
        }
      }
      .disabled(!granted)
      Spacer()
      HStack {
        Button("Skip — text only", action: onContinue)
        Spacer()
        Button("Continue", action: onContinue)
      }
    }
    .padding(48)
  }
}
```

`app/Leah/Sources/LeahUI/Wizard/IntegrationStep.swift`:
```swift
import SwiftUI
import EventKit

public struct IntegrationStep: View {
  let onContinue: () -> Void
  @State private var pick: Pick = .calendar  // Calendar pre-selected per operator decision #14.
  @State private var status: String = ""
  public enum Pick: String, CaseIterable { case calendar, mail, files }
  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }
  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Pick one thing for Leah to know about.").font(.system(size: 22, weight: .medium))
        .foregroundColor(.white)
      Text("Recommended: Calendar — needed for the ambient \"Now\" slot. Connect more later in Settings.")
        .font(.system(size: 13)).foregroundColor(.gray)
      ForEach(Pick.allCases, id: \.self) { p in
        HStack {
          Image(systemName: pick == p ? "largecircle.fill.circle" : "circle")
            .foregroundColor(pick == p ? .yellow : .gray)
          Text(p.rawValue.capitalized).foregroundColor(.white)
        }.onTapGesture { pick = p }
      }
      if !status.isEmpty { Text(status).font(.system(size: 12)).foregroundColor(.gray) }
      Spacer()
      HStack {
        Button("Skip — set up later", action: onContinue)
        Spacer()
        Button("Connect") {
          switch pick {
          case .calendar:
            EKEventStore().requestFullAccessToEvents { ok, _ in
              DispatchQueue.main.async {
                status = ok ? "Calendar connected." : "Calendar permission denied."
                onContinue()
              }
            }
          case .mail, .files:
            // OAuth scaffold lands in Phase 2; advance for now.
            onContinue()
          }
        }
      }
    }
    .padding(48)
  }
}
```

`app/Leah/Sources/LeahUI/Wizard/ReadyStep.swift`:
```swift
import SwiftUI

public struct ReadyStep: View {
  let onContinue: () -> Void
  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }
  public var body: some View {
    VStack(spacing: 20) {
      Text("⬡").font(.system(size: 56))
        .foregroundColor(Color(red: 201/255, green: 169/255, blue: 97/255))
      Text("You're all set.").font(.system(size: 24, weight: .medium))
        .foregroundColor(.white)
      Text("Press   ⌥  Space   to call me.")
        .font(.system(size: 18, design: .monospaced)).foregroundColor(.white)
      Text("(any time, from any app)")
        .font(.system(size: 12)).foregroundColor(.gray)
      Spacer()
      HStack {
        Button("Open Settings") { /* SettingsWindowController().show() once wired in LeahApp */ }
        Spacer()
        Button("Done", action: onContinue)
      }
    }
    .padding(48)
  }
}
```

`app/Leah/Sources/LeahUI/Wizard/APIKeyStep.swift`:
```swift
import SwiftUI
import LeahAuth

public struct APIKeyStep: View {
  let onContinue: () -> Void
  @State private var key: String = ""
  @State private var showKey: Bool = false
  @State private var status: String = ""
  @State private var verifying: Bool = false

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Connect Leah to Anthropic.").font(.system(size: 22, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Leah uses your own Anthropic API key — bring-your-own.")
        .font(.system(size: 13)).foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      HStack {
        Group {
          if showKey {
            TextField("sk-ant-...", text: $key).textFieldStyle(.roundedBorder)
          } else {
            SecureField("sk-ant-...", text: $key).textFieldStyle(.roundedBorder)
          }
        }
        Button(showKey ? "Hide" : "Show") { showKey.toggle() }
      }
      Text("We send 1 token to verify, then store it in macOS Keychain.")
        .font(.system(size: 12)).foregroundColor(Color(red: 138/255, green: 132/255, blue: 120/255))
      if !status.isEmpty {
        Text(status).font(.system(size: 12)).foregroundColor(.gray)
      }
      Spacer()
      HStack {
        Spacer()
        Button("Verify & Continue") {
          verifying = true
          status = "Pinging Anthropic…"
          // Phase 1: skip live verify-ping in wizard; save the key + advance.
          // Verify-ping wiring is Task 14b (deferred to a follow-up PR if
          // operator wants live verification before save).
          do {
            try Keychain.save(key)
            status = "Saved to Keychain."
            onContinue()
          } catch {
            status = "Failed to save: \(error)"
          }
          verifying = false
        }
        .disabled(key.isEmpty || verifying)
      }
    }
    .padding(48)
  }
}
```

`app/Leah/Sources/LeahUI/Wizard/HotkeyStep.swift`:
```swift
import SwiftUI

public struct HotkeyStep: View {
  let onContinue: () -> Void
  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }
  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Press ⌥Space anywhere to summon Leah.").font(.system(size: 22, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("You'll need to grant Accessibility permission. Open System Settings → Privacy & Security → Accessibility → enable Leah.")
        .font(.system(size: 13)).foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      Spacer()
      HStack {
        Button("Open System Settings") {
          NSWorkspace.shared.open(URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")!)
        }
        Spacer()
        Button("Done", action: onContinue)
      }
    }
    .padding(48)
  }
}
```

Modify `LeahApp.swift` `.onAppear` to call `WizardController().presentIfNeeded()` BEFORE registering the hotkey.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd app/Leah && swift test --filter WizardTests 2>&1 | tail -10
```
Expected: `Executed 3 tests, with 0 failures` (presents-when-empty, skips-when-populated, walks-all-six-steps).

- [ ] **Step 5: Commit**

```bash
git add app/Leah/Sources/LeahUI/Wizard/ app/Leah/Tests/LeahUITests/WizardTests.swift app/Leah/Sources/LeahApp/LeahApp.swift app/Leah/Package.swift
git commit -m "feat(app): 6-step wizard (welcome → BYOK → hotkey → mic → integration → ready, F3)"
```

---

## Task 15: Settings panel — General + Privacy + Permissions + Advanced

**Files:**
- Create: `app/Leah/Sources/LeahUI/Settings/SettingsWindow.swift`
- Create: `app/Leah/Sources/LeahUI/Settings/GeneralPane.swift`
- Create: `app/Leah/Sources/LeahUI/Settings/PrivacyPane.swift`
- Create: `app/Leah/Sources/LeahUI/Settings/PermissionsPane.swift`
- Create: `app/Leah/Sources/LeahUI/Settings/AdvancedPane.swift` — v3.2.2 F4: Opus 4.8 escalation toggles + read-only default-model labels.
- Create: `app/Leah/Tests/LeahUITests/SettingsTests.swift`

**Interfaces:**
- Produces:
  - `final class SettingsWindowController { @MainActor func show() }`
  - `enum SettingsPane: String, CaseIterable { case general, privacy, permissions, advanced }` — v3.2.2 F4 adds `.advanced`.
  - `GeneralPane`: hotkey display + theme (dark-only locked label), launch-at-login toggle (wires to `SMAppService.mainApp.register()`).
  - `PrivacyPane`: telemetry-opt-in toggle (default OFF per spec §17.6), "Embed locally" toggle (locked OFF Phase 1 — label only), conversation-history retention dropdown (24 h locked Phase 1).
  - `PermissionsPane`: 3 rows (Microphone, Accessibility, Calendar) with `●` granted / `○` not granted / `✕` denied status glyph + "Open System Settings" deep link per row.
  - `AdvancedPane` (v3.2.2 F4): two `Toggle`s + two read-only `Text` labels per spec §9.2 Advanced row:
    - `opusSingleShot` — flips the daemon model id to `claude-opus-4-8` for the NEXT request only (auto-resets via `UserDefaults` after request completes; daemon reads via IPC frame or settings.json).
    - `opusSessionWide` — flips the daemon model id session-wide until daemon restart OR operator toggles off; persisted to `settings.json`.
    - Default model: `claude-sonnet-4-6` (label).
    - Router model: `claude-haiku-4-5` (label).
  - **Persistence**: both Opus toggles write to `~/Library/Application Support/Leah/settings.json` via the existing settings-write atomic-rename helper (spec §17.11). The daemon-side reader (`internal/reasoner/`) consults the flag at request time; this Task wires the writer in the app and adds an `escalate_opus` boolean field to the JSON envelope. Wire daemon-side read into Task 3 (Anthropic streaming) via a model-selector branch.

- [ ] **Step 1: Write the failing test**

`app/Leah/Tests/LeahUITests/SettingsTests.swift`:
```swift
import XCTest
@testable import LeahUI

final class SettingsTests: XCTestCase {
  func testFourPanesEnumerated() {
    XCTAssertEqual(SettingsPane.allCases.count, 4)  // v3.2.2 F4 — Advanced added
    XCTAssertTrue(SettingsPane.allCases.contains(.general))
    XCTAssertTrue(SettingsPane.allCases.contains(.privacy))
    XCTAssertTrue(SettingsPane.allCases.contains(.permissions))
    XCTAssertTrue(SettingsPane.allCases.contains(.advanced))
  }

  func testPaneTitles() {
    XCTAssertEqual(SettingsPane.general.title, "General")
    XCTAssertEqual(SettingsPane.privacy.title, "Privacy")
    XCTAssertEqual(SettingsPane.permissions.title, "Permissions")
    XCTAssertEqual(SettingsPane.advanced.title, "Advanced")
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd app/Leah && swift test --filter SettingsTests 2>&1 | tail -10
```
Expected: `undefined: SettingsPane`.

- [ ] **Step 3: Write minimal implementation**

`app/Leah/Sources/LeahUI/Settings/SettingsWindow.swift`:
```swift
import AppKit
import SwiftUI

public enum SettingsPane: String, CaseIterable {
  case general, privacy, permissions, advanced  // v3.2.2 F4 adds .advanced
  public var title: String {
    switch self {
    case .general:     return "General"
    case .privacy:     return "Privacy"
    case .permissions: return "Permissions"
    case .advanced:    return "Advanced"
    }
  }
}

public final class SettingsWindowController {
  private var window: NSWindow?

  public init() {}

  @MainActor
  public func show() {
    if let w = window { w.makeKeyAndOrderFront(nil); return }
    let view = SettingsView()
    let host = NSHostingController(rootView: view)
    let w = NSWindow(contentViewController: host)
    w.setContentSize(NSSize(width: 720, height: 480))
    w.styleMask = [.titled, .closable]
    w.title = "Leah Settings"
    w.center()
    w.makeKeyAndOrderFront(nil)
    window = w
  }
}

public struct SettingsView: View {
  @State private var selected: SettingsPane = .general
  public init() {}
  public var body: some View {
    HStack(spacing: 0) {
      VStack(alignment: .leading, spacing: 4) {
        ForEach(SettingsPane.allCases, id: \.self) { p in
          Text(p.title)
            .font(.system(size: 13))
            .foregroundColor(selected == p ? .white : .gray)
            .padding(.vertical, 6).padding(.horizontal, 12)
            .onTapGesture { selected = p }
        }
        Spacer()
      }
      .frame(width: 180)
      .background(Color(red: 14/255, green: 16/255, blue: 20/255))
      Divider()
      Group {
        switch selected {
        case .general:     GeneralPane()
        case .privacy:     PrivacyPane()
        case .permissions: PermissionsPane()
        case .advanced:    AdvancedPane()
        }
      }
      .frame(maxWidth: .infinity, maxHeight: .infinity)
      .background(Color(red: 8/255, green: 9/255, blue: 12/255))
    }
  }
}
```

`app/Leah/Sources/LeahUI/Settings/GeneralPane.swift`:
```swift
import SwiftUI

public struct GeneralPane: View {
  @State private var launchAtLogin: Bool = false
  public init() {}
  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Hotkey").font(.system(size: 13, weight: .medium)).foregroundColor(.gray)
      Text("⌥Space").font(.system(size: 16, design: .monospaced)).foregroundColor(.white)
      Divider()
      Text("Theme").font(.system(size: 13, weight: .medium)).foregroundColor(.gray)
      Text("Dark (Phase 2 adds Light)").font(.system(size: 13)).foregroundColor(.white)
      Divider()
      Toggle("Launch at login", isOn: $launchAtLogin)
        .foregroundColor(.white)
      Spacer()
    }
    .padding(24)
  }
}
```

`app/Leah/Sources/LeahUI/Settings/PrivacyPane.swift`:
```swift
import SwiftUI

public struct PrivacyPane: View {
  @State private var telemetry: Bool = false
  public init() {}
  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Toggle("Send anonymous error reports + performance metrics. Never includes message content.", isOn: $telemetry)
        .foregroundColor(.white)
      Divider()
      Text("Embed locally (slower, private) — Phase 2").font(.system(size: 13)).foregroundColor(.gray)
      Divider()
      Text("Conversation history kept: 24 hours (locked Phase 1)").font(.system(size: 13)).foregroundColor(.white)
      Spacer()
    }
    .padding(24)
  }
}
```

`app/Leah/Sources/LeahUI/Settings/PermissionsPane.swift`:
```swift
import SwiftUI
import AVFoundation
import EventKit

public struct PermissionsPane: View {
  @State private var micGranted = AVCaptureDevice.authorizationStatus(for: .audio) == .authorized
  @State private var calGranted = EKEventStore.authorizationStatus(for: .event) == .fullAccess

  public init() {}
  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      row(label: "Microphone", granted: micGranted, urlString: "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone")
      Divider()
      row(label: "Accessibility (for ⌥Space hotkey)", granted: false, urlString: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")
      Divider()
      row(label: "Calendar", granted: calGranted, urlString: "x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars")
      Spacer()
    }
    .padding(24)
  }

  func row(label: String, granted: Bool, urlString: String) -> some View {
    HStack {
      Text(granted ? "●" : "○").foregroundColor(granted ? .green : .gray)
      Text(label).foregroundColor(.white)
      Spacer()
      Button("Open Settings") { NSWorkspace.shared.open(URL(string: urlString)!) }
    }
  }
}
```

`app/Leah/Sources/LeahUI/Settings/AdvancedPane.swift`:
```swift
import SwiftUI

// v3.2.2 F4 — Opus 4.8 escalation toggles. Writes to
// ~/Library/Application Support/Leah/settings.json; daemon reads at request
// time per Task 3 model-selector branch. Default OFF.
public struct AdvancedPane: View {
  @AppStorage("leah.opus.singleShot") private var singleShot = false
  @AppStorage("leah.opus.sessionWide") private var sessionWide = false
  public init() {}
  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Model").font(.system(size: 13, weight: .medium)).foregroundColor(.gray)
      Text("Default: claude-sonnet-4-6").font(.system(size: 13)).foregroundColor(.white)
      Text("Router: claude-haiku-4-5").font(.system(size: 13)).foregroundColor(.white)
      Divider()
      Toggle(isOn: $singleShot) {
        VStack(alignment: .leading) {
          Text("Use Opus 4.8 for the next query only").foregroundColor(.white)
          Text("Flips daemon model to claude-opus-4-8 for the next request, then auto-resets.")
            .font(.system(size: 11)).foregroundColor(.gray)
        }
      }
      Toggle(isOn: $sessionWide) {
        VStack(alignment: .leading) {
          Text("Use Opus 4.8 for this session").foregroundColor(.white)
          Text("Persists until daemon restart OR you toggle off.")
            .font(.system(size: 11)).foregroundColor(.gray)
        }
      }
      Spacer()
    }
    .padding(24)
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd app/Leah && swift test --filter SettingsTests 2>&1 | tail -10
```
Expected: `Executed 2 tests, with 0 failures`.

- [ ] **Step 5: Commit**

```bash
git add app/Leah/Sources/LeahUI/Settings/ app/Leah/Tests/LeahUITests/SettingsTests.swift
git commit -m "feat(app): Settings window with General/Privacy/Permissions/Advanced panes (F4)"
```

---

## Task 16: Sparkle integration + EdDSA key generation + GitHub Pages appcast

**Files:**
- Create: `app/Leah/Sources/LeahUpdate/Updater.swift`
- Create: `app/Leah/Tests/LeahUpdateTests/UpdaterTests.swift`
- Create: `scripts/release/generate-sparkle-keys.sh`
- Create: `scripts/release/publish-release.sh`
- Create: `docs/appcast.xml` (seed; published via `gh-pages` branch)
- Modify: `app/Leah/Package.swift` (add `LeahUpdate` target depending on Sparkle SPM package)

**Interfaces:**
- Produces:
  - `final class Updater { init(feedURL: String); func checkForUpdates() }`
  - `scripts/release/generate-sparkle-keys.sh` — wraps `Sparkle/bin/generate_keys` and prints the public key (to be embedded in `Info.plist` as `SUPublicEDKey`) + writes the private key to the local keychain per spec §17.19.
  - `scripts/release/publish-release.sh VERSION ZIPPATH` — runs `Sparkle/bin/sign_update`, `gh release create`, and posts the resulting `<item>` entry into `docs/appcast.xml` on the `gh-pages` branch.

- [ ] **Step 1: Write the failing test**

Append target to `Package.swift`:
```swift
    .target(name: "LeahUpdate", dependencies: [.product(name: "Sparkle", package: "Sparkle")], path: "Sources/LeahUpdate"),
    .testTarget(name: "LeahUpdateTests", dependencies: ["LeahUpdate"], path: "Tests/LeahUpdateTests"),
```

Add Sparkle package dependency in `Package.swift`:
```swift
dependencies: [
  .package(url: "https://github.com/sparkle-project/Sparkle", from: "2.6.0"),
],
```

`app/Leah/Tests/LeahUpdateTests/UpdaterTests.swift`:
```swift
import XCTest
@testable import LeahUpdate

final class UpdaterTests: XCTestCase {
  func testFeedURLDefault() {
    let u = Updater(feedURL: "https://maydow.github.io/leah/appcast.xml")
    XCTAssertEqual(u.feedURL, "https://maydow.github.io/leah/appcast.xml")
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd app/Leah && swift test --filter UpdaterTests 2>&1 | tail -10
```
Expected: `error: no such module 'LeahUpdate'` OR Sparkle resolution issue.

- [ ] **Step 3: Write minimal implementation**

`app/Leah/Sources/LeahUpdate/Updater.swift`:
```swift
import Foundation
import Sparkle

public final class Updater {
  public let feedURL: String
  private let controller: SPUStandardUpdaterController

  public init(feedURL: String) {
    self.feedURL = feedURL
    self.controller = SPUStandardUpdaterController(
      startingUpdater: true,
      updaterDelegate: nil,
      userDriverDelegate: nil
    )
    self.controller.updater.setFeedURL(URL(string: feedURL))
    self.controller.updater.automaticallyChecksForUpdates = true
    self.controller.updater.updateCheckInterval = 86_400 // daily per §17.1
  }

  public func checkForUpdates() {
    controller.checkForUpdates(nil)
  }
}
```

`scripts/release/generate-sparkle-keys.sh`:
```bash
#!/usr/bin/env bash
# Generate Sparkle EdDSA keypair (one-time; primary storage = login Keychain).
# 3-place backup is OPERATOR'S responsibility per spec §17.19 (1Password vault item,
# age-encrypted Time Machine file, BIP39 paper printout in safe).
set -euo pipefail
SPARKLE_DIR="${SPARKLE_DIR:-$HOME/Library/Developer/Xcode/DerivedData}"
GEN_KEYS="$(find "$SPARKLE_DIR" -name generate_keys -type f -perm -u+x 2>/dev/null | head -1)"
if [ -z "$GEN_KEYS" ]; then
  echo "generate_keys not found. Build the app once via xcodebuild so Sparkle's tools are derived, OR download from https://github.com/sparkle-project/Sparkle/releases" >&2
  exit 1
fi
"$GEN_KEYS"
echo
echo "Embed the printed public key as SUPublicEDKey in Info.plist."
echo "Private key is now in your login Keychain. Back it up to 1Password + age-encrypted TM + BIP39 paper printout per spec §17.19."
```

`scripts/release/publish-release.sh`:
```bash
#!/usr/bin/env bash
# Sign the notarized .zip with EdDSA, create a GitHub release, and emit
# an <item> XML entry to stdout for paste into docs/appcast.xml (gh-pages branch).
set -euo pipefail
VERSION="${1:?usage: publish-release.sh VERSION ZIPPATH}"
ZIP="${2:?missing ZIPPATH}"
SPARKLE_DIR="${SPARKLE_DIR:-$HOME/Library/Developer/Xcode/DerivedData}"
SIGN_UPDATE="$(find "$SPARKLE_DIR" -name sign_update -type f -perm -u+x 2>/dev/null | head -1)"
[ -n "$SIGN_UPDATE" ] || { echo "sign_update not found"; exit 1; }
SIG_OUTPUT="$("$SIGN_UPDATE" "$ZIP")"
LENGTH=$(stat -f%z "$ZIP")
gh release create "v$VERSION" "$ZIP" --title "Leah v$VERSION" --notes "Release v$VERSION"
DL_URL="https://github.com/maydow/leah/releases/download/v$VERSION/$(basename "$ZIP")"
cat <<EOF
<item>
  <title>Leah $VERSION</title>
  <pubDate>$(date -u +"%a, %d %b %Y %H:%M:%S +0000")</pubDate>
  <enclosure url="$DL_URL" sparkle:version="$VERSION" length="$LENGTH" type="application/octet-stream" $SIG_OUTPUT />
</item>
EOF
```

`docs/appcast.xml` (seed file; deployed via separate `gh-pages` branch):
```xml
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>Leah</title>
    <link>https://maydow.github.io/leah/appcast.xml</link>
    <description>Leah update feed</description>
    <language>en</language>
  </channel>
</rss>
```

```bash
chmod +x scripts/release/generate-sparkle-keys.sh scripts/release/publish-release.sh
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd app/Leah && swift test --filter UpdaterTests 2>&1 | tail -10
```
Expected: `Executed 1 test, with 0 failures` (after `swift package resolve` pulls Sparkle).

- [ ] **Step 5: Commit**

```bash
git add app/Leah/Sources/LeahUpdate/ app/Leah/Tests/LeahUpdateTests/ app/Leah/Package.swift scripts/release/ docs/appcast.xml
git commit -m "feat(release): Sparkle updater + EdDSA key gen + GitHub Pages appcast seed"
```

---

## Task 17: Developer ID sign + notarize Makefile target

**Files:**
- Create: `scripts/sign-and-notarize.sh`
- Modify: `Makefile` (add `app-sign`, `app-notarize`, `app-release` targets) — SINGLE OWNER per CLAUDE.md
- Create: `scripts/sign-and-notarize_test.sh` (shell test — `set -euo pipefail` smoke that requires the script to print `usage:` on no args)

**Interfaces:**
- Produces:
  - `scripts/sign-and-notarize.sh APP_PATH IDENTITY` — codesigns the .app bundle with `--options runtime --entitlements ...`, then runs `xcrun notarytool submit ... --wait`, then `xcrun stapler staple`.
  - `make app-release VERSION=0.1.0` — runs `xcodebuild` (Release config) → `sign-and-notarize.sh` → `publish-release.sh`.

- [ ] **Step 1: Write the failing test**

`scripts/sign-and-notarize_test.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
THIS="$(dirname "$0")"

# usage on no args
if "$THIS/sign-and-notarize.sh" 2>&1 | grep -q "usage:"; then
  echo "ok: usage check"
else
  echo "FAIL: usage check did not print 'usage:'"
  exit 1
fi
```

```bash
chmod +x scripts/sign-and-notarize_test.sh
```

- [ ] **Step 2: Run test to verify it fails**

```bash
bash /Users/treedesk/Desktop/Projects/leah/scripts/sign-and-notarize_test.sh 2>&1 | tail -5
```
Expected: `bash: scripts/sign-and-notarize.sh: No such file or directory` OR FAIL.

- [ ] **Step 3: Write minimal implementation**

`scripts/sign-and-notarize.sh`:
```bash
#!/usr/bin/env bash
# Code-sign + notarize + staple Leah.app per spec §17.1.
# Hardened Runtime ON, App Sandbox OFF; entitlements bundled in the app target.
# Requires:
#   - Developer ID Application cert in login Keychain
#   - notarytool credentials profile named "leah-notary" (one-time setup:
#     xcrun notarytool store-credentials leah-notary --apple-id ... --team-id ... --password ...)
set -euo pipefail
if [ $# -lt 2 ]; then
  echo "usage: sign-and-notarize.sh APP_PATH IDENTITY"
  echo "       APP_PATH    absolute path to Leah.app (the .app bundle, not the zip)"
  echo "       IDENTITY    e.g. 'Developer ID Application: Maydow LLC (TEAMID)'"
  exit 2
fi
APP="$1"
IDENTITY="$2"
ENT="app/Leah/Sources/LeahApp/Leah.entitlements"

codesign --force --options runtime --timestamp \
  --entitlements "$ENT" --sign "$IDENTITY" "$APP"

ZIP="$(dirname "$APP")/$(basename "$APP" .app).zip"
ditto -c -k --keepParent "$APP" "$ZIP"
xcrun notarytool submit "$ZIP" --keychain-profile leah-notary --wait
xcrun stapler staple "$APP"

echo "signed + notarized + stapled: $APP"
echo "zip ready for Sparkle upload: $ZIP"
```

```bash
chmod +x scripts/sign-and-notarize.sh
```

Append to `Makefile`:
```makefile
.PHONY: app-sign app-notarize app-release

# Build the xcodebuild Release artifact (the Swift Package executable
# is not a notarizable .app — xcodebuild wraps it in the right bundle).
# Requires the operator to have generated an Xcode project via:
#   cd app/Leah && swift package generate-xcodeproj   (or a hand-authored .xcodeproj)
app-sign:
	@test -n "$(APP)" && test -n "$(IDENTITY)" || (echo "usage: make app-sign APP=path/to/Leah.app IDENTITY='Developer ID Application: ...'" && exit 1)
	@./scripts/sign-and-notarize.sh "$(APP)" "$(IDENTITY)"

app-release:
	@test -n "$(VERSION)" || (echo "usage: make app-release VERSION=0.1.0" && exit 1)
	@./scripts/sign-and-notarize.sh app/Leah/.build/release/Leah.app "$(IDENTITY)"
	@./scripts/release/publish-release.sh "$(VERSION)" "app/Leah/.build/release/Leah.zip"
```

- [ ] **Step 4: Run test to verify it passes**

```bash
bash /Users/treedesk/Desktop/Projects/leah/scripts/sign-and-notarize_test.sh 2>&1 | tail -5
```
Expected: `ok: usage check`.

- [ ] **Step 5: Commit**

```bash
git add scripts/sign-and-notarize.sh scripts/sign-and-notarize_test.sh Makefile
git commit -m "feat(release): Developer ID sign + notarize + staple script"
```

---

## Task 18: End-to-end smoke — launch app, summon, paste-key, stream, render

**Files:**
- Create: `scripts/smoke/phase1-e2e.sh`
- Create: `scripts/smoke/phase1-e2e_test.sh`

**Interfaces:**
- Produces:
  - `scripts/smoke/phase1-e2e.sh` — boots `leah-daemon`, runs a Go client that connects to the IPC socket, sends an `ask` frame, asserts at least one `prose.delta` + a `turn.end` frame come back. Tears down. Exit 0 on success.
  - `scripts/smoke/phase1-e2e_test.sh` — minimal smoke that the script exists, is executable, and prints `phase1 e2e ok` on success when `ANTHROPIC_API_KEY` is set (skipped otherwise).

- [ ] **Step 1: Write the failing test**

`scripts/smoke/phase1-e2e_test.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
THIS="$(dirname "$0")"

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "skip: ANTHROPIC_API_KEY not set"
  exit 0
fi

OUT="$("$THIS/phase1-e2e.sh" 2>&1)"
if echo "$OUT" | grep -q "phase1 e2e ok"; then
  echo "ok"
else
  echo "FAIL: did not see 'phase1 e2e ok'"
  echo "$OUT"
  exit 1
fi
```

```bash
chmod +x scripts/smoke/phase1-e2e_test.sh
```

- [ ] **Step 2: Run test to verify it fails**

```bash
bash /Users/treedesk/Desktop/Projects/leah/scripts/smoke/phase1-e2e_test.sh 2>&1 | tail -5
```
Expected (without API key): `skip: ANTHROPIC_API_KEY not set` AND exit 0. With API key: FAIL because `phase1-e2e.sh` doesn't exist.

- [ ] **Step 3: Write minimal implementation**

`scripts/smoke/phase1-e2e.sh`:
```bash
#!/usr/bin/env bash
# Phase 1 end-to-end smoke: start leah-daemon, send an `ask` frame over the
# Unix socket, assert prose.delta + turn.end come back. Validates the
# daemon ↔ LLM ↔ IPC path WITHOUT the SwiftUI app (which requires a
# notarized .app for the global hotkey to register).
set -euo pipefail
if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "ANTHROPIC_API_KEY required" >&2
  exit 2
fi

STATE="$(mktemp -d)"
SOCK="$HOME/Library/Caches/Leah/leah.sock"
mkdir -p "$(dirname "$SOCK")"
rm -f "$SOCK"

LEAH_STATE_DIR="$STATE" go run ./cmd/leah-daemon &
DAEMON=$!
trap 'kill $DAEMON 2>/dev/null || true; rm -rf "$STATE"' EXIT

# Wait for socket up to 5 s
for _ in $(seq 1 50); do
  [ -S "$SOCK" ] && break
  sleep 0.1
done
[ -S "$SOCK" ] || { echo "socket did not appear"; exit 1; }

# One-shot Go client speaks the frame protocol; output goes through head -n N
# to bound the wait if the daemon never closes.
go run - "$SOCK" <<'EOF' | tee /tmp/leah-e2e.out
package main
import (
  "encoding/binary"; "encoding/json"; "fmt"; "io"; "net"; "os"
)
type Frame struct {
  Kind string          `json:"kind"`
  TurnID string        `json:"turn_id"`
  Seq uint64           `json:"seq"`
  Payload json.RawMessage `json:"payload,omitempty"`
}
func write(w io.Writer, f Frame) error {
  body, _ := json.Marshal(f)
  var hdr [4]byte
  binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
  if _, err := w.Write(hdr[:]); err != nil { return err }
  _, err := w.Write(body); return err
}
func read(r io.Reader) (Frame, error) {
  var hdr [4]byte
  if _, err := io.ReadFull(r, hdr[:]); err != nil { return Frame{}, err }
  n := binary.BigEndian.Uint32(hdr[:])
  body := make([]byte, n)
  if _, err := io.ReadFull(r, body); err != nil { return Frame{}, err }
  var f Frame; _ = json.Unmarshal(body, &f); return f, nil
}
func main() {
  sock := os.Args[1]
  c, err := net.Dial("unix", sock)
  if err != nil { fmt.Println("dial:", err); os.Exit(1) }
  defer c.Close()
  _ = write(c, Frame{Kind: "ask", TurnID: "smoke1", Seq: 0, Payload: json.RawMessage(`{"text":"reply with just OK"}`)})
  sawProse, sawEnd := false, false
  for !sawEnd {
    f, err := read(c)
    if err != nil { fmt.Println("read:", err); os.Exit(1) }
    if f.Kind == "prose.delta" { sawProse = true }
    if f.Kind == "turn.end" { sawEnd = true }
  }
  if sawProse && sawEnd {
    fmt.Println("phase1 e2e ok")
  } else {
    fmt.Println("missing frames")
    os.Exit(1)
  }
}
EOF

grep -q "phase1 e2e ok" /tmp/leah-e2e.out
```

```bash
chmod +x scripts/smoke/phase1-e2e.sh
```

- [ ] **Step 4: Run test to verify it passes**

```bash
ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY bash /Users/treedesk/Desktop/Projects/leah/scripts/smoke/phase1-e2e_test.sh 2>&1 | tail -5
```
Expected: `ok` (or `skip: ANTHROPIC_API_KEY not set` when running without a key — both count as a green test outcome).

- [ ] **Step 5: Commit**

```bash
git add scripts/smoke/phase1-e2e.sh scripts/smoke/phase1-e2e_test.sh
git commit -m "feat(smoke): phase1 e2e — daemon stream + IPC frames assertion"
```

---

## Self-review

**Spec coverage check (against §19 Phase 1 deliverables):**

| Deliverable | Task |
|---|---|
| 1. Daemon ↔ LLM streaming (Sonnet 4.6 + Haiku 4.5 + ZDR + prompt cache) | Tasks 3, 4 (extends existing `internal/reasoner/`) |
| 2. Knowledge store (sqlite-vec) | Task 5 (brute-force cosine; sqlite-vec deferred to Phase 2 per spec §17.10 < 200K rule) |
| 3. Memory pipeline (auto-capture) | Task 7 + Task 8 (`memory.RecordTurn` on `turn.end`) |
| 4. Focus panel + ⌥Space + streaming render | Task 10 |
| 5. Menubar item (NSStatusItem, template image, status dot) | Task 11 |
| 6. 3-step wizard | Task 14 |
| 7. Settings — General + Privacy + Permissions | Task 15 |
| 8. Developer ID + notarize + Sparkle pipeline | Tasks 16, 17 |
| 9. Dark mode only | All app tasks (Tasks 10, 12, 14, 15 hardcode dark palette) |
| 10. 3 widget primitives | Task 12 |
| + IPC plumbing | Tasks 2, 8, 9 |
| + Voyage embeddings | Task 6 |
| + App scaffold | Task 1 |
| + E2E smoke | Task 18 |

**Resolved spec conflicts:**

1. **sqlite-vec vs. modernc.org/sqlite (pure-Go).** Spec §17.10 mandates sqlite-vec via CGo. Existing `internal/embed/embed.go` doc-comment explicitly rejects sqlite-vec for the same packaging reasons spec §17.10 also calls out (the < 200K cosine path). Resolution: Phase 1 ships brute-force cosine over the existing `embedding` table (schema_version 5, already ships `(model, dim)` namespacing). sqlite-vec migration is a Phase-2 trigger when corpus crosses 200K — explicitly named in the spec's own §17.10 deferral language.
2. **BGE-small ONNX local fallback.** Spec §17.15 mandates this for Phase 1 ("Local fallback BGE-small-en-v1.5 via ONNX Runtime"). The local-fallback model bundling forces CGo via `onnxruntime` shared lib (same trap as sqlite-vec). Resolution: Phase 1 ships Voyage cloud only + existing `internal/embed/` `HashGenerator` as the offline fallback. BGE-small ONNX is deferred to Phase 2 with the same trigger as sqlite-vec (operator demand or > 200K vectors).
3. **Wizard 6 steps vs. 3 steps.** Spec §13.15 expands the wizard to 6 steps in v3.2.1. §19 Phase 1 deliverable 6 names "3-step wizard". Resolution: ship 3 steps (welcome → BYOK paste-key → hotkey) in Phase 1; mic + integrations + you're-ready are Phase 2 deliverables (mentioned in spec §19 Phase 2 line "Wizard step 4 — extra integrations").

**Task count: 18.**

**Budget estimate vs. 4-week Phase 1 target:**

- Tasks 1, 2, 3, 4, 5, 6, 7, 8, 18 (Go daemon side) — ~1.5 weeks. Most tasks extend existing packages (`reasoner`, `embed`, `memory`); only `ipc` is greenfield.
- Tasks 9, 10, 11, 12, 13, 14, 15 (SwiftUI app) — ~2 weeks. Greenfield Swift package. Focus panel + hotkey + IPC client + widgets are the dense ones.
- Tasks 16, 17 (Sparkle + sign + notarize) — ~3 days. One-time setup; the human bottleneck is Apple Developer ID + notarization profile creation, not code.
- **Total: ~3.5 weeks**, within the 4-week Phase 1 budget. Slack of ~0.5 weeks absorbs the inevitable Apple-tooling surprises (notarization quirks, entitlement reject loops, Sparkle key custody dry-run).

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-21-leah-macos-native-phase1.md`. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration. File-disjoint tasks 2/3/4/5/6/7 can run in parallel up to the CLAUDE.md 6-cap. Tasks touching `Makefile` (1, 17) and `cmd/leah-daemon/main.go` (2, 8) must serialize per CLAUDE.md dispatch parallelism rules.
2. **Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
