package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/testutil"
	"github.com/trilam/leah/internal/tts"
)

type fakeProvider struct {
	name      string
	spokeText string
	mu        sync.Mutex
	// blockUntilCtxDone: if true, Speak returns a stream whose Read blocks until ctx.Done.
	blockUntilCtxDone bool
	streamErr         error
	mime              string
	chunks            [][]byte
	speakCtxErr       error // captured ctx.Err() seen by Read when blocking
	calls             int
}

func (f *fakeProvider) Name() string                    { return f.name }
func (f *fakeProvider) PreWarm(_ context.Context) error { return nil }

func (f *fakeProvider) Speak(ctx context.Context, text, _ string) (tts.AudioStream, error) {
	f.mu.Lock()
	f.calls++
	f.spokeText = text
	f.mu.Unlock()
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	if f.blockUntilCtxDone {
		return &ctxBlockingStream{
			ctx: ctx,
			mime: f.mime,
			onDone: func() { f.mu.Lock(); f.speakCtxErr = ctx.Err(); f.mu.Unlock() },
		}, nil
	}
	return &chunkStream{chunks: f.chunks, mime: f.mime}, nil
}

func (f *fakeProvider) Spoke() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.spokeText
}

type chunkStream struct {
	chunks [][]byte
	mime   string
	i      int
	closed bool
}

func (s *chunkStream) Read(p []byte) (int, error) {
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	if s.i >= len(s.chunks) {
		return 0, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	n := copy(p, c)
	return n, nil
}

func (s *chunkStream) Close() error { s.closed = true; return nil }
func (s *chunkStream) MIME() string { return s.mime }

type ctxBlockingStream struct {
	ctx    context.Context
	mime   string
	onDone func()
	once   sync.Once
}

func (s *ctxBlockingStream) Read(p []byte) (int, error) {
	<-s.ctx.Done()
	s.once.Do(s.onDone)
	return 0, s.ctx.Err()
}

func (s *ctxBlockingStream) Close() error { return nil }
func (s *ctxBlockingStream) MIME() string { return s.mime }

type fakeClassifier struct{ route tts.Route }

func (f *fakeClassifier) Route(_ string) tts.Route { return f.route }

func TestHandleTTSSpeak_CloudRoute(t *testing.T) {
	cloud := &fakeProvider{name: "elevenlabs", chunks: [][]byte{[]byte("hello"), []byte("world")}, mime: "audio/mpeg"}
	local := &fakeProvider{name: "apple-ava"}
	reg := newTTSRegistry()
	cls := &fakeClassifier{route: tts.RouteCloud}
	payload, _ := json.Marshal(map[string]string{"text": "hello world", "voice": tts.DefaultVoice})
	req := ipc.Frame{Kind: ipc.KindTTSSpeak, TurnID: "t1", Seq: 1, Payload: payload}
	ch, err := handleTTSSpeak(context.Background(), req, cloud, local, cls, reg)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	var frames []ipc.Frame
	for f := range ch {
		frames = append(frames, f)
	}
	if cloud.Spoke() != "hello world" {
		t.Fatalf("cloud not invoked, spoke=%q", cloud.Spoke())
	}
	if local.Spoke() != "" {
		t.Fatalf("local must NOT be invoked on cloud route, spoke=%q", local.Spoke())
	}
	// Expect: 2 cloud.frame + 1 done.
	if len(frames) < 3 {
		t.Fatalf("want >=3 frames, got %d: %+v", len(frames), frames)
	}
	if frames[0].Kind != ipc.KindTTSCloudFrame {
		t.Fatalf("first frame kind: want %q got %q", ipc.KindTTSCloudFrame, frames[0].Kind)
	}
	if frames[len(frames)-1].Kind != ipc.KindTTSSpeakDone {
		t.Fatalf("last frame kind: want %q got %q", ipc.KindTTSSpeakDone, frames[len(frames)-1].Kind)
	}
}

func TestHandleTTSSpeak_LocalRoute(t *testing.T) {
	cloud := &fakeProvider{name: "elevenlabs"}
	local := &fakeProvider{name: "apple-ava"}
	reg := newTTSRegistry()
	cls := &fakeClassifier{route: tts.RouteLocal}
	payload, _ := json.Marshal(map[string]string{"text": "balance $100", "voice": tts.DefaultVoice})
	req := ipc.Frame{Kind: ipc.KindTTSSpeak, TurnID: "t2", Seq: 1, Payload: payload}
	ch, err := handleTTSSpeak(context.Background(), req, cloud, local, cls, reg)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	var frames []ipc.Frame
	for f := range ch {
		frames = append(frames, f)
	}
	if local.Spoke() != "balance $100" {
		t.Fatalf("local not invoked, spoke=%q", local.Spoke())
	}
	if cloud.Spoke() != "" {
		t.Fatalf("cloud must NOT be invoked on local route, spoke=%q", cloud.Spoke())
	}
	// Apple route emits a single tts.apple.speak frame (HUD plays locally).
	if len(frames) != 1 {
		t.Fatalf("want 1 frame on local route, got %d", len(frames))
	}
	if frames[0].Kind != ipc.KindTTSAppleSpeak {
		t.Fatalf("local route frame kind: want %q got %q", ipc.KindTTSAppleSpeak, frames[0].Kind)
	}
}

// Cancel MUST propagate ctx.Cancel into the in-flight Provider call —
// not merely drop the registration. Verified by observing the Provider's
// blocking Read return with ctx.Err() == context.Canceled.
func TestHandleTTSCancel_PropagatesCtxCancel(t *testing.T) {
	cloud := &fakeProvider{name: "elevenlabs", blockUntilCtxDone: true, mime: "audio/mpeg"}
	local := &fakeProvider{name: "apple-ava"}
	reg := newTTSRegistry()
	cls := &fakeClassifier{route: tts.RouteCloud}

	speakReq := ipc.Frame{
		Kind:    ipc.KindTTSSpeak,
		TurnID:  "cancel-1",
		Seq:     1,
		Payload: json.RawMessage(`{"text":"long","voice":""}`),
	}
	speakCh, err := handleTTSSpeak(context.Background(), speakReq, cloud, local, cls, reg)
	if err != nil {
		t.Fatalf("speak: %v", err)
	}

	testutil.Eventually(t, time.Second, time.Millisecond, func() bool { return reg.has("cancel-1") })

	cancelReq := ipc.Frame{Kind: ipc.KindTTSCancel, TurnID: "cancel-1", Seq: 1}
	cancelCh, err := handleTTSCancel(context.Background(), cancelReq, reg)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	cancelFrames := drain(cancelCh)
	if len(cancelFrames) != 1 || cancelFrames[0].Kind != ipc.KindTTSCancelOK {
		t.Fatalf("cancel must emit one %q frame, got %+v", ipc.KindTTSCancelOK, cancelFrames)
	}

	// Speak channel must close within bounded time (ctx propagated to Read).
	done := make(chan struct{})
	go func() {
		for range speakCh {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("speak channel did not close after cancel — ctx.Cancel did not propagate to Provider")
	}

	// Provider's Read must have observed context.Canceled.
	cloud.mu.Lock()
	gotErr := cloud.speakCtxErr
	cloud.mu.Unlock()
	if !errors.Is(gotErr, context.Canceled) {
		t.Fatalf("provider ctx.Err: want context.Canceled, got %v", gotErr)
	}

	// Registration must be cleared after cancel completes.
	if reg.has("cancel-1") {
		t.Fatal("registration must be removed after cancel")
	}
}

// Cancel for an unknown turn id must be a no-op ack — never panic.
func TestHandleTTSCancel_UnknownTurnIsNoop(t *testing.T) {
	reg := newTTSRegistry()
	cancelReq := ipc.Frame{Kind: ipc.KindTTSCancel, TurnID: "ghost", Seq: 1}
	ch, err := handleTTSCancel(context.Background(), cancelReq, reg)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	frames := drain(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindTTSCancelOK {
		t.Fatalf("want one cancel.ok frame, got %+v", frames)
	}
}

// Normal stream completion must clear the registration entry — no leak.
func TestHandleTTSSpeak_RegistrationClearedOnDone(t *testing.T) {
	cloud := &fakeProvider{name: "elevenlabs", chunks: [][]byte{[]byte("x")}, mime: "audio/mpeg"}
	local := &fakeProvider{name: "apple-ava"}
	reg := newTTSRegistry()
	cls := &fakeClassifier{route: tts.RouteCloud}
	req := ipc.Frame{Kind: ipc.KindTTSSpeak, TurnID: "done-1", Seq: 1, Payload: json.RawMessage(`{"text":"x"}`)}
	ch, err := handleTTSSpeak(context.Background(), req, cloud, local, cls, reg)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	for range ch {
	}
	if reg.has("done-1") {
		t.Fatal("registration must be cleared after stream completion")
	}
}

// Provider error mid-stream must be reported via tts.speak.err + close.
func TestHandleTTSSpeak_ProviderErrorEmitsErrFrame(t *testing.T) {
	cloud := &fakeProvider{name: "elevenlabs", streamErr: errors.New("upstream 503")}
	local := &fakeProvider{name: "apple-ava"}
	reg := newTTSRegistry()
	cls := &fakeClassifier{route: tts.RouteCloud}
	req := ipc.Frame{Kind: ipc.KindTTSSpeak, TurnID: "err-1", Seq: 1, Payload: json.RawMessage(`{"text":"x"}`)}
	ch, err := handleTTSSpeak(context.Background(), req, cloud, local, cls, reg)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	frames := drain(ch)
	if len(frames) != 1 || frames[0].Kind != ipc.KindTTSSpeakErr {
		t.Fatalf("want one %q frame, got %+v", ipc.KindTTSSpeakErr, frames)
	}
}

// Voice defaults to tts.DefaultVoice when payload omits it.
func TestHandleTTSSpeak_DefaultsVoice(t *testing.T) {
	var gotVoice string
	cloud := &voiceCapturingProvider{name: "elevenlabs", capture: &gotVoice}
	local := &fakeProvider{name: "apple-ava"}
	reg := newTTSRegistry()
	cls := &fakeClassifier{route: tts.RouteCloud}
	req := ipc.Frame{Kind: ipc.KindTTSSpeak, TurnID: "v-1", Seq: 1, Payload: json.RawMessage(`{"text":"hi"}`)}
	ch, err := handleTTSSpeak(context.Background(), req, cloud, local, cls, reg)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	for range ch {
	}
	if gotVoice != tts.DefaultVoice {
		t.Fatalf("voice default: got %q want %q", gotVoice, tts.DefaultVoice)
	}
}

type voiceCapturingProvider struct {
	name    string
	capture *string
}

func (p *voiceCapturingProvider) Name() string                    { return p.name }
func (p *voiceCapturingProvider) PreWarm(_ context.Context) error { return nil }
func (p *voiceCapturingProvider) Speak(_ context.Context, _, voice string) (tts.AudioStream, error) {
	*p.capture = voice
	return &chunkStream{}, nil
}

func drain(ch <-chan ipc.Frame) []ipc.Frame {
	var out []ipc.Frame
	for f := range ch {
		out = append(out, f)
	}
	return out
}
