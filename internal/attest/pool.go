package attest

import (
	"bufio"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strings"
)

// ErrUnknownScope is returned by Pick when scope was not registered at Load time (fail-closed).
var ErrUnknownScope = errors.New("attestation: scope not registered with pool")

// ErrEmptyPool signals the question file yielded zero usable lines; validation gate at Load.
var ErrEmptyPool = errors.New("attestation: question pool is empty")

// Pool holds parsed questions + the scope set authorized to draw from them.
type Pool struct {
	questions []string
	scopes    map[string]struct{}
}

// Load parses the question file and registers the scope allowlist (fail-closed on empty pool).
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

// Len exposes question count so tests can assert pool size after Load without reflection.
func (p *Pool) Len() int { return len(p.questions) }

// Pick fails-closed on unknown scope; silent fallback would mask audit-row drift across adapter wiring.
func (p *Pool) Pick(scope string) (string, error) {
	if _, ok := p.scopes[scope]; !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownScope, scope)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(scope))
	return p.questions[int(h.Sum32())%len(p.questions)], nil
}
