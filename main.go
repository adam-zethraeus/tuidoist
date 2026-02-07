package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const cacheTTL = 1 * time.Hour

func main() {
	// Resolve API token: env var > keychain > setup wizard
	token := os.Getenv("TODOIST_API_TOKEN")
	if token == "" {
		token, _ = keychainGet()
	}

	forceSetup := len(os.Args) > 1 && os.Args[1] == "--setup"

	// Set terminal background
	fmt.Fprint(os.Stdout, "\033]11;#1A1B26\007")
	defer fmt.Fprint(os.Stdout, "\033]111;\007")

	if token == "" || forceSetup {
		token = runSetupWizard()
		if token == "" {
			return
		}
	}

	client := NewClient(token)

	// Set up SQLite cache
	var store *Store
	if cacheDir, err := cacheDBPath(); err == nil {
		if s, err := NewStore(cacheDir, cacheTTL); err == nil {
			store = s
			defer store.Close()
		}
	}

	repo := NewRepository(client, store)
	app := NewApp(repo)

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func runSetupWizard() string {
	wizard := newSetupWizard()
	p := tea.NewProgram(wizard, tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return ""
	}
	w := m.(setupWizard)
	return w.token
}

func cacheDBPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "todoist-tui")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache.db"), nil
}
