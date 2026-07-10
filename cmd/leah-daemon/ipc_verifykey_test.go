package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/platform/ipc"
)

// BUG-9 regression: handleVerifyKey used `_ = json.Unmarshal(...)`, silently
// swallowing type errors. A HUD sending `{"key":42}` would decode to
// p.Key == "" and get an ok:false response indistinguishable from a rejected
// key. The fix should return a structured "bad payload" error frame so the
// operator sees a real diagnosis.
func TestHandleVerifyKey_MalformedPayloadReturnsError(t *testing.T) {
	ping := func(_ context.Context, _ string) error { return nil }
	req := ipc.Frame{
		Kind:    "verify-key",
		TurnID:  "vkm",
		Payload: json.RawMessage(`{"key":42}`),
	}
	out, err := handleVerifyKey(context.Background(), req, ping)
	if err != nil {
		t.Fatalf("handleVerifyKey: %v", err)
	}
	f := <-out
	if f.Kind == "verify-key.result" && !strings.Contains(string(f.Payload), "error") {
		t.Fatalf("malformed payload should surface bad-payload error, got kind=%s payload=%s", f.Kind, string(f.Payload))
	}
}

// BUG-9 regression #2: empty payload (no key at all) reached ping() with
// an empty string; the API returned auth error and the HUD saw ok:false
// with no way to distinguish "you forgot to send the key" from "the key
// you sent is wrong". Fix must reject empty key up front.
func TestHandleVerifyKey_EmptyKeyReturnsError(t *testing.T) {
	pingCalled := false
	ping := func(_ context.Context, _ string) error {
		pingCalled = true
		return nil
	}
	req := ipc.Frame{
		Kind:    "verify-key",
		TurnID:  "vke",
		Payload: json.RawMessage(`{}`),
	}
	out, _ := handleVerifyKey(context.Background(), req, ping)
	f := <-out
	if pingCalled {
		t.Fatal("empty key must NOT reach ping()")
	}
	if !strings.Contains(string(f.Payload), "error") && !strings.Contains(string(f.Payload), "required") {
		t.Fatalf("empty key must produce structured error, got kind=%s payload=%s", f.Kind, string(f.Payload))
	}
}

// Guard the happy path still works — do not regress the shipped verify-key
// contract while fixing the malformed/empty edge cases.
func TestHandleVerifyKey_LiveKeyStillReports(t *testing.T) {
	ping := func(_ context.Context, key string) error {
		if key == "good" {
			return nil
		}
		return errors.New("bad")
	}
	// Good key
	req := ipc.Frame{Kind: "verify-key", TurnID: "vk-ok", Payload: json.RawMessage(`{"key":"good"}`)}
	out, _ := handleVerifyKey(context.Background(), req, ping)
	f := <-out
	if f.Kind != "verify-key.result" || !strings.Contains(string(f.Payload), `"ok":true`) {
		t.Fatalf("good key: want ok:true, got kind=%s payload=%s", f.Kind, string(f.Payload))
	}
	// Bad key still reports ok:false via the result frame (not the error path).
	req = ipc.Frame{Kind: "verify-key", TurnID: "vk-bad", Payload: json.RawMessage(`{"key":"nope"}`)}
	out, _ = handleVerifyKey(context.Background(), req, ping)
	f = <-out
	if f.Kind != "verify-key.result" || !strings.Contains(string(f.Payload), `"ok":false`) {
		t.Fatalf("bad key: want ok:false, got kind=%s payload=%s", f.Kind, string(f.Payload))
	}
}
