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
