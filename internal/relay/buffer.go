package relay

import (
	"bytes"
	"slices"
	"sort"
	"time"
)

// A buffer holds the bytes a world has sent that the page hasn't
// acknowledged, from offset base to end, so they can be sent again on a
// new WebSocket. Offsets count every byte the world has sent on the
// connection. When it's too full, whole lines are dropped from the front
// and counted, so the page can say how many it missed.
//
// Each read from the world is its own chunk, so what's acked or dropped
// is freed, not kept behind a slice of one big array.
type buffer struct {
	chunks  []chunk // ascending; each ends past base
	base    int64
	n       int     // bytes held
	cuts    []int64 // ascending offsets past base where a line ends
	drops   []drop  // dropped lines the page may not know about
	cutter  cutter
	dropped int // lines dropped in all
}

// A chunk is one read from the world, and when it arrived. It's sent as
// one frame. p runs up to end.
type chunk struct {
	end int64
	at  time.Time
	p   []byte
}

// chunkCost is roughly what a chunk costs beyond its bytes: the slice,
// the time, and the allocator's rounding.
const chunkCost = 64

// A drop is a dropped line ending at end; n counts the lines dropped
// on the connection up to it.
type drop struct {
	end int64
	n   int
}

func (b *buffer) end() int64 { return b.base + int64(b.n) }

// size is roughly the memory b holds: its bytes and each chunk's cost.
func (b *buffer) size() int { return b.n + chunkCost*len(b.chunks) }

func (b *buffer) add(p []byte, at time.Time) {
	start := b.end()
	b.n += len(p)
	b.chunks = append(b.chunks, chunk{b.end(), at, bytes.Clone(p)})
	for _, c := range b.cutter.cuts(p) {
		b.cuts = append(b.cuts, start+int64(c))
	}
}

// ack forgets what the page has received, below got.
func (b *buffer) ack(got int64) {
	b.forget(got)
	i := 0
	for i < len(b.drops) && b.drops[i].end <= got {
		i++
	}
	b.drops = slices.Delete(b.drops, 0, i)
}

// forget moves base up to off, if that's past it.
func (b *buffer) forget(off int64) {
	if off <= b.base {
		return
	}
	off = min(off, b.end())
	b.n -= int(off - b.base)
	b.base = off
	i := 0
	for i < len(b.chunks) && b.chunks[i].end <= off {
		i++
	}
	// Delete, not reslice: the chunks dropped from the front must not
	// stay reachable through the slice's array.
	b.chunks = slices.Delete(b.chunks, 0, i)
	if len(b.chunks) > 0 {
		c := &b.chunks[0]
		if skip := len(c.p) - int(c.end-off); skip > 0 {
			c.p = bytes.Clone(c.p[skip:])
		}
	}
	i = 0
	for i < len(b.cuts) && b.cuts[i] <= off {
		i++
	}
	b.cuts = slices.Delete(b.cuts, 0, i)
}

// dropLine drops the oldest whole line. It's false when no line in the
// buffer has ended.
func (b *buffer) dropLine() bool {
	if len(b.cuts) == 0 {
		return false
	}
	b.dropped++
	b.drops = append(b.drops, drop{b.cuts[0], b.dropped})
	b.forget(b.cuts[0])
	return true
}

// next is what a writer that has sent up to pos sends next: missed lines
// dropped past pos (say so before the data, which then goes on from
// from), and the rest of the chunk holding from, stamped at. from is past
// pos, with nothing missed, for a writer behind what's been acked. ok is
// false when there's nothing to send.
func (b *buffer) next(pos int64) (missed int, from int64, at time.Time, p []byte, ok bool) {
	if i := sort.Search(len(b.drops), func(i int) bool { return b.drops[i].end > pos }); i < len(b.drops) {
		missed = b.dropped - b.drops[i].n + 1
	}
	from = max(pos, b.base)
	for _, c := range b.chunks {
		if c.end > from {
			return missed, from, c.at, c.p[len(c.p)-int(c.end-from):], true
		}
	}
	if missed > 0 || from > pos {
		return missed, from, time.Time{}, nil, true // the writer says where data goes on from
	}
	return 0, 0, time.Time{}, nil, false
}

// forceCut is the longest run of plain data the cutter lets go without
// a line end before it cuts anyway, so a world that never sends one can't
// make a buffer undroppable.
const forceCut = 64 << 10

// A cutter finds where lines end in a world's telnet stream, never inside
// a telnet command or subnegotiation (where a '\n' is just a byte), and
// cuts a run of forceCut bytes without one anyway.
type cutter struct {
	state byte
	run   int // bytes since the last cut
}

// Telnet bytes.
const tIAC, tSB, tSE, tWILL, tDONT = 255, 250, 240, 251, 254

// The cutter's states.
const (
	inData = iota
	inIAC
	inOption
	inSB
	inSBIAC
)

// cuts returns the offsets in p just past each line end.
func (c *cutter) cuts(p []byte) []int {
	var out []int
	for i, ch := range p {
		c.run++
		switch c.state {
		case inData:
			if ch == tIAC {
				c.state = inIAC
			} else if ch == '\n' || c.run >= forceCut {
				out = append(out, i+1)
				c.run = 0
			}
		case inIAC:
			switch {
			case ch == tSB:
				c.state = inSB
			case ch >= tWILL && ch <= tDONT:
				c.state = inOption
			default: // an escaped 255, or a two-byte command
				c.state = inData
			}
		case inOption:
			c.state = inData
		case inSB:
			if ch == tIAC {
				c.state = inSBIAC
			} else if c.run >= forceCut {
				// Past any real subnegotiation (telnet.Parser gives up at
				// 4 KiB): it's data, and the run can be cut.
				c.state = inData
				out = append(out, i+1)
				c.run = 0
			}
		case inSBIAC:
			if ch == tSE {
				c.state = inData
			} else {
				c.state = inSB
			}
		}
	}
	return out
}
