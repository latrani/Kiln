package smoke

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func solid(w, h int, c color.Color) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.Set(x, y, c)
		}
	}
	return m
}

func TestDiffersCountsPixelsPastTheTolerance(t *testing.T) {
	a := solid(4, 4, color.RGBA{100, 100, 100, 255})
	b := solid(4, 4, color.RGBA{100, 100, 100, 255})
	b.Set(1, 1, color.RGBA{116, 100, 100, 255}) // 16: within
	b.Set(2, 2, color.RGBA{100, 117, 100, 255}) // 17: past
	if n := differs(a, b); n != 1 {
		t.Errorf("differs = %d, want 1", n)
	}
	if n := differs(a, solid(4, 5, color.White)); n != 4*5 {
		t.Errorf("sizes differ: %d, want every pixel", n)
	}
}

func TestDiffImageMarksWhatDiffers(t *testing.T) {
	a := solid(2, 1, color.Black)
	b := solid(2, 1, color.Black)
	b.Set(1, 0, color.White)
	d := diffImage(b, a)
	if r, g, bl, _ := d.At(1, 0).RGBA(); r>>8 != 255 || g>>8 != 0 || bl>>8 != 255 {
		t.Errorf("the differing pixel isn't magenta: %v", d.At(1, 0))
	}
	if d.At(0, 0) == d.At(1, 0) {
		t.Error("the same pixel is marked too")
	}
}

func encode(t *testing.T, m image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// rec is a T that records instead of failing.
type rec struct{ failed, skipped bool }

func (r *rec) Helper()               {}
func (r *rec) Errorf(string, ...any) { r.failed = true }
func (r *rec) Fatalf(string, ...any) { r.failed = true }
func (r *rec) Skipf(string, ...any)  { r.skipped = true }

func TestCheckShotUpdatesThenPassesThenFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "golden", "main-dark.png") // no golden dir yet
	shot := encode(t, solid(3, 3, color.Black))
	failed := filepath.Join(dir, "golden", "failed")

	r := &rec{}
	checkShot(r, path, shot, false)
	if !r.skipped || r.failed {
		t.Errorf("no golden: %+v, want skipped", *r)
	}
	checkShot(r, path, shot, true)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("-update didn't write the golden: %v", err)
	}

	// A different shot fails, leaving what it drew and the diff.
	r = &rec{}
	checkShot(r, path, encode(t, solid(3, 3, color.White)), false)
	if !r.failed {
		t.Error("a shot that differs passed")
	}
	for _, f := range []string{"main-dark.png", "main-dark.diff.png"} {
		if _, err := os.Stat(filepath.Join(failed, f)); err != nil {
			t.Errorf("no %s: %v", f, err)
		}
	}

	// The same shot passes, and clears the last run's failure.
	r = &rec{}
	checkShot(r, path, shot, false)
	if r.failed || r.skipped {
		t.Errorf("the same shot: %+v, want a pass", *r)
	}
	if m, _ := filepath.Glob(filepath.Join(failed, "main-dark*")); len(m) > 0 {
		t.Errorf("a pass left %v", m)
	}
}
