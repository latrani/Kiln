package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

func TestLogFailureKeepsMessageAndDetail(t *testing.T) {
	var buf bytes.Buffer
	cause := errors.New("boom")                                                //str:ok: a fixture
	err := detailed{cause, "Caught panic:\r\n\r\nboom\r\n\r\ngoroutine 1\r\n"} //str:ok: a fixture
	logFailure(&buf, str.CliError(err), err)
	got := buf.String()
	if !strings.Contains(got, str.CliError(cause)+"\n") || !strings.Contains(got, "\ngoroutine 1\n") || strings.Contains(got, "\r") {
		t.Errorf("kiln.log entry %q", got)
	}
	if !errors.Is(err, cause) {
		t.Error("detailed hides the error it wraps")
	}
}

func TestCrashLogStartsOverWhenBig(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	f := openCrashLog()
	if f == nil {
		t.Fatal("no kiln.log")
	}
	f.Write(bytes.Repeat([]byte{'x'}, crashLogMax+1))
	f.Close()
	f = openCrashLog()
	defer f.Close()
	fi, _ := os.Stat(filepath.Join(os.Getenv("XDG_DATA_HOME"), "kiln", crashLogName))
	if fi.Size() != 0 {
		t.Errorf("kiln.log is %d bytes, want it started over", fi.Size())
	}
}

func TestHoldWindowWaitsForEnter(t *testing.T) {
	var out bytes.Buffer
	holdWindow(strings.NewReader("\n"), &out)
	if out.String() != str.CliPressEnter() {
		t.Errorf("prompt %q", out.String())
	}
}

func TestTeeStderrPassesThroughAndCaptures(t *testing.T) {
	real, _ := os.Create(filepath.Join(t.TempDir(), "stderr"))
	saved := os.Stderr
	os.Stderr = real
	defer func() { os.Stderr = saved }()
	stop := teeStderr()
	os.Stderr.WriteString("panic\n") //str:ok: a fixture
	got := stop()
	passed, _ := os.ReadFile(real.Name())
	if got != "panic\n" || string(passed) != got || os.Stderr != real {
		t.Errorf("captured %q, passed %q", got, passed)
	}
}
