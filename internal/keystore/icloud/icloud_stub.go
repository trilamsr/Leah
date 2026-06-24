//go:build !(darwin && cgo)

package icloud

// hasDarwinKeychain toggles the test suite to verify New() surfaces
// ErrUnsupported on non-darwin builds — sync trust root must fail loudly,
// not silently no-op (§2.6).
const hasDarwinKeychain = false

// New returns ErrUnsupported on non-darwin builds. The Sync pane treats this
// as "iCloud Keychain unavailable" and surfaces the operator-actionable
// "Sign into iCloud and enable Keychain" hint (§2.7).
func New() (ICloudKeystore, error) { return nil, ErrUnsupported }
