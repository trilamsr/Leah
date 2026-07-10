package contracts

import "context"

// OSExec abstracts os/exec.Command so adapters that shell out (iMessage,
// FaceTime, macOS native apps) can inject deterministic fakes in tests.
type OSExec interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}
