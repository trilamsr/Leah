package voice

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAITTS_CopyError_CapturesBodyPrefix asserts io.Copy failure wraps response body prefix.
func TestOpenAITTS_CopyError_CapturesBodyPrefix(t *testing.T) {
	const marker = "leah-diagnostic-marker"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Lie about content-length so io.Copy treats the short payload as a
		// truncation and surfaces ErrUnexpectedEOF.
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(marker))
		// Server-side hijack to close before sending the rest. httptest
		// flushes on handler return; the short write + content-length lie
		// is enough to make io.Copy fail with unexpected EOF.
	}))
	defer srv.Close()

	fe := newFakeExec()
	o := &OpenAITTS{APIKey: "sk-test", Exec: fe, Endpoint: srv.URL, HTTPClient: srv.Client()}
	err := o.Speak(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected io.Copy error, got nil")
	}
	if !strings.Contains(err.Error(), "copy") {
		t.Errorf("error missing copy stage: %v", err)
	}
	if !strings.Contains(err.Error(), marker) {
		t.Errorf("error missing body prefix marker %q: %v", marker, err)
	}
}

// TestOpenAITTS_CopyError_PrefixBounded asserts captured prefix is at most 512 bytes.
func TestOpenAITTS_CopyError_PrefixBounded(t *testing.T) {
	// 10 KB of a single byte so any captured prefix is trivially the same
	// repeating char; we count occurrences in the quoted error string.
	const big = 10 * 1024
	payload := strings.Repeat("A", big)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", big*100)) // lie to trigger copy failure
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	fe := newFakeExec()
	o := &OpenAITTS{APIKey: "sk-test", Exec: fe, Endpoint: srv.URL, HTTPClient: srv.Client()}
	err := o.Speak(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected io.Copy error, got nil")
	}
	// Count 'A' occurrences in the error message — must not exceed 512 (the
	// cap), proving the prefix was bounded before being wrapped.
	count := strings.Count(err.Error(), "A")
	if count > 512 {
		t.Fatalf("captured prefix exceeded 512 bytes: count=%d err=%v", count, err)
	}
	if count == 0 {
		t.Fatalf("captured prefix empty: %v", err)
	}
}

// TestOpenAITTS_HappyPath_NoBodyCapture asserts a valid 200 response does not surface body-capture noise.
func TestOpenAITTS_HappyPath_NoBodyCapture(t *testing.T) {
	// 4-byte payload, accurate content-length → io.Copy succeeds.
	payload := []byte("wav!")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	fe := newFakeExec()
	o := &OpenAITTS{APIKey: "sk-test", Exec: fe, Endpoint: srv.URL, HTTPClient: srv.Client()}
	if err := o.Speak(context.Background(), "hello"); err != nil {
		t.Fatalf("speak: %v", err)
	}
	if len(fe.runs) != 1 || fe.runs[0].name != "afplay" {
		t.Errorf("expected single afplay invocation, got %+v", fe.runs)
	}
}
