package mcp

import "regexp"

var redactLints = []struct {
	name string
	re   *regexp.Regexp
}{
	{"email", regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)},
	{"bearer_token", regexp.MustCompile(`(?i)(bearer|token|api[_\-]?key|secret)\s*[=:]\s*[a-z0-9_\-]{16,}`)},
	{"ssh_private_key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"home_path", regexp.MustCompile(`(?i)/(users|home)/[a-z0-9._\-]+/`)},
	{"phone_us", regexp.MustCompile(`(?:\+?1[\-.\s]?)?\(?\d{3}\)?[\-.\s]?\d{3}[\-.\s]?\d{4}`)},
	{"aws_access_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
}

var ccRe = regexp.MustCompile(`(?:\d[ \-]?){13,19}\b`)

// RedactHit returns the first matching lint name, or "" + false when clean.
// Drop-the-row contract from spec §3.2.1 — caller drops rather than rewrites.
func RedactHit(s string) (bool, string) {
	for _, l := range redactLints {
		if l.re.MatchString(s) {
			return true, l.name
		}
	}
	for _, m := range ccRe.FindAllString(s, -1) {
		if luhn(stripNonDigits(m)) {
			return true, "cc_number"
		}
	}
	return false, ""
}

func stripNonDigits(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func luhn(digits string) bool {
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	sum := 0
	dbl := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if dbl {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		dbl = !dbl
	}
	return sum%10 == 0
}
