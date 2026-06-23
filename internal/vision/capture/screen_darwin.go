//go:build darwin

package capture

import (
	"context"
	"errors"
	"image"

	"github.com/trilam/leah/internal/vision"
)

type darwinCapture struct{}

func New() vision.Capture { return &darwinCapture{} }

func (darwinCapture) Screenshot(ctx context.Context, rect image.Rectangle) (vision.Image, error) {
	return vision.Image{}, errors.New("capture: screenshot impl pending CGDisplayCreateImage bridge")
}

func (darwinCapture) StartLiveScreen(ctx context.Context, fps int) (<-chan vision.Frame, vision.CancelFunc, error) {
	return nil, nil, errors.New("capture: live screen pending CGDisplayStream bridge")
}

func (darwinCapture) StartLiveCamera(ctx context.Context, fps int) (<-chan vision.Frame, vision.CancelFunc, error) {
	return nil, nil, errors.New("capture: live camera pending AVCaptureDevice bridge")
}
