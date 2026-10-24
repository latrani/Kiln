//go:build !dev

package main

import (
	"embed"
	"io/fs"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The page, built into frontend/dist. all: keeps the embed compiling
// while only .gitkeep is there.
//
//go:embed all:frontend/dist
var assets embed.FS

// pageFS is the page the app serves: the one built into it.
func pageFS() (fs.FS, error) {
	return fs.Sub(assets, "frontend/dist") //str:ok
}

// watchPage is the dev build's (dev.go); this build's page never changes.
func watchPage(*application.App, *application.WebviewWindow) error { return nil }
