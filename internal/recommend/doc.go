// Package recommend owns the "apply" station of Leah's
// observe→cluster→propose→recommend→confirm-or-auto→feedback loop.
//
// The public surface is Recommendation, Tier, Action, Engine. Keeping this
// layer pure lets unit tests stay hermetic and the dependency arrow one-way
// (recommend → operatormodel, never back).
package recommend
