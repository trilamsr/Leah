//go:build !darwin

package capture

import (
	"context"
	"errors"
	"image"

	"github.com/trilam/leah/internal/vision"
)

type stubCapture struct{}

func New() vision.Capture { return stubCapture{} }

func (stubCapture) Screenshot(ctx context.Context, rect image.Rectangle) (vision.Image, error) {
	return vision.Image{}, errors.New("capture: darwin-only")
}
func (stubCapture) StartLiveScreen(ctx context.Context, fps int) (<-chan vision.Frame, vision.CancelFunc, error) {
	return nil, nil, errors.New("capture: darwin-only")
}
func (stubCapture) StartLiveCamera(ctx context.Context, fps int) (<-chan vision.Frame, vision.CancelFunc, error) {
	return nil, nil, errors.New("capture: darwin-only")
}
