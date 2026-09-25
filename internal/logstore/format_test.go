package logstore

import (
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	pdt := time.FixedZone("PDT", -7*3600)
	ts := time.Date(2026, 9, 24, 21, 14, 3, 120_000_000, pdt)
	cases := []struct {
		name string
		in   Entry
		want string
	}{
		{"received", Entry{ts, In, `Rook says, "Evening!"`}, "2026-09-24T21:14:03.120-07:00 <\tRook says, \"Evening!\""},
		{"sent", Entry{ts, Out, ":grins."}, "2026-09-24T21:14:03.120-07:00 >\t:grins."},
		{"sys", Entry{ts, Sys, "connected"}, "2026-09-24T21:14:03.120-07:00 *\tconnected"},
		{"utc keeps numeric offset", Entry{ts.UTC(), In, "x"}, "2026-09-25T04:14:03.120+00:00 <\tx"},
		{"ansi and tabs untouched", Entry{ts, In, "\x1b[1mA\x1b[0m\tB"}, "2026-09-24T21:14:03.120-07:00 <\t\x1b[1mA\x1b[0m\tB"},
		{"newlines flattened", Entry{ts, Sys, "a\nb\r\nc"}, "2026-09-24T21:14:03.120-07:00 *\ta b c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Format(c.in); got != c.want {
				t.Errorf("Format() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseRoundTrip(t *testing.T) {
	pdt := time.FixedZone("PDT", -7*3600)
	in := Entry{time.Date(2026, 9, 24, 21, 14, 3, 120_000_000, pdt), In, "Mira pages: \"you\taround?\" \x1b[0m"}
	got, err := Parse(Format(in))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Time.Equal(in.Time) || got.Dir != in.Dir || got.Text != in.Text {
		t.Errorf("round trip: got %+v, want %+v", got, in)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, line := range []string{
		"",
		"no tab here",
		"2026-09-24T21:14:03.120-07:00 <",    // no tab
		"2026-09-24T21:14:03.120-07:00 ?\tx", // bad dir
		"yesterday <\tx",                     // bad time
		"2026-09-24T21:14:0",                 // truncated by crash
	} {
		if _, err := Parse(line); err == nil {
			t.Errorf("Parse(%q) = nil error, want error", line)
		}
	}
}
