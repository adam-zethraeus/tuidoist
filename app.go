package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type focus int

const (
	focusSidebar focus = iota
	focusTasks
)

const sidebarWidth = 28

// App is the root Bubbletea model
type App struct {
	repo  *Repository
	focus focus
	width  int
	height int
	ready  bool

	// Sub-views
	projects  ProjectsView
	tasks     TasksView
	queue     QueueView
	completed CompletedView

	// Loading state
	loading bool
	spinner spinner.Model

	// Toast notification
	toast      string
	toastError bool

	// Help overlay
	showHelp bool

	// Queue overlay
	showQueue bool

	// Completed overlay
	showCompleted bool

	// Track last selected project to detect changes
	lastProjectID string

	// Background refresh
	bgRefreshStarted bool
}

func NewApp(repo *Repository) App {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorBlue)

	return App{
		repo:      repo,
		focus:     focusSidebar,
		projects:  NewProjectsView(repo),
		tasks:     NewTasksView(repo),
		queue:     NewQueueView(repo),
		completed: NewCompletedView(repo),
		loading:   true,
		spinner:   s,
	}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.spinner.Tick,
		a.projects.Init(),
		a.repo.FlushNext(),
	)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
		a.updateSizes()
		return a, nil

	case tea.KeyMsg:
		// Always handle quit
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}

		// Queue overlay handles its own input
		if a.showQueue {
			switch msg.String() {
			case "Q", "esc":
				a.showQueue = false
				return a, nil
			default:
				var cmd tea.Cmd
				a.queue, cmd = a.queue.Update(msg)
				return a, cmd
			}
		}

		// Completed overlay handles its own input
		if a.showCompleted {
			switch msg.String() {
			case "C", "esc":
				a.showCompleted = false
				return a, nil
			default:
				var cmd tea.Cmd
				a.completed, cmd = a.completed.Update(msg)
				return a, cmd
			}
		}

		// Don't handle global keys if a dialog is open
		if a.tasks.handlesInput() {
			var cmd tea.Cmd
			a.tasks, cmd = a.tasks.Update(msg)
			return a, cmd
		}

		if a.projects.handlesInput() {
			var cmd tea.Cmd
			a.projects, cmd = a.projects.Update(msg)
			return a, cmd
		}

		switch msg.String() {
		case "q":
			return a, tea.Quit
		case "Q":
			a.showQueue = true
			a.queue.Refresh()
			return a, nil
		case "?":
			a.showHelp = !a.showHelp
			return a, nil
		case "C":
			a.showCompleted = true
			a.completed.SetSize(a.height)
			a.completed.Refresh()
			return a, nil
		case "tab":
			if a.focus == focusSidebar {
				a.focus = focusTasks
			} else {
				a.focus = focusSidebar
			}
			a.projects.SetFocused(a.focus == focusSidebar)
			a.tasks.SetFocused(a.focus == focusTasks)
			return a, nil
		case "r":
			// Refresh — force API fetch
			a.loading = true
			return a, tea.Batch(
				a.repo.RefreshProjects(),
				a.repo.RefreshTasks(a.tasks.projectID),
				a.repo.RefreshSections(a.tasks.projectID),
			)
		case "enter":
			if a.focus == focusSidebar {
				// Just switch focus — project already loaded on cursor move
				a.focus = focusTasks
				a.projects.SetFocused(false)
				a.tasks.SetFocused(true)
				return a, nil
			}
		}

	case cachedProjectsMsg:
		a.loading = true
		var cmd tea.Cmd
		a.projects, cmd = a.projects.Update(msg)
		cmds = append(cmds, cmd)

		// Auto-load inbox tasks on first load
		if a.lastProjectID == "" && len(msg.projects) > 0 {
			p := a.projects.SelectedProject()
			if p != nil {
				a.lastProjectID = p.ID
				var taskCmd tea.Cmd
				a.tasks, taskCmd = a.tasks.LoadProject(p.ID, p.Name)
				cmds = append(cmds, taskCmd)
				a.tasks.SetFocused(false)
				a.projects.SetFocused(true)
			}
		}
		// Start background cache warming
		if !a.bgRefreshStarted {
			a.bgRefreshStarted = true
			cmds = append(cmds, a.repo.FindStaleProjects())
		}
		return a, tea.Batch(cmds...)

	case projectsMsg:
		a.loading = false
		var cmd tea.Cmd
		a.projects, cmd = a.projects.Update(msg)
		cmds = append(cmds, cmd)

		// Auto-load inbox tasks only on first load
		if msg.err == nil && len(msg.projects) > 0 && a.lastProjectID == "" {
			p := a.projects.SelectedProject()
			if p != nil {
				a.lastProjectID = p.ID
				var taskCmd tea.Cmd
				a.tasks, taskCmd = a.tasks.LoadProject(p.ID, p.Name)
				cmds = append(cmds, taskCmd)
				a.tasks.SetFocused(false)
				a.projects.SetFocused(true)
			}
		}
		// Start background cache warming
		if !a.bgRefreshStarted {
			a.bgRefreshStarted = true
			cmds = append(cmds, a.repo.FindStaleProjects())
		}
		return a, tea.Batch(cmds...)

	case cachedTasksMsg:
		var cmd tea.Cmd
		a.tasks, cmd = a.tasks.Update(msg)
		return a, cmd

	case cachedSectionsMsg:
		var cmd tea.Cmd
		a.tasks, cmd = a.tasks.Update(msg)
		return a, cmd

	case tasksMsg:
		if msg.projectID == a.tasks.projectID {
			a.loading = false
		}
		var cmd tea.Cmd
		a.tasks, cmd = a.tasks.Update(msg)
		return a, cmd

	case sectionsMsg:
		var cmd tea.Cmd
		a.tasks, cmd = a.tasks.Update(msg)
		return a, cmd

	case mutationFlushedMsg:
		if msg.err != nil {
			cmds = append(cmds, func() tea.Msg {
				return toastMsg{text: "Sync failed: " + msg.err.Error(), isError: true}
			})
		}
		// If a create was flushed, refresh task list to get the real ID
		if msg.mutation.Action == MutationCreate && msg.err == nil {
			cmds = append(cmds, a.repo.RefreshTasks(a.tasks.projectID))
		}
		// Chain: flush next mutation
		cmds = append(cmds, a.repo.FlushNext())
		return a, tea.Batch(cmds...)

	case taskReopenedMsg:
		// Route to tasks view (adds back to active list if same project)
		var cmd tea.Cmd
		a.tasks, cmd = a.tasks.Update(msg)
		cmds = append(cmds, cmd)
		// Refresh completed view if open
		if a.showCompleted {
			a.completed.Refresh()
		}
		cmds = append(cmds, a.repo.FlushNext())
		return a, tea.Batch(cmds...)

	case projectCreatedMsg:
		var cmd tea.Cmd
		a.projects, cmd = a.projects.Update(msg)
		return a, cmd

	case projectArchivedMsg:
		var cmd tea.Cmd
		a.projects, cmd = a.projects.Update(msg)
		cmds = append(cmds, cmd)
		// If current project was archived, switch to first available
		if msg.err == nil && msg.projectID == a.tasks.projectID {
			if p := a.projects.SelectedProject(); p != nil {
				a.lastProjectID = p.ID
				var taskCmd tea.Cmd
				a.tasks, taskCmd = a.tasks.LoadProject(p.ID, p.Name)
				cmds = append(cmds, taskCmd)
			}
		}
		return a, tea.Batch(cmds...)

	case projectUnarchivedMsg:
		if msg.err != nil {
			return a, func() tea.Msg {
				return toastMsg{text: "Failed to unarchive: " + msg.err.Error(), isError: true}
			}
		}
		// Refresh projects list and completed view
		if a.showCompleted {
			a.completed.Refresh()
		}
		return a, tea.Batch(
			a.repo.RefreshProjects(),
			func() tea.Msg { return toastMsg{text: "List unarchived", isError: false} },
		)

	case mutationConflictMsg:
		cmds = append(cmds, func() tea.Msg {
			return toastMsg{text: "Sync conflict — press Q to review", isError: true}
		})
		return a, tea.Batch(cmds...)

	case mutationEnqueuedMsg:
		// Sync indicator will update on next render
		return a, nil

	case flushNextMsg:
		return a, a.repo.FlushNext()

	case backgroundRefreshMsg:
		if len(msg.staleProjects) == 0 {
			return a, nil
		}
		// Split into 2 independent chains for concurrency=2
		var chain1, chain2 []string
		for i, pid := range msg.staleProjects {
			if i%2 == 0 {
				chain1 = append(chain1, pid)
			} else {
				chain2 = append(chain2, pid)
			}
		}
		cmds = append(cmds, a.repo.BackgroundRefreshProject(chain1[0], chain1[1:]))
		if len(chain2) > 0 {
			cmds = append(cmds, a.repo.BackgroundRefreshProject(chain2[0], chain2[1:]))
		}
		return a, tea.Batch(cmds...)

	case backgroundRefreshDoneMsg:
		if len(msg.remaining) > 0 {
			return a, a.repo.BackgroundRefreshProject(msg.remaining[0], msg.remaining[1:])
		}
		return a, nil

	case toastMsg:
		a.toast = msg.text
		a.toastError = msg.isError
		return a, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return clearToastMsg{}
		})

	case clearToastMsg:
		a.toast = ""
		return a, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd
	}

	// Delegate to focused view
	switch a.focus {
	case focusSidebar:
		prev := a.projects.SelectedProjectID()
		var cmd tea.Cmd
		a.projects, cmd = a.projects.Update(msg)
		cmds = append(cmds, cmd)
		// Auto-load project when sidebar cursor changes
		if p := a.projects.SelectedProject(); p != nil && p.ID != prev {
			a.lastProjectID = p.ID
			var taskCmd tea.Cmd
			a.tasks, taskCmd = a.tasks.LoadProject(p.ID, p.Name)
			cmds = append(cmds, taskCmd)
		}
	case focusTasks:
		var cmd tea.Cmd
		a.tasks, cmd = a.tasks.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Pass mutation results to tasks view even when sidebar is focused
	switch msg.(type) {
	case taskClosedMsg, taskDeletedMsg, taskCreatedMsg, taskUpdatedMsg, quickAddMsg:
		if a.focus != focusTasks {
			var cmd tea.Cmd
			a.tasks, cmd = a.tasks.Update(msg)
			cmds = append(cmds, cmd)
		}
		// Trigger flush after optimistic mutations
		cmds = append(cmds, a.repo.FlushNext())
	}

	// Pass project messages to sidebar when tasks are focused
	switch msg.(type) {
	case projectCreatedMsg, projectArchivedMsg:
		if a.focus != focusSidebar {
			var cmd tea.Cmd
			a.projects, cmd = a.projects.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return a, tea.Batch(cmds...)
}

func (a App) View() string {
	if !a.ready {
		return "Loading..."
	}

	if a.showHelp {
		return a.renderHelp()
	}

	if a.showQueue {
		return a.queue.View(a.width, a.height)
	}

	if a.showCompleted {
		return a.completed.View(a.width, a.height)
	}

	// Header
	header := a.renderHeader()

	// Sidebar + Content
	sidebar := sidebarStyle.
		Width(sidebarWidth).
		Height(a.height - 3).
		Render(a.projects.View())

	content := lipgloss.NewStyle().
		Padding(0, 2).
		Width(a.width - sidebarWidth - 1).
		Height(a.height - 3).
		Render(a.tasks.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)

	// Footer
	footer := a.renderFooter()

	// Compose
	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	return padLines(view, a.width)
}

func (a App) renderHeader() string {
	logo := headerStyle.Render("❏ Todoist")

	// Sync indicator
	var syncIndicator string
	if pending := a.repo.PendingCount(); pending > 0 {
		syncIndicator = syncPendingStyle.Render(fmt.Sprintf("↑ %d pending", pending))
	}
	if conflicts := a.repo.ConflictCount(); conflicts > 0 {
		if syncIndicator != "" {
			syncIndicator += " "
		}
		syncIndicator += syncConflictStyle.Render(fmt.Sprintf("⚠ %d conflicts", conflicts))
	}

	var right string
	if a.loading {
		right = a.spinner.View() + " Loading..."
	} else if a.toast != "" {
		if a.toastError {
			right = toastErrorStyle.Render("✗ " + a.toast)
		} else {
			right = toastSuccessStyle.Render("✓ " + a.toast)
		}
	}

	// Combine sync indicator and status
	if syncIndicator != "" {
		if right != "" {
			right = syncIndicator + "  " + right
		} else {
			right = syncIndicator
		}
	}

	gap := a.width - lipgloss.Width(logo) - lipgloss.Width(right) - 2
	if gap < 0 {
		gap = 0
	}

	line := logo + strings.Repeat(" ", gap) + right

	return lipgloss.NewStyle().
		Background(colorBgDark).
		Width(a.width).
		Padding(0, 1).
		Render(line)
}

func (a App) renderFooter() string {
	var hints []string

	if a.tasks.handlesInput() {
		hints = append(hints,
			keyHint("enter", "confirm"),
			keyHint("esc", "cancel"),
		)
	} else if a.focus == focusSidebar {
		if a.projects.handlesInput() {
			if a.projects.mode == "add" {
				hints = append(hints,
					keyHint("enter", "confirm"),
					keyHint("esc", "cancel"),
				)
			} else {
				hints = append(hints,
					keyHint("y", "confirm"),
					keyHint("n", "cancel"),
				)
			}
		} else {
			hints = append(hints,
				keyHint("j/k", "nav"),
				keyHint("enter/tab", "tasks"),
				keyHint("a", "add list"),
				keyHint("d", "archive"),
				keyHint("C", "completed"),
				keyHint("?", "help"),
				keyHint("q", "quit"),
			)
		}
	} else {
		hints = append(hints,
			keyHint("j/k", "nav"),
			keyHint("x/space", "toggle"),
			keyHint("a", "add"),
			keyHint("A", "quick"),
			keyHint("e", "edit"),
			keyHint("s", "due"),
			keyHint("d", "del"),
			keyHint("1-4", "prio"),
			keyHint("C", "completed"),
			keyHint("Q", "queue"),
			keyHint("tab", "projects"),
			keyHint("?", "help"),
		)
	}

	return lipgloss.NewStyle().
		Background(colorBgDark).
		Width(a.width).
		Padding(0, 1).
		Render(strings.Join(hints, "  "))
}

func (a App) renderHelp() string {
	helpItems := []struct{ key, desc string }{
		{"Navigation", ""},
		{"j / ↓", "Move down"},
		{"k / ↑", "Move up"},
		{"g", "Go to top"},
		{"G", "Go to bottom"},
		{"tab / enter", "Switch sidebar / tasks"},
		{"", ""},
		{"Tasks", ""},
		{"x / space", "Toggle done (complete/reopen)"},
		{"a", "Add new task"},
		{"A", "Quick add (natural language)"},
		{"e", "Edit task content"},
		{"s", "Set due date"},
		{"d", "Delete task"},
		{"1-4", "Set priority (1=highest)"},
		{"", ""},
		{"Projects", ""},
		{"a", "Add new list"},
		{"d", "Archive list"},
		{"", ""},
		{"General", ""},
		{"r", "Refresh"},
		{"C", "Recently completed"},
		{"Q", "Sync queue"},
		{"?", "Toggle help"},
		{"q / ctrl+c", "Quit"},
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true).
		MarginBottom(1).
		Render("Keyboard Shortcuts"))
	b.WriteString("\n\n")

	for _, item := range helpItems {
		if item.key == "" && item.desc == "" {
			b.WriteString("\n")
			continue
		}
		if item.desc == "" {
			b.WriteString(lipgloss.NewStyle().
				Foreground(colorYellow).
				Bold(true).
				Render(item.key))
			b.WriteString("\n")
			continue
		}
		b.WriteString(helpKeyStyle.Render(item.key))
		b.WriteString(helpDescStyle.Render(item.desc))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerKeyStyle.Render("?") + " " + footerDescStyle.Render("close help"))

	return helpStyle.
		Width(a.width).
		Height(a.height).
		Render(b.String())
}

func (a *App) updateSizes() {
	a.projects.SetSize(sidebarWidth-2, a.height-3)
	a.tasks.SetSize(a.width-sidebarWidth-5, a.height-4)
	a.projects.SetFocused(a.focus == focusSidebar)
	a.tasks.SetFocused(a.focus == focusTasks)
}
