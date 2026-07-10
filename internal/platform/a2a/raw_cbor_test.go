package a2a

import "github.com/fxamacker/cbor/v2"

// rawCBOR encodes a free-form map for tests that need to inject malformed
// frames the typed Encode path would reject. Keep test-scoped.
func rawCBOR(v any) ([]byte, error) {
	return cbor.Marshal(v)
}
