// Package telnet is a minimal client-side telnet protocol parser. It
// separates application data from IAC sequences and produces replies:
// NAWS and CHARSET (UTF-8) are accepted; every other option is refused.
package telnet

import (
	"bytes"
	"strings"
)

// Protocol bytes.
const (
	SE   byte = 240
	NOP  byte = 241
	GA   byte = 249
	SB   byte = 250
	WILL byte = 251
	WONT byte = 252
	DO   byte = 253
	DONT byte = 254
	IAC  byte = 255
)

// Options.
const (
	OptNAWS    byte = 31
	OptCharset byte = 42
)

// CHARSET subnegotiation commands (RFC 2066).
const (
	charsetRequest  byte = 1
	charsetAccepted byte = 2
	charsetRejected byte = 3
)

// maxSB bounds subnegotiation data. NAWS and CHARSET need a few dozen
// bytes; a server that opens SB and never closes it would otherwise grow
// the buffer forever and swallow all later output.
const maxSB = 4096

type state int

const (
	stData state = iota
	stIAC
	stOpt   // after WILL/WONT/DO/DONT, waiting for the option byte
	stSBOpt // after SB, waiting for the option byte
	stSB    // inside subnegotiation data
	stSBIAC // IAC inside subnegotiation
)

// Parser is fed raw bytes from the server. It is not safe for concurrent use.
type Parser struct {
	st     state
	verb   byte
	sbOpt  byte
	sbBuf  []byte
	width  uint16
	height uint16
	naws   bool // server asked for NAWS and we agreed
	utf8   bool // server accepted/negotiated UTF-8
}

// NewParser returns a parser that reports the given window size via NAWS.
func NewParser(width, height int) *Parser {
	return &Parser{width: clamp16(width), height: clamp16(height)}
}

// UTF8 reports whether UTF-8 was negotiated via CHARSET.
func (p *Parser) UTF8() bool { return p.utf8 }

// Feed consumes bytes from the server and returns the application data
// they contain plus any bytes that must be written back to the server.
// Sequences split across calls are handled.
func (p *Parser) Feed(in []byte) (data, reply []byte) {
	for _, b := range in {
		switch p.st {
		case stData:
			if b == IAC {
				p.st = stIAC
			} else {
				data = append(data, b)
			}
		case stIAC:
			switch b {
			case IAC:
				data = append(data, IAC)
				p.st = stData
			case WILL, WONT, DO, DONT:
				p.verb = b
				p.st = stOpt
			case SB:
				p.st = stSBOpt
			default: // GA, NOP, and anything else: ignore
				p.st = stData
			}
		case stOpt:
			reply = append(reply, p.negotiate(p.verb, b)...)
			p.st = stData
		case stSBOpt:
			p.sbOpt = b
			p.sbBuf = p.sbBuf[:0]
			p.st = stSB
		case stSB:
			if b == IAC {
				p.st = stSBIAC
			} else {
				p.sbAppend(b)
			}
		case stSBIAC:
			switch b {
			case SE:
				reply = append(reply, p.subneg(p.sbOpt, p.sbBuf)...)
				p.st = stData
			case IAC:
				p.st = stSB
				p.sbAppend(IAC)
			default: // malformed; abandon the subnegotiation
				p.st = stData
			}
		}
	}
	return data, reply
}

// sbAppend adds b to the subnegotiation buffer, or abandons the
// subnegotiation and returns to data state once the buffer is full.
func (p *Parser) sbAppend(b byte) {
	if len(p.sbBuf) >= maxSB {
		p.sbBuf = p.sbBuf[:0]
		p.st = stData
		return
	}
	p.sbBuf = append(p.sbBuf, b)
}

func (p *Parser) negotiate(verb, opt byte) []byte {
	switch verb {
	case DO:
		switch opt {
		case OptNAWS:
			p.naws = true
			return append([]byte{IAC, WILL, OptNAWS}, p.nawsMsg()...)
		case OptCharset:
			return []byte{IAC, WILL, OptCharset}
		}
		return []byte{IAC, WONT, opt}
	case WILL:
		if opt == OptCharset {
			return []byte{IAC, DO, OptCharset}
		}
		return []byte{IAC, DONT, opt}
	}
	return nil // WONT/DONT: nothing to acknowledge for options we never enabled
}

func (p *Parser) subneg(opt byte, data []byte) []byte {
	if opt != OptCharset || len(data) < 2 || data[0] != charsetRequest {
		return nil
	}
	sep := data[1]
	for _, name := range bytes.Split(data[2:], []byte{sep}) {
		if n := strings.ToUpper(string(name)); n == "UTF-8" || n == "UTF8" {
			p.utf8 = true
			msg := []byte{IAC, SB, OptCharset, charsetAccepted}
			msg = append(msg, name...)
			return append(msg, IAC, SE)
		}
	}
	return []byte{IAC, SB, OptCharset, charsetRejected, IAC, SE}
}

// Resize records a new window size and returns the NAWS message to send,
// or nil if NAWS has not been negotiated.
func (p *Parser) Resize(width, height int) []byte {
	p.width, p.height = clamp16(width), clamp16(height)
	if !p.naws {
		return nil
	}
	return p.nawsMsg()
}

func (p *Parser) nawsMsg() []byte {
	msg := []byte{IAC, SB, OptNAWS}
	for _, v := range []uint16{p.width, p.height} {
		hi, lo := byte(v>>8), byte(v)
		msg = append(msg, hi)
		if hi == IAC {
			msg = append(msg, IAC)
		}
		msg = append(msg, lo)
		if lo == IAC {
			msg = append(msg, IAC)
		}
	}
	return append(msg, IAC, SE)
}

// Escape doubles IAC bytes in outgoing application data.
func Escape(b []byte) []byte {
	if bytes.IndexByte(b, IAC) < 0 {
		return b
	}
	return bytes.ReplaceAll(b, []byte{IAC}, []byte{IAC, IAC})
}

func clamp16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 0xffff {
		return 0xffff
	}
	return uint16(v)
}
