package whisper

import "testing"

// TestDecodeTokens_EmptySliceReturnsEmpty pins the nil-safe path.
func TestDecodeTokens_EmptySliceReturnsEmpty(t *testing.T) {
	if got := decodeTokens(nil); got != "" {
		t.Fatalf("decodeTokens(nil) = %q, want empty", got)
	}
	if got := decodeTokens([]int32{}); got != "" {
		t.Fatalf("decodeTokens([]) = %q, want empty", got)
	}
}

// TestDecodeTokens_KnownTokens_Hello asserts the BPE table resolves "hello"-class tokens.
func TestDecodeTokens_KnownTokens_Hello(t *testing.T) {
	// Ġhello = 7751 in the Whisper-large-v3 vocab; the leading byte-level Ġ decodes to a literal space.
	if got := decodeTokens([]int32{7751}); got != " hello" {
		t.Fatalf("decodeTokens([7751]) = %q, want %q", got, " hello")
	}
	// "hell"=21288 + "o"=78 must concatenate to "hello".
	if got := decodeTokens([]int32{21288, 78}); got != "hello" {
		t.Fatalf("decodeTokens([21288,78]) = %q, want %q", got, "hello")
	}
}

// TestDecodeTokens_HandlesSpecialTokens asserts special-token IDs are filtered out.
func TestDecodeTokens_HandlesSpecialTokens(t *testing.T) {
	// 50258=<|startoftranscript|>, 50259=<|en|>, 50360=<|transcribe|>, 50364=<|notimestamps|>, 50257=<|endoftext|>
	got := decodeTokens([]int32{50258, 50259, 50360, 50364, 7751, 50257})
	if got != " hello" {
		t.Fatalf("specials must be filtered: got %q, want %q", got, " hello")
	}
}

// TestDecodeTokens_UnicodeRoundtrip asserts multi-byte UTF-8 round-trips through the byte-level decoder.
func TestDecodeTokens_UnicodeRoundtrip(t *testing.T) {
	// 38088 = "こんにちは" as a single Whisper-large-v3 token.
	if got := decodeTokens([]int32{38088}); got != "こんにちは" {
		t.Fatalf("decodeTokens([38088]) = %q, want %q", got, "こんにちは")
	}
}
