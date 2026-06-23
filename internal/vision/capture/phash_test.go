package capture

import (
	"testing"

	"github.com/trilam/leah/internal/vision"
)

func TestPHash_IdenticalImagesMatch(t *testing.T) {
	img := vision.Image{Pixels: make([]byte, 64*64*4), Width: 64, Height: 64, MIME: "image/rgba"}
	for i := range img.Pixels {
		img.Pixels[i] = byte(i % 256)
	}
	h1 := PHash(img)
	h2 := PHash(img)
	if h1 != h2 {
		t.Fatal("identical images must hash equal")
	}
}

func TestPHash_DifferentImagesDiffer(t *testing.T) {
	a := vision.Image{Pixels: make([]byte, 64*64*4), Width: 64, Height: 64, MIME: "image/rgba"}
	b := vision.Image{Pixels: make([]byte, 64*64*4), Width: 64, Height: 64, MIME: "image/rgba"}
	for i := range a.Pixels {
		a.Pixels[i] = 0
		b.Pixels[i] = 255
	}
	if PHashDistance(PHash(a), PHash(b)) < 20 {
		t.Fatal("opposite images must have large hamming distance")
	}
}

func TestPHash_UnsupportedMIMEReturnsZero(t *testing.T) {
	img := vision.Image{Pixels: make([]byte, 64*64), Width: 64, Height: 64, MIME: "image/png"}
	if PHash(img) != 0 {
		t.Fatal("unsupported MIME must return 0 (cache miss sentinel)")
	}
}
