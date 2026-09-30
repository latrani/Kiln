package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/version"
)

func TestVersionCommand(t *testing.T) {
	for _, arg := range []string{"version", "--version"} {
		var out bytes.Buffer
		stdout = &out
		err := run([]string{arg})
		stdout = os.Stdout
		if err != nil || out.String() != version.String()+"\n" {
			t.Errorf("kiln %s: %q, %v; want %q", arg, out.String(), err, version.String()+"\n")
		}
	}
}

func TestNoAutoconnectTakesNoOtherArguments(t *testing.T) {
	for _, args := range [][]string{{"--no-autoconnect", "extra"}, {"--bogus"}} {
		if err := run(args); err == nil || err.Error() != str.CliUsage() {
			t.Errorf("kiln %v: %v, want the usage", args, err)
		}
	}
}
