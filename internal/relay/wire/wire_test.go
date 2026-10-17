package wire

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestDownFrameRoundTrip(t *testing.T) {
	at := time.UnixMilli(1_760_000_000_123)
	f := DownFrame(at, []byte("hi\n"))
	got, p, err := ReadDown(f)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(at) || string(p) != "hi\n" {
		t.Errorf("ReadDown = %v %q, want %v %q", got, p, at, "hi\n")
	}
	if _, _, err := ReadDown(f[:7]); err == nil {
		t.Error("ReadDown of a 7-byte frame: no error")
	}
}

func TestMsgRoundTrip(t *testing.T) {
	for _, m := range []Msg{
		{T: Open, Token: "abc"},
		{T: Resume, Token: "abc", Got: 12},
		{T: Resumed, Got: 5, Away: 61_000},
		{T: Ack, Got: 1 << 40},
		{T: Missed, Lines: 3, At: 99},
		{T: Ping}, {T: Pong}, {T: Close},
		{T: End, At: 7, Why: "reset"},
	} {
		b := Encode(m)
		got, err := Decode(b)
		if err != nil {
			t.Fatalf("Decode(%s): %v", b, err)
		}
		if got != m {
			t.Errorf("round trip of %+v gave %+v", m, got)
		}
	}
	if b := Encode(Msg{T: Ping}); !bytes.Equal(b, []byte(`{"t":"ping"}`)) {
		t.Errorf("Encode(ping) = %s, want zero fields left out", b)
	}
	if _, err := Decode([]byte("open")); err == nil || strings.Contains(err.Error(), "\x00") {
		t.Errorf("Decode(not JSON) = %v, want an error", err)
	}
}

func TestAckerAcksAtAckBytes(t *testing.T) {
	sent := make(chan int64, 4)
	a := NewAcker(10, func(got int64) { sent <- got })
	defer a.Stop()
	a.Received(AckBytes)
	select {
	case got := <-sent:
		if got != 10+AckBytes {
			t.Errorf("acked %d, want %d", got, 10+AckBytes)
		}
	default:
		t.Error("no ack at AckBytes")
	}
}

func TestAckerAcksAfterAckEvery(t *testing.T) {
	sent := make(chan int64, 4)
	a := NewAcker(0, func(got int64) { sent <- got })
	defer a.Stop()
	a.Received(3)
	a.Skip(7)
	select {
	case got := <-sent:
		if got != 7 {
			t.Errorf("acked %d, want 7", got)
		}
	case <-time.After(3 * AckEvery):
		t.Fatal("no ack after AckEvery")
	}
	select {
	case got := <-sent:
		t.Errorf("a second ack %d with nothing new", got)
	case <-time.After(AckEvery + 200*time.Millisecond):
	}
}
