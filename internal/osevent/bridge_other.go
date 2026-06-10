//go:build !darwin

package osevent

import (
	"context"
	"errors"
)

// NewSource returns a no-op Source on non-darwin builds. Subscribe yields an
// immediately-closed channel; the daemon stays push-source agnostic so a Linux
// dev box compiles and runs without macOS-specific shims.
func NewSource(_ Config) (Source, error) {
	return &noopSource{}, nil
}

type noopSource struct{}

func (n *noopSource) Subscribe(_ context.Context, _ Kind) (<-chan Event, error) {
	ch := make(chan Event)
	close(ch)
	return ch, nil
}

func (n *noopSource) Close() error { return nil }

// ErrUnsupported is returned by helpers that require darwin runtime support.
var ErrUnsupported = errors.New("osevent: unsupported on non-darwin builds")
