//go:build ignore

// phase2-probe is the embedded Go helper for scripts/smoke/phase2-e2e.sh.
// Each mode asserts one Phase 2 runtime invariant against the production
// internal/ APIs (read-only — does not export anything).
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/embed"
	"github.com/trilam/leah/internal/hud"
	"github.com/trilam/leah/internal/ipc"
)

type frame struct {
	Kind    string          `json:"kind"`
	TurnID  string          `json:"turn_id"`
	Seq     uint64          `json:"seq"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func writeFrame(w io.Writer, f frame) error {
	body, _ := json.Marshal(f)
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

func readFrame(r io.Reader) (frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return frame{}, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return frame{}, err
	}
	var f frame
	_ = json.Unmarshal(body, &f)
	return f, nil
}

func die(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "probe: "+msg+"\n", args...)
	os.Exit(1)
}

// localTableName mirrors internal/embed/embed.go:tableName so the probe can
// assert the BGE table name without exporting the production helper. Drift
// is caught by internal/embed unit tests on every build.
func localTableName(modelID string, dim int) string {
	var b strings.Builder
	b.WriteString("embeddings_")
	for _, r := range strings.ToLower(modelID) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	fmt.Fprintf(&b, "_%d", dim)
	return b.String()
}

func main() {
	if len(os.Args) < 2 {
		die("usage: phase2-probe <mode> [args...]")
	}
	switch os.Args[1] {

	case "widget-mount":
		sock := os.Args[2]
		c, err := net.Dial("unix", sock)
		if err != nil {
			die("dial: %v", err)
		}
		defer c.Close()
		_ = c.(*net.UnixConn).SetDeadline(time.Now().Add(30 * time.Second))
		payload := json.RawMessage(`{"text":"show me a market widget for AAPL"}`)
		if err := writeFrame(c, frame{Kind: "ask", TurnID: "phase2-w1", Seq: 0, Payload: payload}); err != nil {
			die("write: %v", err)
		}
		saw := false
		for {
			f, err := readFrame(c)
			if err != nil {
				break
			}
			if f.Kind == "widget.mount" {
				saw = true
				break
			}
			if f.Kind == "turn.end" {
				break
			}
		}
		if !saw {
			die("no widget.mount frame returned")
		}
		fmt.Println("ok")

	case "pin-cap":
		dir := os.Args[2]
		pinPath := filepath.Join(dir, "pinned-widgets.json")
		regPath := filepath.Join(dir, "widget-registry.json")
		_ = os.WriteFile(regPath, []byte(`{}`), 0o644)
		w, err := hud.NewWatcher(pinPath, regPath)
		if err != nil {
			die("NewWatcher: %v", err)
		}
		defer w.Close()
		mk := func(id string) hud.PinnedEntry {
			return hud.PinnedEntry{ID: id, Type: "market", Props: json.RawMessage(`{}`), Refresh: 60}
		}
		var toast *ipc.Frame
		for i, id := range []string{"w1", "w2", "w3"} {
			t, err := w.Pin(mk(id))
			if err != nil {
				die("Pin %d: %v", i, err)
			}
			if i == 2 {
				toast = t
			}
		}
		if toast == nil {
			die("third Pin produced no toast — cap not enforced")
		}
		if toast.Kind != "notification.toast" {
			die("toast kind %q, want notification.toast", toast.Kind)
		}
		var p struct {
			Level string `json:"level"`
			Text  string `json:"text"`
		}
		if err := json.Unmarshal(toast.Payload, &p); err != nil {
			die("unmarshal toast: %v", err)
		}
		if p.Level != "info" || p.Text == "" {
			die("toast payload bad: level=%q text=%q", p.Level, p.Text)
		}
		list, err := w.List()
		if err != nil {
			die("List: %v", err)
		}
		if len(list) != 2 {
			die("post-cap list len=%d, want 2", len(list))
		}
		fmt.Println("ok")

	case "debounce":
		dir := os.Args[2]
		pinPath := filepath.Join(dir, "pinned-widgets.json")
		regPath := filepath.Join(dir, "widget-registry.json")
		_ = os.WriteFile(regPath, []byte(`{}`), 0o644)
		_ = os.WriteFile(pinPath, []byte(`[]`), 0o644)
		w, err := hud.NewWatcher(pinPath, regPath)
		if err != nil {
			die("NewWatcher: %v", err)
		}
		defer w.Close()
		events := make(chan hud.PinnedChanged, 16)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = w.Run(ctx, func(e hud.PinnedChanged) { events <- e }) }()
		time.Sleep(150 * time.Millisecond)
		for i := 0; i < 3; i++ {
			body := fmt.Sprintf(`[{"id":"w%d","type":"market","props":{},"refresh":60}]`, i)
			if err := os.WriteFile(pinPath, []byte(body), 0o644); err != nil {
				die("write burst %d: %v", i, err)
			}
			time.Sleep(30 * time.Millisecond)
		}
		time.Sleep(700 * time.Millisecond)
		cancel()
		got := len(events)
		if got != 1 {
			die("debounce: got %d PinnedChanged, want 1", got)
		}
		fmt.Println("ok")

	case "bge-table":
		os.Setenv("LEAH_EMBED_BACKEND", "bge")
		g, err := embed.SelectGenerator()
		if err != nil {
			die("SelectGenerator: %v", err)
		}
		if g.Name() != "bge-small-en-v1.5" || g.Dim() != 384 {
			die("bge generator mismatch: name=%q dim=%d", g.Name(), g.Dim())
		}
		want := "embeddings_bge_small_en_v1_5_384"
		got := localTableName(g.Name(), g.Dim())
		if got != want {
			die("bge table name: got %q want %q", got, want)
		}
		fmt.Println("ok")

	default:
		die("unknown mode %q", os.Args[1])
	}
}
