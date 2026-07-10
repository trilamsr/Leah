package elevenlabs_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/actions/tts"
	"github.com/trilam/leah/internal/actions/tts/elevenlabs"
)

// Plan §17.17: POST /v1/text-to-speech/<voice>/stream with model_id eleven_flash_v2_5.
func TestClient_Speak_PostsExpectedPayload(t *testing.T) {
	var gotPath, gotModel, gotKey, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotKey = r.Header.Get("xi-api-key")
		gotCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"eleven_flash_v2_5"`) {
			gotModel = "eleven_flash_v2_5"
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("FAKE_MP3"))
	}))
	defer srv.Close()

	c := elevenlabs.New("k", "vid", srv.Client())
	c.SetBaseURL(srv.URL)
	stream, err := c.Speak(context.Background(), "hi", "")
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	defer func() { _ = stream.Close() }()
	out, _ := io.ReadAll(stream)
	if string(out) != "FAKE_MP3" {
		t.Fatalf("body: %q", out)
	}
	if !strings.Contains(gotPath, "/v1/text-to-speech/vid/stream") {
		t.Fatalf("path: %q", gotPath)
	}
	if !strings.Contains(gotPath, "optimize_streaming_latency=4") || !strings.Contains(gotPath, "output_format=mp3_44100_128") {
		t.Fatalf("query: %q", gotPath)
	}
	if gotModel != "eleven_flash_v2_5" {
		t.Fatalf("model: %q", gotModel)
	}
	if gotKey != "k" {
		t.Fatalf("api key header: %q", gotKey)
	}
	if gotCT != "application/json" {
		t.Fatalf("content type: %q", gotCT)
	}
	if stream.MIME() != "audio/mpeg" {
		t.Fatalf("mime: %q", stream.MIME())
	}
}

// Name is the §17.17 provider identifier.
func TestClient_Name(t *testing.T) {
	c := elevenlabs.New("k", "vid", nil)
	if c.Name() != "elevenlabs" {
		t.Fatalf("name: %q", c.Name())
	}
}

// Voice arg overrides constructor voice; empty falls back.
func TestClient_Speak_VoiceArgOverrides(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "audio/mpeg")
	}))
	defer srv.Close()
	c := elevenlabs.New("k", "default-vid", srv.Client())
	c.SetBaseURL(srv.URL)
	stream, err := c.Speak(context.Background(), "x", "override-vid")
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	_ = stream.Close()
	if !strings.Contains(gotPath, "/v1/text-to-speech/override-vid/stream") {
		t.Fatalf("path: %q", gotPath)
	}
}

// Cancel mid-stream must terminate the read; ctx.Cancel propagates via the body.
func TestClient_Speak_CancelMidStream(t *testing.T) {
	released := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("first-chunk"))
			f.Flush()
		}
		<-r.Context().Done()
		once.Do(func() { close(released) })
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := elevenlabs.New("k", "vid", srv.Client())
	c.SetBaseURL(srv.URL)
	stream, err := c.Speak(ctx, "hi", "")
	if err != nil {
		t.Fatalf("speak: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("first read: %v", err)
	}

	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(stream)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected error after cancel, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("read did not unblock within 2s of cancel — ctx not honored")
	}
	_ = stream.Close()

	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatalf("server handler did not see ctx.Done within 2s")
	}
}

// Non-2xx surfaces a clear error and closes the body (no leak).
func TestClient_Speak_NonOKError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"bad key"}`))
	}))
	defer srv.Close()
	c := elevenlabs.New("k", "vid", srv.Client())
	c.SetBaseURL(srv.URL)
	_, err := c.Speak(context.Background(), "hi", "")
	if err == nil {
		t.Fatalf("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error missing status: %v", err)
	}
}

// Transport error wraps cleanly.
func TestClient_Speak_TransportError(t *testing.T) {
	c := elevenlabs.New("k", "vid", &http.Client{Transport: errTransport{}})
	c.SetBaseURL("http://127.0.0.1:1")
	_, err := c.Speak(context.Background(), "hi", "")
	if err == nil {
		t.Fatalf("expected transport error")
	}
}

type errTransport struct{}

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial refused")
}

// Provider interface satisfied — compile-time check.
func TestClient_ImplementsProvider(t *testing.T) {
	var _ tts.Provider = elevenlabs.New("k", "vid", nil)
}

// PreWarm issues a request and drains.
func TestClient_PreWarm(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()
	c := elevenlabs.New("k", "vid", srv.Client())
	c.SetBaseURL(srv.URL)
	if err := c.PreWarm(context.Background()); err != nil {
		t.Fatalf("prewarm: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 hit, got %d", hits)
	}
}
