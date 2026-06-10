package main

import "testing"

// TestShouldShowHelp asserts that --help / -h anywhere in args is detected
// before any downstream parsing or LLM dispatch. Defect-3: `leah self-build
// --help` was forwarded to the Reasoner as intent text and burned tokens
// returning clarifying questions instead of printing usage.
func TestShouldShowHelp(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"empty", []string{}, false},
		{"nil", nil, false},
		{"only --help", []string{"--help"}, true},
		{"only -h", []string{"-h"}, true},
		{"--help first", []string{"--help", "foo"}, true},
		{"-h first", []string{"-h", "foo"}, true},
		{"--help middle", []string{"add", "--help", "--name", "x"}, true},
		{"-h trailing", []string{"add", "--name", "x", "-h"}, true},
		{"no help flag", []string{"add", "--name", "x"}, false},
		{"intent text mentioning help word", []string{"add help text"}, false},
		{"intent text mentioning -help no double dash", []string{"-help"}, false},
		{"long flag with help suffix", []string{"--helpme"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldShowHelp(tc.args)
			if got != tc.want {
				t.Errorf("shouldShowHelp(%v) = %v; want %v", tc.args, got, tc.want)
			}
		})
	}
}
