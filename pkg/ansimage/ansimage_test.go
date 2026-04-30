package ansimage

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"
)

// TestRenderExt_NoDithering_LineCount verifies that the NoDithering branch of
// RenderExt produces exactly ai.h/2 cell rows. The pre-fix implementation
// undercounted by one because rows[0] was never assigned (the inner loop
// started r at y+1) and strings.Join silently drops the empty leading slot.
func TestRenderExt_NoDithering_LineCount(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	a, err := NewScaledFromImage(src, 40, 80, color.Transparent, ScaleModeFit, NoDithering)
	if err != nil {
		t.Fatalf("NewScaledFromImage: %v", err)
	}
	out := strings.TrimRight(a.RenderExt(false, false), "\n")
	got := strings.Count(out, "\n") + 1
	want := a.h / 2
	if got != want {
		t.Errorf("NoDithering: RenderExt produced %d lines, want %d (ai.h=%d)", got, want, a.h)
	}
}

// TestRenderExt_DitheringWithBlocks_LineCount verifies that the dithering
// branch produces exactly ai.h cell rows. The pre-fix implementation
// undercounted by one at the OTHER end because the bound check `r+1 < ai.h`
// excluded r = ai.h-1, so the FINAL row was never assigned.
func TestRenderExt_DitheringWithBlocks_LineCount(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 50, G: 200, B: 50, A: 255})
		}
	}
	a, err := NewScaledFromImage(src, 40, 80, color.Transparent, ScaleModeFit, DitheringWithBlocks)
	if err != nil {
		t.Fatalf("NewScaledFromImage: %v", err)
	}
	out := strings.TrimRight(a.RenderExt(false, false), "\n")
	got := strings.Count(out, "\n") + 1
	want := a.h
	if got != want {
		t.Errorf("DitheringWithBlocks: RenderExt produced %d lines, want %d (ai.h=%d)", got, want, a.h)
	}
}

// TestRenderExt_DitheringWithChars_LineCount mirrors the DitheringWithBlocks
// test for the second dithering mode, since both go through the same loop.
func TestRenderExt_DitheringWithChars_LineCount(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 50, G: 50, B: 200, A: 255})
		}
	}
	a, err := NewScaledFromImage(src, 40, 80, color.Transparent, ScaleModeFit, DitheringWithChars)
	if err != nil {
		t.Fatalf("NewScaledFromImage: %v", err)
	}
	out := strings.TrimRight(a.RenderExt(false, false), "\n")
	got := strings.Count(out, "\n") + 1
	want := a.h
	if got != want {
		t.Errorf("DitheringWithChars: RenderExt produced %d lines, want %d (ai.h=%d)", got, want, a.h)
	}
}

// renderWithTimeout runs RenderExt in a goroutine and fails the test if it
// does not complete within the deadline. Used to detect infinite-loop bugs
// in the render machinery (e.g. when maxprocs is set to a non-positive value
// the outer `for y := 0; y < ai.h; y += ai.maxprocs` would never advance).
func renderWithTimeout(t *testing.T, a *ANSImage, deadline time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		_ = a.RenderExt(false, false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(deadline):
		t.Fatalf("RenderExt did not return within %s (likely infinite loop)", deadline)
	}
}

func gradientRGBA(w, h int) *image.RGBA {
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

type wantANSIPixel struct {
	r, g, b, brightness uint8
}

func requirePixel(t *testing.T, a *ANSImage, y, x int, want wantANSIPixel) {
	t.Helper()
	p, err := a.GetAt(y, x)
	if err != nil {
		t.Fatalf("GetAt(%d,%d): %v", y, x, err)
	}
	if p.R != want.r || p.G != want.g || p.B != want.b || p.Brightness != want.brightness {
		t.Fatalf("pixel at (%d,%d): got RGBA(%d,%d,%d,%d), want RGBA(%d,%d,%d,%d)",
			y, x, p.R, p.G, p.B, p.Brightness, want.r, want.g, want.b, want.brightness)
	}
}

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name string
		h, w int
		dm   DitheringMode
		want error
	}{
		{"NoDitheringOddHeight", 3, 2, NoDithering, ErrHeightNonMoT},
		{"TooShort", 1, 2, DitheringWithBlocks, ErrInvalidBoundsMoT},
		{"TooNarrow", 2, 1, DitheringWithChars, ErrInvalidBoundsMoT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.h, tt.w, color.Transparent, tt.dm)
			if err != tt.want {
				t.Fatalf("New(%d,%d): got %v, want %v", tt.h, tt.w, err, tt.want)
			}
		})
	}
}

func TestSetAtGetAtBoundsAndCopy(t *testing.T) {
	a, err := New(2, 2, color.Transparent, NoDithering)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := a.SetAt(0, 0, 10, 20, 30, 40); err != nil {
		t.Fatalf("SetAt: %v", err)
	}
	p, err := a.GetAt(0, 0)
	if err != nil {
		t.Fatalf("GetAt: %v", err)
	}
	p.R = 99
	again, err := a.GetAt(0, 0)
	if err != nil {
		t.Fatalf("GetAt again: %v", err)
	}
	if again.R != 10 {
		t.Fatalf("GetAt returned an alias: got R=%d after mutating copy, want 10", again.R)
	}
	if got, want := again.Render(), "\033[48;2;10;20;30m"; got != want {
		t.Fatalf("GetAt returned a non-renderable pixel: Render() = %q, want %q", got, want)
	}

	if err := a.SetAt(-1, 0, 0, 0, 0, 0); err != ErrOutOfBounds {
		t.Fatalf("SetAt out of bounds: got %v, want %v", err, ErrOutOfBounds)
	}
	if _, err := a.GetAt(0, 2); err != ErrOutOfBounds {
		t.Fatalf("GetAt out of bounds: got %v, want %v", err, ErrOutOfBounds)
	}
}

func TestWriteUint8(t *testing.T) {
	tests := []struct {
		value uint8
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{42, "42"},
		{99, "99"},
		{100, "100"},
		{101, "101"},
		{255, "255"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			var builder strings.Builder
			writeUint8(&builder, tt.value)
			if got := builder.String(); got != tt.want {
				t.Fatalf("writeUint8(%d) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestRenderExt_NoDithering_ExactANSI(t *testing.T) {
	a, err := New(2, 2, color.Transparent, NoDithering)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	values := []struct {
		y, x         int
		r, g, b, bri uint8
	}{
		{0, 0, 1, 2, 3, 0},
		{1, 0, 7, 8, 9, 0},
		{0, 1, 4, 5, 6, 0},
		{1, 1, 10, 11, 12, 0},
	}
	for _, v := range values {
		if err := a.SetAt(v.y, v.x, v.r, v.g, v.b, v.bri); err != nil {
			t.Fatalf("SetAt(%d,%d): %v", v.y, v.x, err)
		}
	}

	want := "\033[48;2;1;2;3m" +
		"\033[38;2;7;8;9m" + lowerHalfBlock +
		"\033[48;2;4;5;6m" +
		"\033[38;2;10;11;12m" + lowerHalfBlock +
		"\033[0m\n"
	if got := a.RenderExt(false, false); got != want {
		t.Fatalf("RenderExt exact output mismatch:\ngot  %q\nwant %q", got, want)
	}
}

func TestRenderExt_DitheringWithBlocks_ExactANSI(t *testing.T) {
	a, err := New(2, 2, color.RGBA{R: 1, G: 2, B: 3, A: 255}, DitheringWithBlocks)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	values := []struct {
		y, x         int
		r, g, b, bri uint8
	}{
		{0, 0, 10, 20, 30, 205},
		{0, 1, 40, 50, 60, 153},
		{1, 0, 70, 80, 90, 101},
		{1, 1, 100, 110, 120, 49},
	}
	for _, v := range values {
		if err := a.SetAt(v.y, v.x, v.r, v.g, v.b, v.bri); err != nil {
			t.Fatalf("SetAt(%d,%d): %v", v.y, v.x, err)
		}
	}

	want := "\033[48;2;1;2;3m\033[38;2;10;20;30m" + fullBlock +
		"\033[48;2;1;2;3m\033[38;2;40;50;60m" + darkShadeBlock +
		"\033[0m\n" +
		"\033[48;2;1;2;3m\033[38;2;70;80;90m" + mediumShadeBlock +
		"\033[48;2;1;2;3m\033[38;2;100;110;120m" + lightShadeBlock +
		"\033[0m\n"
	if got := a.RenderExt(false, false); got != want {
		t.Fatalf("RenderExt exact output mismatch:\ngot  %q\nwant %q", got, want)
	}
}

func TestRenderExt_DitheringWithChars_DisableBackground_ExactANSI(t *testing.T) {
	a, err := New(2, 2, color.RGBA{R: 1, G: 2, B: 3, A: 255}, DitheringWithChars)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	values := []struct {
		y, x         int
		r, g, b, bri uint8
	}{
		{0, 0, 10, 20, 30, 231},
		{0, 1, 40, 50, 60, 208},
		{1, 0, 70, 80, 90, 24},
		{1, 1, 100, 110, 120, 23},
	}
	for _, v := range values {
		if err := a.SetAt(v.y, v.x, v.r, v.g, v.b, v.bri); err != nil {
			t.Fatalf("SetAt(%d,%d): %v", v.y, v.x, err)
		}
	}

	want := "\033[38;2;10;20;30m#" +
		"\033[38;2;40;50;60m&" +
		"\033[0m\n" +
		"\033[38;2;70;80;90m." +
		"\033[38;2;100;110;120m " +
		"\033[0m\n"
	if got := a.RenderExt(false, true); got != want {
		t.Fatalf("RenderExt exact output mismatch:\ngot  %q\nwant %q", got, want)
	}
}

func TestRenderExt_GoCodeOutput(t *testing.T) {
	a, err := New(2, 2, color.Transparent, NoDithering)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.SetAt(0, 0, 1, 2, 3, 0); err != nil {
		t.Fatalf("SetAt: %v", err)
	}
	if err := a.SetAt(1, 0, 4, 5, 6, 0); err != nil {
		t.Fatalf("SetAt: %v", err)
	}
	if err := a.SetAt(0, 1, 7, 8, 9, 0); err != nil {
		t.Fatalf("SetAt: %v", err)
	}
	if err := a.SetAt(1, 1, 10, 11, 12, 0); err != nil {
		t.Fatalf("SetAt: %v", err)
	}

	got := a.RenderExt(true, false)
	if !strings.HasPrefix(got, `fmt.Print("\033[48;2;1;2;3m`) {
		t.Fatalf("Go-code output prefix mismatch: %q", got)
	}
	if !strings.HasSuffix(got, `\033[0m\n")`+"\n") {
		t.Fatalf("Go-code output suffix mismatch: %q", got)
	}
}

// TestSetMaxProcsZero_DoesNotHang ensures that calling SetMaxProcs(0) does
// not cause RenderExt to spin forever. Without a clamp, the outer render
// loop is `for y := 0; y < ai.h; y += ai.maxprocs` and `y += 0` never makes
// progress.
func TestSetMaxProcsZero_DoesNotHang(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 16, 8))
	a, err := NewScaledFromImage(src, 8, 16, color.Transparent, ScaleModeFit, NoDithering)
	if err != nil {
		t.Fatalf("NewScaledFromImage: %v", err)
	}
	a.SetMaxProcs(0)
	if got := a.GetMaxProcs(); got < 1 {
		t.Fatalf("GetMaxProcs after SetMaxProcs(0) = %d, want >= 1", got)
	}
	renderWithTimeout(t, a, 2*time.Second)
}

// TestSetMaxProcsNegative_DoesNotHang ensures that a negative maxprocs is
// also clamped. With a negative value the outer loop step is negative, so
// `y` decreases and never reaches `ai.h`.
func TestSetMaxProcsNegative_DoesNotHang(t *testing.T) {
	// Dithering needs at least BlockSizeY rows and BlockSizeX cols of source
	// per output pixmap cell, so we use a source large enough to give ai.h>=2.
	src := image.NewRGBA(image.Rect(0, 0, 32, 16))
	a, err := NewScaledFromImage(src, 16, 32, color.Transparent, ScaleModeResize, DitheringWithBlocks)
	if err != nil {
		t.Fatalf("NewScaledFromImage: %v", err)
	}
	a.SetMaxProcs(-3)
	if got := a.GetMaxProcs(); got < 1 {
		t.Fatalf("GetMaxProcs after SetMaxProcs(-3) = %d, want >= 1", got)
	}
	renderWithTimeout(t, a, 2*time.Second)
}

// TestNewFromImage_SubImage_NoDithering exercises createANSImage with an
// image whose Bounds().Min is non-zero (i.e., the result of img.SubImage).
// The pre-fix loops used bounds.Min.{X,Y} as loop start indices but treated
// those values as pixmap indices, leaving the first bounds.Min rows/cols of
// the pixmap zero-initialized and producing the wrong overall size.
func TestNewFromImage_SubImage_NoDithering(t *testing.T) {
	parent := image.NewRGBA(image.Rect(0, 0, 80, 80))
	fill := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			parent.SetRGBA(x, y, fill)
		}
	}
	sub := parent.SubImage(image.Rect(20, 10, 60, 50))
	if sub.Bounds().Min == (image.Point{}) {
		t.Fatalf("test setup: expected non-zero Bounds().Min, got %v", sub.Bounds())
	}

	a, err := NewFromImage(sub, color.Transparent, NoDithering)
	if err != nil {
		t.Fatalf("NewFromImage: %v", err)
	}

	// Sub-image is 40x40 in size; NoDithering keeps dimensions when the
	// height is already even, so the result should be 40x40.
	if a.h != 40 || a.w != 40 {
		t.Fatalf("dimensions: got %dx%d, want 40x40", a.h, a.w)
	}

	for y := 0; y < a.h; y++ {
		for x := 0; x < a.w; x++ {
			p, _ := a.GetAt(y, x)
			if p.R != fill.R || p.G != fill.G || p.B != fill.B {
				t.Fatalf("pixel at (%d,%d): got RGB(%d,%d,%d), want RGB(%d,%d,%d)",
					y, x, p.R, p.G, p.B, fill.R, fill.G, fill.B)
			}
		}
	}
}

// TestNewFromImage_SubImage_Dithering is the dithering counterpart. With the
// pre-fix code the dithering branch was even worse: yMax was divided by
// BlockSizeY but yMin was left in pixel space, so for any sub-image with
// yMin > yMax/BlockSizeY the inner loop never ran at all.
func TestNewFromImage_SubImage_Dithering(t *testing.T) {
	parent := image.NewRGBA(image.Rect(0, 0, 80, 80))
	fill := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			parent.SetRGBA(x, y, fill)
		}
	}
	sub := parent.SubImage(image.Rect(16, 16, 80, 80))

	a, err := NewFromImage(sub, color.Transparent, DitheringWithBlocks)
	if err != nil {
		t.Fatalf("NewFromImage: %v", err)
	}

	// Sub-image is 64x64; dithering divides height by BlockSizeY and width
	// by BlockSizeX, so the result should be 8 rows by 16 cols.
	if a.h != 64/BlockSizeY || a.w != 64/BlockSizeX {
		t.Fatalf("dimensions: got %dx%d, want %dx%d",
			a.h, a.w, 64/BlockSizeY, 64/BlockSizeX)
	}

	// Every block-averaged cell should reflect the uniform input color
	// (allow ±1 for RGB→HSV→RGB rounding).
	for y := 0; y < a.h; y++ {
		for x := 0; x < a.w; x++ {
			p, _ := a.GetAt(y, x)
			dR := int(p.R) - int(fill.R)
			dG := int(p.G) - int(fill.G)
			dB := int(p.B) - int(fill.B)
			if dR < -1 || dR > 1 || dG < -1 || dG > 1 || dB < -1 || dB > 1 {
				t.Fatalf("pixel at (%d,%d): got RGB(%d,%d,%d), want ~RGB(%d,%d,%d)",
					y, x, p.R, p.G, p.B, fill.R, fill.G, fill.B)
			}
		}
	}
}

// TestNewFromImage_SubImage_OpaqueBg_NoDithering exercises the compositing
// path of createANSImage (taken when the bg color is opaque) with a
// sub-image. The pre-fix call `draw.Draw(rgbaOut, bounds, img, image.Point{},
// draw.Over)` passes (0,0) as the source point, so for each destination
// pixel `p` it reads `img.At(p - bounds.Min)`. When bounds.Min is non-zero,
// those reads land outside img's bounds and return transparent — so the
// uniform-bg fill drawn beforehand is never overwritten and the output is
// solid bg color instead of the sub-image's pixels.
func TestNewFromImage_SubImage_OpaqueBg_NoDithering(t *testing.T) {
	parent := image.NewRGBA(image.Rect(0, 0, 80, 80))
	fill := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			parent.SetRGBA(x, y, fill)
		}
	}
	sub := parent.SubImage(image.Rect(20, 10, 60, 50))
	// color.Black is opaque (alpha 0xffff), so this triggers the composite
	// branch even though the source is *image.RGBA.
	a, err := NewFromImage(sub, color.Black, NoDithering)
	if err != nil {
		t.Fatalf("NewFromImage: %v", err)
	}
	if a.h != 40 || a.w != 40 {
		t.Fatalf("dimensions: got %dx%d, want 40x40", a.h, a.w)
	}
	for y := 0; y < a.h; y++ {
		for x := 0; x < a.w; x++ {
			p, _ := a.GetAt(y, x)
			if p.R != fill.R || p.G != fill.G || p.B != fill.B {
				t.Fatalf("pixel at (%d,%d): got RGB(%d,%d,%d), want RGB(%d,%d,%d)",
					y, x, p.R, p.G, p.B, fill.R, fill.G, fill.B)
			}
		}
	}
}

// TestNewFromImage_SubImage_NonRGBA covers the third draw.Draw call (the
// `else` branch reached when bg is transparent and the source isn't already
// *image.RGBA). Same off-by-bounds.Min bug, same fix.
func TestNewFromImage_SubImage_NonRGBA(t *testing.T) {
	parent := image.NewNRGBA(image.Rect(0, 0, 80, 80))
	fill := color.NRGBA{R: 50, G: 200, B: 100, A: 255}
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			parent.SetNRGBA(x, y, fill)
		}
	}
	sub := parent.SubImage(image.Rect(20, 10, 60, 50))
	a, err := NewFromImage(sub, color.Transparent, NoDithering)
	if err != nil {
		t.Fatalf("NewFromImage: %v", err)
	}
	if a.h != 40 || a.w != 40 {
		t.Fatalf("dimensions: got %dx%d, want 40x40", a.h, a.w)
	}
	for y := 0; y < a.h; y++ {
		for x := 0; x < a.w; x++ {
			p, _ := a.GetAt(y, x)
			if p.R != fill.R || p.G != fill.G || p.B != fill.B {
				t.Fatalf("pixel at (%d,%d): got RGB(%d,%d,%d), want RGB(%d,%d,%d)",
					y, x, p.R, p.G, p.B, fill.R, fill.G, fill.B)
			}
		}
	}
}

func TestNewFromImage_NoDithering_OddHeightTruncatesLastRow(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 3))
	rowColors := []color.RGBA{
		{R: 10, G: 20, B: 30, A: 255},
		{R: 40, G: 50, B: 60, A: 255},
		{R: 70, G: 80, B: 90, A: 255},
	}
	for y, c := range rowColors {
		for x := 0; x < 3; x++ {
			src.SetRGBA(x, y, c)
		}
	}

	a, err := NewFromImage(src, color.Transparent, NoDithering)
	if err != nil {
		t.Fatalf("NewFromImage: %v", err)
	}
	if a.h != 2 || a.w != 3 {
		t.Fatalf("dimensions: got %dx%d, want 2x3", a.h, a.w)
	}
	for x := 0; x < a.w; x++ {
		requirePixel(t, a, 0, x, wantANSIPixel{r: 10, g: 20, b: 30})
		requirePixel(t, a, 1, x, wantANSIPixel{r: 40, g: 50, b: 60})
	}
}

func TestNewFromImage_OpaqueBackgroundCompositesTransparentPixels(t *testing.T) {
	bg := color.RGBA{R: 11, G: 22, B: 33, A: 255}
	opaque := color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	src.SetNRGBA(0, 0, color.NRGBA{R: 250, G: 240, B: 230, A: 0})
	src.SetNRGBA(1, 0, opaque)
	src.SetNRGBA(0, 1, opaque)
	src.SetNRGBA(1, 1, color.NRGBA{R: 250, G: 240, B: 230, A: 0})

	a, err := NewFromImage(src, bg, NoDithering)
	if err != nil {
		t.Fatalf("NewFromImage: %v", err)
	}
	requirePixel(t, a, 0, 0, wantANSIPixel{r: bg.R, g: bg.G, b: bg.B})
	requirePixel(t, a, 0, 1, wantANSIPixel{r: opaque.R, g: opaque.G, b: opaque.B})
	requirePixel(t, a, 1, 0, wantANSIPixel{r: opaque.R, g: opaque.G, b: opaque.B})
	requirePixel(t, a, 1, 1, wantANSIPixel{r: bg.R, g: bg.G, b: bg.B})
}

func TestNewFromImage_DitheringAveragesBlocks(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 8; x++ {
			switch {
			case y < 8 && x < 4:
				if (x+y)%2 == 0 {
					src.SetRGBA(x, y, color.RGBA{R: 200, A: 255})
				} else {
					src.SetRGBA(x, y, color.RGBA{A: 255})
				}
			case y < 8 && x >= 4:
				src.SetRGBA(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
			case y >= 8 && x < 4:
				src.SetRGBA(x, y, color.RGBA{R: 40, G: 90, B: 70, A: 255})
			default:
				src.SetRGBA(x, y, color.RGBA{R: 255, G: 128, B: 0, A: 255})
			}
		}
	}

	a, err := NewFromImage(src, color.Transparent, DitheringWithBlocks)
	if err != nil {
		t.Fatalf("NewFromImage: %v", err)
	}
	if a.h != 2 || a.w != 2 {
		t.Fatalf("dimensions: got %dx%d, want 2x2", a.h, a.w)
	}
	requirePixel(t, a, 0, 0, wantANSIPixel{r: 100, brightness: 100})
	requirePixel(t, a, 0, 1, wantANSIPixel{r: 10, g: 20, b: 30, brightness: 30})
	requirePixel(t, a, 1, 0, wantANSIPixel{r: 40, g: 90, b: 70, brightness: 90})
	requirePixel(t, a, 1, 1, wantANSIPixel{r: 255, g: 128, brightness: 255})
}

func TestRenderExt_MaxProcsDeterministic(t *testing.T) {
	tests := []struct {
		name string
		dm   DitheringMode
		w, h int
	}{
		{"NoDithering", NoDithering, 48, 24},
		{"DitheringWithBlocks", DitheringWithBlocks, 48, 24},
		{"DitheringWithChars", DitheringWithChars, 48, 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewFromImage(gradientRGBA(tt.w, tt.h), color.Transparent, tt.dm)
			if err != nil {
				t.Fatalf("NewFromImage: %v", err)
			}
			a.SetMaxProcs(1)
			want := a.RenderExt(false, false)
			for _, maxprocs := range []int{2, 3, 4, 7, 16} {
				a.SetMaxProcs(maxprocs)
				if got := a.RenderExt(false, false); got != want {
					t.Fatalf("RenderExt maxprocs=%d differed from maxprocs=1", maxprocs)
				}
			}
		})
	}
}

// TestRenderExt_LineCount_Sweep verifies that RenderExt produces the correct
// number of lines across a matrix of (target ai.h, maxprocs) values. This
// locks in the loop bounds: outer step, inner condition, and (for
// NoDithering) the fact that the outer loop iterates over output rows, not
// pixmap rows.
//
// Output line count must equal ai.h/2 for NoDithering and ai.h for the
// dithering modes, regardless of how maxprocs partitions the work.
func TestRenderExt_LineCount_Sweep(t *testing.T) {
	targetAIH := []int{2, 4, 8, 16, 40}
	maxprocs := []int{1, 2, 3, 4, 8}
	modes := []struct {
		name    string
		dm      DitheringMode
		wantDiv int // ai.h / wantDiv == expected line count
	}{
		{"NoDithering", NoDithering, 2},
		{"DitheringWithBlocks", DitheringWithBlocks, 1},
		{"DitheringWithChars", DitheringWithChars, 1},
	}

	for _, m := range modes {
		for _, aih := range targetAIH {
			// Pick scaled-image dimensions so createANSImage produces
			// ai.h == aih, ai.w == 2*aih. Dithering divides by BlockSize.
			var sy, sx int
			if m.dm == NoDithering {
				sy, sx = aih, 2*aih
			} else {
				sy, sx = aih*BlockSizeY, 2*aih*BlockSizeX
			}
			for _, mp := range maxprocs {
				name := fmt.Sprintf("%s/aih=%d/mp=%d", m.name, aih, mp)
				t.Run(name, func(t *testing.T) {
					src := image.NewRGBA(image.Rect(0, 0, sx, sy))
					for y := 0; y < sy; y++ {
						for x := 0; x < sx; x++ {
							src.SetRGBA(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
						}
					}
					a, err := NewScaledFromImage(src, sy, sx, color.Transparent, ScaleModeResize, m.dm)
					if err != nil {
						t.Fatalf("NewScaledFromImage: %v", err)
					}
					if a.h != aih {
						t.Fatalf("setup: got ai.h=%d, want %d", a.h, aih)
					}
					a.SetMaxProcs(mp)
					out := strings.TrimRight(a.RenderExt(false, false), "\n")
					got := strings.Count(out, "\n") + 1
					want := a.h / m.wantDiv
					if got != want {
						t.Errorf("got %d lines, want %d (ai.h=%d, maxprocs=%d)", got, want, a.h, mp)
					}
				})
			}
		}
	}
}
