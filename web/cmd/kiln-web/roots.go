//go:build js

package main

// A browser has no root certificates Go can read (crypto/x509's js/wasm
// system pool is empty), so tls_trust = "ca" worlds would never verify.
// This bundles Mozilla's roots as the fallback pool.
import _ "golang.org/x/crypto/x509roots/fallback"
