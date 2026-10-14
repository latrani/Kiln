package smoke

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// tolerance is how far a channel may be off (of 255) and still match:
// enough for antialiasing jitter, not for a different color.
const tolerance = 16

// differs reports how many pixels of got and want differ; every pixel
// when their sizes don't match.
func differs(got, want image.Image) int {
	gb, wb := got.Bounds(), want.Bounds()
	if gb.Size() != wb.Size() {
		return max(gb.Dx()*gb.Dy(), wb.Dx()*wb.Dy())
	}
	n := 0
	for y := 0; y < gb.Dy(); y++ {
		for x := 0; x < gb.Dx(); x++ {
			if !near(got.At(gb.Min.X+x, gb.Min.Y+y), want.At(wb.Min.X+x, wb.Min.Y+y)) {
				n++
			}
		}
	}
	return n
}

// near reports whether a and b match, each channel within tolerance.
func near(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	off := func(x, y uint32) bool {
		d := int(x>>8) - int(y>>8)
		return d > tolerance || d < -tolerance
	}
	return !off(ar, br) && !off(ag, bg) && !off(ab, bb) && !off(aa, ba)
}

// diffImage is want at a third of its brightness, each pixel that
// differs from got in magenta (past got's edge, too).
func diffImage(got, want image.Image) image.Image {
	gb, wb := got.Bounds(), want.Bounds()
	d := image.NewRGBA(image.Rect(0, 0, wb.Dx(), wb.Dy()))
	for y := 0; y < wb.Dy(); y++ {
		for x := 0; x < wb.Dx(); x++ {
			w := want.At(wb.Min.X+x, wb.Min.Y+y)
			if x >= gb.Dx() || y >= gb.Dy() || !near(got.At(gb.Min.X+x, gb.Min.Y+y), w) {
				d.Set(x, y, color.RGBA{255, 0, 255, 255})
				continue
			}
			r, g, b, _ := w.RGBA()
			d.Set(x, y, color.RGBA{uint8(r >> 8 / 3), uint8(g >> 8 / 3), uint8(b >> 8 / 3), 255})
		}
	}
	return d
}

// tb is the part of *testing.T checkShot uses.
type tb interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
}

// checkShot compares shot, a PNG, with the golden at path. With update
// it writes the golden instead. With no golden it says how to make one
// and reports false, so the caller can take its other shots and skip
// after. When they differ it fails, leaving in failed/ next to the
// golden what the page drew (<name>.png) and the diff (<name>.diff.png);
// when they match it clears those from an earlier run.
func checkShot(t tb, path string, shot []byte, update bool) bool {
	t.Helper()
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("%v", err)
		}
		if err := os.WriteFile(path, shot, 0o644); err != nil {
			t.Fatalf("%v", err)
		}
		return true
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Logf("no golden %s yet: run with -update to make it", path)
		return false
	}
	if err != nil {
		t.Fatalf("%v", err)
		return true
	}
	want, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
		return true
	}
	got, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		t.Fatalf("the screenshot: %v", err)
		return true
	}
	failed := filepath.Join(filepath.Dir(path), "failed")
	base := strings.TrimSuffix(filepath.Base(path), ".png")
	n := differs(got, want)
	if n == 0 {
		for _, f := range []string{base + ".png", base + ".diff.png"} {
			os.Remove(filepath.Join(failed, f))
		}
		return true
	}
	var diff bytes.Buffer
	if err := png.Encode(&diff, diffImage(got, want)); err != nil {
		t.Fatalf("%v", err)
		return true
	}
	if err := os.MkdirAll(failed, 0o755); err != nil {
		t.Fatalf("%v", err)
		return true
	}
	os.WriteFile(filepath.Join(failed, base+".png"), shot, 0o644)
	os.WriteFile(filepath.Join(failed, base+".diff.png"), diff.Bytes(), 0o644)
	t.Errorf("%s: %d pixels differ; see %s", path, n, failed)
	return true
}
