package main

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type setupStep int

const (
	setupInput    setupStep = iota
	setupVerifying
	setupDone
)

type setupWizard struct {
	input   textinput.Model
	spinner spinner.Model
	step    setupStep
	width   int
	height  int

	token       string // validated token (empty until success)
	err         string // error message from last attempt
	keychainErr string // keychain storage error (empty if saved OK)
}

type setupValidMsg struct {
	token       string
	keychainErr error
}

type setupInvalidMsg struct {
	err string
}

func newSetupWizard() setupWizard {
	ti := textinput.New()
	ti.Placeholder = "paste your API token here..."
	ti.Focus()
	ti.CharLimit = 200
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorBlue)

	return setupWizard{
		input:   ti,
		spinner: s,
		step:    setupInput,
	}
}

func (w setupWizard) Init() tea.Cmd {
	return textinput.Blink
}

func (w setupWizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
		w.input.Width = msg.Width - 10
		return w, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return w, tea.Quit
		}

		switch w.step {
		case setupInput:
			switch msg.String() {
			case "esc":
				return w, tea.Quit
			case "enter":
				token := strings.TrimSpace(w.input.Value())
				if token == "" {
					return w, nil
				}
				w.step = setupVerifying
				w.err = ""
				return w, tea.Batch(validateAndStoreToken(token), w.spinner.Tick)
			}

		case setupDone:
			if w.token != "" {
				// Success — any key to continue
				return w, tea.Quit
			}
			// Error — enter to retry, esc to quit
			switch msg.String() {
			case "esc":
				return w, tea.Quit
			case "enter":
				w.step = setupInput
				w.err = ""
				w.input.Reset()
				w.input.Focus()
				return w, textinput.Blink
			}
			return w, nil
		}

	case setupValidMsg:
		w.token = msg.token
		w.step = setupDone
		if msg.keychainErr != nil {
			w.keychainErr = msg.keychainErr.Error()
		}
		return w, nil

	case setupInvalidMsg:
		w.step = setupDone
		w.err = msg.err
		return w, nil

	case spinner.TickMsg:
		if w.step == setupVerifying {
			var cmd tea.Cmd
			w.spinner, cmd = w.spinner.Update(msg)
			return w, cmd
		}
		return w, nil
	}

	// Pass through to text input (cursor blink, typing, paste)
	if w.step == setupInput {
		var cmd tea.Cmd
		w.input, cmd = w.input.Update(msg)
		return w, cmd
	}
	return w, nil
}

func (w setupWizard) View() string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true).
		MarginBottom(1).
		Render("❏ Todoist — Setup"))
	b.WriteString("\n\n")

	switch w.step {
	case setupInput:
		step := lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
		bright := lipgloss.NewStyle().Foreground(colorBright).Bold(true)
		dim := lipgloss.NewStyle().Foreground(colorTextDim)

		b.WriteString("Welcome! Let's connect your Todoist account.\n\n")
		b.WriteString(step.Render("1") + "  Open your browser to:\n")
		b.WriteString("   " + bright.Render("todoist.com/prefs/integrations/developer") + "\n\n")
		b.WriteString(step.Render("2") + "  Copy your API token\n\n")
		b.WriteString(step.Render("3") + "  Paste it here:\n\n")
		b.WriteString("   " + w.input.View() + "\n")

		if w.err != "" {
			b.WriteString("\n")
			b.WriteString("   " + lipgloss.NewStyle().Foreground(colorRed).Render("✗ "+w.err) + "\n")
		}

		b.WriteString("\n")
		b.WriteString(dim.Render("   enter submit   esc quit"))

	case setupVerifying:
		b.WriteString(w.spinner.View() + " Verifying your token...")

	case setupDone:
		if w.token != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render("✓ Connected!") + "\n\n")
			if w.keychainErr == "" {
				b.WriteString("Token saved to " + tokenStorageLocation() + ".\n")
			} else {
				b.WriteString("Token verified but could not save to " + tokenStorageLocation() + ".\n")
				b.WriteString(lipgloss.NewStyle().Foreground(colorTextDim).Render(w.keychainErr) + "\n")
				b.WriteString("Set " + lipgloss.NewStyle().Bold(true).Render("TODOIST_API_TOKEN") + " env var for persistence.\n")
			}
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorTextDim).Render("Press any key to start..."))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("✗ Could not connect") + "\n\n")
			b.WriteString(w.err + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorTextDim).Render("enter try again   esc quit"))
		}
	}

	return helpStyle.Width(w.width).Height(w.height).Render(b.String())
}

func validateAndStoreToken(token string) tea.Cmd {
	return func() tea.Msg {
		client := NewClient(token)
		_, err := client.GetProjects(context.Background())
		if err != nil {
			if strings.Contains(err.Error(), "401") {
				return setupInvalidMsg{err: "Invalid token — check that you copied the full token"}
			}
			return setupInvalidMsg{err: err.Error()}
		}
		keychainErr := keychainSet(token)
		return setupValidMsg{token: token, keychainErr: keychainErr}
	}
}
