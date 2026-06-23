//go:build !cgo

package embed

import (
	"context"
	"fmt"
)

// BGEGenerator is a no-cgo stub. Real ONNX inference lives in bge.go behind
// the cgo build tag (onnxruntime_go requires cgo). When the daemon is built
// CGO_ENABLED=0, LEAH_EMBED_BACKEND=bge fails fast at constructor time
// rather than silently degrading to hash embeddings.
type BGEGenerator struct{}

// NewBGEGenerator always errors when cgo is disabled.
func NewBGEGenerator(modelPath string) (*BGEGenerator, error) {
	return nil, fmt.Errorf("embed: BGE requires cgo build (this binary was built CGO_ENABLED=0); rebuild with cgo or set LEAH_EMBED_BACKEND=hash|openai")
}

func (g *BGEGenerator) Name() string                                                    { return "bge-small-en-v1.5" }
func (g *BGEGenerator) Dim() int                                                        { return 384 }
func (g *BGEGenerator) Embed(_ context.Context, _ []string) ([][]float32, error)        { return nil, fmt.Errorf("embed: BGE unavailable without cgo") }
