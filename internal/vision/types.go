package vision

import (
	"context"
	"image"
	"time"
)

type Image struct {
	Pixels []byte
	Width  int
	Height int
	MIME   string
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
