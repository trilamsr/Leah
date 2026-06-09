package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Entry struct {
	Timestamp   string  `json:"ts"`
	Kind        string  `json:"kind"`
	ArgsHash    string  `json:"args_hash"`
	BlastRadius int     `json:"blast_radius"`
	Outcome     string  `json:"outcome"`
	CostDollars float64 `json:"cost_dollars,omitempty"`
	Detail      string  `json:"detail,omitempty"`
}

type Logger struct {
	Path string
	Now  func() time.Time
}

func (l *Logger) Append(e Entry) error {
	if l.Now != nil {
		e.Timestamp = l.Now().Format(time.RFC3339)
	} else {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()
	buf, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	if _, err := f.Write(append(buf, '\n')); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	return nil
}
