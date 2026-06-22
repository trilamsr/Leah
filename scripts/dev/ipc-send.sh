#!/usr/bin/env bash
# Send a raw IPC frame to leah-daemon and print response frames until turn.end
# or timeout (default 10 s).
# Usage: ipc-send.sh '{"kind":"ask","turn_id":"dev-1","text":"hello"}'
#        ipc-send.sh --socket /path/to/leah.sock '{"kind":"ping"}'
set -euo pipefail

SOCK="${LEAH_SOCK:-$HOME/Library/Caches/Leah/leah.sock}"
PAYLOAD=""
TIMEOUT=10

while [ $# -gt 0 ]; do
  case "$1" in
    --socket) SOCK="$2"; shift 2 ;;
    --timeout) TIMEOUT="$2"; shift 2 ;;
    *) PAYLOAD="$1"; shift ;;
  esac
done

if [ -z "$PAYLOAD" ]; then
  echo "usage: ipc-send.sh [--socket PATH] [--timeout N] '{\"kind\":\"ask\",...}'" >&2
  exit 1
fi

if [ ! -S "$SOCK" ]; then
  echo "socket not found: $SOCK (is leah-daemon running?)" >&2
  exit 1
fi

# ipc-send requires a Go helper that speaks the length-prefix protocol.
# Compile on demand from the reference ipc-ping helper, patching in our payload.
TMPDIR_SEND="$(mktemp -d /tmp/leah-ipc-send-XXXXXX)"
trap 'rm -rf "$TMPDIR_SEND"' EXIT

REPO="$(cd "$(dirname "$0")/../.." && pwd)"

cat > "$TMPDIR_SEND/send.go" <<'GOEOF'
//go:build ignore

package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
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

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: send <socket> <json-payload>")
		os.Exit(1)
	}
	sock := os.Args[1]
	raw := os.Args[2]
	timeoutSec := 10
	if len(os.Args) >= 4 {
		fmt.Sscan(os.Args[3], &timeoutSec)
	}

	// Wrap bare text payloads into an ask frame.
	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		msg = map[string]interface{}{"kind": "ask", "turn_id": "dev-1", "seq": 0,
			"payload": map[string]string{"text": raw}}
	}
	// Ensure kind/turn_id/seq are set.
	if _, ok := msg["kind"]; !ok { msg["kind"] = "ask" }
	if _, ok := msg["turn_id"]; !ok { msg["turn_id"] = "dev-1" }
	if _, ok := msg["seq"]; !ok { msg["seq"] = 0 }

	body, _ := json.Marshal(msg)
	var f frame
	_ = json.Unmarshal(body, &f)

	c, err := net.Dial("unix", sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer c.Close()
	_ = c.(*net.UnixConn).SetDeadline(time.Now().Add(time.Duration(timeoutSec) * time.Second))

	if err := writeFrame(c, f); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}

	for {
		resp, err := readFrame(c)
		if err != nil {
			break
		}
		out, _ := json.Marshal(resp)
		fmt.Println(string(out))
		if resp.Kind == "turn.end" {
			break
		}
	}
}
GOEOF

go run "$TMPDIR_SEND/send.go" "$SOCK" "$PAYLOAD" "$TIMEOUT"
