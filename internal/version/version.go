// Package version describes the running build, from what the Go
// toolchain stamps into every binary: the module version (a release tag,
// or a pseudo-version for builds between releases) and the git commit.
package version

import (
	"runtime/debug"
	"strings"

	"golang.org/x/mod/module"
)

// String is "v0.2.1" for a release, "v0.2.1+4eec204" for a build after
// it (the commit it's built from), or "dev+4eec204" before any release;
// "*" marks a build with uncommitted changes.
func String() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	var rev string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return describe(info.Main.Version, rev, modified)
}

func describe(v, rev string, modified bool) string {
	v, _, _ = strings.Cut(v, "+") // drop build metadata like "+dirty"
	out := v
	switch {
	case module.IsPseudoVersion(v):
		if rev == "" {
			rev, _ = module.PseudoVersionRev(v)
		}
		base, _ := module.PseudoVersionBase(v)
		if base == "" {
			base = "dev"
		}
		out = base + "+" + short(rev)
	case !strings.HasPrefix(v, "v"): // "(devel)" or nothing
		out = "dev"
		if rev != "" {
			out += "+" + short(rev)
		}
	}
	if modified {
		out += "*"
	}
	return out
}

func short(rev string) string { return rev[:min(len(rev), 7)] }
