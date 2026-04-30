package ansimage

import (
	"image"
	"image/color"
	"runtime"
	"testing"
)

var benchImageSink *ANSImage
var benchRenderSink string

func benchmarkRGBA(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x*3 + y*5) & 0xff),
				G: uint8((x*7 + y*11) & 0xff),
				B: uint8((x*13 + y*17) & 0xff),
				A: 0xff,
			})
		}
	}
	return img
}

func benchmarkNRGBA(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			alpha := uint8(255)
			if (x+y)%11 == 0 {
				alpha = 96
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x*3 + y*5) & 0xff),
				G: uint8((x*7 + y*11) & 0xff),
				B: uint8((x*13 + y*17) & 0xff),
				A: alpha,
			})
		}
	}
	return img
}

func BenchmarkANSImageCreate(b *testing.B) {
	bg := color.Transparent
	opaqueBG := color.RGBA{R: 12, G: 24, B: 36, A: 255}

	cases := []struct {
		name string
		src  image.Image
		bg   color.Color
		dm   DitheringMode
	}{
		{"NoDithering/RGBA_160x96", benchmarkRGBA(160, 96), bg, NoDithering},
		{"NoDithering/NRGBAComposite_160x96", benchmarkNRGBA(160, 96), opaqueBG, NoDithering},
		{"DitheringWithBlocks/RGBA_320x192", benchmarkRGBA(320, 192), bg, DitheringWithBlocks},
		{"DitheringWithChars/RGBA_320x192", benchmarkRGBA(320, 192), bg, DitheringWithChars},
	}

	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				img, err := NewFromImage(bc.src, bc.bg, bc.dm)
				if err != nil {
					b.Fatal(err)
				}
				benchImageSink = img
			}
		})
	}
}

func BenchmarkANSImageScaleAndCreate(b *testing.B) {
	src := benchmarkRGBA(640, 384)
	bg := color.Transparent

	cases := []struct {
		name string
		y, x int
		sm   ScaleMode
		dm   DitheringMode
	}{
		{"NoDithering/Resize_160x96", 96, 160, ScaleModeResize, NoDithering},
		{"NoDithering/Fit_160x96", 96, 160, ScaleModeFit, NoDithering},
		{"DitheringWithBlocks/Resize_320x192", 192, 320, ScaleModeResize, DitheringWithBlocks},
		{"DitheringWithBlocks/Fit_320x192", 192, 320, ScaleModeFit, DitheringWithBlocks},
	}

	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				img, err := NewScaledFromImage(src, bc.y, bc.x, bg, bc.sm, bc.dm)
				if err != nil {
					b.Fatal(err)
				}
				benchImageSink = img
			}
		})
	}
}

func BenchmarkANSImageRender(b *testing.B) {
	noDither, err := NewFromImage(benchmarkRGBA(160, 96), color.Transparent, NoDithering)
	if err != nil {
		b.Fatal(err)
	}
	ditherBlocks, err := NewFromImage(benchmarkRGBA(320, 192), color.Transparent, DitheringWithBlocks)
	if err != nil {
		b.Fatal(err)
	}
	ditherChars, err := NewFromImage(benchmarkRGBA(320, 192), color.Transparent, DitheringWithChars)
	if err != nil {
		b.Fatal(err)
	}

	cases := []struct {
		name           string
		img            *ANSImage
		maxprocs       int
		renderGoCode   bool
		disableBgColor bool
	}{
		{"NoDithering/Serial", noDither, 1, false, false},
		{"NoDithering/Parallel", noDither, runtime.NumCPU(), false, false},
		{"NoDithering/GoCode/Serial", noDither, 1, true, false},
		{"DitheringWithBlocks/Serial", ditherBlocks, 1, false, false},
		{"DitheringWithBlocks/Parallel", ditherBlocks, runtime.NumCPU(), false, false},
		{"DitheringWithBlocks/NoBg/Serial", ditherBlocks, 1, false, true},
		{"DitheringWithChars/Serial", ditherChars, 1, false, false},
		{"DitheringWithChars/Parallel", ditherChars, runtime.NumCPU(), false, false},
	}

	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			bc.img.SetMaxProcs(bc.maxprocs)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchRenderSink = bc.img.RenderExt(bc.renderGoCode, bc.disableBgColor)
			}
		})
	}
}

func BenchmarkANSImagePipeline(b *testing.B) {
	src := benchmarkRGBA(640, 384)
	bg := color.Transparent

	cases := []struct {
		name string
		y, x int
		dm   DitheringMode
	}{
		{"NoDithering_160x96", 96, 160, NoDithering},
		{"DitheringWithBlocks_320x192", 192, 320, DitheringWithBlocks},
	}

	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				img, err := NewScaledFromImage(src, bc.y, bc.x, bg, ScaleModeResize, bc.dm)
				if err != nil {
					b.Fatal(err)
				}
				img.SetMaxProcs(1)
				benchRenderSink = img.RenderExt(false, false)
			}
		})
	}
}
