//go:build js

package main

import (
	"crypto/x509"
	"testing"
)

func TestSystemRootsAreBundled(t *testing.T) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		t.Fatal(err)
	}
	if pool.Equal(x509.NewCertPool()) {
		t.Error("no root certificates: CA-verified worlds can't connect")
	}
}
