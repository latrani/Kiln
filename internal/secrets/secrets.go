// Package secrets stores character passwords in the OS keychain.
package secrets

import "github.com/zalando/go-keyring"

const service = "kiln"

func key(world, char string) string { return world + "/" + char }

// Get returns the saved password for a character.
func Get(world, char string) (string, error) {
	return keyring.Get(service, key(world, char))
}

// Set saves the password for a character.
func Set(world, char, password string) error {
	return keyring.Set(service, key(world, char), password)
}
