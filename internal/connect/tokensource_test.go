package connect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type fakeSource struct {
	toks []*oauth2.Token
	i    int
}

func (f *fakeSource) Token() (*oauth2.Token, error) {
	t := f.toks[f.i]
	if f.i < len(f.toks)-1 {
		f.i++
	}
	return t, nil
}

func readTok(t *testing.T, path string) oauth2.Token {
	t.Helper()
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(buf, &tok); err != nil {
		t.Fatalf("parse token: %v", err)
	}
	return tok
}

func TestTokenWriteBackOnRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tok.json")
	orig := &oauth2.Token{AccessToken: "a1", RefreshToken: "r1", Expiry: time.Unix(1000, 0)}
	if err := WriteToken(path, orig); err != nil {
		t.Fatal(err)
	}
	rotated := &oauth2.Token{AccessToken: "a2", RefreshToken: "r2", Expiry: time.Unix(2000, 0)}
	r := &RefreshingSource{src: &fakeSource{toks: []*oauth2.Token{rotated}}, path: path, last: *orig}

	if _, err := r.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := readTok(t, path)
	if got.AccessToken != "a2" || got.RefreshToken != "r2" {
		t.Fatalf("rotated token not persisted: got access=%q refresh=%q", got.AccessToken, got.RefreshToken)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode = %o, want 0600", fi.Mode().Perm())
	}
}

func TestTokenNoRewriteWhenUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tok.json")
	orig := &oauth2.Token{AccessToken: "a1", RefreshToken: "r1", Expiry: time.Unix(1000, 0)}
	if err := WriteToken(path, orig); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	r := &RefreshingSource{src: &fakeSource{toks: []*oauth2.Token{orig}}, path: path, last: *orig}

	time.Sleep(10 * time.Millisecond)
	if _, err := r.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("unchanged token was rewritten: mtime %v -> %v", before.ModTime(), after.ModTime())
	}
}
