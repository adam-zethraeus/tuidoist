//go:build !darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
)

func tokenFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "todoist-tui")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "token"), nil
}

// keychainGet reads the API token from the config file.
func keychainGet() (string, error) {
	path, err := tokenFilePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// keychainSet writes the API token to the config file with 0600 permissions.
func keychainSet(token string) error {
	path, err := tokenFilePath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token+"\n"), 0o600)
}

// keychainDelete removes the token config file.
func keychainDelete() error {
	path, err := tokenFilePath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func tokenStorageLocation() string {
	path, err := tokenFilePath()
	if err != nil {
		return "config file"
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, path); err == nil {
			return "~/" + rel
		}
	}
	return path
}
