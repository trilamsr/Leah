//go:build !darwin

package ocr

import (
	"context"

	"github.com/trilam/leah/internal/vision"
)

type stubEngine struct{}

func newDarwinEngine() vision.OCREngine { return stubEngine{} }

func (stubEngine) Recognize(ctx context.Context, img vision.Image) ([]vision.TextBlock, error) {
	return nil, nil
}
