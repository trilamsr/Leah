// Package attestation hosts the shared operator-attestation question pool.
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

var ErrUnknownScope = errors.New("attestation: scope not registered with pool")

var ErrEmptyPool = errors.New("attestation: question pool is empty")

type Pool struct {
	questions []string
	scopes    map[string]struct{}
}

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
