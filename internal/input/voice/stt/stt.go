// Package stt is the local-default streaming speech-to-text contract.
// Spec §1.3.1: audio is 16 kHz mono PCM16; the daemon owns the stream so
// raw frames never cross the IPC boundary into the HUD process.
package stt

import (
	"context"
	"time"
)

type AudioFrame struct {
	PCM        []int16
	SampleRate int
	Ts         time.Time
}

type Partial struct {
	Text       string
	IsFinal    bool
	Confidence float64
	LatencyMS  int
}

type ProviderInfo struct {
	Name    string
	IsLocal bool
	ModelID string
	RAMmb   int
}

type STT interface {
	Stream(ctx context.Context, audio <-chan AudioFrame) (<-chan Partial, error)
	Info() ProviderInfo
}
