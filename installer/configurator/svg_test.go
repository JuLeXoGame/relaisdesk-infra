//go:build !linux

package main

import (
	"bytes"
	"image"
	"testing"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

func TestSvgParse(t *testing.T) {
	for name, s := range map[string]string{
		"windows": svgWindows,
		"apple":   svgApple,
		"linux":   svgLinux,
		"generic": svgGeneric,
	} {
		icon, err := oksvg.ReadIconStream(bytes.NewReader([]byte(s)))
		if err != nil {
			t.Errorf("Error parsing %s: %v", name, err)
			continue
		}
		w, h := int(icon.ViewBox.W), int(icon.ViewBox.H)
		if w == 0 || h == 0 {
			w, h = 24, 24
		}
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
		raster := rasterx.NewDasher(w, h, scanner)
		icon.Draw(raster, 1.0)
		nonZero := 0
		for _, p := range img.Pix {
			if p != 0 {
				nonZero++
			}
		}
		if nonZero == 0 {
			t.Errorf("Icon %s rendered 0 non-zero pixels!", name)
		} else {
			t.Logf("Icon %s: drawn %d non-zero pixel bytes (dimensions %dx%d)", name, nonZero, w, h)
		}
	}
}
