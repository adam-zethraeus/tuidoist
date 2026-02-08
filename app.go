package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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
	today     TodayView
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

	// Search overlay
	showSearch bool
	search     SearchView

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
		today:     NewTodayView(repo),
		queue:     NewQueueView(repo),
		completed: NewCompletedView(repo),
		search:    NewSearchView(repo),
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

func (a App) isTodayActive() bool { return a.projects.IsTodaySelected() }

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
		a.updateSizes()
		return a, nil

	case tea.MouseMsg:
		m := tea.MouseEvent(msg)

		// Filter out motion and release events
		if m.Action == tea.MouseActionMotion || m.Action == tea.MouseActionRelease {
			return a, nil
		}

		// Ignore mouse if a dialog is active
		if a.tasks.handlesInput() || a.projects.handlesInput() {
			return a, nil
		}

		// Ignore mouse on help/search overlays
		if a.showHelp || a.showSearch {
			return a, nil
		}

		// Queue overlay
		if a.showQueue {
			var cmd tea.Cmd
			a.queue, cmd = a.queue.Update(msg)
			return a, cmd
		}

		// Completed overlay
		if a.showCompleted {
			var cmd tea.Cmd
			a.completed, cmd = a.completed.Update(msg)
			return a, cmd
		}

		// Ignore clicks on header and footer
		if m.Y == 0 || m.Y >= a.height-1 {
			return a, nil
		}

		// Route to sidebar vs tasks
		if m.X < sidebarWidth {
			a.focus = focusSidebar
			a.projects.SetFocused(true)
			a.tasks.SetFocused(false)
			a.today.SetFocused(false)

			prevToday := a.projects.IsTodaySelected()
			prev := a.projects.SelectedProjectID()
			a.projects, _ = a.projects.HandleMouse(m, 1)

			// Switched to Today
			if a.projects.IsTodaySelected() && !prevToday {
				a.lastProjectID = ""
				a.today.Refresh()
				return a, nil
			}
			// Switched to a project
			if p := a.projects.SelectedProject(); p != nil && p.ID != prev {
				a.lastProjectID = p.ID
				var taskCmd tea.Cmd
				a.tasks, taskCmd = a.tasks.LoadProject(p.ID, p.Name)
				return a, taskCmd
			}
			return a, nil
		}

		// Content area
		a.focus = focusTasks
		a.projects.SetFocused(false)
		if a.isTodayActive() {
			a.today.SetFocused(true)
			a.tasks.SetFocused(false)
			a.today, _ = a.today.HandleMouse(m, 1)
		} else {
			a.tasks.SetFocused(true)
			a.today.SetFocused(false)
			a.tasks, _ = a.tasks.HandleMouse(m, 1)
		}
		return a, nil

	case tea.KeyMsg:
		// Always handle quit
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}

		// Search overlay handles its own input
		if a.showSearch {
			var cmd tea.Cmd
			a.search, cmd = a.search.Update(msg)
			if !a.search.IsActive() {
				a.showSearch = false
			}
			return a, cmd
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

		// Don't handle global keys if a dialog or search is open
		if a.isTodayActive() && a.today.handlesInput() {
			var cmd tea.Cmd
			a.today, cmd = a.today.Update(msg)
			return a, cmd
		}

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
		case "ctrl+p", "alt+p":
			a.showSearch = true
			a.search.Open()
			return a, textinput.Blink
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
			if a.isTodayActive() {
				a.today.SetFocused(a.focus == focusTasks)
				a.tasks.SetFocused(false)
			} else {
				a.tasks.SetFocused(a.focus == focusTasks)
				a.today.SetFocused(false)
			}
			return a, nil
		case "r":
			// Refresh — force API fetch
			a.loading = true
			if a.isTodayActive() {
				return a, a.repo.RefreshProjects()
			}
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
				if a.isTodayActive() {
					a.today.SetFocused(true)
					a.tasks.SetFocused(false)
				} else {
					a.tasks.SetFocused(true)
					a.today.SetFocused(false)
				}
				return a, nil
			}
		}

	case cachedProjectsMsg:
		a.loading = true
		var cmd tea.Cmd
		a.projects, cmd = a.projects.Update(msg)
		cmds = append(cmds, cmd)

		// On first load, cursor=0 means Today is selected
		if a.lastProjectID == "" && len(msg.projects) > 0 {
			if a.isTodayActive() {
				a.today.Refresh()
				a.today.SetFocused(false)
				a.projects.SetFocused(true)
			} else if p := a.projects.SelectedProject(); p != nil {
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

		// On first load or refresh, populate the active view
		if msg.err == nil && len(msg.projects) > 0 && a.lastProjectID == "" {
			if a.isTodayActive() {
				a.today.Refresh()
				a.today.SetFocused(false)
				a.projects.SetFocused(true)
			} else if p := a.projects.SelectedProject(); p != nil {
				a.lastProjectID = p.ID
				var taskCmd tea.Cmd
				a.tasks, taskCmd = a.tasks.LoadProject(p.ID, p.Name)
				cmds = append(cmds, taskCmd)
				a.tasks.SetFocused(false)
				a.projects.SetFocused(true)
			}
		}
		// Refresh today if active (cache may have been updated by background refresh)
		if msg.err == nil && a.isTodayActive() {
			a.today.Refresh()
		}
		// Enqueue sync for stale projects. Always run after API refresh
		// to catch newly discovered projects whose tasks/sections have
		// never been synced (no sync_meta entry = stale).
		if msg.err == nil {
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
		// Route to appropriate view
		if a.isTodayActive() {
			var cmd tea.Cmd
			a.today, cmd = a.today.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			var cmd tea.Cmd
			a.tasks, cmd = a.tasks.Update(msg)
			cmds = append(cmds, cmd)
		}
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
		// Background warming done — refresh Today if active
		if a.isTodayActive() {
			a.today.Refresh()
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

	case navigateToTaskMsg:
		a.showSearch = false
		// Navigate to the project containing the task
		a.projects.SelectProjectByID(msg.projectID)
		a.lastProjectID = msg.projectID
		p := a.projects.SelectedProject()
		projectName := ""
		if p != nil {
			projectName = p.Name
		}
		a.tasks.jumpToTaskID = msg.taskID
		var taskCmd tea.Cmd
		a.tasks, taskCmd = a.tasks.LoadProject(msg.projectID, projectName)
		// Switch focus to tasks
		a.focus = focusTasks
		a.projects.SetFocused(false)
		a.tasks.SetFocused(true)
		a.today.SetFocused(false)
		return a, taskCmd

	case navigateToProjectMsg:
		a.showSearch = false
		if msg.projectID == "" {
			// Navigate to Today
			a.projects.cursor = 0
			a.lastProjectID = ""
			a.today.Refresh()
			a.focus = focusTasks
			a.projects.SetFocused(false)
			a.today.SetFocused(true)
			a.tasks.SetFocused(false)
			return a, nil
		}
		a.projects.SelectProjectByID(msg.projectID)
		a.lastProjectID = msg.projectID
		p := a.projects.SelectedProject()
		projectName := ""
		if p != nil {
			projectName = p.Name
		}
		var taskCmd tea.Cmd
		a.tasks, taskCmd = a.tasks.LoadProject(msg.projectID, projectName)
		a.focus = focusTasks
		a.projects.SetFocused(false)
		a.tasks.SetFocused(true)
		a.today.SetFocused(false)
		return a, taskCmd

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd
	}

	// Route blink/tick messages to search overlay when active
	if a.showSearch {
		var cmd tea.Cmd
		a.search, cmd = a.search.Update(msg)
		return a, cmd
	}

	// Delegate to focused view
	switch a.focus {
	case focusSidebar:
		prevToday := a.projects.IsTodaySelected()
		prev := a.projects.SelectedProjectID()
		var cmd tea.Cmd
		a.projects, cmd = a.projects.Update(msg)
		cmds = append(cmds, cmd)
		// Switched to Today
		if a.projects.IsTodaySelected() && !prevToday {
			a.lastProjectID = ""
			a.today.Refresh()
		} else if p := a.projects.SelectedProject(); p != nil && p.ID != prev {
			// Auto-load project when sidebar cursor changes
			a.lastProjectID = p.ID
			var taskCmd tea.Cmd
			a.tasks, taskCmd = a.tasks.LoadProject(p.ID, p.Name)
			cmds = append(cmds, taskCmd)
		}
	case focusTasks:
		if a.isTodayActive() {
			var cmd tea.Cmd
			a.today, cmd = a.today.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			var cmd tea.Cmd
			a.tasks, cmd = a.tasks.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Pass mutation results to the active content view even when sidebar is focused
	switch msg.(type) {
	case taskClosedMsg, taskDeletedMsg, taskCreatedMsg, taskUpdatedMsg, quickAddMsg:
		if a.isTodayActive() {
			if a.focus != focusTasks {
				var cmd tea.Cmd
				a.today, cmd = a.today.Update(msg)
				cmds = append(cmds, cmd)
			}
		} else {
			if a.focus != focusTasks {
				var cmd tea.Cmd
				a.tasks, cmd = a.tasks.Update(msg)
				cmds = append(cmds, cmd)
			}
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

	if a.showSearch {
		return a.search.View(a.width, a.height)
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

	var contentView string
	if a.isTodayActive() {
		contentView = a.today.View()
	} else {
		contentView = a.tasks.View()
	}

	content := lipgloss.NewStyle().
		Padding(0, 2).
		Width(a.width - sidebarWidth - 1).
		Height(a.height - 3).
		Render(contentView)

	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)

	// Footer
	footer := a.renderFooter()

	// Compose
	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	view = padLines(view, a.width)

	// Overlay floating search panel if local search is active
	if panel := a.localSearchPanel(); panel != "" {
		fgLines := strings.Split(panel, "\n")
		panelW := 0
		for _, l := range fgLines {
			if w := lipgloss.Width(l); w > panelW {
				panelW = w
			}
		}
		panelH := len(fgLines)
		x := a.width - panelW - 1
		if x < sidebarWidth+2 {
			x = sidebarWidth + 2
		}
		y := a.height - panelH - 2
		view = placeOverlay(view, panel, x, y)
	}

	return view
}

func (a App) localSearchPanel() string {
	if a.isTodayActive() {
		return a.today.SearchPanel()
	}
	return a.tasks.SearchPanel()
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

	if a.tasks.handlesInput() && !a.tasks.searchMode {
		hints = append(hints,
			keyHint("enter", "confirm"),
			keyHint("esc", "cancel"),
		)
	} else if a.tasks.searchMode || (a.isTodayActive() && a.today.searchMode) {
		hints = append(hints,
			keyHint("enter", "search"),
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
				keyHint("^P", "search"),
				keyHint("C", "completed"),
				keyHint("?", "help"),
				keyHint("q", "quit"),
			)
		}
	} else if a.isTodayActive() {
		hints = append(hints,
			keyHint("j/k", "nav"),
			keyHint("x/space", "toggle"),
			keyHint("/", "search"),
		)
		if a.today.searchQuery != "" {
			hints = append(hints, keyHint("n/N", "next/prev"))
		}
		hints = append(hints,
			keyHint("tab", "projects"),
			keyHint("^P", "search all"),
			keyHint("C", "completed"),
			keyHint("Q", "queue"),
			keyHint("?", "help"),
			keyHint("q", "quit"),
		)
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
			keyHint("/", "search"),
		)
		if a.tasks.searchQuery != "" {
			hints = append(hints, keyHint("n/N", "next/prev"))
		}
		hints = append(hints,
			keyHint("^P", "search all"),
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
		{"Search", ""},
		{"ctrl+p", "Global search (tasks + projects)"},
		{"/", "Search in current view"},
		{"n / N", "Next / previous match"},
		{"esc", "Clear search"},
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
	contentW := a.width - sidebarWidth - 5
	contentH := a.height - 4
	a.tasks.SetSize(contentW, contentH)
	a.today.SetSize(contentW, contentH)
	a.projects.SetFocused(a.focus == focusSidebar)
	if a.isTodayActive() {
		a.today.SetFocused(a.focus == focusTasks)
		a.tasks.SetFocused(false)
	} else {
		a.tasks.SetFocused(a.focus == focusTasks)
		a.today.SetFocused(false)
	}
}
