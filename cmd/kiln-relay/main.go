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
	cfg := fs.String("config", "", str.RelayFlagConfig())     //str:ok
	web := fs.String("web", "", str.RelayFlagWeb())           //str:ok
	preset := fs.String("preset", "", str.RelayFlagPreset())  //str:ok
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cfg == "" {
		return errors.New(str.RelayNeedConfig())
	}
	for _, d := range []struct{ flag, dir string }{{"web", *web}, {"preset", *preset}} { //str:ok
		if d.dir != "" {
			if err := checkDir(d.flag, d.dir); err != nil {
				return err
			}
		}
	}
	addr := *listen
	if addr == "" {
		addr = "127.0.0.1:7801"
	}
	logf := func(line string) { fmt.Fprintln(logw, time.Now().Format(time.DateTime), line) }
	s := &relay.Server{Log: logf}
	w, err := relay.WatchConfig(s, *cfg)
	if err != nil {
		return err
	}
	defer w.Close()
	mux := http.NewServeMux()
	mux.Handle("/kiln/relay", s) //str:ok
	if *web != "" {
		if *preset != "" {
			checkPresetWorlds(*preset, logf)
		}
		webRoutes(mux, *web, *preset, func() []relay.World { return s.Config().Worlds }, logf)
		logf(str.RelayServingPage("http://" + addr + "/kiln/")) //str:ok
	}
	logf(str.RelayListening(addr))
	// No WriteTimeout: relayed connections are long-lived WebSockets.
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	return srv.ListenAndServe()
}
