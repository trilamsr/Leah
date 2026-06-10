// Package sources composes pattern signals from external observability
// seams (macOS mirror DB, knowledge graph) into recommend.Recommendation
// candidates. Each Source is independent — one erroring never blocks
// another. Wiring into recommend.Engine.Propose lands in a follow-up wave;
// this package only delivers the Source interface plus two implementations.
package sources
