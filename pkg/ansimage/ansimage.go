//       ___  _____  ____
//      / _ \/  _/ |/_/ /____ ______ _
//     / ___// /_>  </ __/ -_) __/  ' \
//    /_/  /___/_/|_|\__/\__/_/ /_/_/_/
//
//    Copyright 2017 Eliuk Blau
//
//    This Source Code Form is subject to the terms of the Mozilla Public
//    License, v. 2.0. If a copy of the MPL was not distributed with this
//    file, You can obtain one at https://mozilla.org/MPL/2.0/.

package ansimage

import (
	"errors"
	"fmt"

	"io"
	"net/http"
	"os"
	"strings"

	"image"
	"image/color"
	"image/draw"

	_ "image/gif"  // initialize decoder
	_ "image/jpeg" // initialize decoder
	_ "image/png"  // initialize decoder

	_ "golang.org/x/image/bmp"  // initialize decoder
	_ "golang.org/x/image/tiff" // initialize decoder
	_ "golang.org/x/image/webp" // initialize decoder

	"github.com/disintegration/imaging"
	"github.com/lucasb-eyer/go-colorful"
)

// Unicode Block Element character used to represent lower pixel in terminal row.
// INFO: https://en.wikipedia.org/wiki/Block_Elements
const lowerHalfBlock = "\u2584"

// Unicode Block Element characters used to represent dithering in terminal row.
// INFO: https://en.wikipedia.org/wiki/Block_Elements
const fullBlock = "\u2588"
const darkShadeBlock = "\u2593"
const mediumShadeBlock = "\u2592"
const lightShadeBlock = "\u2591"

// ANSImage scale modes:
// resize (full scaled to area),
// fill (resize and crop the image with a center anchor point to fill area),
// fit (resize the image to fit area, preserving the aspect ratio).
const (
	ScaleModeResize = ScaleMode(iota)
	ScaleModeFill
	ScaleModeFit
)

// ANSImage dithering modes:
// no dithering (classic mode: half block based),
// chars (use characters to represent brightness),
// blocks (use character blocks to represent brightness).
const (
	NoDithering = DitheringMode(iota)
	DitheringWithBlocks
	DitheringWithChars
)

// ANSImage block size in pixels (dithering mode)
const (
	BlockSizeY = 8
	BlockSizeX = 4
)

var (
	// ErrImageDownloadFailed occurs in the attempt to download an image and the status code of the response is not "200 OK".
	ErrImageDownloadFailed = errors.New("ANSImage: image download failed")

	// ErrHeightNonMoT occurs when ANSImage height is not a Multiple of Two value.
	ErrHeightNonMoT = errors.New("ANSImage: height must be a Multiple of Two value")

	// ErrInvalidBoundsMoT occurs when ANSImage height or width are invalid values (Multiple of Two).
	ErrInvalidBoundsMoT = errors.New("ANSImage: height or width must be >=2")

	// ErrOutOfBounds occurs when ANSI-pixel coordinates are out of ANSImage bounds.
	ErrOutOfBounds = errors.New("ANSImage: out of bounds")

	// errUnknownScaleMode occurs when scale mode is invalid.
	errUnknownScaleMode = errors.New("ANSImage: unknown scale mode")

	// errUnknownDitheringMode occurs when dithering mode is invalid.
	errUnknownDitheringMode = errors.New("ANSImage: unknown dithering mode")
)

// ScaleMode type is used for image scale mode constants.
type ScaleMode uint8

// DitheringMode type is used for image scale dithering mode constants.
type DitheringMode uint8

// ANSIpixel represents a pixel of an ANSImage.
type ANSIpixel struct {
	Brightness uint8
	R, G, B    uint8
	upper      bool
	source     *ANSImage
}

// ANSImage represents an image encoded in ANSI escape codes.
type ANSImage struct {
	h, w      int
	maxprocs  int
	bgR       uint8
	bgG       uint8
	bgB       uint8
	dithering DitheringMode
	pixmap    []ANSIpixel
}

// Render returns the ANSI-compatible string form of ANSI-pixel.
func (ap *ANSIpixel) Render() string {
	return ap.RenderExt(false, false)
}

// RenderExt returns the ANSI-compatible string form of ANSI-pixel.
// Can specify if it renders in form of Go code 'fmt.Printf()'.
// Can specify if background color will be disabled in dithering mode.
func (ap *ANSIpixel) RenderExt(renderGoCode, disableBgColor bool) string {
	var builder strings.Builder
	builder.Grow(48)
	writeANSIPixel(&builder, ap, ansiEscape(renderGoCode), disableBgColor)
	return builder.String()
}

func ansiEscape(renderGoCode bool) string {
	if renderGoCode {
		return "\\033"
	}
	return "\033"
}

func writeANSIPixel(builder *strings.Builder, ap *ANSIpixel, backslash033 string, disableBgColor bool) {
	// WITHOUT DITHERING
	if ap.source.dithering == NoDithering {
		if ap.upper {
			writeANSIColor(builder, backslash033, "48", ap.R, ap.G, ap.B)
		} else {
			writeANSIColor(builder, backslash033, "38", ap.R, ap.G, ap.B)
			builder.WriteString(lowerHalfBlock)
		}
		return
	}

	// WITH DITHERING
	block := " "
	if ap.source.dithering == DitheringWithBlocks {
		switch bri := ap.Brightness; {
		case bri > 204:
			block = fullBlock
		case bri > 152:
			block = darkShadeBlock
		case bri > 100:
			block = mediumShadeBlock
		case bri > 48:
			block = lightShadeBlock
		}
	} else if ap.source.dithering == DitheringWithChars {
		switch bri := ap.Brightness; {
		case bri > 230:
			block = "#"
		case bri > 207:
			block = "&"
		case bri > 184:
			block = "$"
		case bri > 161:
			block = "X"
		case bri > 138:
			block = "x"
		case bri > 115:
			block = "="
		case bri > 92:
			block = "+"
		case bri > 69:
			block = ";"
		case bri > 46:
			block = ":"
		case bri > 23:
			block = "."
		}
	} else {
		panic(errUnknownDitheringMode)
	}

	if !disableBgColor {
		writeANSIColor(builder, backslash033, "48", ap.source.bgR, ap.source.bgG, ap.source.bgB)
	}
	writeANSIColor(builder, backslash033, "38", ap.R, ap.G, ap.B)
	builder.WriteString(block)
}

func writeANSIColor(builder *strings.Builder, backslash033, colorCode string, r, g, b uint8) {
	builder.WriteString(backslash033)
	builder.WriteByte('[')
	builder.WriteString(colorCode)
	builder.WriteString(";2;")
	writeUint8(builder, r)
	builder.WriteByte(';')
	writeUint8(builder, g)
	builder.WriteByte(';')
	writeUint8(builder, b)
	builder.WriteByte('m')
}

func writeUint8(builder *strings.Builder, v uint8) {
	if v >= 100 {
		builder.WriteByte('0' + v/100)
		builder.WriteByte('0' + v/10%10)
		builder.WriteByte('0' + v%10)
		return
	}
	if v >= 10 {
		builder.WriteByte('0' + v/10)
		builder.WriteByte('0' + v%10)
		return
	}
	builder.WriteByte('0' + v)
}

func writeReset(builder *strings.Builder, backslash033, backslashN string) {
	builder.WriteString(backslash033)
	builder.WriteString("[0m")
	builder.WriteString(backslashN)
}

func wrapGoPrint(render string) string {
	var builder strings.Builder
	builder.Grow(len(render) + len(`fmt.Print("")`) + 1)
	builder.WriteString(`fmt.Print("`)
	builder.WriteString(render)
	builder.WriteString(`")`)
	builder.WriteByte('\n')
	return builder.String()
}

// Height gets total rows of ANSImage.
func (ai *ANSImage) Height() int {
	return ai.h
}

// Width gets total columns of ANSImage.
func (ai *ANSImage) Width() int {
	return ai.w
}

// DitheringMode gets the dithering mode of ANSImage.
func (ai *ANSImage) DitheringMode() DitheringMode {
	return ai.dithering
}

// SetMaxProcs sets the maximum number of parallel goroutines to render the ANSImage
// (user should manually sets `runtime.GOMAXPROCS(max)` before to this change takes effect).
// Values less than 1 are clamped to 1; otherwise the render loop's outer
// step `y += ai.maxprocs` would never advance (or go negative).
func (ai *ANSImage) SetMaxProcs(max int) {
	if max < 1 {
		max = 1
	}
	ai.maxprocs = max
}

// GetMaxProcs gets the maximum number of parallels goroutines to render the ANSImage.
func (ai *ANSImage) GetMaxProcs() int {
	return ai.maxprocs
}

func (ai *ANSImage) pixelIndex(y, x int) int {
	return y*ai.w + x
}

func (ai *ANSImage) pixelAt(y, x int) *ANSIpixel {
	return &ai.pixmap[ai.pixelIndex(y, x)]
}

// SetAt sets ANSI-pixel color (RBG) and brightness in coordinates (y,x).
func (ai *ANSImage) SetAt(y, x int, r, g, b, brightness uint8) error {
	if y >= 0 && y < ai.h && x >= 0 && x < ai.w {
		p := ai.pixelAt(y, x)
		p.R = r
		p.G = g
		p.B = b
		p.Brightness = brightness
		p.upper = ((ai.dithering == NoDithering) && (y%2 == 0))
		p.source = ai
		return nil
	}
	return ErrOutOfBounds
}

// GetAt gets ANSI-pixel in coordinates (y,x).
func (ai *ANSImage) GetAt(y, x int) (*ANSIpixel, error) {
	if y >= 0 && y < ai.h && x >= 0 && x < ai.w {
		p := ai.pixelAt(y, x)
		return &ANSIpixel{
				R:          p.R,
				G:          p.G,
				B:          p.B,
				Brightness: p.Brightness,
				upper:      p.upper,
				source:     p.source,
			},
			nil
	}
	return nil, ErrOutOfBounds
}

// Render returns the ANSI-compatible string form of ANSImage.
func (ai *ANSImage) Render() string {
	return ai.RenderExt(false, false)
}

// RenderExt returns the ANSI-compatible string form of ANSImage.
// Can specify if it renders in form of Go code 'fmt.Printf()'.
// Can specify if background color will be disabled in dithering mode.
// (Nice info for ANSI True Colour - https://gist.github.com/XVilka/8346728)
func (ai *ANSImage) RenderExt(renderGoCode, disableBgColor bool) string {
	type renderData struct {
		row    int
		render string
	}

	backslashN := "\n"
	backslash033 := ansiEscape(renderGoCode)
	if renderGoCode {
		backslashN = "\\n"
	}

	// WITHOUT DITHERING
	// Each output row consumes two pixmap rows (upper + lower), so there are
	// ai.h/2 output rows. The outer loop iterates over output-row batches of
	// size ai.maxprocs; the inner loop launches one goroutine per output row
	// in the batch.
	if ai.dithering == NoDithering {
		nRows := ai.h / 2
		rows := make([]string, nRows)
		for batch := 0; batch < nRows; batch += ai.maxprocs {
			ch := make(chan renderData, ai.maxprocs)
			for n, r := 0, batch; (n < ai.maxprocs) && (r < nRows); n, r = n+1, batch+n+1 {
				go func(r int) {
					py := 2 * r
					upperRow := py * ai.w
					lowerRow := (py + 1) * ai.w
					var builder strings.Builder
					builder.Grow(ai.w*48 + 16)
					for x := 0; x < ai.w; x++ {
						writeANSIPixel(&builder, &ai.pixmap[upperRow+x], backslash033, disableBgColor) // upper pixel
						writeANSIPixel(&builder, &ai.pixmap[lowerRow+x], backslash033, disableBgColor) // lower pixel
					}
					writeReset(&builder, backslash033, backslashN) // reset ansi style
					ch <- renderData{row: r, render: builder.String()}
				}(r)
			}
			for n, r := 0, batch; (n < ai.maxprocs) && (r < nRows); n, r = n+1, batch+n+1 {
				data := <-ch
				if renderGoCode {
					data.render = wrapGoPrint(data.render)
				}
				rows[data.row] = data.render
			}
		}
		return strings.Join(rows, "")
	}

	// WITH DITHERING
	// One output row per pixmap row; outer loop iterates over batches of ai.maxprocs rows.
	rows := make([]string, ai.h)
	for batch := 0; batch < ai.h; batch += ai.maxprocs {
		ch := make(chan renderData, ai.maxprocs)
		for n, r := 0, batch; (n < ai.maxprocs) && (r < ai.h); n, r = n+1, batch+n+1 {
			go func(r int) {
				row := r * ai.w
				var builder strings.Builder
				builder.Grow(ai.w*48 + 16)
				for x := 0; x < ai.w; x++ {
					writeANSIPixel(&builder, &ai.pixmap[row+x], backslash033, disableBgColor)
				}
				writeReset(&builder, backslash033, backslashN) // reset ansi style
				ch <- renderData{row: r, render: builder.String()}
			}(r)
		}
		for n, r := 0, batch; (n < ai.maxprocs) && (r < ai.h); n, r = n+1, batch+n+1 {
			data := <-ch
			if renderGoCode {
				data.render = wrapGoPrint(data.render)
			}
			rows[data.row] = data.render
		}
	}
	return strings.Join(rows, "")
}

// Draw writes the ANSImage to standard output (terminal).
func (ai *ANSImage) Draw() {
	ai.DrawExt(false, false)
}

// DrawExt writes the ANSImage to standard output (terminal).
// Can specify if it prints in form of Go code 'fmt.Printf()'.
// Can specify if background color will be disabled in dithering mode.
func (ai *ANSImage) DrawExt(renderGoCode, disableBgColor bool) {
	fmt.Print(ai.RenderExt(renderGoCode, disableBgColor))
}

// New creates a new empty ANSImage ready to draw on it.
func New(h, w int, bg color.Color, dm DitheringMode) (*ANSImage, error) {
	if (dm == NoDithering) && (h%2 != 0) {
		return nil, ErrHeightNonMoT
	}

	if h < 2 || w < 2 {
		return nil, ErrInvalidBoundsMoT
	}

	r, g, b, _ := bg.RGBA()
	ansimage := &ANSImage{
		h: h, w: w,
		maxprocs:  1,
		bgR:       uint8(r),
		bgG:       uint8(g),
		bgB:       uint8(b),
		dithering: dm,
		pixmap:    make([]ANSIpixel, h*w),
	}

	for y := 0; y < h; y++ {
		upper := (dm == NoDithering) && (y%2 == 0)
		row := y * w
		for x := 0; x < w; x++ {
			p := &ansimage.pixmap[row+x]
			p.source = ansimage
			p.upper = upper
		}
	}

	return ansimage, nil
}

// NewFromImage creates a new ANSImage from an image.Image.
// Background color is used to fill when image has transparency or dithering mode is enabled.
// Dithering mode is used to specify the way that ANSImage render ANSI-pixels (char/block elements).
func NewFromImage(image image.Image, bg color.Color, dm DitheringMode) (*ANSImage, error) {
	return createANSImage(image, bg, dm)
}

// NewScaledFromImage creates a new scaled ANSImage from an image.Image.
// Background color is used to fill when image has transparency or dithering mode is enabled.
// Dithering mode is used to specify the way that ANSImage render ANSI-pixels (char/block elements).
func NewScaledFromImage(image image.Image, y, x int, bg color.Color, sm ScaleMode, dm DitheringMode) (*ANSImage, error) {
	switch sm {
	case ScaleModeResize:
		image = imaging.Resize(image, x, y, imaging.Lanczos)
	case ScaleModeFill:
		image = imaging.Fill(image, x, y, imaging.Center, imaging.Lanczos)
	case ScaleModeFit:
		image = imaging.Fit(image, x, y, imaging.Lanczos)
	default:
		panic(errUnknownScaleMode)
	}

	return createANSImage(image, bg, dm)
}

// NewFromReader creates a new ANSImage from an io.Reader.
// Background color is used to fill when image has transparency or dithering mode is enabled.
// Dithering mode is used to specify the way that ANSImage render ANSI-pixels (char/block elements).
func NewFromReader(reader io.Reader, bg color.Color, dm DitheringMode) (*ANSImage, error) {
	image, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}

	return createANSImage(image, bg, dm)
}

// NewScaledFromReader creates a new scaled ANSImage from an io.Reader.
// Background color is used to fill when image has transparency or dithering mode is enabled.
// Dithering mode is used to specify the way that ANSImage render ANSI-pixels (char/block elements).
func NewScaledFromReader(reader io.Reader, y, x int, bg color.Color, sm ScaleMode, dm DitheringMode) (*ANSImage, error) {
	image, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}

	switch sm {
	case ScaleModeResize:
		image = imaging.Resize(image, x, y, imaging.Lanczos)
	case ScaleModeFill:
		image = imaging.Fill(image, x, y, imaging.Center, imaging.Lanczos)
	case ScaleModeFit:
		image = imaging.Fit(image, x, y, imaging.Lanczos)
	default:
		panic(errUnknownScaleMode)
	}

	return createANSImage(image, bg, dm)
}

// NewFromFile creates a new ANSImage from a file.
// Background color is used to fill when image has transparency or dithering mode is enabled.
// Dithering mode is used to specify the way that ANSImage render ANSI-pixels (char/block elements).
func NewFromFile(name string, bg color.Color, dm DitheringMode) (*ANSImage, error) {
	reader, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return NewFromReader(reader, bg, dm)
}

// NewScaledFromFile creates a new scaled ANSImage from a file.
// Background color is used to fill when image has transparency or dithering mode is enabled.
// Dithering mode is used to specify the way that ANSImage render ANSI-pixels (char/block elements).
func NewScaledFromFile(name string, y, x int, bg color.Color, sm ScaleMode, dm DitheringMode) (*ANSImage, error) {
	reader, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return NewScaledFromReader(reader, y, x, bg, sm, dm)
}

// NewFromURL creates a new ANSImage from an image URL.
// Background color is used to fill when image has transparency or dithering mode is enabled.
// Dithering mode is used to specify the way that ANSImage render ANSI-pixels (char/block elements).
func NewFromURL(url string, bg color.Color, dm DitheringMode) (*ANSImage, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, ErrImageDownloadFailed
	}
	defer res.Body.Close()
	return NewFromReader(res.Body, bg, dm)
}

// NewScaledFromURL creates a new scaled ANSImage from an image URL.
// Background color is used to fill when image has transparency or dithering mode is enabled.
// Dithering mode is used to specify the way that ANSImage render ANSI-pixels (char/block elements).
func NewScaledFromURL(url string, y, x int, bg color.Color, sm ScaleMode, dm DitheringMode) (*ANSImage, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, ErrImageDownloadFailed
	}
	defer res.Body.Close()
	return NewScaledFromReader(res.Body, y, x, bg, sm, dm)
}

// ClearTerminal clears current terminal buffer using ANSI escape code.
// (Nice info for ANSI escape codes - https://unix.stackexchange.com/questions/124762/how-does-clear-command-work)
func ClearTerminal() {
	fmt.Print("\033[H\033[2J")
}

// createANSImage loads data from an image and returns an ANSImage.
// Background color is used to fill when image has transparency or dithering mode is enabled.
// Dithering mode is used to specify the way that ANSImage render ANSI-pixels (char/block elements).
func createANSImage(img image.Image, bg color.Color, dm DitheringMode) (*ANSImage, error) {
	var rgbaOut *image.RGBA
	bounds := img.Bounds()

	// do compositing only if background color has no transparency (thank you @disq for the idea!)
	// (info - https://stackoverflow.com/questions/36595687/transparent-pixel-color-go-lang-image)
	//
	// Source point for the img copies is bounds.Min, not (0,0): draw.Draw
	// reads `img.At(p - bounds.Min + sp)` for each destination pixel `p`,
	// so for sub-images (where bounds.Min is non-zero) sp=(0,0) would land
	// outside img's actual data. The uniform-bg fill keeps sp=(0,0) since
	// image.NewUniform has infinite bounds.
	if _, _, _, a := bg.RGBA(); a >= 0xffff {
		rgbaOut = image.NewRGBA(bounds)
		draw.Draw(rgbaOut, bounds, image.NewUniform(bg), image.Point{}, draw.Src)
		draw.Draw(rgbaOut, bounds, img, bounds.Min, draw.Over)
	} else {
		if v, ok := img.(*image.RGBA); ok {
			rgbaOut = v
		} else {
			rgbaOut = image.NewRGBA(bounds)
			draw.Draw(rgbaOut, bounds, img, bounds.Min, draw.Src)
		}
	}

	// Pixel-space bounds of the source. For sub-images returned from
	// img.SubImage, Min may be non-zero; we read from rgbaOut at absolute
	// pixel coordinates and write to the pixmap at zero-based indices.
	yMin, xMin := bounds.Min.Y, bounds.Min.X
	yMax, xMax := bounds.Max.Y, bounds.Max.X

	// Pixmap dimensions, in pixmap-index space.
	h := yMax - yMin
	w := xMax - xMin

	if dm == NoDithering {
		// always sets an even number of ANSIPixel rows...
		h = h - h%2 // one for upper pixel and another for lower pixel --> without dithering
	} else {
		h = h / BlockSizeY // always sets 1 ANSIPixel block...
		w = w / BlockSizeX // per 8x4 real pixels --> with dithering
	}

	ansimage, err := New(h, w, bg, dm)
	if err != nil {
		return nil, err
	}

	if dm == NoDithering {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := rgbaOut.RGBAAt(xMin+x, yMin+y)
				if err := ansimage.SetAt(y, x, v.R, v.G, v.B, 0); err != nil {
					return nil, err
				}
			}
		}
	} else {
		pixelCount := BlockSizeY * BlockSizeX

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {

				var sumR, sumG, sumB, sumBri float64
				for dy := 0; dy < BlockSizeY; dy++ {
					py := yMin + BlockSizeY*y + dy

					for dx := 0; dx < BlockSizeX; dx++ {
						px := xMin + BlockSizeX*x + dx

						pixel := rgbaOut.At(px, py)
						color, _ := colorful.MakeColor(pixel)
						_, _, v := color.Hsv()
						sumR += color.R
						sumG += color.G
						sumB += color.B
						sumBri += v
					}
				}

				r := uint8(sumR/float64(pixelCount)*255.0 + 0.5)
				g := uint8(sumG/float64(pixelCount)*255.0 + 0.5)
				b := uint8(sumB/float64(pixelCount)*255.0 + 0.5)
				brightness := uint8(sumBri/float64(pixelCount)*255.0 + 0.5)

				if err := ansimage.SetAt(y, x, r, g, b, brightness); err != nil {
					return nil, err
				}
			}
		}
	}

	return ansimage, nil
}
