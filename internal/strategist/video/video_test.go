package video

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAdapter scripts Submit + a sequence of Poll responses so the
// orchestrator's poll loop is exercised without timing-sensitive RPCs.
type fakeAdapter struct {
	mu          sync.Mutex
	submitID    string
	submitErr   error
	submitCalls int
	submitArgs  struct {
		prompt  string
		seconds int
	}
	pollSeq     []pollResult
	pollCalls   int
	pollNeverOK bool // if true, always returns "queued" — used to drive timeout
}

type pollResult struct {
	status string
	mp4    []byte
	err    error
}

func (f *fakeAdapter) SubmitClip(_ context.Context, prompt string, seconds int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitCalls++
	f.submitArgs.prompt = prompt
	f.submitArgs.seconds = seconds
	return f.submitID, f.submitErr
}

func (f *fakeAdapter) PollClip(_ context.Context, _ string) (string, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pollCalls++
	if f.pollNeverOK {
		return "queued", nil, nil
	}
	if f.pollCalls > len(f.pollSeq) {
		return "queued", nil, nil
	}
	r := f.pollSeq[f.pollCalls-1]
	return r.status, r.mp4, r.err
}

// fakeFFmpeg records its inputs and (when configured) reports a missing
// binary so the orchestrator's pre-flight LookPath check can be tested.
type fakeFFmpeg struct {
	burnCalls int
	burnArgs  struct {
		in, out, text, ratio string
	}
	burnErr error

	available bool // default false -> the "missing" path; set true in happy-path tests
}

func (f *fakeFFmpeg) Available() bool { return f.available }

func (f *fakeFFmpeg) BurnCaption(_ context.Context, in, out, text, ratio string) error {
	f.burnCalls++
	f.burnArgs.in = in
	f.burnArgs.out = out
	f.burnArgs.text = text
	f.burnArgs.ratio = ratio
	return f.burnErr
}

// fakeFetcher returns scripted bytes for the URL the orchestrator hands it.
type fakeFetcher struct {
	calls int
	body  []byte
	err   error
}

func (f *fakeFetcher) Get(_ context.Context, _ string) ([]byte, error) {
	f.calls++
	return f.body, f.err
}

// fakeFS records the final write so we can assert path + bytes without
// touching the real filesystem.
type fakeFS struct {
	tmpFiles  []string // sequence of WriteTemp paths
	tmpBytes  [][]byte // sequence of WriteTemp bytes
	finalPath string
	finalSrc  string // path the orchestrator handed to Persist
	removed   []string
}

func (f *fakeFS) WriteTemp(_, _ string, b []byte) (string, error) {
	// WHY: synthesize a path that BurnCaption can echo back; tests inspect
	// the recorded args, not the bytes.
	p := fmt.Sprintf("/tmp/fake-%d.mp4", len(f.tmpFiles))
	f.tmpFiles = append(f.tmpFiles, p)
	f.tmpBytes = append(f.tmpBytes, b)
	return p, nil
}

func (f *fakeFS) ReserveOutput(itemID, name string) (string, error) {
	return "/assets/" + itemID + "/" + name, nil
}

func (f *fakeFS) Persist(src, dst string) error {
	f.finalSrc = src
	f.finalPath = dst
	return nil
}

func (f *fakeFS) Remove(path string) { f.removed = append(f.removed, path) }

func happyOrchestrator(t *testing.T) (*Orchestrator, *fakeAdapter, *fakeFFmpeg, *fakeFetcher, *fakeFS) {
	t.Helper()
	a := &fakeAdapter{
		submitID: "job-42",
		pollSeq: []pollResult{
			{status: "queued"},
			{status: "ready", mp4: []byte("fakempegbytes")},
		},
	}
	ff := &fakeFFmpeg{available: true}
	fe := &fakeFetcher{body: []byte("fakempegbytes")}
	fs := &fakeFS{}
	o := New(Config{
		Adapter:      a,
		FFmpeg:       ff,
		Fetcher:      fe,
		FS:           fs,
		PollInterval: time.Millisecond,
		PollDeadline: 200 * time.Millisecond,
		Now:          time.Now,
	})
	return o, a, ff, fe, fs
}

func TestOrchestrator_HappyPath_SubmitPollDownloadComposeWrite(t *testing.T) {
	o, a, ff, _, fs := happyOrchestrator(t)
	clip, err := o.Compose(context.Background(), ItemRef{ID: "01J8AAAAAA"}, ClipRequest{
		Prompt:  "calm minimalist desk workflow b-roll",
		Seconds: 5,
		Caption: "shipping leah",
		Ratio:   Ratio9x16,
	})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if a.submitCalls != 1 {
		t.Fatalf("submit calls = %d, want 1", a.submitCalls)
	}
	if a.submitArgs.seconds != 5 || a.submitArgs.prompt == "" {
		t.Fatalf("submit args = %+v", a.submitArgs)
	}
	if a.pollCalls < 2 {
		t.Fatalf("poll calls = %d, want >=2", a.pollCalls)
	}
	if ff.burnCalls != 1 {
		t.Fatalf("burn calls = %d, want 1", ff.burnCalls)
	}
	if ff.burnArgs.text != "shipping leah" || ff.burnArgs.ratio != "9:16" {
		t.Fatalf("burn args = %+v", ff.burnArgs)
	}
	if fs.finalPath != "/assets/01J8AAAAAA/clip.mp4" || clip != fs.finalPath {
		t.Fatalf("final path = %q, clip = %q", fs.finalPath, clip)
	}
}

func TestOrchestrator_AdapterError_PropagatesWrapped(t *testing.T) {
	o, a, _, _, _ := happyOrchestrator(t)
	a.submitErr = errors.New("boom")
	_, err := o.Compose(context.Background(), ItemRef{ID: "x"}, ClipRequest{
		Prompt: "p", Seconds: 5, Ratio: Ratio9x16,
	})
	if err == nil || !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("err = %v, want wraps ErrAdapterFailed", err)
	}
}

func TestOrchestrator_PollTimeout_ReturnsErrTimeout(t *testing.T) {
	a := &fakeAdapter{submitID: "j", pollNeverOK: true}
	ff := &fakeFFmpeg{available: true}
	fe := &fakeFetcher{}
	fs := &fakeFS{}
	// WHY: deterministic clock — each call advances by 1s so the deadline
	// (100ms) is exceeded on the second now() check regardless of CI load.
	base := time.Unix(1_700_000_000, 0)
	calls := 0
	clock := func() time.Time {
		ts := base.Add(time.Duration(calls) * time.Second)
		calls++
		return ts
	}
	o := New(Config{
		Adapter: a, FFmpeg: ff, Fetcher: fe, FS: fs,
		PollInterval: time.Microsecond,
		PollDeadline: 100 * time.Millisecond,
		Now:          clock,
	})
	_, err := o.Compose(context.Background(), ItemRef{ID: "x"}, ClipRequest{
		Prompt: "p", Seconds: 5, Ratio: Ratio9x16,
	})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestOrchestrator_UnknownRatio_RejectedUpFront(t *testing.T) {
	o, a, _, _, _ := happyOrchestrator(t)
	_, err := o.Compose(context.Background(), ItemRef{ID: "x"}, ClipRequest{
		Prompt: "p", Seconds: 5, Ratio: "4:3",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown ratio") {
		t.Fatalf("err = %v, want unknown-ratio rejection", err)
	}
	if a.submitCalls != 0 {
		t.Fatalf("submit called %d times before validation", a.submitCalls)
	}
}

func TestOrchestrator_FFmpegMissing_ReturnsErrFFmpegMissing(t *testing.T) {
	o, _, ff, _, _ := happyOrchestrator(t)
	ff.available = false
	_, err := o.Compose(context.Background(), ItemRef{ID: "x"}, ClipRequest{
		Prompt: "p", Seconds: 5, Caption: "hi", Ratio: Ratio9x16,
	})
	if !errors.Is(err, ErrFFmpegMissing) {
		t.Fatalf("err = %v, want ErrFFmpegMissing", err)
	}
}

func TestOrchestrator_CaptionEmpty_SkipsBurnIn(t *testing.T) {
	o, _, ff, _, fs := happyOrchestrator(t)
	_, err := o.Compose(context.Background(), ItemRef{ID: "01J8BBBBBB"}, ClipRequest{
		Prompt: "p", Seconds: 5, Caption: "", Ratio: Ratio9x16,
	})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if ff.burnCalls != 0 {
		t.Fatalf("burn calls = %d, want 0", ff.burnCalls)
	}
	// WHY: caption-less path still writes the raw clip into the slot, so
	// the operator's queue item still has a `clip:` ref.
	if fs.finalPath != "/assets/01J8BBBBBB/clip.mp4" {
		t.Fatalf("final path = %q", fs.finalPath)
	}
}

func TestFFmpeg_BurnCaption_BuildsCorrectArgs(t *testing.T) {
	var got []string
	r := &DefaultFFmpegRunner{
		lookPath: func(string) (string, error) { return "/usr/bin/ffmpeg", nil },
		runCmd: func(_ context.Context, _ string, args []string) error {
			got = args
			return nil
		},
	}
	if !r.Available() {
		t.Fatalf("Available = false, want true (lookPath returns binary)")
	}
	if err := r.BurnCaption(context.Background(), "/in.mp4", "/out.mp4", "ship it; tonight", "9:16"); err != nil {
		t.Fatalf("BurnCaption: %v", err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "-i /in.mp4") {
		t.Fatalf("missing -i flag: %v", got)
	}
	if !strings.Contains(joined, "/out.mp4") {
		t.Fatalf("missing output path: %v", got)
	}
	if !strings.Contains(joined, "scale=1080:1920") {
		t.Fatalf("missing 9:16 scale filter: %v", got)
	}
	if !strings.Contains(joined, "drawtext=") {
		t.Fatalf("missing drawtext filter: %v", got)
	}
	// Sanitization: a literal `;` in caption must NOT appear unescaped in
	// the drawtext expression — drawtext uses `;` as filter-graph separator.
	if strings.Contains(joined, "ship it; tonight") {
		t.Fatalf("unescaped semicolon leaked into drawtext: %v", got)
	}
}

func TestFFmpeg_VerticalReformat_BuildsScaleArgs(t *testing.T) {
	var got []string
	r := &DefaultFFmpegRunner{
		lookPath: func(string) (string, error) { return "/usr/bin/ffmpeg", nil },
		runCmd: func(_ context.Context, _ string, args []string) error {
			got = args
			return nil
		},
	}
	if err := r.BurnCaption(context.Background(), "/in.mp4", "/out.mp4", "x", "9:16"); err != nil {
		t.Fatalf("BurnCaption: %v", err)
	}
	joined := strings.Join(got, " ")
	want := "scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920"
	if !strings.Contains(joined, want) {
		t.Fatalf("missing %q in args: %v", want, got)
	}
}

func TestFFmpeg_LandscapeReformat_BuildsScaleArgs(t *testing.T) {
	var got []string
	r := &DefaultFFmpegRunner{
		lookPath: func(string) (string, error) { return "/usr/bin/ffmpeg", nil },
		runCmd: func(_ context.Context, _ string, args []string) error {
			got = args
			return nil
		},
	}
	if err := r.BurnCaption(context.Background(), "/in.mp4", "/out.mp4", "x", "16:9"); err != nil {
		t.Fatalf("BurnCaption: %v", err)
	}
	joined := strings.Join(got, " ")
	want := "scale=1920:1080:force_original_aspect_ratio=increase,crop=1920:1080"
	if !strings.Contains(joined, want) {
		t.Fatalf("missing %q in args: %v", want, got)
	}
}

func TestSanitizeCaption_StripsFiltergraphMeta(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"clean text", "clean text"},
		{"a;b", "a b"},
		{"a:b", "a b"},
		{"a'b", "a b"},
		{"a\\b", "a b"},
		{"line1\nline2", "line1 line2"},
		{"back`tick", "back tick"},
		{"  hi  ", "hi"},
		// drawtext expansion: %{pts} would leak presentation timestamp.
		{"frame %{pts} time", "frame pts time"},
		{"a,b[c]d", "a b c d"},
	}
	for _, tc := range cases {
		got := sanitizeCaption(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeCaption(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOrchestrator_WritesFetchedBytesToTemp(t *testing.T) {
	o, _, _, _, fs := happyOrchestrator(t)
	_, err := o.Compose(context.Background(), ItemRef{ID: "x"}, ClipRequest{
		Prompt: "p", Seconds: 5, Caption: "c", Ratio: Ratio9x16,
	})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if len(fs.tmpFiles) == 0 || !bytes.Equal(fs.tmpBytes[0], []byte("fakempegbytes")) {
		t.Fatalf("expected raw bytes in first temp file; got %d files", len(fs.tmpFiles))
	}
}
