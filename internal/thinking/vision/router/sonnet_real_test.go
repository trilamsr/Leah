package router

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/trilam/leah/internal/vision"
)

func TestEncodeFrameForSonnet_RGBARoundtrip(t *testing.T) {
	// 2x2 RGBA frame → PNG. Decode and confirm dimensions match — proves the
	// encoder didn't mis-stride the byte buffer.
	frame := vision.Image{
		Pixels: []byte{
			0xFF, 0x00, 0x00, 0xFF,
			0x00, 0xFF, 0x00, 0xFF,
			0x00, 0x00, 0xFF, 0xFF,
			0xFF, 0xFF, 0xFF, 0xFF,
		},
		Width:  2,
		Height: 2,
		MIME:   "image/rgba",
	}
	mt, encoded, err := encodeFrameForSonnet(frame)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if mt != "image/png" {
		t.Fatalf("media type: want image/png, got %q", mt)
	}
	dec, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	if dec.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("bounds: %v", dec.Bounds())
	}
}

func TestEncodeFrameForSonnet_GrayRoundtrip(t *testing.T) {
	frame := vision.Image{
		Pixels: []byte{0x00, 0x55, 0xAA, 0xFF},
		Width:  2,
		Height: 2,
		MIME:   "image/gray",
	}
	_, encoded, err := encodeFrameForSonnet(frame)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	if dec.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("bounds: %v", dec.Bounds())
	}
}

func TestEncodeFrameForSonnet_RejectsUnknownMIME(t *testing.T) {
	frame := vision.Image{Pixels: []byte{0, 0, 0, 0}, Width: 1, Height: 1, MIME: "image/yuv"}
	if _, _, err := encodeFrameForSonnet(frame); err == nil {
		t.Fatal("unknown MIME must error — caller cannot stride without bpp")
	}
}

func TestEncodeFrameForSonnet_RejectsShortBuffer(t *testing.T) {
	// 4x4 RGBA frame declared but only 8 bytes provided — caller bug or
	// truncated capture. Must error rather than panic on slice access.
	frame := vision.Image{Pixels: make([]byte, 8), Width: 4, Height: 4, MIME: "image/rgba"}
	if _, _, err := encodeFrameForSonnet(frame); err == nil {
		t.Fatal("short Pixels buffer must error")
	}
}

func TestNewSonnetClient_RequiresAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, err := NewSonnetClient(); err == nil {
		t.Fatal("missing ANTHROPIC_API_KEY must error — fail-fast in main beats a runtime 401")
	}
}
