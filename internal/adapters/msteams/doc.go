// Package msteams is Leah's adapter for Microsoft Teams via the Microsoft
// Graph v1.0 REST API. It exposes a narrow Client surface (ListTeams,
// ListChannels, PostMessage, Search) and routes every secret-touching RPC
// through the operator-attestation gate (Attestor) before loading the OAuth
// bearer. Direct REST against https://graph.microsoft.com/v1.0; the
// msgraph-sdk-go module is deferred to keep go.mod lean. Wiring (Azure AD
// app registration, device-code OAuth, brief enrichment, dispatcher post)
// lands in the W51 wiring wave; the constructor performs no I/O.
package msteams
