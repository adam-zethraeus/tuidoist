//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

const (
	keychainService = "todoist-tui"
	keychainAccount = "api-token"
)

// keychainGet retrieves the API token from macOS Keychain.
func keychainGet() (string, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-s", keychainService, "-a", keychainAccount, "-w").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// keychainSet stores the API token in macOS Keychain.
func keychainSet(token string) error {
	_ = keychainDelete()
	return exec.Command("security", "add-generic-password",
		"-s", keychainService, "-a", keychainAccount, "-w", token).Run()
}

// keychainDelete removes the API token from macOS Keychain.
func keychainDelete() error {
	return exec.Command("security", "delete-generic-password",
		"-s", keychainService, "-a", keychainAccount).Run()
}

func tokenStorageLocation() string {
	return "macOS Keychain"
}
