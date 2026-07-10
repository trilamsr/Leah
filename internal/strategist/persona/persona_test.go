package persona

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		write    string // content to write; "" = do not create the file
		wantBody string
		wantErr  error
	}{
		{
			name:     "file present returns full body verbatim",
			write:    "# Persona\n\nPragmatic engineer. First person.\n",
			wantBody: "# Persona\n\nPragmatic engineer. First person.\n",
		},
		{
			name:    "file missing returns ErrNotExist",
			wantErr: os.ErrNotExist,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.write != "" {
				if err := os.MkdirAll(filepath.Join(dir, "strategist"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "strategist", "persona.md"), []byte(tc.write), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			got, err := Load(dir)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.wantBody {
				t.Fatalf("body mismatch:\ngot:  %q\nwant: %q", got, tc.wantBody)
			}
		})
	}
}

func TestDefaultVoiceNonEmpty(t *testing.T) {
	// The CLI substitutes DefaultVoice on ErrNotExist; an empty default
	// would route an empty system prompt to the LLM.
	if DefaultVoice == "" {
		t.Fatal("DefaultVoice must be non-empty")
	}
}
