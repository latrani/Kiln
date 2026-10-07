// Package backup packs Kiln's config and TLS pins into a zip and unpacks
// one back: the web build's Back up and Restore. Browsers can clear what
// a page saves, so this is the copy the user keeps.
package backup

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/latrani/Kiln/internal/str"
)

// MaxUnpacked caps a restore's total size.
const MaxUnpacked = 10 << 20

const (
	configPrefix = "config/"     //str:ok
	knownName    = "known_hosts" //str:ok
)

// Zip packs every file under configDir as config/… and knownHosts as
// known_hosts, if it exists.
func Zip(configDir, knownHosts string) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	add := func(name, file string) error {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		f, err := w.Create(name)
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		return err
	}
	err := filepath.WalkDir(configDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(configDir, p)
		if err != nil {
			return err
		}
		return add(configPrefix+filepath.ToSlash(rel), p)
	})
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(knownHosts); err == nil {
		if err := add(knownName, knownHosts); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unzip writes a backup's files over configDir and knownHosts. It checks
// every entry, and reads every file into memory, before writing any, so a
// bad name or an oversized backup leaves disk untouched. It never deletes
// a file, and returns how many files it wrote.
func Unzip(data []byte, configDir, knownHosts string) (int, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, str.Wrap(str.BackupNotZip(), err)
	}
	type item struct {
		dest string
		body []byte
	}
	var items []item
	var total int
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/") {
			continue // a folder: made as files need it
		}
		clean := path.Clean(f.Name)
		var dest string
		switch {
		case clean != f.Name || path.IsAbs(clean):
		case clean == knownName:
			dest = knownHosts
		case strings.HasPrefix(clean, configPrefix):
			dest = filepath.Join(configDir, filepath.FromSlash(strings.TrimPrefix(clean, configPrefix)))
		}
		if dest == "" {
			return 0, errors.New(str.BackupStrayFile(f.Name))
		}
		// Count the bytes actually read. The stdlib reader already fails
		// content longer than its header, so this is defense in depth.
		rc, err := f.Open()
		if err != nil {
			return 0, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, int64(MaxUnpacked-total)+1))
		rc.Close()
		if err != nil {
			return 0, err
		}
		total += len(b)
		if total > MaxUnpacked {
			return 0, errors.New(str.BackupTooBig(MaxUnpacked >> 20))
		}
		items = append(items, item{dest, b})
	}
	if len(items) == 0 {
		return 0, errors.New(str.BackupEmpty())
	}
	for _, it := range items {
		if err := os.MkdirAll(filepath.Dir(it.dest), 0o700); err != nil {
			return 0, err
		}
		if err := os.WriteFile(it.dest, it.body, 0o600); err != nil {
			return 0, err
		}
	}
	return len(items), nil
}
