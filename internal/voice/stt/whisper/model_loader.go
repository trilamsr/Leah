package whisper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// ErrModelMissing fails closed when whisper-large-v3.onnx isn't on disk.
// Spec §1.3 + §0.2 invariant 6 — silent degradation forbidden.
var ErrModelMissing = errors.New("whisper-large-v3.onnx missing from model dir")

const modelFilename = "whisper-large-v3.onnx"

// loadModel returns the on-disk path + sha256. The runner streams from the
// file via ONNX Runtime's session-from-path API, so we keep the path (not
// the bytes) to avoid pinning ~3 GB in process memory.
func loadModel(dir string) (path, sha string, err error) {
	p := filepath.Join(dir, modelFilename)
	f, err := os.Open(p)
	if err != nil {
		return "", "", ErrModelMissing
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", "", err
	}
	return p, hex.EncodeToString(h.Sum(nil)), nil
}
