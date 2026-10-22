//go:build windows

package main

import (
	"github.com/yomezza/test/internal/application"
	"github.com/yomezza/test/internal/security"
	"github.com/zalando/go-keyring"
	"testing"
)

// Uses an isolated, random test credential. Never reads or overwrites application credentials.
func TestNativeWindowsCredentialManager(t *testing.T) {
	service := "CatalogDB-smoke-" + security.ID()
	account := "synthetic-account"
	secret := security.ID()
	t.Cleanup(func() {
		if err := keyring.Delete(service, account); err != nil {
			t.Error("could not remove synthetic credential")
		}
	})
	store := application.OSStore{}
	if err := store.Set(service, account, secret); err != nil {
		t.Fatal("Credential Manager write failed:", err)
	}
	actual, err := store.Get(service, account)
	if err != nil {
		t.Fatal("Credential Manager read failed:", err)
	}
	if actual != secret {
		t.Fatal("Credential Manager returned a different value")
	}
}
