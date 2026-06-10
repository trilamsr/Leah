// Package gcal is Leah's Google Calendar adapter: a typed seam over
// google.golang.org/api/calendar/v3 (wired in a later wave) that the
// morning-brief flow reads and the meeting-create flow writes. Every RPC
// crosses the operator-attestation gate BEFORE the OAuth bearer loads, so
// a denied action can never materialize the token. MVP exposes list-today
// and create-event; richer surface lands when a concrete caller files an
// issue citing the gap (spec docs/engineer/specs/2026-06-09-gcal-adapter.md).
package gcal
