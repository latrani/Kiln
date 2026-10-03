package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/version"
)

// crashLogName is the file in the data directory that keeps what made kiln
// exit with an error: the message, and a panic's stack. A window that closes
// as kiln exits takes stderr with it; this doesn't.
const crashLogName = "kiln.log" //str:ok: a file name

// crashLogMax is how big kiln.log may grow before it starts over.
const crashLogMax = 1 << 20

// openCrashLog opens kiln.log for appending, starting it over once it's past
// crashLogMax, and has the runtime copy a crash there too, so a panic on any
// goroutine leaves its stack behind. It returns nil if there's no data
// directory to put it in.
func openCrashLog() *os.File {
	dir, err := config.DataDir()
	if err != nil || os.MkdirAll(dir, 0o700) != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, crashLogName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	if fi, err := f.Stat(); err == nil && fi.Size() > crashLogMax {
		f.Truncate(0)
	}
	debug.SetCrashOutput(f, debug.CrashOptions{})
	return f
}

// logFailure appends one entry to kiln.log: when, which build, the message,
// and any detail (a recovered panic's stack) below it.
func logFailure(w io.Writer, msg string, err error) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "%s %s: %s\n", time.Now().Format(time.RFC3339), version.String(), msg) //str:ok: a log line's format
	var d detailed
	if errors.As(err, &d) && d.detail != "" {
		fmt.Fprintf(w, "%s\n", strings.TrimRight(strings.ReplaceAll(d.detail, "\r\n", "\n"), "\n"))
	}
	fmt.Fprintln(w)
}

// detailed is an error that carries more for kiln.log than its message says.
type detailed struct {
	err    error
	detail string
}

func (d detailed) Error() string { return d.err.Error() }
func (d detailed) Unwrap() error { return d.err }

// teeStderr copies everything written to os.Stderr into a buffer as well,
// until the returned stop restores it and hands back what was written.
// Bubble Tea prints a panic it recovers to stderr and returns only
// ErrProgramPanic; this is how its stack reaches kiln.log.
func teeStderr() (stop func() string) {
	r, w, err := os.Pipe()
	if err != nil {
		return func() string { return "" }
	}
	real := os.Stderr
	os.Stderr = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		io.Copy(io.MultiWriter(real, &buf), r)
		close(done)
	}()
	return func() string {
		os.Stderr = real
		w.Close()
		<-done
		r.Close()
		return buf.String()
	}
}
