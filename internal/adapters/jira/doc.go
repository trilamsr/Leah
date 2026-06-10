// Package jira is Leah's adapter for Atlassian Cloud Jira (REST v3). It
// exposes a narrow Client surface (ListMyIssues, GetIssue, CreateIssue,
// Comment) and routes every secret-touching RPC through the operator-
// attestation gate (Attestor) before loading the API token. Daemon
// integration (morning-brief assigned-issues, ship-from-dispatcher) composes
// this adapter via internal/dispatcher; the constructor performs no I/O so
// wiring stays deterministic.
package jira
