package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// displayItem represents either a section header or a task in the flat list
type displayItem struct {
	isSection bool
	section   *Section
	task      *Task
	completed bool
}

// TasksView displays tasks for the selected project
type TasksView struct {
	tasks    []Task
	sections []Section
	items    []displayItem // flat list of sections + tasks for display
	cursor   int
	width    int
	height   int
	repo     *Repository
	focused  bool

	// Current project context
	projectID   string
	projectName string

	// Dialog state
	mode      string // "", "quick-add", "edit", "delete", "due"
	editInput textinput.Model
	dueInput   textinput.Model
	quickInput textinput.Model
	quickAddProject string // project name to default to for quick-add (empty = Inbox)

	// Scroll offset
	scrollOffset int

	// Loading state (waiting for async task fetch)
	loading bool

	// Completed tasks (in-memory, cleared on project switch)
	completedTasks []Task

	// Page-local search
	searchMode   bool
	searchInput  textinput.Model
	searchQuery  string
	matchIndices []int
	currentMatch int

	// Jump target (set by global search navigation)
	jumpToTaskID string
}

func NewTasksView(repo *Repository) TasksView {
	ei := textinput.New()
	ei.Placeholder = "Edit task..."
	ei.CharLimit = 500

	di := textinput.New()
	di.Placeholder = "e.g. tomorrow, next monday, every day"
	di.CharLimit = 200

	qi := textinput.New()
	qi.Placeholder = "Buy milk tomorrow #Work @urgent p1"
	qi.CharLimit = 500

	si := textinput.New()
	si.Placeholder = "Search..."
	si.CharLimit = 200

	return TasksView{
		repo:        repo,
		editInput:   ei,
		dueInput:    di,
		quickInput:  qi,
		searchInput: si,
	}
}

func (v TasksView) Init() tea.Cmd {
	return nil
}

func (v TasksView) LoadProject(projectID, projectName string) (TasksView, tea.Cmd) {
	v.projectID = projectID
	v.projectName = projectName
	v.cursor = 0
	v.scrollOffset = 0
	v.completedTasks = nil
	v.searchMode = false
	v.searchQuery = ""
	v.matchIndices = nil
	v.currentMatch = 0

	// Sync-load cache for instant display
	v.tasks = v.repo.GetCachedTasks(projectID)
	v.sections = v.repo.GetCachedSections(projectID)
	v.rebuildItems()
	v.loading = len(v.tasks) == 0

	return v, tea.Batch(
		v.repo.FetchTasks(projectID),
		v.repo.FetchSections(projectID),
	)
}

func (v TasksView) Update(msg tea.Msg) (TasksView, tea.Cmd) {
	switch msg := msg.(type) {
	case cachedTasksMsg:
		if msg.projectID != v.projectID {
			return v, nil
		}
		v.tasks = msg.tasks
		v.loading = false
		v.rebuildItems()
		return v, v.repo.RefreshTasks(v.projectID)

	case cachedSectionsMsg:
		if msg.projectID != v.projectID {
			return v, nil
		}
		v.sections = msg.sections
		v.rebuildItems()
		return v, v.repo.RefreshSections(v.projectID)

	case tasksMsg:
		if msg.projectID != v.projectID {
			return v, nil
		}
		if msg.err != nil {
			v.loading = false
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to load tasks: " + msg.err.Error(), isError: true}
			}
		}
		v.tasks = msg.tasks
		v.loading = false
		v.rebuildItems()
		v.clampCursor()
		return v, nil

	case sectionsMsg:
		if msg.projectID != v.projectID {
			return v, nil
		}
		if msg.err != nil {
			return v, nil
		}
		v.sections = msg.sections
		v.rebuildItems()
		v.clampCursor()
		return v, nil

	case taskClosedMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to complete task: " + msg.err.Error(), isError: true}
			}
		}
		// Move task to completedTasks (shown at bottom with strikethrough)
		for i, t := range v.tasks {
			if t.ID == msg.taskID {
				v.completedTasks = append(v.completedTasks, v.tasks[i])
				v.tasks = append(v.tasks[:i], v.tasks[i+1:]...)
				break
			}
		}
		v.rebuildItems()
		v.clampCursor()
		return v, func() tea.Msg {
			return toastMsg{text: "Task completed", isError: false}
		}

	case taskReopenedMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to reopen task: " + msg.err.Error(), isError: true}
			}
		}
		// Move from completedTasks back to active if same project
		for i, t := range v.completedTasks {
			if t.ID == msg.task.ID {
				v.completedTasks = append(v.completedTasks[:i], v.completedTasks[i+1:]...)
				break
			}
		}
		if msg.task.ProjectID == v.projectID {
			v.tasks = append(v.tasks, msg.task)
		}
		v.rebuildItems()
		return v, func() tea.Msg {
			return toastMsg{text: "Task reopened", isError: false}
		}

	case taskDeletedMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to delete task: " + msg.err.Error(), isError: true}
			}
		}
		for i, t := range v.tasks {
			if t.ID == msg.taskID {
				v.tasks = append(v.tasks[:i], v.tasks[i+1:]...)
				break
			}
		}
		v.rebuildItems()
		v.clampCursor()
		return v, func() tea.Msg {
			return toastMsg{text: "Task deleted", isError: false}
		}

	case taskCreatedMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to create task: " + msg.err.Error(), isError: true}
			}
		}
		v.tasks = append(v.tasks, msg.task)
		v.rebuildItems()
		return v, func() tea.Msg {
			return toastMsg{text: "Task created", isError: false}
		}

	case taskUpdatedMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to update task: " + msg.err.Error(), isError: true}
			}
		}
		for i, t := range v.tasks {
			if t.ID == msg.task.ID {
				v.tasks[i] = msg.task
				break
			}
		}
		v.rebuildItems()
		return v, func() tea.Msg {
			return toastMsg{text: "Task updated", isError: false}
		}

	case quickAddMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Quick add failed: " + msg.err.Error(), isError: true}
			}
		}
		// Refresh tasks
		return v, tea.Batch(
			v.repo.RefreshTasks(v.projectID),
			func() tea.Msg { return toastMsg{text: "Task added", isError: false} },
		)

	case tea.KeyMsg:
		if !v.focused && v.mode == "" && !v.searchMode {
			return v, nil
		}
		return v.handleKey(msg)
	}

	// Update text inputs if in a dialog mode
	if v.mode == "edit" {
		var cmd tea.Cmd
		v.editInput, cmd = v.editInput.Update(msg)
		return v, cmd
	}
	if v.mode == "due" {
		var cmd tea.Cmd
		v.dueInput, cmd = v.dueInput.Update(msg)
		return v, cmd
	}
	if v.mode == "quick-add" {
		var cmd tea.Cmd
		v.quickInput, cmd = v.quickInput.Update(msg)
		return v, cmd
	}
	if v.searchMode {
		var cmd tea.Cmd
		v.searchInput, cmd = v.searchInput.Update(msg)
		return v, cmd
	}

	return v, nil
}

func (v TasksView) handleKey(msg tea.KeyMsg) (TasksView, tea.Cmd) {
	key := msg.String()

	// Search mode
	if v.searchMode {
		switch key {
		case "enter":
			v.searchMode = false
			v.searchQuery = strings.TrimSpace(v.searchInput.Value())
			v.matchIndices = findMatchIndices(v.items, v.searchQuery)
			if len(v.matchIndices) > 0 {
				v.cursor = v.matchIndices[0]
				v.currentMatch = 0
				v.ensureVisible()
			}
			return v, nil
		case "esc":
			v.searchMode = false
			return v, nil
		}
		var cmd tea.Cmd
		v.searchInput, cmd = v.searchInput.Update(msg)
		// Live filter as user types
		q := strings.TrimSpace(v.searchInput.Value())
		v.matchIndices = findMatchIndices(v.items, q)
		return v, cmd
	}

	// Dialog mode key handling
	switch v.mode {
	case "quick-add":
		switch key {
		case "enter":
			text := strings.TrimSpace(v.quickInput.Value())
			if text == "" {
				v.mode = ""
				return v, nil
			}
			// Default to current project if the user didn't specify one
			if v.quickAddProject != "" && !strings.Contains(text, "#") {
				text += " #" + v.quickAddProject
			}
			v.mode = ""
			return v, v.repo.QuickAdd(text)
		case "esc":
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.quickInput, cmd = v.quickInput.Update(msg)
		return v, cmd

	case "edit":
		switch key {
		case "enter":
			content := strings.TrimSpace(v.editInput.Value())
			if content == "" {
				v.mode = ""
				return v, nil
			}
			task := v.selectedTask()
			if task == nil {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			return v, v.repo.UpdateTask(task.ID, updateTaskRequest{Content: &content})
		case "esc":
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.editInput, cmd = v.editInput.Update(msg)
		return v, cmd

	case "due":
		switch key {
		case "enter":
			dueStr := strings.TrimSpace(v.dueInput.Value())
			task := v.selectedTask()
			if task == nil {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			if dueStr == "" {
				empty := ""
				return v, v.repo.UpdateTask(task.ID, updateTaskRequest{DueString: &empty})
			}
			return v, v.repo.UpdateTask(task.ID, updateTaskRequest{DueString: &dueStr})
		case "esc":
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.dueInput, cmd = v.dueInput.Update(msg)
		return v, cmd

	case "delete":
		switch key {
		case "y", "enter":
			task := v.selectedTask()
			if task != nil {
				v.mode = ""
				return v, v.repo.DeleteTask(task.ID)
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
	case "/":
		v.searchMode = true
		v.searchInput.Reset()
		v.searchInput.Focus()
		return v, textinput.Blink
	case "n":
		if v.searchQuery != "" && len(v.matchIndices) > 0 {
			idx, num := nextMatchIndex(v.matchIndices, v.cursor)
			if idx >= 0 {
				v.cursor = idx
				v.currentMatch = num
				v.ensureVisible()
			}
		}
		return v, nil
	case "N":
		if v.searchQuery != "" && len(v.matchIndices) > 0 {
			idx, num := prevMatchIndex(v.matchIndices, v.cursor)
			if idx >= 0 {
				v.cursor = idx
				v.currentMatch = num
				v.ensureVisible()
			}
		}
		return v, nil
	case "esc":
		if v.searchQuery != "" {
			v.searchQuery = ""
			v.matchIndices = nil
			v.currentMatch = 0
			return v, nil
		}
		return v, nil
	case "j", "down":
		v.moveDown()
		v.ensureVisible()
		return v, nil
	case "k", "up":
		v.moveUp()
		v.ensureVisible()
		return v, nil
	case "g":
		v.cursor = 0
		v.scrollOffset = 0
		v.skipToNextTask(1)
		return v, nil
	case "G":
		if len(v.items) > 0 {
			v.cursor = len(v.items) - 1
			v.skipToNextTask(-1)
			v.ensureVisible()
		}
		return v, nil
	case "x", " ":
		item := v.selectedItem()
		if item != nil && item.task != nil {
			if item.completed {
				return v, v.repo.ReopenTask(*item.task)
			}
			return v, v.repo.CloseTask(item.task.ID)
		}
	case "e":
		task := v.selectedTask()
		if task != nil {
			v.mode = "edit"
			v.editInput.SetValue(task.Content)
			v.editInput.Focus()
			return v, textinput.Blink
		}
	case "d":
		task := v.selectedTask()
		if task != nil {
			v.mode = "delete"
		}
		return v, nil
	case "s":
		task := v.selectedTask()
		if task != nil {
			v.mode = "due"
			v.dueInput.Reset()
			if task.Due != nil {
				v.dueInput.SetValue(task.Due.String)
			}
			v.dueInput.Focus()
			return v, textinput.Blink
		}
	case "1", "2", "3", "4":
		task := v.selectedTask()
		if task != nil {
			p := int(key[0] - '0')
			return v, v.repo.UpdateTask(task.ID, updateTaskRequest{Priority: &p})
		}
	}

	return v, nil
}

func (v TasksView) View() string {
	if v.projectID == "" {
		return emptyStyle.Render("Select a project")
	}
	if v.loading {
		return lipgloss.NewStyle().
			Foreground(colorBright).
			Bold(true).
			Padding(0, 0, 1, 0).
			Render(v.projectName) + "\n" +
			emptyStyle.Render("Loading tasks...")
	}
	if len(v.items) == 0 && len(v.tasks) == 0 {
		content := emptyStyle.Render("No tasks - press 'n' to add one")
		if v.mode != "" && v.mode != "quick-add" {
			content += "\n" + v.renderDialog()
		}
		return content
	}

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(colorBright).
		Bold(true).
		Padding(0, 0, 1, 0).
		Render(v.projectName)
	b.WriteString(title)
	b.WriteString("\n")

	// Calculate visible area
	visibleHeight := v.height - 4 // account for title + dialog space
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	end := v.scrollOffset + visibleHeight
	if end > len(v.items) {
		end = len(v.items)
	}

	for i := v.scrollOffset; i < end; i++ {
		item := v.items[i]
		if item.isSection {
			name := item.section.Name
			b.WriteString(sectionStyle.Render("━━ " + name))
			b.WriteString("\n")
			continue
		}

		task := item.task
		selected := i == v.cursor && v.focused

		line := v.renderTask(task, selected, item.completed)
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Render dialog overlay if active (quick-add is rendered as app-level overlay)
	if v.mode != "" && v.mode != "quick-add" {
		b.WriteString("\n\n")
		b.WriteString(v.renderDialog())
	}

	return b.String()
}

func (v TasksView) renderTask(task *Task, selected bool, completed bool) string {
	maxContentWidth := v.width - 20
	if maxContentWidth < 20 {
		maxContentWidth = 20
	}

	if selected {
		// Plain text avoids inner ANSI resets breaking the selection background
		check := "○"
		if completed {
			check = "✓"
		}
		content := truncate(task.Content, maxContentWidth)
		if v.searchQuery != "" {
			content = highlightMatchPlain(content, v.searchQuery)
		}
		var parts []string
		parts = append(parts, check, content)
		if task.Due != nil {
			dueText := formatDue(task.Due)
			if dueText != "" {
				if task.Due.IsRecurring {
					dueText += " ↻"
				}
				parts = append(parts, dueText)
			}
		}
		if task.Priority > 0 && task.Priority < 4 {
			parts = append(parts, priorityLabel(task.Priority))
		}
		if len(task.Labels) > 0 {
			lbls := make([]string, len(task.Labels))
			for i, l := range task.Labels {
				lbls[i] = "@" + l
			}
			parts = append(parts, strings.Join(lbls, " "))
		}
		return lipgloss.NewStyle().
			Background(colorBgHL).
			Foreground(colorBright).
			Bold(true).
			Width(v.width).
			Render("  " + strings.Join(parts, "  "))
	}

	// Non-selected: full styled rendering
	var parts []string
	parts = append(parts, styledCheckbox(completed, task.Priority))

	content := truncate(task.Content, maxContentWidth)
	if completed {
		if v.searchQuery != "" {
			content = highlightMatch(content, v.searchQuery, taskCompletedStyle, searchMatchStyle)
		} else {
			content = taskCompletedStyle.Render(content)
		}
	} else {
		if v.searchQuery != "" {
			content = highlightMatch(content, v.searchQuery, taskContentStyle, searchMatchStyle)
		} else {
			content = taskContentStyle.Render(content)
		}
	}
	parts = append(parts, content)

	if task.Due != nil {
		dueText := formatDue(task.Due)
		if dueText != "" {
			if isOverdue(task.Due) {
				dueText = dueOverdueStyle.Render(dueText)
			} else if isDueToday(task.Due) {
				dueText = dueTodayStyle.Render(dueText)
			} else {
				dueText = dueUpcomingStyle.Render(dueText)
			}
			if task.Due.IsRecurring {
				dueText += " " + recurringStyle.Render("↻")
			}
			parts = append(parts, dueText)
		}
	}

	if task.Priority > 0 && task.Priority < 4 {
		pl := priorityLabel(task.Priority)
		parts = append(parts, priorityStyle(task.Priority).Render(pl))
	}

	if len(task.Labels) > 0 {
		lbls := make([]string, len(task.Labels))
		for i, l := range task.Labels {
			lbls[i] = "@" + l
		}
		parts = append(parts, labelStyle.Render(strings.Join(lbls, " ")))
	}

	return "  " + strings.Join(parts, "  ")
}

func (v TasksView) renderDialog() string {
	switch v.mode {
	case "quick-add":
		return dialogStyle.Width(v.width - 4).Render(
			dialogTitleStyle.Render("Quick Add") + "\n" +
				inputLabelStyle.Render("Supports: dates, #project, @label, p1-p4, //description") + "\n" +
				v.quickInput.View(),
		)
	case "edit":
		return dialogStyle.Width(v.width - 4).Render(
			dialogTitleStyle.Render("Edit Task") + "\n" +
				v.editInput.View(),
		)
	case "due":
		return dialogStyle.Width(v.width - 4).Render(
			dialogTitleStyle.Render("Set Due Date") + "\n" +
				inputLabelStyle.Render("e.g. today, tomorrow, next monday, every friday") + "\n" +
				v.dueInput.View(),
		)
	case "delete":
		task := v.selectedTask()
		name := ""
		if task != nil {
			name = task.Content
		}
		return dialogStyle.Width(v.width - 4).Render(
			dialogTitleStyle.Render("Delete Task?") + "\n" +
				taskContentStyle.Render("\""+truncate(name, 60)+"\"") + "\n\n" +
				footerKeyStyle.Render("y") + " confirm  " +
				footerKeyStyle.Render("n") + " cancel",
		)
	}
	return ""
}

func (v *TasksView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.editInput.Width = width - 8
	v.dueInput.Width = width - 8
	v.quickInput.Width = width - 8
	v.searchInput.Width = 30
}

func (v *TasksView) SetFocused(focused bool) {
	v.focused = focused
}

func (v TasksView) handlesInput() bool {
	return v.mode != "" || v.searchMode
}

// SearchPanel returns the floating search panel content, or "" if inactive.
func (v TasksView) SearchPanel() string {
	panelWidth := 40
	if v.searchMode {
		return searchPanelStyle.Width(panelWidth).Render("/ " + v.searchInput.View())
	}
	if v.searchQuery != "" {
		matchInfo := ""
		if len(v.matchIndices) > 0 {
			matchInfo = fmt.Sprintf("  %d/%d", v.currentMatch+1, len(v.matchIndices))
		} else {
			matchInfo = "  (no matches)"
		}
		return searchPanelStyle.Width(panelWidth).Render(
			footerKeyStyle.Render("/") + " " + v.searchQuery + matchInfo)
	}
	return ""
}

// rebuildItems creates the flat display list from sections and tasks
func (v *TasksView) rebuildItems() {
	v.items = nil

	// Group tasks by section
	sectionTasks := make(map[string][]Task)
	var noSection []Task

	for i := range v.tasks {
		t := v.tasks[i]
		if t.SectionID != "" {
			sectionTasks[t.SectionID] = append(sectionTasks[t.SectionID], t)
		} else {
			noSection = append(noSection, t)
		}
	}

	// Add unsectioned tasks first
	for i := range noSection {
		v.items = append(v.items, displayItem{task: &noSection[i]})
	}

	// Add sections with their tasks
	for i := range v.sections {
		sec := v.sections[i]
		tasks := sectionTasks[sec.ID]
		if len(tasks) == 0 && len(v.sections) > 0 {
			// Still show empty sections
		}
		v.items = append(v.items, displayItem{isSection: true, section: &sec})
		for j := range tasks {
			v.items = append(v.items, displayItem{task: &tasks[j]})
		}
	}

	// Add completed tasks at the bottom
	if len(v.completedTasks) > 0 {
		completedSection := Section{Name: "Completed"}
		v.items = append(v.items, displayItem{isSection: true, section: &completedSection})
		for i := range v.completedTasks {
			v.items = append(v.items, displayItem{task: &v.completedTasks[i], completed: true})
		}
	}

	// Jump to task if requested (from global search navigation)
	if v.jumpToTaskID != "" {
		for i, item := range v.items {
			if item.task != nil && item.task.ID == v.jumpToTaskID {
				v.cursor = i
				v.ensureVisible()
				break
			}
		}
		v.jumpToTaskID = ""
	}
}

func (v *TasksView) clampCursor() {
	if v.cursor >= len(v.items) {
		v.cursor = len(v.items) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	// Skip section headers
	if v.cursor < len(v.items) && v.items[v.cursor].isSection {
		v.skipToNextTask(1)
	}
}

func (v *TasksView) moveDown() {
	if v.cursor < len(v.items)-1 {
		v.cursor++
		if v.cursor < len(v.items) && v.items[v.cursor].isSection {
			if v.cursor < len(v.items)-1 {
				v.cursor++
			}
		}
	}
}

func (v *TasksView) moveUp() {
	if v.cursor > 0 {
		v.cursor--
		if v.cursor >= 0 && v.items[v.cursor].isSection {
			if v.cursor > 0 {
				v.cursor--
			}
		}
	}
}

func (v *TasksView) skipToNextTask(dir int) {
	for v.cursor >= 0 && v.cursor < len(v.items) && v.items[v.cursor].isSection {
		v.cursor += dir
	}
	v.clampCursorSimple()
}

func (v *TasksView) ensureVisible() {
	visibleHeight := v.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	}
	if v.cursor >= v.scrollOffset+visibleHeight {
		v.scrollOffset = v.cursor - visibleHeight + 1
	}
}

func (v *TasksView) clampCursorSimple() {
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= len(v.items) {
		v.cursor = len(v.items) - 1
	}
}

// HandleMouse processes mouse events for the tasks view.
func (v TasksView) HandleMouse(m tea.MouseEvent, yOffset int) (TasksView, tea.Cmd) {
	if m.Action == tea.MouseActionMotion || m.Action == tea.MouseActionRelease {
		return v, nil
	}

	// Scroll wheel
	if m.Button == tea.MouseButtonWheelDown {
		v.moveDown()
		v.ensureVisible()
		return v, nil
	}
	if m.Button == tea.MouseButtonWheelUp {
		v.moveUp()
		v.ensureVisible()
		return v, nil
	}

	// Left click
	if m.Button != tea.MouseButtonLeft {
		return v, nil
	}

	localY := m.Y - yOffset  // subtract header row
	itemOffset := localY - 2 // subtract title + padding rows
	clickedIndex := v.scrollOffset + itemOffset

	if clickedIndex >= 0 && clickedIndex < len(v.items) && !v.items[clickedIndex].isSection {
		v.cursor = clickedIndex
		v.ensureVisible()
	}

	return v, nil
}

func (v TasksView) selectedTask() *Task {
	if v.cursor >= 0 && v.cursor < len(v.items) && !v.items[v.cursor].isSection {
		return v.items[v.cursor].task
	}
	return nil
}

func (v TasksView) selectedItem() *displayItem {
	if v.cursor >= 0 && v.cursor < len(v.items) && !v.items[v.cursor].isSection {
		return &v.items[v.cursor]
	}
	return nil
}

