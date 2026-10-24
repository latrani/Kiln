//go:build dev

// The dev build (wails3 task run DEV=true): the page from frontend/dist
// on disk, which web/ui's build.mjs keeps rebuilding as src changes,
// and the window reloads each time it does. A reloaded page's Start
// sends it everything again (Kiln.Start), so the sessions carry on.

package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// pageDir is the page, from desktop/, where the task runs the app.
const pageDir = "frontend/dist" //str:ok

// pageFS is the page on disk, read afresh on every load.
func pageFS() (fs.FS, error) { return os.DirFS(pageDir), nil }

// watchPage starts the page's build in watch mode and reloads w after
// each rebuild. The build ends when its stdin does, so with the app,
// however the app ends.
func watchPage(a *application.App, w *application.WebviewWindow) error {
	dir, err := filepath.Abs(pageDir)
	if err != nil {
		return err
	}
	build := exec.Command("node", "build.mjs", dir, "desktop", "watch") //str:ok
	build.Dir = filepath.Join("..", "web", "ui")                       //str:ok
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	stdin, err := build.StdinPipe()
	if err != nil {
		return err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return err
	}
	if err := build.Start(); err != nil {
		watcher.Close()
		return err
	}
	a.OnShutdown(func() {
		stdin.Close()
		watcher.Close()
		build.Wait()
	})
	go func() {
		// One reload for a rebuild's app.js and app.css together.
		reload := time.AfterFunc(time.Hour, w.Reload)
		reload.Stop()
		for e := range watcher.Events {
			if e.Has(fsnotify.Write) || e.Has(fsnotify.Create) {
				reload.Reset(100 * time.Millisecond)
			}
		}
		reload.Stop()
	}()
	return nil
}
