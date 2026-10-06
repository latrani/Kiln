// Command kiln-relay lets Kiln's web build reach MUCKs: browsers can't
// open TCP connections, so the page opens a WebSocket here and the relay
// connects to the MUCK, for the worlds in its allowlist. See
// docs/web-relay.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/latrani/Kiln/internal/relay"
	"github.com/latrani/Kiln/internal/str"
)

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, str.CliError(err))
		os.Exit(1)
	}
}

func run(args []string, logw io.Writer) error {
	fs := flag.NewFlagSet("kiln-relay", flag.ContinueOnError) //str:ok
	listen := fs.String("listen", "", str.RelayFlagListen())  //str:ok
	allow := fs.String("allow", "", str.RelayFlagAllow())     //str:ok
	dev := fs.Bool("dev", false, str.RelayFlagDev())          //str:ok
	web := fs.String("web", "", str.RelayFlagWeb())           //str:ok
	preset := fs.String("preset", "", str.RelayFlagPreset())  //str:ok
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *allow == "" {
		return errors.New(str.RelayNeedAllow())
	}
	if *dev && (*web == "" || *preset == "") {
		return errors.New(str.RelayDevNeedsDirs())
	}
	addr := *listen
	if addr == "" {
		addr = "127.0.0.1:7801"
		if *dev {
			addr = "localhost:8080" //str:ok
		}
	}
	logf := func(line string) { fmt.Fprintln(logw, time.Now().Format(time.DateTime), line) }
	s := &relay.Server{Log: logf}
	w, err := relay.WatchAllowlist(s, *allow)
	if err != nil {
		return err
	}
	defer w.Close()
	mux := http.NewServeMux()
	mux.Handle("/kiln/relay", s) //str:ok
	if *dev {
		devRoutes(mux, *web, *preset)
		logf(str.RelayDevServing("http://" + addr + "/kiln/")) //str:ok
	}
	logf(str.RelayListening(addr))
	return http.ListenAndServe(addr, mux)
}
