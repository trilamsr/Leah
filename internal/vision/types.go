package vision

import (
	"context"
	"errors"
	"image"
	"time"
)

type Image struct {
	Pixels []byte
	Width  int
	Height int
	MIME   string
}

var ErrUnsupportedMIME = errors.New("vision: unsupported MIME — want image/rgba or image/gray")

// BytesPerPixel gates stride arithmetic in PHash + OCR; any unknown MIME
// would silently mis-index Pixels and read garbage. Empty MIME = rgba default.
func BytesPerPixel(mime string) (int, error) {
	switch mime {
	case "image/rgba", "":
		return 4, nil
	case "image/gray":
		return 1, nil
	default:
		return 0, ErrUnsupportedMIME
	}
}

type FrameSource int

const (
	SourceScreen FrameSource = iota
	SourceCamera
	SourceSelection
)

type Frame struct {
	Image  Image
	Ts     time.Time
	Source FrameSource
}

type TextBlock struct {
	Text       string
	Rect       image.Rectangle
	Confidence float64
}

type CancelFunc func()

type Capture interface {
	Screenshot(ctx context.Context, rect image.Rectangle) (Image, error)
	StartLiveScreen(ctx context.Context, fps int) (<-chan Frame, CancelFunc, error)
	StartLiveCamera(ctx context.Context, fps int) (<-chan Frame, CancelFunc, error)
}

type OCREngine interface {
	Recognize(ctx context.Context, img Image) ([]TextBlock, error)
}
