//go:build ignore

// ipc-ping connects to the leah-daemon Unix socket, sends an `ask` frame,
// and asserts at least one prose.delta + a turn.end frame come back.
// Run via: go run scripts/smoke/ipc-ping.go <socket-path>
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
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
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: ipc-ping <socket>")
		os.Exit(1)
	}
	c, err := net.Dial("unix", os.Args[1])
	if err != nil {
		fmt.Println("dial:", err)
		os.Exit(1)
	}
	defer c.Close()

	_ = writeFrame(c, frame{
		Kind:    "ask",
		TurnID:  "smoke1",
		Seq:     0,
		Payload: json.RawMessage(`{"text":"reply with just OK"}`),
	})

	sawProse, sawEnd := false, false
	for !sawEnd {
		f, err := readFrame(c)
		if err != nil {
			fmt.Println("read:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "frame: %s\n", f.Kind)
		if f.Kind == "prose.delta" {
			sawProse = true
		}
		if f.Kind == "turn.end" {
			sawEnd = true
		}
	}

	if sawProse && sawEnd {
		fmt.Println("phase1 e2e ok")
	} else {
		fmt.Println("missing frames: sawProse=", sawProse, "sawEnd=", sawEnd)
		os.Exit(1)
	}
}
