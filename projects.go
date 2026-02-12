package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProjectsView is the sidebar project list.
// cursor=0 is the virtual "Today" entry; cursor>=1 maps to projects[cursor-1].
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
		cursor:   0, // default to Today
		addInput: ai,
	}
}

func (v ProjectsView) Init() tea.Cmd {
	return v.repo.FetchProjects()
}

// IsTodaySelected returns true when the virtual Today entry is selected.
func (v ProjectsView) IsTodaySelected() bool {
	return v.cursor == 0
}

func (v ProjectsView) Update(msg tea.Msg) (ProjectsView, tea.Cmd) {
	switch msg := msg.(type) {
	case cachedProjectsMsg:
		v.projects = sortProjects(msg.projects)
		// cursor stays at 0 (Today) on first load
		return v, v.repo.RefreshProjects()

	case projectsMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to load projects: " + msg.err.Error(), isError: true}
			}
		}
		v.projects = sortProjects(msg.projects)
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
		// Clamp cursor (remember: max valid is len(projects) since 0=Today)
		if v.cursor > len(v.projects) {
			v.cursor = len(v.projects)
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
	// Dialog mode
	switch v.mode {
	case "add":
		switch ResolveAction(ContextMainTasksSearch, msg.String()) {
		case ActionConfirm:
			name := strings.TrimSpace(v.addInput.Value())
			if name == "" {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			return v, v.repo.CreateProject(name)
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.addInput, cmd = v.addInput.Update(msg)
		return v, cmd

	case "archive":
		switch ResolveAction(ContextMainSidebarDialog, msg.String()) {
		case ActionConfirm:
			p := v.SelectedProject()
			if p != nil {
				v.mode = ""
				return v, v.repo.ArchiveProject(p.ID)
			}
			v.mode = ""
			return v, nil
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		return v, nil
	}

	// Normal mode — bounds: 0 (Today) to len(projects) inclusive
	maxCursor := len(v.projects) // cursor=len(projects) maps to projects[len-1]
	switch ResolveAction(ContextMainSidebar, msg.String()) {
	case ActionNavDown:
		if v.cursor < maxCursor {
			v.cursor++
		}
		return v, nil
	case ActionNavUp:
		if v.cursor > 0 {
			v.cursor--
		}
		return v, nil
	case ActionNavTop:
		v.cursor = 0
		return v, nil
	case ActionNavBottom:
		v.cursor = maxCursor
		return v, nil
	case ActionAddProject:
		v.mode = "add"
		v.addInput.Reset()
		v.addInput.Focus()
		return v, textinput.Blink
	case ActionArchiveProject:
		// No-op for Today (cursor=0) or Inbox
		if v.cursor == 0 {
			return v, nil
		}
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

	// Total items = 1 (Today) + len(projects)
	totalItems := 1 + len(v.projects)

	maxVisible := v.height - 3
	if maxVisible < 1 {
		maxVisible = 1
	}

	// Scrolling window over the combined list
	start := 0
	if v.cursor >= maxVisible {
		start = v.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > totalItems {
		end = totalItems
	}

	for i := start; i < end; i++ {
		selected := i == v.cursor

		if i == 0 {
			// Virtual "Today" entry
			line := "☀ Today"
			if selected {
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
				b.WriteString(projectNormalStyle.Width(v.width - 2).Render(
					lipgloss.NewStyle().Foreground(colorYellow).Render("☀") + " Today"))
			}
		} else {
			// Real project at projects[i-1]
			p := v.projects[i-1]
			name := truncate(p.Name, v.width-6)

			dotChar := "●"
			if p.InboxProject {
				dotChar = "⌂"
			}

			if selected {
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

func (v ProjectsView) DialogMode() string {
	return v.mode
}

// SelectedProject returns the currently selected project, or nil if Today is selected.
func (v ProjectsView) SelectedProject() *Project {
	if v.cursor == 0 {
		return nil // Today
	}
	idx := v.cursor - 1
	if idx >= 0 && idx < len(v.projects) {
		return &v.projects[idx]
	}
	return nil
}

// SelectedProjectID returns the ID of the selected project, or "" if Today is selected.
func (v ProjectsView) SelectedProjectID() string {
	if p := v.SelectedProject(); p != nil {
		return p.ID
	}
	return ""
}

// HandleMouse processes mouse events for the sidebar.
func (v ProjectsView) HandleMouse(m tea.MouseEvent, yOffset int) (ProjectsView, tea.Cmd) {
	if m.Action == tea.MouseActionMotion || m.Action == tea.MouseActionRelease {
		return v, nil
	}

	// Total items = 1 (Today) + len(projects)
	totalItems := 1 + len(v.projects)

	// Scroll wheel
	if m.Button == tea.MouseButtonWheelDown {
		if v.cursor < totalItems-1 {
			v.cursor++
		}
		return v, nil
	}
	if m.Button == tea.MouseButtonWheelUp {
		if v.cursor > 0 {
			v.cursor--
		}
		return v, nil
	}

	// Left click
	if m.Button != tea.MouseButtonLeft {
		return v, nil
	}

	localY := m.Y - yOffset  // subtract header row
	itemOffset := localY - 2 // subtract title + margin

	// Recompute scroll window (same as View)
	maxVisible := v.height - 3
	if maxVisible < 1 {
		maxVisible = 1
	}
	start := 0
	if v.cursor >= maxVisible {
		start = v.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > totalItems {
		end = totalItems
	}

	clickedIndex := start + itemOffset
	if clickedIndex >= start && clickedIndex < end {
		v.cursor = clickedIndex
	}

	return v, nil
}

// SelectProjectByID moves the cursor to the project with the given ID.
// Returns true if found, false if not found.
func (v *ProjectsView) SelectProjectByID(id string) bool {
	for i, p := range v.projects {
		if p.ID == id {
			v.cursor = i + 1 // +1 because cursor=0 is Today
			return true
		}
	}
	return false
}

func (v *ProjectsView) SelectToday() {
	v.cursor = 0
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
