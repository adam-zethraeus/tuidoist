package main

import (
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
	mode       string // "", "add", "quick-add", "edit", "delete", "due"
	addInput   textinput.Model
	editInput  textinput.Model
	dueInput   textinput.Model
	quickInput textinput.Model

	// Scroll offset
	scrollOffset int
}

func NewTasksView(repo *Repository) TasksView {
	ai := textinput.New()
	ai.Placeholder = "Task name..."
	ai.CharLimit = 500

	ei := textinput.New()
	ei.Placeholder = "Edit task..."
	ei.CharLimit = 500

	di := textinput.New()
	di.Placeholder = "e.g. tomorrow, next monday, every day"
	di.CharLimit = 200

	qi := textinput.New()
	qi.Placeholder = "Buy milk tomorrow #Work @urgent p1"
	qi.CharLimit = 500

	return TasksView{
		repo:       repo,
		addInput:   ai,
		editInput:  ei,
		dueInput:   di,
		quickInput: qi,
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
	v.tasks = nil
	v.sections = nil
	v.items = nil

	return v, tea.Batch(
		v.repo.FetchTasks(projectID),
		v.repo.FetchSections(projectID),
	)
}

func (v TasksView) Update(msg tea.Msg) (TasksView, tea.Cmd) {
	switch msg := msg.(type) {
	case cachedTasksMsg:
		v.tasks = msg.tasks
		v.rebuildItems()
		return v, v.repo.RefreshTasks(v.projectID)

	case cachedSectionsMsg:
		v.sections = msg.sections
		v.rebuildItems()
		return v, v.repo.RefreshSections(v.projectID)

	case tasksMsg:
		if msg.err != nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Failed to load tasks: " + msg.err.Error(), isError: true}
			}
		}
		v.tasks = msg.tasks
		v.rebuildItems()
		v.clampCursor()
		return v, nil

	case sectionsMsg:
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
		// Remove completed task from list
		for i, t := range v.tasks {
			if t.ID == msg.taskID {
				v.tasks = append(v.tasks[:i], v.tasks[i+1:]...)
				break
			}
		}
		v.rebuildItems()
		v.clampCursor()
		return v, func() tea.Msg {
			return toastMsg{text: "Task completed", isError: false}
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
		if !v.focused {
			return v, nil
		}
		return v.handleKey(msg)
	}

	// Update text inputs if in a dialog mode
	if v.mode == "add" {
		var cmd tea.Cmd
		v.addInput, cmd = v.addInput.Update(msg)
		return v, cmd
	}
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

	return v, nil
}

func (v TasksView) handleKey(msg tea.KeyMsg) (TasksView, tea.Cmd) {
	key := msg.String()

	// Dialog mode key handling
	switch v.mode {
	case "add":
		switch key {
		case "enter":
			content := strings.TrimSpace(v.addInput.Value())
			if content == "" {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			return v, v.repo.CreateTask(createTaskRequest{
				Content:   content,
				ProjectID: v.projectID,
			})
		case "esc":
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.addInput, cmd = v.addInput.Update(msg)
		return v, cmd

	case "quick-add":
		switch key {
		case "enter":
			text := strings.TrimSpace(v.quickInput.Value())
			if text == "" {
				v.mode = ""
				return v, nil
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
		task := v.selectedTask()
		if task != nil {
			return v, v.repo.CloseTask(task.ID)
		}
	case "a":
		v.mode = "add"
		v.addInput.Reset()
		v.addInput.Focus()
		return v, textinput.Blink
	case "A":
		v.mode = "quick-add"
		v.quickInput.Reset()
		v.quickInput.Focus()
		return v, textinput.Blink
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
	if len(v.items) == 0 && len(v.tasks) == 0 {
		content := emptyStyle.Render("No tasks - press 'a' to add one")
		if v.mode != "" {
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

		line := v.renderTask(task, selected)
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Render dialog overlay if active
	if v.mode != "" {
		b.WriteString("\n\n")
		b.WriteString(v.renderDialog())
	}

	return b.String()
}

func (v TasksView) renderTask(task *Task, selected bool) string {
	var parts []string

	// Checkbox
	parts = append(parts, styledCheckbox(false, task.Priority))

	// Content
	maxContentWidth := v.width - 20
	if maxContentWidth < 20 {
		maxContentWidth = 20
	}
	content := truncate(task.Content, maxContentWidth)
	if selected {
		content = lipgloss.NewStyle().Foreground(colorBright).Bold(true).Render(content)
	} else {
		content = taskContentStyle.Render(content)
	}
	parts = append(parts, content)

	// Due date
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

	// Priority (only show for p1-p3)
	if task.Priority > 0 && task.Priority < 4 {
		pl := priorityLabel(task.Priority)
		parts = append(parts, priorityStyle(task.Priority).Render(pl))
	}

	// Labels
	if len(task.Labels) > 0 {
		lbls := make([]string, len(task.Labels))
		for i, l := range task.Labels {
			lbls[i] = "@" + l
		}
		parts = append(parts, labelStyle.Render(strings.Join(lbls, " ")))
	}

	line := "  " + strings.Join(parts, "  ")

	if selected {
		return lipgloss.NewStyle().
			Background(colorBgHL).
			Width(v.width).
			Render(line)
	}
	return line
}

func (v TasksView) renderDialog() string {
	switch v.mode {
	case "add":
		return dialogStyle.Width(v.width - 4).Render(
			dialogTitleStyle.Render("Add Task") + "\n" +
				v.addInput.View(),
		)
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
	v.addInput.Width = width - 8
	v.editInput.Width = width - 8
	v.dueInput.Width = width - 8
	v.quickInput.Width = width - 8
}

func (v *TasksView) SetFocused(focused bool) {
	v.focused = focused
}

func (v TasksView) handlesInput() bool {
	return v.mode != ""
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

func (v TasksView) selectedTask() *Task {
	if v.cursor >= 0 && v.cursor < len(v.items) && !v.items[v.cursor].isSection {
		return v.items[v.cursor].task
	}
	return nil
}

