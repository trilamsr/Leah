package capture

import (
	"math/bits"

	"github.com/trilam/leah/internal/vision"
)

// PHash returns a 64-bit perceptual hash via 8x8 average-luminance threshold.
// Used to cache OCR results within a 5-second window per spec §4.4.
// Returns 0 (cache miss sentinel) for empty, sub-8px, or non-rgba/gray images.
func PHash(img vision.Image) uint64 {
	if img.Width == 0 || img.Height == 0 || len(img.Pixels) == 0 {
		return 0
	}
	bpp, err := vision.BytesPerPixel(img.MIME)
	if err != nil {
		return 0
	}
	const N = 8
	cw := img.Width / N
	ch := img.Height / N
	if cw == 0 || ch == 0 {
		return 0
	}
	var cells [N * N]uint64
	var total uint64
	for cy := 0; cy < N; cy++ {
		for cx := 0; cx < N; cx++ {
			var sum uint64
			var count uint64
			for y := cy * ch; y < (cy+1)*ch && y < img.Height; y++ {
				for x := cx * cw; x < (cx+1)*cw && x < img.Width; x++ {
					off := (y*img.Width + x) * bpp
					if off+bpp > len(img.Pixels) {
						continue
					}
					var lum uint64
					if bpp == 4 {
						lum = uint64(img.Pixels[off])*299 + uint64(img.Pixels[off+1])*587 + uint64(img.Pixels[off+2])*114
						lum /= 1000
					} else {
						lum = uint64(img.Pixels[off])
					}
					sum += lum
					count++
				}
			}
			if count == 0 {
				cells[cy*N+cx] = 0
			} else {
				cells[cy*N+cx] = sum / count
			}
			total += cells[cy*N+cx]
		}
	}
	avg := total / uint64(N*N)
	var h uint64
	for i, v := range cells {
		if v > avg {
			h |= 1 << uint(i)
		}
	}
	// Mix global luminance into the hash so uniform-but-opposite images
	// (e.g. all-black vs all-white) produce distant hashes; their cells all
	// equal avg, so the threshold loop above yields the same bits for both.
	return h ^ ((avg & 0xFF) * 0x0101010101010101)
}

func PHashDistance(a, b uint64) int { return bits.OnesCount64(a ^ b) }
