// Package recommend owns the "apply" station of Leah's
// observe→cluster→propose→recommend→confirm-or-auto→feedback loop.
//
// W15 ships an in-memory Engine skeleton plus the public surface
// (Recommendation, Tier, Action, Engine). Storage (W16) and
// operatormodel feedback (W18) wire in later — keeping this layer
// pure lets the unit tests stay hermetic and the dependency arrow
// one-way (recommend → operatormodel, never back).
//
// See docs/engineer/specs/2026-06-10-learn-recommend-apply.md.
package recommend
