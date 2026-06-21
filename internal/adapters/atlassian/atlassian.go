// Package atlassian holds the Basic-auth, status-mapping, and workspace-claim helpers shared by Leah's Confluence and Jira adapters.
package atlassian

import (
	"encoding/base64"
	"errors"
	"fmt"
)

var (
	ErrAuthFailed        = errors.New("atlassian: auth failed")
	ErrNotFound          = errors.New("atlassian: not found")
	ErrRateLimited       = errors.New("atlassian: rate limited")
	ErrUpstream          = errors.New("atlassian: upstream error")
	ErrWorkspaceMismatch = errors.New("atlassian: workspace claim mismatch")
)

func BasicAuthHeader(email, apiToken string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+apiToken))
}

func MapStatus(code int) error {
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == 401 || code == 403:
		return ErrAuthFailed
	case code == 404:
		return ErrNotFound
	case code == 429:
		return ErrRateLimited
	default:
		return fmt.Errorf("%w: status %d", ErrUpstream, code)
	}
}

func VerifyWorkspace(want, got string) error {
	if want == "" || want == got {
		return nil
	}
	return ErrWorkspaceMismatch
}
