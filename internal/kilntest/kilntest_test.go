package kilntest

import (
	"slices"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
)

func TestConnRecordsAndFeeds(t *testing.T) {
	c := NewConn()
	c.Feed("hello")
	if got := <-c.Lines(); got != "hello" {
		t.Errorf("Lines gave %q", got)
	}
	c.Send("look")
	c.Resize(80, 24)
	if !slices.Equal(c.Sent(), []string{"look"}) || !slices.Equal(c.Sizes(), [][2]int{{80, 24}}) {
		t.Errorf("sent %q, sizes %v", c.Sent(), c.Sizes())
	}
	c.Close()
	c.Close() // twice is fine
	if _, ok := <-c.Lines(); ok {
		t.Error("Lines still open after Close")
	}
}

func TestWriteLogWritesADay(t *testing.T) {
	l := logstore.Layout{Root: t.TempDir(), World: "fm", Char: "kit", CharName: "Kit"}
	start := time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)
	WriteLog(t, l, start, "hi", "> wave")
	h, err := history.NewReader(l)
	if err != nil {
		t.Fatal(err)
	}
	es, _, _, err := h.LoadOlder()
	if err != nil || len(es) != 2 || es[1].Dir != logstore.Out || es[1].Text != "wave" || !es[1].Time.Equal(start.Add(time.Minute)) {
		t.Errorf("read %+v, %v", es, err)
	}
}
