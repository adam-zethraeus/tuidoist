package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TodayView shows tasks due today and overdue, plus upcoming high-priority tasks.
type TodayView struct {
	repo         *Repository
	items        []displayItem
	tasks        []Task
	projectNames map[string]string
	cursor       int
	scrollOffset int
	width        int
	height       int
	focused      bool

	// Page-local search
	searchMode   bool
	searchInput  textinput.Model
	searchQuery  string
	matchIndices []int
	currentMatch int

	mode          string // "", "due", "deadline"
	dueInput      textinput.Model
	deadlineInput textinput.Model
}

func NewTodayView(repo *Repository) TodayView {
	si := textinput.New()
	si.Placeholder = "Search..."
	si.CharLimit = 200

	di := textinput.New()
	di.Placeholder = "e.g. today, tomorrow, next monday, every day"
	di.CharLimit = 200

	dli := textinput.New()
	dli.Placeholder = "YYYY-MM-DD (empty to clear)"
	dli.CharLimit = 32

	return TodayView{
		repo:          repo,
		searchInput:   si,
		dueInput:      di,
		deadlineInput: dli,
	}
}

func (v *TodayView) Refresh() {
	allTasks := v.repo.GetAllCachedTasks()
	v.projectNames = v.repo.GetProjectNameMap()
	v.searchMode = false
	v.searchQuery = ""
	v.matchIndices = nil
	v.currentMatch = 0

	var overdue, today, upcoming []Task

	for _, task := range allTasks {
		hasDue := task.Due != nil && task.Due.Date != ""
		hasDeadline := task.Deadline != nil && task.Deadline.Date != ""
		if !hasDue && !hasDeadline {
			continue
		}
		if isTaskOverdue(&task) {
			overdue = append(overdue, task)
		} else if isDueToday(task.Due) || isDeadlineToday(task.Deadline) {
			today = append(today, task)
		} else {
			// Future — candidate for Up Next
			upcoming = append(upcoming, task)
		}
	}

	// Sort overdue: date ascending, then priority descending
	sort.Slice(overdue, func(i, j int) bool {
		di, dj := todaySortDateKey(overdue[i]), todaySortDateKey(overdue[j])
		if di != dj {
			return di < dj
		}
		return overdue[i].Priority < overdue[j].Priority // lower number = higher priority in Todoist
	})

	// Sort today: priority descending (lower number = higher priority), then alphabetical
	sort.Slice(today, func(i, j int) bool {
		if today[i].Priority != today[j].Priority {
			return today[i].Priority < today[j].Priority
		}
		return today[i].Content < today[j].Content
	})

	// Sort upcoming: date ascending, then priority descending
	sort.Slice(upcoming, func(i, j int) bool {
		di, dj := todaySortDateKey(upcoming[i]), todaySortDateKey(upcoming[j])
		if di != dj {
			return di < dj
		}
		return upcoming[i].Priority < upcoming[j].Priority
	})
	if len(upcoming) > 10 {
		upcoming = upcoming[:10]
	}

	// Build items
	v.items = nil
	v.tasks = nil

	if len(overdue) > 0 {
		overdueSection := Section{Name: "Overdue"}
		v.items = append(v.items, displayItem{isSection: true, section: &overdueSection})
		for i := range overdue {
			v.items = append(v.items, displayItem{task: &overdue[i]})
			v.tasks = append(v.tasks, overdue[i])
		}
	}

	if len(today) > 0 {
		todaySection := Section{Name: "Today"}
		v.items = append(v.items, displayItem{isSection: true, section: &todaySection})
		for i := range today {
			v.items = append(v.items, displayItem{task: &today[i]})
			v.tasks = append(v.tasks, today[i])
		}
	}

	if len(upcoming) > 0 {
		upNextSection := Section{Name: "Up Next"}
		v.items = append(v.items, displayItem{isSection: true, section: &upNextSection})
		for i := range upcoming {
			v.items = append(v.items, displayItem{task: &upcoming[i]})
			v.tasks = append(v.tasks, upcoming[i])
		}
	}

	v.clampCursor()
}

func (v TodayView) Update(msg tea.Msg) (TodayView, tea.Cmd) {
	switch msg := msg.(type) {
	case taskUpdatedMsg:
		if msg.err == nil {
			v.Refresh()
			return v, func() tea.Msg {
				return toastMsg{text: "Task updated", isError: false}
			}
		}
		return v, func() tea.Msg {
			return toastMsg{text: "Failed to update task: " + msg.err.Error(), isError: true}
		}

	case taskClosedMsg:
		if msg.err == nil {
			v.Refresh()
			return v, func() tea.Msg {
				return toastMsg{text: "Task completed", isError: false}
			}
		}
		return v, func() tea.Msg {
			return toastMsg{text: "Failed to complete task: " + msg.err.Error(), isError: true}
		}

	case taskReopenedMsg:
		if msg.err == nil {
			v.Refresh()
			return v, func() tea.Msg {
				return toastMsg{text: "Task reopened", isError: false}
			}
		}
		return v, func() tea.Msg {
			return toastMsg{text: "Failed to reopen task: " + msg.err.Error(), isError: true}
		}

	case tea.KeyMsg:
		if !v.focused {
			return v, nil
		}
		return v.handleKey(msg)
	}

	// Update text input for non-key messages (e.g. cursor blink)
	if v.searchMode {
		var cmd tea.Cmd
		v.searchInput, cmd = v.searchInput.Update(msg)
		return v, cmd
	}
	if v.mode == "due" {
		var cmd tea.Cmd
		v.dueInput, cmd = v.dueInput.Update(msg)
		return v, cmd
	}
	if v.mode == "deadline" {
		var cmd tea.Cmd
		v.deadlineInput, cmd = v.deadlineInput.Update(msg)
		return v, cmd
	}

	return v, nil
}

func (v TodayView) handleKey(msg tea.KeyMsg) (TodayView, tea.Cmd) {
	switch v.mode {
	case "due":
		switch ResolveAction(ContextMainTodayDialog, msg.String()) {
		case ActionConfirm:
			dueStr := strings.TrimSpace(v.dueInput.Value())
			item := v.selectedItem()
			if item == nil || item.task == nil {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			if dueStr == "" {
				empty := ""
				return v, v.repo.UpdateTask(item.task.ID, updateTaskRequest{DueString: &empty})
			}
			return v, v.repo.UpdateTask(item.task.ID, updateTaskRequest{DueString: &dueStr})
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.dueInput, cmd = v.dueInput.Update(msg)
		return v, cmd

	case "deadline":
		switch ResolveAction(ContextMainTodayDialog, msg.String()) {
		case ActionConfirm:
			deadlineStr := strings.TrimSpace(v.deadlineInput.Value())
			item := v.selectedItem()
			if item == nil || item.task == nil {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			if deadlineStr == "" {
				return v, v.repo.UpdateTask(item.task.ID, updateTaskRequest{ClearDeadline: true})
			}
			return v, v.repo.UpdateTask(item.task.ID, updateTaskRequest{DeadlineDate: &deadlineStr})
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.deadlineInput, cmd = v.deadlineInput.Update(msg)
		return v, cmd
	}

	if v.searchMode {
		switch ResolveAction(ContextMainTodaySearch, msg.String()) {
		case ActionConfirm:
			v.searchMode = false
			v.searchQuery = strings.TrimSpace(v.searchInput.Value())
			v.matchIndices = findMatchIndices(v.items, v.searchQuery)
			if len(v.matchIndices) > 0 {
				v.cursor = v.matchIndices[0]
				v.currentMatch = 0
				v.ensureVisible()
			}
			return v, nil
		case ActionCancel:
			v.searchMode = false
			return v, nil
		}
		var cmd tea.Cmd
		v.searchInput, cmd = v.searchInput.Update(msg)
		v.matchIndices = findMatchIndices(v.items, strings.TrimSpace(v.searchInput.Value()))
		return v, cmd
	}

	action := ResolveAction(ContextMainToday, msg.String())
	if msg.String() == "n" && v.searchQuery != "" {
		action = ActionSearchNext
	}
	if msg.String() == "N" && v.searchQuery != "" {
		action = ActionSearchPrev
	}

	switch action {
	case ActionSearchLocal:
		v.searchMode = true
		v.searchInput.Reset()
		v.searchInput.Focus()
		return v, textinput.Blink
	case ActionSearchNext:
		if v.searchQuery != "" && len(v.matchIndices) > 0 {
			idx, num := nextMatchIndex(v.matchIndices, v.cursor)
			if idx >= 0 {
				v.cursor = idx
				v.currentMatch = num
				v.ensureVisible()
			}
		}
		return v, nil
	case ActionSearchPrev:
		if v.searchQuery != "" && len(v.matchIndices) > 0 {
			idx, num := prevMatchIndex(v.matchIndices, v.cursor)
			if idx >= 0 {
				v.cursor = idx
				v.currentMatch = num
				v.ensureVisible()
			}
		}
		return v, nil
	case ActionClearSearch:
		if v.searchQuery != "" {
			v.searchQuery = ""
			v.matchIndices = nil
			v.currentMatch = 0
		}
		return v, nil
	case ActionNavDown:
		v.moveDown()
		v.ensureVisible()
		return v, nil
	case ActionNavUp:
		v.moveUp()
		v.ensureVisible()
		return v, nil
	case ActionNavTop:
		listJumpTop(&v.cursor, &v.scrollOffset, len(v.items), func(idx int) bool { return v.items[idx].isSection })
		return v, nil
	case ActionNavBottom:
		listJumpBottom(&v.cursor, len(v.items), func(idx int) bool { return v.items[idx].isSection })
		v.ensureVisible()
		return v, nil
	case ActionToggleDone:
		item := v.selectedItem()
		if item != nil && item.task != nil {
			if item.completed {
				return v, v.repo.ReopenTask(*item.task)
			}
			return v, v.repo.CloseTask(item.task.ID)
		}
	case ActionSetDue:
		item := v.selectedItem()
		if item != nil && item.task != nil {
			v.mode = "due"
			v.dueInput.Reset()
			if item.task.Due != nil {
				if item.task.Due.String != "" {
					v.dueInput.SetValue(item.task.Due.String)
				} else {
					v.dueInput.SetValue(item.task.Due.Date)
				}
			}
			v.dueInput.Focus()
			return v, textinput.Blink
		}
	case ActionSetDeadline:
		item := v.selectedItem()
		if item != nil && item.task != nil {
			v.mode = "deadline"
			v.deadlineInput.Reset()
			if item.task.Deadline != nil {
				v.deadlineInput.SetValue(item.task.Deadline.Date)
			}
			v.deadlineInput.Focus()
			return v, textinput.Blink
		}
	case ActionClearDates:
		item := v.selectedItem()
		if item == nil || item.task == nil {
			return v, nil
		}
		if !isTaskOverdue(item.task) {
			return v, func() tea.Msg {
				return toastMsg{text: "Selected task is not overdue", isError: true}
			}
		}
		empty := ""
		return v, v.repo.UpdateTask(item.task.ID, updateTaskRequest{
			DueString:     &empty,
			ClearDeadline: true,
		})
	}

	return v, nil
}

func (v TodayView) View() string {
	if len(v.items) == 0 {
		return lipgloss.NewStyle().
			Foreground(colorBright).
			Bold(true).
			Padding(0, 0, 1, 0).
			Render("Today") + "\n" +
			emptyStyle.Render("Nothing due today")
	}

	var b strings.Builder
	mutationStatus := v.repo.TaskMutationStatusMap()
	assigneeNames := v.repo.GetAssigneeNameMap()

	title := lipgloss.NewStyle().
		Foreground(colorBright).
		Bold(true).
		Padding(0, 0, 1, 0).
		Render("Today")
	b.WriteString(title)
	b.WriteString("\n")

	visibleHeight := v.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	end := v.scrollOffset + visibleHeight
	if end > len(v.items) {
		end = len(v.items)
	}

	// Determine if an item is in the "Up Next" section
	inUpNext := false

	// Pre-scan to find Up Next section boundary for fading
	upNextStart := -1
	for i, item := range v.items {
		if item.isSection && item.section != nil && item.section.Name == "Up Next" {
			upNextStart = i
			break
		}
	}

	for i := v.scrollOffset; i < end; i++ {
		item := v.items[i]
		inUpNext = upNextStart >= 0 && i >= upNextStart

		if item.isSection {
			name := item.section.Name
			b.WriteString(sectionStyle.Render("━━ " + name))
			b.WriteString("\n")
			continue
		}

		task := item.task
		selected := i == v.cursor && v.focused

		line := v.renderTask(task, selected, inUpNext, mutationStatus[task.ID], assigneeNames)
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if v.mode != "" {
		b.WriteString("\n\n")
		b.WriteString(v.renderDialog())
	}

	return b.String()
}

func (v TodayView) renderTask(task *Task, selected bool, faded bool, syncStatus MutationStatus, assigneeNames map[string]string) string {
	maxContentWidth := v.width - 44
	if maxContentWidth < 20 {
		maxContentWidth = 20
	}

	projectName := v.projectNames[task.ProjectID]

	if selected {
		check := "○"
		content := truncate(task.Content, maxContentWidth)
		if v.searchQuery != "" {
			content = highlightMatchPlain(content, v.searchQuery)
		}
		var parts []string
		parts = append(parts, check, content)
		if projectName != "" {
			parts = append(parts, projectName)
		}
		if task.Due != nil {
			dueText := formatDue(task.Due)
			if dueText != "" {
				if task.Due.IsRecurring {
					dueText += " ↻"
				}
				parts = append(parts, dueText)
			}
		}
		if deadlineText := formatDeadline(task.Deadline); deadlineText != "" {
			parts = append(parts, deadlineText)
		}
		if task.Priority > 0 && task.Priority < 4 {
			parts = append(parts, priorityLabel(task.Priority))
		}
		if assignee := formatAssignee(task, assigneeNames); assignee != "" {
			parts = append(parts, assignee)
		}
		if badge := mutationBadgePlain(syncStatus); badge != "" {
			parts = append(parts, badge)
		}
		return lipgloss.NewStyle().
			Background(colorBgHL).
			Foreground(colorBright).
			Bold(true).
			Width(v.width).
			Render("  " + strings.Join(parts, "  "))
	}

	// Non-selected
	var parts []string
	parts = append(parts, styledCheckbox(false, task.Priority))

	content := truncate(task.Content, maxContentWidth)
	if v.searchQuery != "" {
		baseStyle := taskContentStyle
		if faded {
			baseStyle = todayUpNextStyle
		}
		content = highlightMatch(content, v.searchQuery, baseStyle, searchMatchStyle)
	} else if faded {
		content = todayUpNextStyle.Render(content)
	} else {
		content = taskContentStyle.Render(content)
	}
	parts = append(parts, content)

	if projectName != "" {
		parts = append(parts, todayProjectTagStyle.Render(projectName))
	}

	if task.Due != nil {
		dueText := formatDue(task.Due)
		if dueText != "" {
			if faded {
				dueText = todayUpNextStyle.Render(dueText)
			} else if isOverdue(task.Due) {
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

	if deadlineText := formatDeadline(task.Deadline); deadlineText != "" {
		if faded {
			deadlineText = todayUpNextStyle.Render(deadlineText)
		} else if isDeadlineOverdue(task.Deadline) {
			deadlineText = dueOverdueStyle.Render(deadlineText)
		} else if isDeadlineToday(task.Deadline) {
			deadlineText = dueTodayStyle.Render(deadlineText)
		} else {
			deadlineText = deadlineStyle.Render(deadlineText)
		}
		parts = append(parts, deadlineText)
	}

	if task.Priority > 0 && task.Priority < 4 {
		pl := priorityLabel(task.Priority)
		if faded {
			parts = append(parts, todayUpNextStyle.Render(pl))
		} else {
			parts = append(parts, priorityStyle(task.Priority).Render(pl))
		}
	}

	if assignee := formatAssignee(task, assigneeNames); assignee != "" {
		if faded {
			parts = append(parts, todayUpNextStyle.Render(assignee))
		} else {
			parts = append(parts, assigneeStyle.Render(assignee))
		}
	}

	if badge := mutationBadgeStyled(syncStatus); badge != "" {
		parts = append(parts, badge)
	}

	return "  " + strings.Join(parts, "  ")
}

func (v *TodayView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.searchInput.Width = 30
	v.dueInput.Width = width - 8
	v.deadlineInput.Width = width - 8
}

func (v *TodayView) SetFocused(focused bool) {
	v.focused = focused
}

func (v TodayView) handlesInput() bool {
	return v.searchMode || v.mode != ""
}

func (v TodayView) IsSearchMode() bool {
	return v.searchMode
}

func (v TodayView) HasSearchQuery() bool {
	return v.searchQuery != ""
}

func (v TodayView) renderDialog() string {
	switch v.mode {
	case "due":
		return dialogStyle.Width(v.width - 4).Render(
			dialogTitleStyle.Render("Set Due Date") + "\n" +
				inputLabelStyle.Render("e.g. today, tomorrow, next monday, every friday (empty to clear)") + "\n" +
				v.dueInput.View(),
		)
	case "deadline":
		return dialogStyle.Width(v.width - 4).Render(
			dialogTitleStyle.Render("Set Deadline") + "\n" +
				inputLabelStyle.Render("YYYY-MM-DD (empty to clear)") + "\n" +
				v.deadlineInput.View(),
		)
	default:
		return ""
	}
}

func todaySortDateKey(task Task) string {
	if task.Due != nil && task.Due.Date != "" {
		return task.Due.Date
	}
	if task.Deadline != nil {
		return task.Deadline.Date
	}
	return ""
}

// SearchPanel returns the floating search panel content, or "" if inactive.
func (v TodayView) SearchPanel() string {
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

func (v TodayView) HandleMouse(m tea.MouseEvent, yOffset int) (TodayView, tea.Cmd) {
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

	localY := m.Y - yOffset
	itemOffset := localY - 2
	clickedIndex := v.scrollOffset + itemOffset

	if clickedIndex >= 0 && clickedIndex < len(v.items) && !v.items[clickedIndex].isSection {
		v.cursor = clickedIndex
		v.ensureVisible()
	}

	return v, nil
}

func (v TodayView) selectedItem() *displayItem {
	if v.cursor >= 0 && v.cursor < len(v.items) && !v.items[v.cursor].isSection {
		return &v.items[v.cursor]
	}
	return nil
}

func (v *TodayView) clampCursor() {
	listClampCursor(&v.cursor, len(v.items), func(idx int) bool { return v.items[idx].isSection })
}

func (v *TodayView) moveDown() {
	listMoveDown(&v.cursor, len(v.items), func(idx int) bool { return v.items[idx].isSection })
}

func (v *TodayView) moveUp() {
	listMoveUp(&v.cursor, len(v.items), func(idx int) bool { return v.items[idx].isSection })
}

func (v *TodayView) skipToNextTask(dir int) {
	listSkip(&v.cursor, len(v.items), dir, func(idx int) bool { return v.items[idx].isSection })
}

func (v *TodayView) ensureVisible() {
	visibleHeight := v.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	listEnsureVisible(v.cursor, &v.scrollOffset, visibleHeight)
}
