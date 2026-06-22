package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/adapters/slack"
)

// fakeSlackTransport scripts each Transport method so runSlack tests stay
// network-free. Zero-value fields signal "method unexpected" via t.Errorf
// inside the test that wires it.
type fakeSlackTransport struct {
	channels    []slack.Channel
	postCalls   []postCall
	postErr     error
	thread      slack.Thread
	searchHits  []slack.Result
	searchErr   error
	listErr     error
	getThreadErr error
}

type postCall struct{ channel, text string }

func (f *fakeSlackTransport) ListChannels(_ context.Context, _ string) ([]slack.Channel, error) {
	return f.channels, f.listErr
}
func (f *fakeSlackTransport) PostMessage(_ context.Context, _, channel, text string) error {
	f.postCalls = append(f.postCalls, postCall{channel, text})
	return f.postErr
}
func (f *fakeSlackTransport) GetThread(_ context.Context, _, channel, ts string) (slack.Thread, error) {
	if f.getThreadErr != nil {
		return slack.Thread{}, f.getThreadErr
	}
	th := f.thread
	if th.Channel == "" {
		th.Channel = channel
	}
	if th.TS == "" {
		th.TS = ts
	}
	return th, nil
}
func (f *fakeSlackTransport) Search(_ context.Context, _, _ string) ([]slack.Result, error) {
	return f.searchHits, f.searchErr
}

// writeSlackToken stages a connect-shaped JSON token under stateDir so
// runSlack's token-path resolution finds it without the OAuth dance.
func writeSlackToken(t *testing.T, dir, token string) {
	t.Helper()
	path := filepath.Join(dir, "secrets", "slack-token.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"access_token": token})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

// TestSlackCLI_Help prints usage on -h.
func TestSlackCLI_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runSlack(context.Background(), []string{"--help"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(buf.String(), "usage: leah slack") {
		t.Errorf("missing usage line: %q", buf.String())
	}
}

// TestSlackCLI_NoSubcommand exits 2 with usage on bare invocation.
func TestSlackCLI_NoSubcommand(t *testing.T) {
	var buf bytes.Buffer
	if code := runSlack(context.Background(), nil, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestSlackCLI_NotConnected exits 2 with the connect hint when no token file.
func TestSlackCLI_NotConnected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	// stash stderr so the test runner doesn't dump the hint
	r, w, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	var buf bytes.Buffer
	code := runSlack(context.Background(), []string{"list"}, &buf)
	_ = w.Close()
	errBytes := make([]byte, 4096)
	n, _ := r.Read(errBytes)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(string(errBytes[:n]), "leah connect slack") {
		t.Errorf("expected connect hint in stderr, got %q", string(errBytes[:n]))
	}
}

// TestSlackCLI_Send_RequiresChannelAndText rejects send with no args.
func TestSlackCLI_Send_RequiresChannelAndText(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	writeSlackToken(t, dir, "xoxb-test")
	var buf bytes.Buffer
	if code := runSlack(context.Background(), []string{"send"}, &buf); code != 2 {
		t.Fatalf("send (no args): exit %d, want 2", code)
	}
	if code := runSlack(context.Background(), []string{"send", "C123"}, &buf); code != 2 {
		t.Fatalf("send (no text): exit %d, want 2", code)
	}
}

// TestSlackCLI_List_PrintsChannelsTable renders channels via the fake transport.
func TestSlackCLI_List_PrintsChannelsTable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	writeSlackToken(t, dir, "xoxb-test")
	tr := &fakeSlackTransport{channels: []slack.Channel{
		{ID: "C001", Name: "general"},
		{ID: "C002", Name: "random"},
	}}
	prev := slackTransportFactory
	slackTransportFactory = func() slack.Transport { return tr }
	defer func() { slackTransportFactory = prev }()

	var buf bytes.Buffer
	if code := runSlack(context.Background(), []string{"list"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	out := buf.String()
	for _, want := range []string{"C001", "general", "C002", "random"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

// TestSlackCLI_Send_HappyPath calls PostMessage with the parsed channel + text.
func TestSlackCLI_Send_HappyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	writeSlackToken(t, dir, "xoxb-test")
	tr := &fakeSlackTransport{}
	prev := slackTransportFactory
	slackTransportFactory = func() slack.Transport { return tr }
	defer func() { slackTransportFactory = prev }()

	var buf bytes.Buffer
	code := runSlack(context.Background(), []string{"send", "C001", "hello", "world"}, &buf)
	if code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	if len(tr.postCalls) != 1 {
		t.Fatalf("want 1 post call, got %d", len(tr.postCalls))
	}
	got := tr.postCalls[0]
	if got.channel != "C001" || got.text != "hello world" {
		t.Errorf("post=%+v, want {C001, hello world}", got)
	}
}

// TestSlackCLI_Send_TransportError surfaces underlying transport failures.
func TestSlackCLI_Send_TransportError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	writeSlackToken(t, dir, "xoxb-test")
	tr := &fakeSlackTransport{postErr: errors.New("boom")}
	prev := slackTransportFactory
	slackTransportFactory = func() slack.Transport { return tr }
	defer func() { slackTransportFactory = prev }()

	var buf bytes.Buffer
	if code := runSlack(context.Background(), []string{"send", "C001", "hi"}, &buf); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

// TestSlackCLI_Thread_RendersMessages prints replies returned by the fake.
func TestSlackCLI_Thread_RendersMessages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	writeSlackToken(t, dir, "xoxb-test")
	tr := &fakeSlackTransport{thread: slack.Thread{Replies: []slack.Message{
		{User: "U1", TS: "1.0", Text: "first"},
		{User: "U2", TS: "2.0", Text: "second"},
	}}}
	prev := slackTransportFactory
	slackTransportFactory = func() slack.Transport { return tr }
	defer func() { slackTransportFactory = prev }()

	var buf bytes.Buffer
	if code := runSlack(context.Background(), []string{"thread", "C001", "1.0"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	out := buf.String()
	for _, want := range []string{"U1", "first", "U2", "second"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

// TestSlackCLI_Search_TopHitFormat — output contains channel + permalink.
func TestSlackCLI_Search_TopHitFormat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	writeSlackToken(t, dir, "xoxp-test")
	tr := &fakeSlackTransport{searchHits: []slack.Result{
		{Channel: "C001", TS: "1.0", Snippet: "match alpha", URL: "https://slack.example/p1"},
		{Channel: "C002", TS: "2.0", Snippet: "match beta", URL: "https://slack.example/p2"},
	}}
	prev := slackTransportFactory
	slackTransportFactory = func() slack.Transport { return tr }
	defer func() { slackTransportFactory = prev }()

	var buf bytes.Buffer
	if code := runSlack(context.Background(), []string{"search", "alpha", "beta"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	out := buf.String()
	for _, want := range []string{"C001", "https://slack.example/p1", "match alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}
