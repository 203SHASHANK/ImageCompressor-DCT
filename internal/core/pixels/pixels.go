// Package pixels gives the codec and the quality metrics fast, uniform
// access to image pixel data.
package pixels

import (
	"image"
	"image/draw"
)

// ToRGBA returns img as an *image.RGBA whose bounds start at (0, 0) and whose
// Pix slice has no row padding, so pixel (x, y) is at Pix[4*(y*width+x)].
//
// Reading pixels through image.Image.At costs an interface call and a color
// conversion per pixel, which dominates the runtime on large images. One
// up-front conversion lets callers index the byte slice directly instead.
// The image is returned unchanged if it already has the required layout.
//
// Colors are alpha-premultiplied, exactly as At(x, y).RGBA() reports them.
func ToRGBA(img image.Image) *image.RGBA {
	bounds := img.Bounds()
	if rgba, ok := img.(*image.RGBA); ok &&
		bounds.Min == (image.Point{}) && rgba.Stride == 4*bounds.Dx() {
		return rgba
	}
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(out, out.Bounds(), img, bounds.Min, draw.Src)
	return out
}
