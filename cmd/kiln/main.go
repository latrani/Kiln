// Command kiln is a terminal MUCK client.
//
//	kiln                              the full-screen client
//	kiln tail <world> <char>          connect, print output, send stdin lines
//	kiln passwd <world> <char>        save a character's password (see password_store)
//	kiln trust <world> <fingerprint>  accept a changed server certificate
package main

import (
	"bufio"

	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"

	"golang.org/x/term"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/secrets"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/ui"
)

const usage = `usage:
  kiln
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
	if len(args) != 0 && len(args) != 3 {
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
	if len(args) == 0 {
		return tui(cfgDir, dataDir, cfg)
	}
	switch args[0] {
	case "tail":
		ch, ok := cfg.Find(args[1], args[2])
		if !ok {
			return fmt.Errorf("no character %s/%s (define it in %s)", args[1], args[2], filepath.Join(cfgDir, "worlds", args[1]+".toml"))
		}
		return tail(ch, dataDir, cfg.PasswordStore)
	case "passwd":
		if _, ok := cfg.Find(args[1], args[2]); !ok {
			return fmt.Errorf("no character %s/%s", args[1], args[2])
		}
		if cfg.PasswordStore == "none" {
			return errors.New(`password_store is "none" in config.toml; set it to "keychain" or "file" to save passwords`)
		}
		fmt.Fprintf(os.Stderr, "password for %s/%s: ", args[1], args[2])
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		return secrets.Open(cfg.PasswordStore, dataDir).Set(args[1], args[2], string(pw))
	case "trust":
		w := findWorld(cfg, args[1])
		if w == nil {
			return fmt.Errorf("no world %s", args[1])
		}
		if err := conn.ValidateFingerprint(args[2]); err != nil {
			return err
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

func tail(ch config.Character, dataDir, pwStore string) error {
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
		Password: func() (string, error) { return secrets.Open(pwStore, dataDir).Get(ch.World, ch.ID) },
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
				res = hl.Apply(plain, cls.Tags(plain))
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

func tui(cfgDir, dataDir string, cfg *config.Config) error {
	watcher, err := config.Watch(cfgDir)
	if err != nil {
		return err
	}
	defer watcher.Close()
	logRoot := filepath.Join(dataDir, "logs")
	var mu sync.Mutex
	writers := map[string]*logstore.Writer{}
	defer func() {
		for _, w := range writers {
			w.Close()
		}
	}()
	m := ui.New(ui.Deps{
		ConfigDir:  cfgDir,
		LogRoot:    logRoot,
		KnownHosts: knownHosts(dataDir),
		Load:       config.Load,
		Dial: func(ctx context.Context, ch config.Character) (session.LineConn, error) {
			w, h, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				w, h = 80, 24
			}
			// Roughly the right pane's size, where text is shown; the UI
			// reports the exact size (and later resizes) via Session.Resize.
			w -= min(22, max(12, w/5)) + 1
			return conn.Dial(ctx, conn.Options{
				Host: ch.Host, Port: ch.Port, TLS: ch.TLS, TLSTrust: ch.TLSTrust,
				KnownHosts: knownHosts(dataDir), Width: w, Height: h,
			})
		},
		NewLog: func(world, char string) session.Appender {
			mu.Lock()
			defer mu.Unlock()
			k := world + "/" + char
			if writers[k] == nil {
				writers[k] = logstore.NewWriter(logRoot, world, char)
			}
			return writers[k]
		},
		Password: func(store, world, char string) (string, error) {
			return secrets.Open(store, dataDir).Get(world, char)
		},
		SavePassword: func(store, world, char, password string) error {
			return secrets.Open(store, dataDir).Set(world, char, password)
		},
		Changes: watcher.Changes(),
	}, cfg)
	_, err = tea.NewProgram(m).Run()
	return err
}
