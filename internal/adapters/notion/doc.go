// Package notion is Leah's adapter for the Notion v1 REST API. It exposes a
// narrow Adapter surface (ListDatabases, QueryDatabase, GetPage, CreatePage)
// and routes every secret-touching RPC through the operator-attestation gate
// (Attestor) before loading the integration secret. Wiring (morning brief,
// leah ship --notion) composes the adapter via internal/dispatcher; the
// constructor performs no I/O so daemon shutdown stays deterministic.
package notion
