package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const cacheTTL = 2 * time.Minute

func main() {
	token := os.Getenv("TODOIST_API_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "TODOIST_API_TOKEN environment variable is required.")
		fmt.Fprintln(os.Stderr, "Get your token from: https://app.todoist.com/prefs/integrations")
		os.Exit(1)
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

	// Set terminal background
	fmt.Fprint(os.Stdout, "\033]11;#1A1B26\007")
	defer fmt.Fprint(os.Stdout, "\033]111;\007")

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
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
