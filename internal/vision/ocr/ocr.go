package ocr

import (
	"github.com/trilam/leah/internal/vision"
)

func NewEngine() vision.OCREngine { return newDarwinEngine() }
