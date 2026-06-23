//go:build darwin

package ocr

/*
#cgo LDFLAGS: -framework Vision -framework CoreGraphics -framework CoreImage -framework Foundation
#include <stdlib.h>
typedef struct { const char* text; int x; int y; int w; int h; double conf; } OCRHit;
int leah_ocr_recognize(const unsigned char* px, int w, int h, int bpp, OCRHit** out_hits);
void leah_ocr_free(OCRHit* hits, int n);
*/
import "C"

import (
	"context"
	"image"
	"unsafe"

	"github.com/trilam/leah/internal/vision"
)

type darwinEngine struct{}

func newDarwinEngine() vision.OCREngine { return &darwinEngine{} }

func (darwinEngine) Recognize(ctx context.Context, img vision.Image) ([]vision.TextBlock, error) {
	if img.Width == 0 || img.Height == 0 || len(img.Pixels) == 0 {
		return nil, nil
	}
	bpp := 4
	if img.MIME == "image/gray" {
		bpp = 1
	}
	var hits *C.OCRHit
	n := int(C.leah_ocr_recognize((*C.uchar)(unsafe.Pointer(&img.Pixels[0])), C.int(img.Width), C.int(img.Height), C.int(bpp), &hits))
	if n <= 0 {
		return nil, nil
	}
	defer C.leah_ocr_free(hits, C.int(n))
	out := make([]vision.TextBlock, 0, n)
	hitsSlice := unsafe.Slice(hits, n)
	for _, h := range hitsSlice {
		out = append(out, vision.TextBlock{
			Text:       C.GoString(h.text),
			Rect:       image.Rect(int(h.x), int(h.y), int(h.x+h.w), int(h.y+h.h)),
			Confidence: float64(h.conf),
		})
	}
	return out, nil
}
