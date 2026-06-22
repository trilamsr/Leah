//go:build ignore

// diag-state sends a diag.state IPC frame to leah-daemon and prints the response.
// Run via: go run scripts/smoke/diag-state.go <socket-path>
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
		fmt.Fprintln(os.Stderr, "usage: diag-state <socket>")
		os.Exit(1)
	}
	c, err := net.Dial("unix", os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer c.Close()
	_ = writeFrame(c, frame{Kind: "diag.state", TurnID: "diag1", Seq: 0})
	f, err := readFrame(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(f, "", "  ")
	fmt.Println(string(out))
}
