package metrics

import (
	"fmt"
	"image"
)

const (
	// ssimWindow is the side of the square window statistics are taken over,
	// and ssimStep how far the window moves each time. A step of half the
	// window makes neighbouring windows overlap.
	ssimWindow = 8
	ssimStep   = ssimWindow / 2

	// Stabilizing constants from the SSIM paper (Wang et al., 2004):
	// C1 = (0.01*L)^2 and C2 = (0.03*L)^2 for a dynamic range of L = 255.
	// They keep the ratio defined in flat regions where the means or
	// variances are close to zero.
	ssimC1 = (0.01 * 255) * (0.01 * 255)
	ssimC2 = (0.03 * 255) * (0.03 * 255)
)

// CalculateSSIM returns the mean structural similarity index of two images,
// computed on brightness. 1 means identical; lower means less similar.
//
// Unlike PSNR, which treats every pixel error alike, SSIM compares local
// mean, contrast and structure, which tracks perceived quality more closely.
//
// This is the uniform-window variant: 8x8 windows moved in steps of 4. The
// reference implementation uses an 11x11 Gaussian window, so values from the
// two are close but not interchangeable.
func CalculateSSIM(original, compressed image.Image) (float64, error) {
	a, b, err := comparable(original, compressed)
	if err != nil {
		return 0, err
	}
	width, height := a.Bounds().Dx(), a.Bounds().Dy()
	if width < ssimWindow || height < ssimWindow {
		return 0, fmt.Errorf("image too small for SSIM (minimum %dx%d)", ssimWindow, ssimWindow)
	}

	x, y := luminance(a), luminance(b)

	sum, windows := 0.0, 0
	for top := 0; top+ssimWindow <= height; top += ssimStep {
		for left := 0; left+ssimWindow <= width; left += ssimStep {
			sum += windowSSIM(x, y, width, top, left)
			windows++
		}
	}
	return sum / float64(windows), nil
}

// windowSSIM computes SSIM for one window of two luminance planes:
//
//	(2*meanX*meanY + C1) * (2*cov + C2)
//	-------------------------------------------------
//	(meanX^2 + meanY^2 + C1) * (varX + varY + C2)
func windowSSIM(x, y []float64, width, top, left int) float64 {
	const n = ssimWindow * ssimWindow

	// One pass collecting raw sums; variance and covariance follow from
	// E[x^2] - E[x]^2 and E[xy] - E[x]E[y].
	var sumX, sumY, sumXX, sumYY, sumXY float64
	for r := top; r < top+ssimWindow; r++ {
		rowX := x[r*width+left:][:ssimWindow]
		rowY := y[r*width+left:][:ssimWindow]
		for i, vx := range rowX {
			vy := rowY[i]
			sumX += vx
			sumY += vy
			sumXX += vx * vx
			sumYY += vy * vy
			sumXY += vx * vy
		}
	}

	meanX, meanY := sumX/n, sumY/n
	varX := sumXX/n - meanX*meanX
	varY := sumYY/n - meanY*meanY
	cov := sumXY/n - meanX*meanY

	return ((2*meanX*meanY + ssimC1) * (2*cov + ssimC2)) /
		((meanX*meanX + meanY*meanY + ssimC1) * (varX + varY + ssimC2))
}

// luminance returns the brightness (BT.601 luma) of every pixel, row-major.
func luminance(img *image.RGBA) []float64 {
	out := make([]float64, len(img.Pix)/4)
	for i := range out {
		p := img.Pix[4*i:]
		out[i] = 0.299*float64(p[0]) + 0.587*float64(p[1]) + 0.114*float64(p[2])
	}
	return out
}
