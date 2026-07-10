// Package watchlist persists the operator's ticker watchlist as
// {"symbols":[...]} so the morning-brief consumer can read it without
// a schema negotiation. Symbols are uppercase 1-5 letter tickers.
package watchlist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var symbolRE = regexp.MustCompile(`^[A-Z]{1,5}$`)

type Store struct {
	Path string
}

type doc struct {
	Symbols []string `json:"symbols"`
}

func (s *Store) List() ([]string, error) {
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	return d.Symbols, nil
}

func (s *Store) Add(sym string) error {
	norm, err := normalize(sym)
	if err != nil {
		return err
	}
	d, err := s.load()
	if err != nil {
		return err
	}
	for _, existing := range d.Symbols {
		if existing == norm {
			return nil
		}
	}
	d.Symbols = append(d.Symbols, norm)
	return s.save(d)
}

func (s *Store) Remove(sym string) error {
	norm, err := normalize(sym)
	if err != nil {
		return err
	}
	d, err := s.load()
	if err != nil {
		return err
	}
	kept := d.Symbols[:0]
	for _, existing := range d.Symbols {
		if existing != norm {
			kept = append(kept, existing)
		}
	}
	d.Symbols = kept
	return s.save(d)
}

func normalize(sym string) (string, error) {
	up := strings.ToUpper(strings.TrimSpace(sym))
	if !symbolRE.MatchString(up) {
		return "", fmt.Errorf("invalid symbol %q (want ^[A-Z]{1,5}$)", sym)
	}
	return up, nil
}

func (s *Store) load() (*doc, error) {
	d := &doc{Symbols: []string{}}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return d, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return d, nil
	}
	if err := json.Unmarshal(b, d); err != nil {
		return nil, err
	}
	if d.Symbols == nil {
		d.Symbols = []string{}
	}
	return d, nil
}

// save writes via tmp+rename so a crash mid-write can't corrupt the file
// the morning brief reads at 06:00.
func (s *Store) save(d *doc) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".watchlist-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, s.Path); err != nil {
		cleanup()
		return err
	}
	return nil
}
