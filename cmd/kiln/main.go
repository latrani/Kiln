// Command kiln is a terminal MUCK client.
//
//	kiln [--no-autoconnect]           the full-screen client, optionally with nothing opened at start
//	kiln tail <world> <char>          connect, print output, send stdin lines
//	kiln passwd <world> <char>        save a character's password (see password_store)
//	kiln trust <world> <fingerprint>  accept a changed server certificate
//	kiln theme show <theme>           print a theme, merged into one file
package main

import (
	"bufio"
	"io"

	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
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
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
	"github.com/latrani/Kiln/internal/ui"
	"github.com/latrani/Kiln/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, str.CliError(err))
		os.Exit(1)
	}
}

// stdout is where commands print; tests swap it.
var stdout io.Writer = os.Stdout

// noAutoconnectFlag starts the full-screen client without opening any
// character, whatever their autoconnect says.
const noAutoconnectFlag = "--no-autoconnect" //str:ok: a flag

func run(args []string) error {
	noAuto := false
	if len(args) == 1 && args[0] == noAutoconnectFlag {
		noAuto, args = true, nil
	}
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version") { //str:ok
		_, err := fmt.Fprintln(stdout, version.String())
		return err
	}
	if len(args) != 0 && len(args) != 3 {
		return errors.New(str.CliUsage())
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
		return tui(cfgDir, dataDir, cfg, noAuto)
	}
	switch args[0] {
	case "tail":
		ch, ok := cfg.Find(args[1], args[2])
		if !ok {
			return errors.New(str.CliNoCharacterDefine(args[1], args[2]))
		}
		return tail(ch, cfgDir, dataDir, cfg)
	case "passwd":
		if _, ok := cfg.Find(args[1], args[2]); !ok {
			return errors.New(str.CliNoCharacter(args[1], args[2]))
		}
		if cfg.PasswordStore == "none" {
			return errors.New(str.CliStoreNone())
		}
		fmt.Fprint(os.Stderr, str.CliPasswordPrompt(args[1], args[2]))
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		return secrets.Open(cfg.PasswordStore, dataDir).Set(args[1], args[2], string(pw))
	case "trust":
		w := findWorld(cfg, args[1])
		if w == nil {
			return errors.New(str.CliNoWorld(args[1]))
		}
		if err := conn.ValidateFingerprint(args[2]); err != nil {
			return err
		}
		ch := w.Characters[0]
		hp := ch.Host + ":" + strconv.Itoa(ch.Port)
		return knownHosts(dataDir).Trust(hp, args[2])
	case "theme":
		if args[1] != "show" { //str:ok: a subcommand
			break
		}
		out, err := theme.Show(cfgDir, args[2])
		if err != nil {
			return err
		}
		if isTerminal(stdout) && os.Getenv("NO_COLOR") == "" { //str:ok: an environment variable
			ap := theme.Dark
			if cfg.Appearance == "light" {
				ap = theme.Light
			}
			if t, err := theme.Load(cfgDir, args[2], ap); err == nil {
				out = theme.Swatches(out, t)
			}
		}
		_, err = fmt.Fprint(stdout, out)
		return err
	}
	return errors.New(str.CliUsage())
}

// isTerminal reports whether w is a terminal, where escape codes are for
// people rather than a file.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
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

func tail(ch config.Character, cfgDir, dataDir string, cfg *config.Config) error {
	ap := theme.Dark // tail never asks the terminal
	if cfg.Appearance == "light" {
		ap = theme.Light
	}
	if th, err := theme.Load(cfgDir, cfg.Theme, ap); err != nil {
		fmt.Fprintln(os.Stderr, "*", str.StatusThemeNotLoaded(err))
	} else {
		theme.SetActive(th)
	}
	cls, err := classify.New(ch.Rules.Classify, ch.Name, ch.Aliases)
	if err != nil {
		return err
	}
	hl := rules.New(lookTheme(ch, os.Stderr), ch.Rules.Attention, ch.Rules.Quiet)
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		w, h = 80, 24
	}
	logw := logstore.NewWriter(logstore.Layout{Root: filepath.Join(dataDir, "logs"), Dir: cfg.LogDir, Name: cfg.LogName,
		World: ch.World, Char: ch.ID, CharName: ch.Name})
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
		Password: func() (string, error) { return secrets.Open(cfg.PasswordStore, dataDir).Get(ch.World, ch.ID) },
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			e, err := s.Send(sc.Text())
			var logErr *session.LogError
			if errors.As(err, &logErr) {
				fmt.Fprintln(os.Stderr, theme.Paint(theme.StatusError, "* "+err.Error()))
			} else if err != nil {
				fmt.Fprintln(os.Stderr, theme.Paint(theme.ScrollbackSys, "* "+str.CliNotSent(err)))
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
				plain := ansi.Strip(ansi.Sanitize(ev.Entry.Text)) // as render draws it
				res = hl.Apply(plain, cls.Tags(plain))
			}
			fmt.Println(render(ev.Entry, res))
		case session.EventLogError:
			fmt.Fprintln(os.Stderr, theme.Paint(theme.StatusError, "* "+str.CliLogWriteFailed(ev.Err)))
		case session.EventState:
			var pin *conn.PinMismatchError
			if errors.As(ev.Err, &pin) {
				fmt.Fprintln(os.Stderr, "*", str.CliTrustHint(ch.World, pin.Got))
				stop()
			}
		}
	}
	return nil
}

func tui(cfgDir, dataDir string, cfg *config.Config, noAutoconnect bool) error {
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
		Tmux:          os.Getenv("TMUX") != "",
		NoAutoconnect: noAutoconnect,
		ConfigDir:     cfgDir,
		LogRoot:       logRoot,
		KnownHosts:    knownHosts(dataDir),
		Load:          config.Load,
		Dial: func(ctx context.Context, ch config.Character) (session.LineConn, error) {
			w, h, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				w, h = 80, 24
			}
			// Roughly the right pane's size, where text is shown; the UI
			// reports the exact size (and later resizes) via Session.Resize.
			w -= ui.SidebarWidth(w) + 1
			return conn.Dial(ctx, conn.Options{
				Host: ch.Host, Port: ch.Port, TLS: ch.TLS, TLSTrust: ch.TLSTrust,
				KnownHosts: knownHosts(dataDir), Width: w, Height: h,
			})
		},
		NewLog: func(l logstore.Layout) session.Appender {
			mu.Lock()
			defer mu.Unlock()
			k := fmt.Sprintf("%+v", l) // a changed log_dir or log_name gets a new writer
			if writers[k] == nil {
				writers[k] = logstore.NewWriter(l)
			}
			return writers[k]
		},
		Password: func(store, world, char string) (string, error) {
			return secrets.Open(store, dataDir).Get(world, char)
		},
		SavePassword: func(store, world, char, password string) error {
			return secrets.Open(store, dataDir).Set(world, char, password)
		},
		DeletePassword: func(store, world, char string) error {
			return secrets.Open(store, dataDir).Delete(world, char)
		},
		Changes: watcher.Changes(),
		OpenURL: openURL,
	}, cfg)
	_, err = tea.NewProgram(m).Run()
	return err
}

// openURL hands a link to the system's opener, without waiting for it.
// url is always an http(s) link, passed as an argument, never to a shell.
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //str:ok
	default:
		cmd = exec.Command("xdg-open", url) //str:ok
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
