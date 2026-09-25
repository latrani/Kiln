package ui

import (
	"strings"
	"testing"
	"time"
)

func TestStartupOpensOnlyAutoconnect(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if got := strings.Join(h.m.order, " "); got != "fm/kit" {
		t.Errorf("order = %q, want only the autoconnect character", got)
	}
	if h.m.active != "fm/kit" {
		t.Errorf("active = %q", h.m.active)
	}
}

func TestOpenKeepsAlphabeticalOrder(t *testing.T) {
	// Case-sensitively these would sort Sp < fm and Kit < Rook < bo.
	fm := fmWorld + "\n[[characters]]\nid = \"bo\"\nname = \"bo\"\n"
	sp := "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n"
	h := newHarness(t, map[string]string{"fm": fm, "Sp": sp})
	h.open("Sp/ash", "fm/rook", "fm/bo")
	if got, want := strings.Join(h.m.order, " "), "fm/bo fm/kit fm/rook Sp/ash"; got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
	if h.m.open("fm/nobody") != nil {
		t.Error("opened a character that isn't configured")
	}
}

func TestCloseHandsActiveToNeighbor(t *testing.T) {
	sp := "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n"
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": sp})
	h.open("fm/rook", "sp/ash")
	h.m.switchTo("fm/rook")
	h.m.close("fm/rook") // the next one down
	if h.m.active != "sp/ash" || h.m.chars["fm/rook"] != nil {
		t.Fatalf("active = %q after closing rook", h.m.active)
	}
	h.m.close("sp/ash") // the last one: the one above
	if h.m.active != "fm/kit" {
		t.Fatalf("active = %q after closing ash", h.m.active)
	}
	h.m.close("fm/kit")
	if h.m.active != "" || len(h.m.order) != 0 || h.m.cur() != nil {
		t.Errorf("active = %q, order = %v; want nothing open", h.m.active, h.m.order)
	}
}

func TestLateEventFromClosedCharacterIgnored(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	sess := h.m.chars["fm/kit"].sess
	h.m.close("fm/kit")
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-sess.Events():
			h.m.Update(eventMsg{key: "fm/kit", sess: sess, ev: ev, ok: ok})
			if h.m.chars["fm/kit"] != nil {
				t.Fatal("a late event reopened the closed character")
			}
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("closed session still running")
		}
	}
}
