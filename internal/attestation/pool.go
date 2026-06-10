// Package attestation hosts the operator-attestation question pool shared by
// the dispatcher's self-build path and the gmail/gcal adapters. Centralizing
// the pool lets future adapters reuse one question source instead of each
// inventing its own, while keeping selection deterministic-by-scope so audit
// rows can be replayed against the recorded pool state.
package attestation

import (
	"bufio"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strings"
)

// ErrUnknownScope is returned by Pick when scope was not registered at Load
// time — fail-closed; silent fallback to questions[0] would mask audit-row
// drift across adapter wiring waves.
var ErrUnknownScope = errors.New("attestation: scope not registered with pool")

// ErrEmptyPool is returned when the question file yields zero usable lines
// (all blanks/comments). Operator-habituation defense requires real entropy
// in the pool, so loading an empty file is treated as misconfiguration.
var ErrEmptyPool = errors.New("attestation: question pool is empty")

// Pool holds the parsed question list and the set of scopes allowed to draw
// from it. Constructed via Load; selection is via Pick.
type Pool struct {
	questions []string
	scopes    map[string]struct{}
}

// Load reads path (one question per line; #-prefixed lines + blanks ignored)
// and registers scopes as the allow-list for Pick. Questions are sorted at
// load so the scope→question mapping is stable across runs regardless of
// editor reordering of the source file.
func Load(path string, scopes ...string) (*Pool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open attestation file: %w", err)
	}
	defer func() { _ = f.Close() }()
	var qs []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		qs = append(qs, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan attestation file: %w", err)
	}
	if len(qs) == 0 {
		return nil, ErrEmptyPool
	}
	sort.Strings(qs)
	sc := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		sc[s] = struct{}{}
	}
	return &Pool{questions: qs, scopes: sc}, nil
}

// Len returns the number of questions in the pool.
func (p *Pool) Len() int { return len(p.questions) }

// Pick returns the question deterministically assigned to scope. Same scope +
// same pool state always returns the same question — load-bearing for audit
// replay: a recorded scope can be re-resolved to verify which question the
// operator was prompted with. Unknown scope errors fail-closed (no silent
// fallback to questions[0], which would mask scope drift in adapter wiring).
func (p *Pool) Pick(scope string) (string, error) {
	if _, ok := p.scopes[scope]; !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownScope, scope)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(scope))
	return p.questions[int(h.Sum32())%len(p.questions)], nil
}
