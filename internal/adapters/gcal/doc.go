// Package gcal is Leah's Google Calendar adapter: a typed seam over
// google.golang.org/api/calendar/v3 that the morning-brief and
// meeting-create flows depend on. OAuth tokens are read (never written)
// from a path managed by the operator-attestation gate in
// internal/dispatcher/selfbuild.go + internal/audit, so a token refresh
// always crosses an auditable boundary. The MVP exposes only list-today
// and create-event; richer surface area lands when a concrete caller
// asks for it.
package gcal
