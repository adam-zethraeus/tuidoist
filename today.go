package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

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
}

func NewTodayView(repo *Repository) TodayView {
	si := textinput.New()
	si.Placeholder = "Search..."
	si.CharLimit = 200

	return TodayView{repo: repo, searchInput: si}
}

func (v *TodayView) Refresh() {
	allTasks := v.repo.GetAllCachedTasks()
	v.projectNames = v.repo.GetProjectNameMap()
	v.searchMode = false
	v.searchQuery = ""
	v.matchIndices = nil
	v.currentMatch = 0

	var overdue, today, upcoming []Task
	now := time.Now()
	todayStr := now.Format("2006-01-02")

	for _, task := range allTasks {
		if task.Due == nil || task.Due.Date == "" {
			continue
		}
		dateStr := task.Due.Date
		if isOverdue(task.Due) {
			overdue = append(overdue, task)
		} else if strings.HasPrefix(dateStr, todayStr) {
			today = append(today, task)
		} else {
			// Future — candidate for Up Next
			upcoming = append(upcoming, task)
		}
	}

	// Sort overdue: date ascending, then priority descending
	sort.Slice(overdue, func(i, j int) bool {
		di, dj := overdue[i].Due.Date, overdue[j].Due.Date
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
		di, dj := upcoming[i].Due.Date, upcoming[j].Due.Date
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

	return v, nil
}

func (v TodayView) handleKey(msg tea.KeyMsg) (TodayView, tea.Cmd) {
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
		v.matchIndices = findMatchIndices(v.items, strings.TrimSpace(v.searchInput.Value()))
		return v, cmd
	}

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

		line := v.renderTask(task, selected, inUpNext)
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Search bar
	if v.searchMode {
		b.WriteString("\n")
		b.WriteString(searchInputStyle.Width(v.width - 4).Render("/ " + v.searchInput.View()))
	} else if v.searchQuery != "" {
		b.WriteString("\n")
		matchInfo := ""
		if len(v.matchIndices) > 0 {
			matchInfo = fmt.Sprintf("  (%d/%d)", v.currentMatch+1, len(v.matchIndices))
		} else {
			matchInfo = "  (no matches)"
		}
		b.WriteString(searchInputStyle.Width(v.width - 4).Render(
			footerKeyStyle.Render("/") + " " + v.searchQuery + matchInfo))
	}

	return b.String()
}

func (v TodayView) renderTask(task *Task, selected bool, faded bool) string {
	maxContentWidth := v.width - 30
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
		if task.Priority > 0 && task.Priority < 4 {
			parts = append(parts, priorityLabel(task.Priority))
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

	if task.Priority > 0 && task.Priority < 4 {
		pl := priorityLabel(task.Priority)
		if faded {
			parts = append(parts, todayUpNextStyle.Render(pl))
		} else {
			parts = append(parts, priorityStyle(task.Priority).Render(pl))
		}
	}

	return "  " + strings.Join(parts, "  ")
}

func (v *TodayView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.searchInput.Width = width - 12
}

func (v *TodayView) SetFocused(focused bool) {
	v.focused = focused
}

func (v TodayView) handlesInput() bool {
	return v.searchMode
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
	if v.cursor >= len(v.items) {
		v.cursor = len(v.items) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor < len(v.items) && v.items[v.cursor].isSection {
		v.skipToNextTask(1)
	}
}

func (v *TodayView) moveDown() {
	if v.cursor < len(v.items)-1 {
		v.cursor++
		if v.cursor < len(v.items) && v.items[v.cursor].isSection {
			if v.cursor < len(v.items)-1 {
				v.cursor++
			}
		}
	}
}

func (v *TodayView) moveUp() {
	if v.cursor > 0 {
		v.cursor--
		if v.cursor >= 0 && v.items[v.cursor].isSection {
			if v.cursor > 0 {
				v.cursor--
			}
		}
	}
}

func (v *TodayView) skipToNextTask(dir int) {
	for v.cursor >= 0 && v.cursor < len(v.items) && v.items[v.cursor].isSection {
		v.cursor += dir
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= len(v.items) {
		v.cursor = len(v.items) - 1
	}
}

func (v *TodayView) ensureVisible() {
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
