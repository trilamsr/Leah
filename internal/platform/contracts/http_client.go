package contracts

import "net/http"

// HTTPClient is the smallest stable seam over net/http.Client.
// Adapters depend on this, not the concrete *http.Client, so tests
// inject httptest-backed fakes and prod injects singleton clients
// (see internal/voice/openai for the singleton pattern).
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}
