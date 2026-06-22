package reasoner

import (
	"context"
	"testing"
)

// haikuStub fakes the Haiku one-shot call; returns a fixed JSON blob.
type haikuStub struct{ resp string }

func (h *haikuStub) OneShot(ctx context.Context, system, user string) (string, error) {
	return h.resp, nil
}

func TestClassifyWidgetIntent(t *testing.T) {
	stub := &haikuStub{resp: `{"kind":"widget","widget":"stat","confidence":0.92}`}
	got, err := classifyWith(context.Background(), stub, "what's the status of MAY-19?")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got.Kind != "widget" || got.Widget != "stat" {
		t.Fatalf("want widget/stat, got %+v", got)
	}
}

func TestClassifyChatFallback(t *testing.T) {
	stub := &haikuStub{resp: `{"kind":"chat"}`}
	got, _ := classifyWith(context.Background(), stub, "hi")
	if got.Kind != "chat" {
		t.Fatalf("want chat, got %+v", got)
	}
}

func TestClassifyBadJSONIsChat(t *testing.T) {
	stub := &haikuStub{resp: `not json`}
	got, err := classifyWith(context.Background(), stub, "hi")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got.Kind != "chat" {
		t.Fatalf("malformed Haiku output must degrade to chat, got %+v", got)
	}
}
