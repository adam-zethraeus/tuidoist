package main

import (
	"strings"

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
}

func NewProjectsView(repo *Repository) ProjectsView {
	return ProjectsView{
		repo: repo,
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

	case tea.KeyMsg:
		if !v.focused {
			return v, nil
		}
		switch msg.String() {
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
		}
	}
	return v, nil
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

		// Color dot
		dot := lipgloss.NewStyle().Foreground(projectColor(p.Color)).Render("●")
		if p.InboxProject {
			dot = lipgloss.NewStyle().Foreground(colorBlue).Render("⌂")
		}

		line := dot + " " + name
		if p.IsFavorite {
			line += " " + projectFavStyle.Render("★")
		}

		if i == v.cursor && v.focused {
			b.WriteString(projectSelectedStyle.Width(v.width - 2).Render(line))
		} else if i == v.cursor {
			b.WriteString(lipgloss.NewStyle().
				Foreground(colorBright).
				Padding(0, 1).
				Width(v.width - 2).
				Render(line))
		} else {
			b.WriteString(projectNormalStyle.Width(v.width - 2).Render(line))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (v *ProjectsView) SetSize(width, height int) {
	v.width = width
	v.height = height
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
