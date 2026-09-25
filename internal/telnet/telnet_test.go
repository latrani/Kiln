package telnet

import (
	"bytes"
	"testing"
)

func TestPlainDataPassesThrough(t *testing.T) {
	p := NewParser(80, 24)
	data, reply := p.Feed([]byte("hello\r\n"))
	if string(data) != "hello\r\n" || reply != nil {
		t.Errorf("data=%q reply=%v", data, reply)
	}
}

func TestEscapedIACIsData(t *testing.T) {
	p := NewParser(80, 24)
	data, _ := p.Feed([]byte{'a', IAC, IAC, 'b'})
	if !bytes.Equal(data, []byte{'a', IAC, 'b'}) {
		t.Errorf("data=%v", data)
	}
}

func TestRefusesUnknownOptions(t *testing.T) {
	p := NewParser(80, 24)
	// MCCP2 (86) offered, TTYPE (24) requested.
	data, reply := p.Feed([]byte{IAC, WILL, 86, IAC, DO, 24, 'x'})
	if string(data) != "x" {
		t.Errorf("data=%q", data)
	}
	want := []byte{IAC, DONT, 86, IAC, WONT, 24}
	if !bytes.Equal(reply, want) {
		t.Errorf("reply=%v want %v", reply, want)
	}
}

func TestIgnoresGAAndWontDont(t *testing.T) {
	p := NewParser(80, 24)
	data, reply := p.Feed([]byte{'a', IAC, GA, IAC, WONT, 1, IAC, DONT, 3, 'b'})
	if string(data) != "ab" || reply != nil {
		t.Errorf("data=%q reply=%v", data, reply)
	}
}

func TestNAWS(t *testing.T) {
	p := NewParser(100, 40)
	if msg := p.Resize(120, 50); msg != nil {
		t.Errorf("Resize before negotiation = %v, want nil", msg)
	}
	_, reply := p.Feed([]byte{IAC, DO, OptNAWS})
	want := []byte{IAC, WILL, OptNAWS, IAC, SB, OptNAWS, 0, 120, 0, 50, IAC, SE}
	if !bytes.Equal(reply, want) {
		t.Errorf("reply=%v want %v", reply, want)
	}
	got := p.Resize(255, 300) // 255 must be escaped
	want = []byte{IAC, SB, OptNAWS, 0, IAC, IAC, 1, 44, IAC, SE}
	if !bytes.Equal(got, want) {
		t.Errorf("Resize=%v want %v", got, want)
	}
}

func TestCharsetAcceptsUTF8(t *testing.T) {
	p := NewParser(80, 24)
	_, reply := p.Feed([]byte{IAC, DO, OptCharset})
	if !bytes.Equal(reply, []byte{IAC, WILL, OptCharset}) {
		t.Errorf("reply=%v", reply)
	}
	req := append([]byte{IAC, SB, OptCharset, charsetRequest}, []byte(";ISO-8859-1;utf-8")...)
	req = append(req, IAC, SE)
	_, reply = p.Feed(req)
	want := append([]byte{IAC, SB, OptCharset, charsetAccepted}, []byte("utf-8")...)
	want = append(want, IAC, SE)
	if !bytes.Equal(reply, want) {
		t.Errorf("reply=%q want %q", reply, want)
	}
	if !p.UTF8() {
		t.Error("UTF8() = false")
	}
}

func TestCharsetRejectsOthers(t *testing.T) {
	p := NewParser(80, 24)
	req := append([]byte{IAC, SB, OptCharset, charsetRequest}, []byte(" ASCII KOI8-R")...)
	_, reply := p.Feed(append(req, IAC, SE))
	if !bytes.Equal(reply, []byte{IAC, SB, OptCharset, charsetRejected, IAC, SE}) || p.UTF8() {
		t.Errorf("reply=%v utf8=%v", reply, p.UTF8())
	}
}

func TestSequencesSplitAcrossFeeds(t *testing.T) {
	p := NewParser(80, 24)
	stream := []byte{'h', 'i', IAC, DO, OptNAWS, IAC, SB, 99, 1, IAC, IAC, 2, IAC, SE, IAC, IAC, '!'}
	var data, reply []byte
	for _, b := range stream { // worst case: one byte per read
		d, r := p.Feed([]byte{b})
		data = append(data, d...)
		reply = append(reply, r...)
	}
	if !bytes.Equal(data, []byte{'h', 'i', IAC, '!'}) {
		t.Errorf("data=%v", data)
	}
	if !bytes.HasPrefix(reply, []byte{IAC, WILL, OptNAWS}) {
		t.Errorf("reply=%v", reply)
	}
}

func TestEscape(t *testing.T) {
	if got := Escape([]byte{'a', IAC, 'b'}); !bytes.Equal(got, []byte{'a', IAC, IAC, 'b'}) {
		t.Errorf("Escape=%v", got)
	}
	if got := Escape([]byte("plain")); string(got) != "plain" {
		t.Errorf("Escape=%q", got)
	}
}
