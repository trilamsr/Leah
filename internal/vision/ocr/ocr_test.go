package ocr

import (
	"context"
	"testing"

	"github.com/trilam/leah/internal/vision"
)

func TestOCR_EmptyImageReturnsEmpty(t *testing.T) {
	eng := NewEngine()
	blocks, err := eng.Recognize(context.Background(), vision.Image{Pixels: nil, Width: 0, Height: 0})
	if err == nil && len(blocks) != 0 {
		t.Fatalf("empty image: want []TextBlock, got %d blocks", len(blocks))
	}
}
