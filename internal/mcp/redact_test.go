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

// Regression: phone_us must not match build IDs / PR numbers / hash-dash IDs
// or audit rows mentioning these get silently dropped.
func TestRedact_PhoneUS_DoesNotOverMatch(t *testing.T) {
	clean := []string{
		"build 123-456-7890123 finished",        // build id w/ extra digit
		"pr #199 merged",                        // PR number
		"sha a1b2c3d-456-7890",                  // hash-with-dashes
		"123-456-7890 in body without context",  // bare digits, no parens/+1/phone:
		"version 1.2.3 released 555 234 5678ms", // version + millis suffix
		"req_id 415-555-1212-xyz",               // id-like, not phone
	}
	for _, s := range clean {
		if hit, lint := RedactHit(s); hit {
			t.Fatalf("over-match: %q hit %s", s, lint)
		}
	}
	// Real phones must still hit.
	phones := []string{
		"call (415) 555-1212 now",
		"phone +1 415-555-1212",
		"+1 (415) 555-1212",
	}
	for _, s := range phones {
		if hit, _ := RedactHit(s); !hit {
			t.Fatalf("real phone missed: %q", s)
		}
	}
}
