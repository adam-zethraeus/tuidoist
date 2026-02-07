package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProjectsView is the sidebar project list
type ProjectsView struct {
	projects []Project
	cursor   int
	width    int
	height   int
	repo     *Repository
	focused  bool

	// Dialog state
	mode     string // "", "add", "archive"
	addInput textinput.Model
}

func NewProjectsView(repo *Repository) ProjectsView {
	ai := textinput.New()
	ai.Placeholder = "List name..."
	ai.CharLimit = 200

	return ProjectsView{
		repo:     repo,
		addInput: ai,
	}
}

func (v ProjectsView) Init() tea.Cmd {
	return v.repo.FetchProjects()
}

func (v ProjectsView) Update(msg tea.Msg) (ProjectsView, tea.Cmd) {
	switch msg := msg.(type) {
	case cachedProjectsMsg:
		v.projects = sortProjects(msg.projects)
		if v.cursor == 0 {
			for i, p := range v.projects {
				if p.InboxProject {
					v.cursor = i
					break
				}
			}
		}
		return v, v.repo.RefreshProjects()

	case projectsMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to load projects: " + msg.err.Error(), isError: true}
			}
		}
		firstLoad := len(v.projects) == 0
		v.projects = sortProjects(msg.projects)
		if firstLoad {
			for i, p := range v.projects {
				if p.InboxProject {
					v.cursor = i
					break
				}
			}
		}
		return v, nil

	case projectCreatedMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to create list: " + msg.err.Error(), isError: true}
			}
		}
		return v, tea.Batch(
			v.repo.RefreshProjects(),
			func() tea.Msg { return toastMsg{text: "List created", isError: false} },
		)

	case projectArchivedMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to archive list: " + msg.err.Error(), isError: true}
			}
		}
		// Remove archived project from local list
		for i, p := range v.projects {
			if p.ID == msg.projectID {
				v.projects = append(v.projects[:i], v.projects[i+1:]...)
				break
			}
		}
		if v.cursor >= len(v.projects) {
			v.cursor = len(v.projects) - 1
		}
		if v.cursor < 0 {
			v.cursor = 0
		}
		return v, func() tea.Msg {
			return toastMsg{text: "List archived", isError: false}
		}

	case tea.KeyMsg:
		if !v.focused {
			return v, nil
		}
		return v.handleKey(msg)
	}

	// Update text input for non-key messages (e.g. cursor blink)
	if v.mode == "add" {
		var cmd tea.Cmd
		v.addInput, cmd = v.addInput.Update(msg)
		return v, cmd
	}

	return v, nil
}

func (v ProjectsView) handleKey(msg tea.KeyMsg) (ProjectsView, tea.Cmd) {
	key := msg.String()

	// Dialog mode
	switch v.mode {
	case "add":
		switch key {
		case "enter":
			name := strings.TrimSpace(v.addInput.Value())
			if name == "" {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			return v, v.repo.CreateProject(name)
		case "esc":
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.addInput, cmd = v.addInput.Update(msg)
		return v, cmd

	case "archive":
		switch key {
		case "y", "enter":
			p := v.SelectedProject()
			if p != nil {
				v.mode = ""
				return v, v.repo.ArchiveProject(p.ID)
			}
			v.mode = ""
			return v, nil
		case "n", "esc":
			v.mode = ""
			return v, nil
		}
		return v, nil
	}

	// Normal mode
	switch key {
	case "j", "down":
		if v.cursor < len(v.projects)-1 {
			v.cursor++
		}
		return v, nil
	case "k", "up":
		if v.cursor > 0 {
			v.cursor--
		}
		return v, nil
	case "g":
		v.cursor = 0
		return v, nil
	case "G":
		if len(v.projects) > 0 {
			v.cursor = len(v.projects) - 1
		}
		return v, nil
	case "a":
		v.mode = "add"
		v.addInput.Reset()
		v.addInput.Focus()
		return v, textinput.Blink
	case "d":
		p := v.SelectedProject()
		if p != nil && !p.InboxProject {
			v.mode = "archive"
		}
		return v, nil
	}

	return v, nil
}

func (v ProjectsView) handlesInput() bool {
	return v.mode != ""
}

func (v ProjectsView) View() string {
	if len(v.projects) == 0 {
		return emptyStyle.Render("No projects")
	}

	var b strings.Builder
	b.WriteString(sidebarTitleStyle.Render("Projects"))
	b.WriteString("\n")

	maxVisible := v.height - 3
	if maxVisible < 1 {
		maxVisible = 1
	}

	// Scrolling window
	start := 0
	if v.cursor >= maxVisible {
		start = v.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(v.projects) {
		end = len(v.projects)
	}

	for i := start; i < end; i++ {
		p := v.projects[i]
		name := truncate(p.Name, v.width-6)
		selected := i == v.cursor

		dotChar := "●"
		if p.InboxProject {
			dotChar = "⌂"
		}

		if selected {
			// Plain text avoids inner ANSI resets breaking the selection background
			line := dotChar + " " + name
			if p.IsFavorite {
				line += " ★"
			}
			if v.focused {
				b.WriteString(projectSelectedStyle.Width(v.width - 2).Render(line))
			} else {
				b.WriteString(lipgloss.NewStyle().
					Foreground(colorBright).
					Padding(0, 1).
					Width(v.width - 2).
					Render(line))
			}
		} else {
			dot := lipgloss.NewStyle().Foreground(projectColor(p.Color)).Render(dotChar)
			if p.InboxProject {
				dot = lipgloss.NewStyle().Foreground(colorBlue).Render(dotChar)
			}
			line := dot + " " + name
			if p.IsFavorite {
				line += " " + projectFavStyle.Render("★")
			}
			b.WriteString(projectNormalStyle.Width(v.width - 2).Render(line))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Dialog overlay
	if v.mode == "add" {
		b.WriteString("\n\n")
		b.WriteString(dialogTitleStyle.Render("New List") + "\n")
		b.WriteString(v.addInput.View())
	}
	if v.mode == "archive" {
		b.WriteString("\n\n")
		b.WriteString(dialogTitleStyle.Render("Archive List?") + "\n")
		name := ""
		if p := v.SelectedProject(); p != nil {
			name = p.Name
		}
		b.WriteString(taskContentStyle.Render(name) + "\n")
		b.WriteString(footerKeyStyle.Render("y") + " yes  " + footerKeyStyle.Render("n") + " no")
	}

	return b.String()
}

func (v *ProjectsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.addInput.Width = width - 6
}

func (v *ProjectsView) SetFocused(focused bool) {
	v.focused = focused
}

func (v ProjectsView) SelectedProject() *Project {
	if v.cursor >= 0 && v.cursor < len(v.projects) {
		return &v.projects[v.cursor]
	}
	return nil
}

func (v ProjectsView) SelectedProjectID() string {
	if p := v.SelectedProject(); p != nil {
		return p.ID
	}
	return ""
}

// sortProjects puts Inbox first, then favorites, then the rest by order
func sortProjects(projects []Project) []Project {
	var inbox []Project
	var favs []Project
	var rest []Project

	for _, p := range projects {
		if p.InboxProject {
			inbox = append(inbox, p)
		} else if p.IsFavorite {
			favs = append(favs, p)
		} else {
			rest = append(rest, p)
		}
	}

	result := make([]Project, 0, len(projects))
	result = append(result, inbox...)
	result = append(result, favs...)
	result = append(result, rest...)
	return result
}

// projectColor maps Todoist color names to lipgloss colors
func projectColor(name string) lipgloss.Color {
	if hex, ok := colorHex[name]; ok {
		return lipgloss.Color(hex)
	}
	return colorTextDim
}
