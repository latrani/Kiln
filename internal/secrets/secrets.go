// Package secrets stores character passwords in the OS keychain or, for
// systems without one, a file only the user can read.
package secrets

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const service = "kiln"

// ErrNotFound means no password is saved for the character.
var ErrNotFound = keyring.ErrNotFound

func key(world, char string) string { return world + "/" + char }

// Store saves character passwords.
type Store interface {
	Get(world, char string) (string, error)
	Set(world, char, password string) error
}

// Open returns the store named by config's password_store: "keychain",
// "file" (passwords.json in dataDir) or "none" (nothing is saved).
func Open(kind, dataDir string) Store {
	switch kind {
	case "file":
		return File{Path: filepath.Join(dataDir, "passwords.json")}
	case "none":
		return None{}
	}
	return Keychain{}
}

// Keychain is the OS keychain.
type Keychain struct{}

func (Keychain) Get(world, char string) (string, error) {
	return keyring.Get(service, key(world, char))
}

func (Keychain) Set(world, char, password string) error {
	return keyring.Set(service, key(world, char), password)
}

// None saves nothing.
type None struct{}

func (None) Get(string, string) (string, error) { return "", ErrNotFound }

func (None) Set(string, string, string) error {
	return errors.New(`password_store is "none"`)
}

// File is a JSON object of "world/char" to password, readable only by the
// user (mode 0600, in a 0700 directory).
type File struct{ Path string }

func (f File) load() (map[string]string, error) {
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (f File) Get(world, char string) (string, error) {
	m, err := f.load()
	if err != nil {
		return "", err
	}
	pw, ok := m[key(world, char)]
	if !ok {
		return "", ErrNotFound
	}
	return pw, nil
}

func (f File) Set(world, char, password string) error {
	m, err := f.load()
	if err != nil {
		return err
	}
	m[key(world, char)] = password
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".passwords-*") // created 0600
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.Path)
}
