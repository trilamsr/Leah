// Package eval scheduler — fixture-based scoring on a fixed cadence.
// Default OFF; cmd/leah-daemon gates on LEAH_EVAL=1.
package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

type Fixture struct {
	ID               string   `yaml:"id"`
	Question         string   `yaml:"question"`
	ExpectedContains []string `yaml:"expected_contains"`
}

type Summary struct {
	Total  int
	Passed int
}

type Scheduler struct {
	Asker       Asker
	FixturesDir string
	Store       *Store
	Interval    time.Duration
	Source      string
	Now         func() time.Time
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *Scheduler) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = time.Hour
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, _, err := s.RunOnce(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "eval scheduler: %v\n", err)
			}
		}
	}
}

func (s *Scheduler) RunOnce(ctx context.Context) (int64, Summary, error) {
	fixtures, err := loadFixtures(s.FixturesDir)
	if err != nil {
		return 0, Summary{}, fmt.Errorf("load fixtures: %w", err)
	}
	source := s.Source
	if source == "" {
		source = "scheduler"
	}
	rec := RunRecord{StartedAt: s.now(), Source: source}
	for _, f := range fixtures {
		actual, askErr := s.Asker.Ask(ctx, f.Question)
		res := ResultRecord{FixtureID: f.ID, Actual: actual}
		if askErr != nil {
			res.Reason = "ask_error: " + askErr.Error()
			rec.Results = append(rec.Results, res)
			continue
		}
		if reason := matchAll(actual, f.ExpectedContains); reason != "" {
			res.Reason = reason
		} else {
			res.Pass = true
			res.Score = 1.0
		}
		rec.Results = append(rec.Results, res)
	}
	runID, err := s.Store.RecordRun(ctx, rec)
	if err != nil {
		return 0, Summary{}, err
	}
	sum := Summary{Total: len(rec.Results)}
	for _, r := range rec.Results {
		if r.Pass {
			sum.Passed++
		}
	}
	return runID, sum, nil
}

func matchAll(actual string, expected []string) string {
	for _, s := range expected {
		if !strings.Contains(actual, s) {
			return fmt.Sprintf("must_contain missing: %q", s)
		}
	}
	return ""
}

func loadFixtures(dir string) ([]Fixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".yaml") && !strings.HasSuffix(n, ".yml") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Fixture, 0, len(names))
	for _, n := range names {
		path := filepath.Join(dir, n)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, fmt.Errorf("read %s: %w", n, rerr)
		}
		var f Fixture
		dec := yaml.NewDecoder(strings.NewReader(string(data)))
		dec.KnownFields(true)
		if derr := dec.Decode(&f); derr != nil {
			return nil, fmt.Errorf("decode %s: %w", n, derr)
		}
		if f.ID == "" {
			return nil, fmt.Errorf("fixture %s: missing id", n)
		}
		out = append(out, f)
	}
	return out, nil
}
