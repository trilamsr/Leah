// Package loop wires wake → STT → intent → reasoner → TTS into the single
// goroutine that owns voice-turn arbitration. Structural seams keep
// dispatcher / reasoner / planner imports out of this layer.
package loop
