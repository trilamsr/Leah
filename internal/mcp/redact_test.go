package mcp

import "testing"

func TestRedact_AllSevenPatternsDrop(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"email", "contact alice@example.com today"},
		{"bearer_token", "Authorization: Bearer abcdef0123456789abcd"},
		{"ssh_private_key", "-----BEGIN OPENSSH PRIVATE KEY-----"},
		{"home_path", "wrote /Users/tri/.ssh/id_rsa"},
		{"phone_us", "call (415) 555-1212 now"},
		{"cc_number", "card 4242 4242 4242 4242 charged"},
		{"aws_access_key", "key AKIAIOSFODNN7EXAMPLE leaked"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if hit, _ := RedactHit(c.in); !hit {
				t.Fatalf("%s: want hit, got clean for %q", c.name, c.in)
			}
		})
	}
}

func TestRedact_CleanRowPasses(t *testing.T) {
	clean := []string{"ship ok", "deployed widget to prod", "", "blast_radius=3"}
	for _, s := range clean {
		if hit, lint := RedactHit(s); hit {
			t.Fatalf("clean row %q hit %s", s, lint)
		}
	}
}

func TestRedact_CCNumberRequiresLuhn(t *testing.T) {
	// 16 digits but not valid Luhn → must NOT hit
	if hit, _ := RedactHit("number 1234 5678 9012 3456 here"); hit {
		t.Fatalf("non-luhn 16-digit should not hit")
	}
}
