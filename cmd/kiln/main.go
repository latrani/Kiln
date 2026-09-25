// Command kiln is a terminal MUCK client.
//
// Until the full TUI lands, it offers:
//
//	kiln tail <world> <char>          connect, print output, send stdin lines
//	kiln passwd <world> <char>        save a character's password in the keychain
//	kiln trust <world> <fingerprint>  accept a changed server certificate
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"

	"golang.org/x/term"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/secrets"
	"github.com/latrani/Kiln/internal/session"
)

const usage = `usage:
  kiln tail <world> <char>
  kiln passwd <world> <char>
  kiln trust <world> <fingerprint>`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kiln:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 3 {
		return errors.New(usage)
	}
	cfgDir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := config.EnsureDefaults(cfgDir); err != nil {
		return err
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return err
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	switch args[0] {
	case "tail":
		ch, ok := cfg.Find(args[1], args[2])
		if !ok {
			return fmt.Errorf("no character %s/%s (define it in %s)", args[1], args[2], filepath.Join(cfgDir, "worlds", args[1]+".toml"))
		}
		return tail(ch, dataDir)
	case "passwd":
		if _, ok := cfg.Find(args[1], args[2]); !ok {
			return fmt.Errorf("no character %s/%s", args[1], args[2])
		}
		fmt.Fprintf(os.Stderr, "password for %s/%s: ", args[1], args[2])
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		return secrets.Set(args[1], args[2], string(pw))
	case "trust":
		w := findWorld(cfg, args[1])
		if w == nil {
			return fmt.Errorf("no world %s", args[1])
		}
		ch := w.Characters[0]
		hp := ch.Host + ":" + strconv.Itoa(ch.Port)
		return knownHosts(dataDir).Trust(hp, args[2])
	}
	return errors.New(usage)
}

func findWorld(cfg *config.Config, id string) *config.World {
	for i := range cfg.Worlds {
		if cfg.Worlds[i].ID == id && len(cfg.Worlds[i].Characters) > 0 {
			return &cfg.Worlds[i]
		}
	}
	return nil
}

func knownHosts(dataDir string) conn.KnownHosts {
	return conn.KnownHosts{Path: filepath.Join(dataDir, "known_hosts")}
}

func tail(ch config.Character, dataDir string) error {
	cls, err := classify.New(ch.Rules.Classify, ch.Name, ch.Aliases)
	if err != nil {
		return err
	}
	hl, err := rules.New(ch.Rules.Highlight)
	if err != nil {
		return err
	}
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		w, h = 80, 24
	}
	logw := logstore.NewWriter(filepath.Join(dataDir, "logs"), ch.World, ch.ID)
	defer logw.Close()

	s := session.New(session.Options{
		Char: ch,
		Log:  logw,
		Dial: func(ctx context.Context) (session.LineConn, error) {
			return conn.Dial(ctx, conn.Options{
				Host: ch.Host, Port: ch.Port, TLS: ch.TLS, TLSTrust: ch.TLSTrust,
				KnownHosts: knownHosts(dataDir), Width: w, Height: h,
			})
		},
		Password: func() (string, error) { return secrets.Get(ch.World, ch.ID) },
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			e, err := s.Send(sc.Text())
			var logErr *session.LogError
			if errors.As(err, &logErr) {
				fmt.Fprintln(os.Stderr, "\x1b[31m*", err, "\x1b[0m")
			} else if err != nil {
				fmt.Fprintln(os.Stderr, "\x1b[2m* not sent:", err, "\x1b[0m")
				continue
			}
			fmt.Println(render(e, rules.Result{}))
		}
		stop() // stdin closed: quit
	}()
	go s.Run(ctx)

	for ev := range s.Events() {
		switch ev.Kind {
		case session.EventLine:
			var res rules.Result
			if ev.Entry.Dir == logstore.In {
				plain := ansi.Strip(ev.Entry.Text)
				res = hl.Apply(plain, cls.Classify(plain))
			}
			fmt.Println(render(ev.Entry, res))
		case session.EventLogError:
			fmt.Fprintln(os.Stderr, "\x1b[31m* log write failed:", ev.Err, "\x1b[0m")
		case session.EventState:
			var pin *conn.PinMismatchError
			if errors.As(ev.Err, &pin) {
				fmt.Fprintf(os.Stderr, "* if you expected this, run: kiln trust %s %s\n", ch.World, pin.Got)
				stop()
			}
		}
	}
	return nil
}
