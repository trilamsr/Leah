package bluetooth

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

const sampleOn = `Bluetooth:

      Bluetooth Controller:
          State: On
          Chipset: BCM_4388
      Connected:
          Magic Keyboard:
              Address: AA-BB-CC-DD-EE-FF
              Vendor ID: 0x004C
          Tri's AirPods Pro:
              Address: 11-22-33-44-55-66
              Minor Type: Headphones
      Not Connected:
          Old Mouse:
              Address: 99-88-77-66-55-44
`

const sampleOff = `Bluetooth:

      Bluetooth Controller:
          State: Off
      Not Connected:
          Magic Keyboard:
              Address: AA-BB-CC-DD-EE-FF
`

func TestBluetooth_Name(t *testing.T) {
	b, err := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Name(); got != "Bluetooth" {
		t.Fatalf("Name=%q want Bluetooth", got)
	}
}

func TestBluetooth_New_RequiresDeps(t *testing.T) {
	if _, err := New(Config{Exec: &fakeExec{}}); err == nil {
		t.Fatal("want error on missing Attestor")
	}
	if _, err := New(Config{Attestor: &fakeAttestor{}}); err == nil {
		t.Fatal("want error on missing Exec")
	}
}

func TestBluetooth_Available_True(t *testing.T) {
	b, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{stdout: []byte(sampleOn)}})
	if !b.Available(context.Background()) {
		t.Fatal("Available=false; want true")
	}
}

func TestBluetooth_Available_False(t *testing.T) {
	b, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{err: errors.New("no profiler")}})
	if b.Available(context.Background()) {
		t.Fatal("Available=true; want false on exec error")
	}
}

func TestBluetooth_Query_AttestationDenied_NoExec(t *testing.T) {
	att := &fakeAttestor{err: errors.New("no")}
	ex := &fakeExec{}
	b, _ := New(Config{Attestor: att, Exec: ex})
	_, err := b.Query(context.Background())
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if ex.calls != 0 {
		t.Fatalf("exec ran %d times after denial", ex.calls)
	}
}

func TestBluetooth_Query_HappyPath_On(t *testing.T) {
	ex := &fakeExec{stdout: []byte(sampleOn)}
	b, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	s, err := b.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !s.Powered {
		t.Fatal("Powered=false; want true")
	}
	if len(s.Connected) != 2 {
		t.Fatalf("Connected=%v want 2 devices", s.Connected)
	}
	if s.Connected[0] != "Magic Keyboard" || s.Connected[1] != "Tri's AirPods Pro" {
		t.Fatalf("Connected=%v want [Magic Keyboard, Tri's AirPods Pro]", s.Connected)
	}
	if ex.last.name != "system_profiler" {
		t.Fatalf("exec name=%q want system_profiler", ex.last.name)
	}
}

func TestBluetooth_Query_HappyPath_Off(t *testing.T) {
	ex := &fakeExec{stdout: []byte(sampleOff)}
	b, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	s, err := b.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if s.Powered {
		t.Fatal("Powered=true; want false")
	}
	if len(s.Connected) != 0 {
		t.Fatalf("Connected=%v want empty when powered off", s.Connected)
	}
}

func TestBluetooth_Query_ExecError(t *testing.T) {
	ex := &fakeExec{err: errors.New("boom")}
	b, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := b.Query(context.Background())
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestBluetooth_Query_PermissionDenied(t *testing.T) {
	ex := &fakeExec{stderr: []byte("-1743 Not authorized"), err: errors.New("exit 1")}
	b, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := b.Query(context.Background())
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err=%v want ErrPermissionDenied", err)
	}
}

func TestBluetooth_Parse_Empty(t *testing.T) {
	s := parseProfiler("")
	if s.Powered {
		t.Fatal("Powered=true on empty input")
	}
	if len(s.Connected) != 0 {
		t.Fatalf("Connected=%v want empty", s.Connected)
	}
}
