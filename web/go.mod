module github.com/latrani/Kiln/web

go 1.27.1

require (
	charm.land/bubbletea/v2 v2.0.10
	github.com/charmbracelet/colorprofile v0.4.3
	github.com/coder/websocket v1.8.15
	github.com/latrani/Kiln v0.0.0
	golang.org/x/crypto/x509roots/fallback v0.0.0-20261005185213-c3db4df58582
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260703014108-f5a850f9c2b7 // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/zalando/go-keyring v0.2.8 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/latrani/Kiln => ../

// v2.0.10 plus charmbracelet/bubbletea#1790 (GOOS=js support) and the
// three upstream fixes it sits on. Drop once a release ships #1790; see
// bubbletea_test.go and .github/workflows/bubbletea-watch.yml.
replace charm.land/bubbletea/v2 => github.com/0magnet/bubbletea/v2 v2.0.9-0.20261004194220-80c2fc5bd53f
