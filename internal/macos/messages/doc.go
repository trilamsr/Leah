// Package messages reads iMessage/SMS history from Messages.app's chat.db
// (read-only, attestation-gated) and exposes them via the MacApp interface
// per docs/engineer/specs/2026-06-10-macos-ecosystem-integration.md (W30).
//
// Privacy: message bodies are HIGH-sensitivity per spec §7. Audit rows
// store only the sha256[:8] of Item.Body; plaintext is returned to the
// operator-direct call site and MUST NOT be marshaled into a reasoner
// prompt.
package messages
