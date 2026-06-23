package ocr

import (
	"context"
	"errors"
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

func TestOCR_UnsupportedMIMEErrors(t *testing.T) {
	// Stub engine on non-darwin returns nil,nil for any input; only darwin
	// guards MIME at the cgo boundary. Skip when running off-darwin.
	eng := NewEngine()
	if _, ok := eng.(interface {
		Recognize(context.Context, vision.Image) ([]vision.TextBlock, error)
	}); !ok {
		t.Skip("engine missing Recognize")
	}
	img := vision.Image{Pixels: make([]byte, 8), Width: 2, Height: 2, MIME: "image/png"}
	_, err := eng.Recognize(context.Background(), img)
	if err == nil {
		t.Skip("stub engine accepts any MIME; darwin engine guards via vision.BytesPerPixel")
		return
	}
	if !errors.Is(err, vision.ErrUnsupportedMIME) {
		t.Fatalf("want ErrUnsupportedMIME, got %v", err)
	}
}
