package wifi

import (
	"context"
	"errors"
	"testing"
)

type fakeAttestor struct {
	err   error
	calls int
}

func (f *fakeAttestor) Attest(_ context.Context, _ string) error {
	f.calls++
	return f.err
}

type fakeExec struct {
	stdout []byte
	stderr []byte
	err    error
	calls  int
	last   struct {
		name string
		args []string
	}
}

func (f *fakeExec) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls++
	f.last.name = name
	f.last.args = args
	return f.stdout, f.stderr, f.err
}

func TestWifi_Name(t *testing.T) {
	w, err := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := w.Name(); got != "Wi-Fi" {
		t.Fatalf("Name=%q want Wi-Fi", got)
	}
}

func TestWifi_New_RequiresDeps(t *testing.T) {
	if _, err := New(Config{Exec: &fakeExec{}}); err == nil {
		t.Fatal("want error on missing Attestor")
	}
	if _, err := New(Config{Attestor: &fakeAttestor{}}); err == nil {
		t.Fatal("want error on missing Exec")
	}
}

func TestWifi_Available_True(t *testing.T) {
	out := "Current Wi-Fi Network: HomeNet\n"
	w, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{stdout: []byte(out)}})
	if !w.Available(context.Background()) {
		t.Fatal("Available=false; want true")
	}
}

func TestWifi_Available_False(t *testing.T) {
	w, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{err: errors.New("no iface")}})
	if w.Available(context.Background()) {
		t.Fatal("Available=true; want false on exec error")
	}
}

func TestWifi_Query_AttestationDenied_NoExec(t *testing.T) {
	att := &fakeAttestor{err: errors.New("no")}
	ex := &fakeExec{}
	w, _ := New(Config{Attestor: att, Exec: ex})
	_, err := w.Query(context.Background())
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if ex.calls != 0 {
		t.Fatalf("exec ran %d times after denial", ex.calls)
	}
}

func TestWifi_Query_HappyPath_Connected(t *testing.T) {
	ex := &fakeExec{stdout: []byte("Current Wi-Fi Network: HomeNet\n")}
	w, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	s, err := w.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !s.Connected {
		t.Fatal("Connected=false; want true")
	}
	if s.SSID != "HomeNet" {
		t.Fatalf("SSID=%q want HomeNet", s.SSID)
	}
	if ex.last.name != "networksetup" {
		t.Fatalf("exec name=%q want networksetup", ex.last.name)
	}
}

func TestWifi_Query_HappyPath_NotAssociated(t *testing.T) {
	ex := &fakeExec{stdout: []byte("You are not associated with an AirPort network.\n")}
	w, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	s, err := w.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if s.Connected {
		t.Fatal("Connected=true; want false when not associated")
	}
	if s.SSID != "" {
		t.Fatalf("SSID=%q want empty", s.SSID)
	}
}

func TestWifi_Query_ExecError(t *testing.T) {
	ex := &fakeExec{err: errors.New("boom")}
	w, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := w.Query(context.Background())
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestWifi_Query_PermissionDenied(t *testing.T) {
	ex := &fakeExec{stderr: []byte("-1743 Not authorized"), err: errors.New("exit 1")}
	w, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := w.Query(context.Background())
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err=%v want ErrPermissionDenied", err)
	}
}

func TestWifi_Parse_SSIDWithColon(t *testing.T) {
	s := parseNetwork("Current Wi-Fi Network: Cafe: Free WiFi\n")
	if !s.Connected {
		t.Fatal("Connected=false; want true")
	}
	if s.SSID != "Cafe: Free WiFi" {
		t.Fatalf("SSID=%q want 'Cafe: Free WiFi'", s.SSID)
	}
}
