package relay

import (
	"bytes"
	"slices"
	"testing"
	"time"
)

const (
	iac  = 255
	sb   = 250
	se   = 240
	naws = 31
)

func TestCutterSkipsNewlinesInsideTelnet(t *testing.T) {
	// NAWS 10x24: the width's low byte is '\n'.
	p := append([]byte{iac, sb, naws, 0, '\n', 0, 24, iac, se}, "hi\n"...)
	var c cutter
	if got := c.cuts(p); !slices.Equal(got, []int{len(p)}) {
		t.Errorf("cuts = %v, want [%d]", got, len(p))
	}
	var d cutter
	a, b := p[:4], p[4:]
	got := append(d.cuts(a), d.cuts(b)...)
	if !slices.Equal(got, []int{len(b)}) {
		t.Errorf("split cuts = %v, want [%d] in the second part", got, len(b))
	}
}

func TestCutterForcesACutOnHugeLines(t *testing.T) {
	var c cutter
	p := bytes.Repeat([]byte("x"), 64<<10+10)
	if got := c.cuts(p); !slices.Equal(got, []int{64 << 10}) {
		t.Errorf("cuts = %v, want [%d]", got, 64<<10)
	}
}

func TestCutterTakesAnEscapedIACAsData(t *testing.T) {
	var c cutter
	p := []byte{'a', iac, iac, '\n'}
	if got := c.cuts(p); !slices.Equal(got, []int{4}) {
		t.Errorf("cuts = %v, want [4]", got)
	}
}

var t1, t2, t3 = time.UnixMilli(1000), time.UnixMilli(2000), time.UnixMilli(3000)

type step struct {
	missed int
	from   int64
	at     time.Time
	p      string
	ok     bool
}

func next(b *buffer, pos int64) step {
	m, from, at, p, ok := b.next(pos)
	return step{m, from, at, string(p), ok}
}

func TestNextResendsFromPos(t *testing.T) {
	var b buffer
	b.add([]byte("a\n"), t1)
	b.add([]byte("b\n"), t2)
	for _, c := range []struct {
		pos  int64
		want step
	}{
		{0, step{0, 0, t1, "a\n", true}},
		{2, step{0, 2, t2, "b\n", true}},
		{4, step{}},
		{1, step{0, 1, t1, "\n", true}},
	} {
		if got := next(&b, c.pos); got != c.want {
			t.Errorf("next(%d) = %+v, want %+v", c.pos, got, c.want)
		}
	}
	if b.end() != 4 {
		t.Errorf("end = %d, want 4", b.end())
	}
}

func TestAckForgets(t *testing.T) {
	var b buffer
	b.add([]byte("a\n"), t1)
	b.add([]byte("b\n"), t2)
	b.ack(2)
	if b.n != 2 || len(b.chunks) != 1 {
		t.Errorf("held %d bytes in %d chunks, want 2 in 1", b.n, len(b.chunks))
	}
	if got, want := next(&b, 0), (step{0, 2, t2, "b\n", true}); got != want {
		t.Errorf("next(0) = %+v, want %+v", got, want)
	}
}

func TestDropLineCountsMissed(t *testing.T) {
	var b buffer
	b.add([]byte("a\nb\n"), t1)
	b.add([]byte("c\n"), t2)
	if !b.dropLine() || !b.dropLine() {
		t.Fatal("dropLine = false")
	}
	if got, want := next(&b, 0), (step{2, 4, t2, "c\n", true}); got != want {
		t.Errorf("next(0) = %+v, want %+v", got, want)
	}
	if got, want := next(&b, 4), (step{0, 4, t2, "c\n", true}); got != want {
		t.Errorf("next(4) = %+v, want %+v", got, want)
	}
	if got, want := next(&b, 2), (step{1, 4, t2, "c\n", true}); got != want {
		t.Errorf("next(2) = %+v, want %+v (one dropped line is past 2)", got, want)
	}
	b.ack(4)
	if len(b.drops) != 0 {
		t.Errorf("drops after ack = %v, want none", b.drops)
	}
}

func TestDropTheLastLineStillSaysMissed(t *testing.T) {
	var b buffer
	b.add([]byte("a\n"), t1)
	b.dropLine()
	if got, want := next(&b, 0), (step{1, 2, time.Time{}, "", true}); got != want {
		t.Errorf("next(0) = %+v, want %+v", got, want)
	}
	if got := next(&b, 2); got.ok {
		t.Errorf("next(2) = %+v, want nothing", got)
	}
}

func TestDropLineNeedsACut(t *testing.T) {
	var b buffer
	b.add([]byte("prompt> "), t3)
	if b.dropLine() {
		t.Error("dropLine = true with no line end")
	}
}

// retained is the memory b's chunks hold, dead bytes included.
func retained(b *buffer) int {
	n := 0
	for _, c := range b.chunks {
		n += cap(c.p)
	}
	return n
}

func TestAckFreesMemory(t *testing.T) {
	var b buffer
	chunk := bytes.Repeat([]byte("x"), 10<<10-1)
	for range 100 {
		b.add(append(chunk, '\n'), t1)
	}
	b.ack(b.end() - 10)
	if r := retained(&b); r > 10<<10 {
		t.Errorf("after acking all but 10 bytes, %d bytes held, want one chunk's at most", r)
	}
	for b.dropLine() {
	}
	if r := retained(&b); r != 0 {
		t.Errorf("after dropping every line, %d bytes held", r)
	}
}

func TestSizeCountsEachChunk(t *testing.T) {
	var b buffer
	for range 100 {
		b.add([]byte("a"), t1)
	}
	if b.size() <= 100 {
		t.Errorf("size %d for 100 one-byte chunks: it should count what each costs", b.size())
	}
}

func TestCutterGivesUpOnEndlessSubnegotiation(t *testing.T) {
	var c cutter
	p := append([]byte{iac, sb, 24}, bytes.Repeat([]byte("x"), 64<<10)...)
	if got := c.cuts(p); len(got) != 1 {
		t.Errorf("cuts = %v, want one forced cut in a subnegotiation that never ends", got)
	}
}
