// Package linear is Leah's adapter for Linear's GraphQL API. It exposes a
// narrow Client surface (ListMyIssues, GetIssue, CreateIssue, Comment) and
// routes every secret-touching RPC through the operator-attestation gate
// (Attestor) before loading the API key. A shared 10/60s write budget caps
// create+comment burst-spam before consent is prompted; reads are unlimited.
// HTTPTransport speaks GraphQL directly over net/http — genqlient is deferred
// to the wiring wave per adopt-over-build.
package linear
