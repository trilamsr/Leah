package connect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultRegistry_NativeMacResolvable proves imessage + facetime are
// connectable — Lookup must NOT return ErrUnknownProvider.
func TestDefaultRegistry_NativeMacResolvable(t *testing.T) {
	reg := DefaultRegistry()
	for _, name := range []string{"imessage", "facetime"} {
		if _, err := reg.Lookup(name); err != nil {
			t.Fatalf("Lookup(%q) = %v, want resolvable provider", name, err)
		}
	}
}

// TestNativeMac_ProbeAttestMarker: a reachable binary + granted attestation
// writes the presence marker; there is no OAuth token to fetch.
func TestNativeMac_ProbeAttestMarker(t *testing.T) {
	for _, name := range []string{"imessage", "facetime"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			tokenPath := filepath.Join(dir, name+"-token.json")
			p := &nativeMacProvider{
				name:      name,
				binary:    "osascript",
				tokenPath: tokenPath,
				lookPath:  func(string) (string, error) { return "/usr/bin/osascript", nil },
			}
			att := &fakeAttestor{}
			if _, err := Authorize(context.Background(), p, att, nil); err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			if len(att.calls) != 1 || att.calls[0] != "connect:"+name {
				t.Fatalf("attest calls = %v, want [connect:%s]", att.calls, name)
			}
			if _, err := os.Stat(tokenPath); err != nil {
				t.Fatalf("marker not written: %v", err)
			}
		})
	}
}

// TestNativeMac_AttestationDenied_NoMarker: declined consent short-circuits
// before the probe and writes no marker.
func TestNativeMac_AttestationDenied_NoMarker(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "imessage-token.json")
	probed := false
	p := &nativeMacProvider{
		name:      "imessage",
		binary:    "osascript",
		tokenPath: tokenPath,
		lookPath:  func(string) (string, error) { probed = true; return "/usr/bin/osascript", nil },
	}
	att := &fakeAttestor{err: errors.New("operator denied")}
	if _, err := Authorize(context.Background(), p, att, nil); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if probed {
		t.Fatal("probe ran after denied attestation")
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marker written despite denied attestation")
	}
}

// TestNativeMac_BinaryMissing_NotConnected: an unreachable host binary fails
// connect and writes no marker.
func TestNativeMac_BinaryMissing_NotConnected(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "facetime-token.json")
	p := &nativeMacProvider{
		name:      "facetime",
		binary:    "open",
		tokenPath: tokenPath,
		lookPath:  func(string) (string, error) { return "", errors.New("not found") },
	}
	if _, err := Authorize(context.Background(), p, &fakeAttestor{}, nil); !errors.Is(err, ErrNativeUnavailable) {
		t.Fatalf("err = %v, want ErrNativeUnavailable", err)
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marker written despite missing binary")
	}
}
