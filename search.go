package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Shared highlight/match helpers ---

// highlightMatch highlights the first occurrence of query in text using matchStyle,
// rendering the rest with baseStyle. Case-insensitive.
func highlightMatch(text, query string, baseStyle, matchStyle lipgloss.Style) string {
	if query == "" {
		return baseStyle.Render(text)
	}
	lower := strings.ToLower(text)
	lowerQ := strings.ToLower(query)
	idx := strings.Index(lower, lowerQ)
	if idx < 0 {
		return baseStyle.Render(text)
	}
	before := text[:idx]
	match := text[idx : idx+len(query)]
	after := text[idx+len(query):]
	return baseStyle.Render(before) + matchStyle.Render(match) + baseStyle.Render(after)
}

// highlightMatchPlain wraps matched text in brackets for selected rows
// (avoids ANSI reset issues with background colors).
func highlightMatchPlain(text, query string) string {
	if query == "" {
		return text
	}
	lower := strings.ToLower(text)
	lowerQ := strings.ToLower(query)
	idx := strings.Index(lower, lowerQ)
	if idx < 0 {
		return text
	}
	before := text[:idx]
	match := text[idx : idx+len(query)]
	after := text[idx+len(query):]
	return before + "[" + match + "]" + after
}

// containsMatch returns true if text contains query (case-insensitive).
func containsMatch(text, query string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(query))
}

// findMatchIndices returns indices of items whose task content matches the query.
func findMatchIndices(items []displayItem, query string) []int {
	if query == "" {
		return nil
	}
	var matches []int
	for i, item := range items {
		if item.task != nil && containsMatch(item.task.Content, query) {
			matches = append(matches, i)
		}
	}
	return matches
}

// nextMatchIndex returns the next match index after cursor, wrapping around.
// Returns (matchIdx, matchNumber) where matchNumber is 0-based position in matches.
func nextMatchIndex(matches []int, cursor int) (int, int) {
	if len(matches) == 0 {
		return -1, -1
	}
	for i, idx := range matches {
		if idx > cursor {
			return idx, i
		}
	}
	return matches[0], 0 // wrap
}

// prevMatchIndex returns the previous match index before cursor, wrapping around.
func prevMatchIndex(matches []int, cursor int) (int, int) {
	if len(matches) == 0 {
		return -1, -1
	}
	for i := len(matches) - 1; i >= 0; i-- {
		if matches[i] < cursor {
			return matches[i], i
		}
	}
	return matches[len(matches)-1], len(matches) - 1 // wrap
}

func mutationBadgePlain(status MutationStatus) string {
	switch status {
	case MutationPending, MutationFlushing:
		return "[sync]"
	case MutationConflicted:
		return "[conflict]"
	default:
		return ""
	}
}

func mutationBadgeStyled(status MutationStatus) string {
	switch status {
	case MutationPending, MutationFlushing:
		return lipgloss.NewStyle().Foreground(colorBlue).Render("sync")
	case MutationConflicted:
		return lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("conflict")
	default:
		return ""
	}
}

// --- Overlay helpers ---

// ansiTruncate truncates s to at most maxWidth visible characters,
// preserving ANSI escape sequences.
func ansiTruncate(s string, maxWidth int) string {
	var buf strings.Builder
	width := 0
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if width >= maxWidth {
			break
		}
		// ANSI escape sequence: ESC [
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			j := i + 2
			for j < len(runes) && !((runes[j] >= 'A' && runes[j] <= 'Z') || (runes[j] >= 'a' && runes[j] <= 'z')) {
				j++
			}
			if j < len(runes) {
				j++ // include the terminator
			}
			buf.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		buf.WriteRune(runes[i])
		width++
		i++
	}
	return buf.String()
}

// placeOverlay renders fg on top of bg at position (x, y).
func placeOverlay(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		bgLine := bgLines[row]
		bgW := lipgloss.Width(bgLine)

		left := ansiTruncate(bgLine, x)
		leftW := lipgloss.Width(left)
		if leftW < x {
			left += strings.Repeat(" ", x-leftW)
		}

		fgW := lipgloss.Width(fgLine)
		right := ""
		if x+fgW < bgW {
			right = strings.Repeat(" ", bgW-x-fgW)
		}

		bgLines[row] = left + "\x1b[0m" + fgLine + "\x1b[0m" + right
	}

	return strings.Join(bgLines, "\n")
}

// --- Search result types ---

type searchResultKind int

const (
	searchResultTask searchResultKind = iota
	searchResultProject
)

type searchResult struct {
	kind        searchResultKind
	task        *Task
	project     *Project
	projectName string // for task results: the project name
}

// --- SearchView ---

type SearchView struct {
	input   textinput.Model
	results []searchResult
	cursor  int
	repo    *Repository
	width   int
	height  int
	active  bool

	// Cached data for filtering
	allTasks    []Task
	allProjects []Project
	projectMap  map[string]string // id -> name
}

func NewSearchView(repo *Repository) SearchView {
	ti := textinput.New()
	ti.Placeholder = "Search tasks and lists..."
	ti.CharLimit = 200

	return SearchView{
		input: ti,
		repo:  repo,
	}
}

func (v *SearchView) Open() {
	v.active = true
	v.cursor = 0
	v.input.Reset()
	v.input.Focus()

	// Load all data from cache
	v.allTasks = v.repo.GetAllCachedTasks()
	v.allProjects = v.repo.GetCachedProjects()
	v.projectMap = v.repo.GetProjectNameMap()

	v.filter()
}

func (v *SearchView) Close() {
	v.active = false
	v.input.Blur()
}

func (v *SearchView) IsActive() bool {
	return v.active
}

func (v *SearchView) filter() {
	query := strings.TrimSpace(v.input.Value())
	v.results = nil
	v.cursor = 0

	if query == "" {
		return
	}

	// Tasks (max 15): match on content or project name
	taskCount := 0
	for i := range v.allTasks {
		if taskCount >= 15 {
			break
		}
		t := &v.allTasks[i]
		pName := v.projectMap[t.ProjectID]
		if containsMatch(t.Content, query) || containsMatch(pName, query) {
			v.results = append(v.results, searchResult{
				kind:        searchResultTask,
				task:        t,
				projectName: pName,
			})
			taskCount++
		}
	}

	// Lists (max 5)
	projCount := 0
	for i := range v.allProjects {
		if projCount >= 5 {
			break
		}
		p := &v.allProjects[i]
		if containsMatch(p.Name, query) {
			v.results = append(v.results, searchResult{
				kind:    searchResultProject,
				project: p,
			})
			projCount++
		}
	}
}

func (v SearchView) Update(msg tea.Msg) (SearchView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch ResolveAction(ContextSearchOverlay, msg.String()) {
		case ActionCancel:
			v.Close()
			return v, nil
		case ActionSearchCreate:
			// Create a new task via quick-add with the search text
			text := strings.TrimSpace(v.input.Value())
			if text == "" {
				return v, nil
			}
			v.Close()
			return v, v.repo.QuickAdd(text, "")
		case ActionConfirm:
			// If no results, create a new task via quick-add
			if len(v.results) == 0 {
				text := strings.TrimSpace(v.input.Value())
				if text == "" {
					return v, nil
				}
				v.Close()
				return v, v.repo.QuickAdd(text, "")
			}
			r := v.results[v.cursor]
			v.Close()
			switch r.kind {
			case searchResultTask:
				return v, func() tea.Msg {
					return navigateToTaskMsg{
						projectID: r.task.ProjectID,
						taskID:    r.task.ID,
					}
				}
			case searchResultProject:
				return v, func() tea.Msg {
					return navigateToProjectMsg{projectID: r.project.ID}
				}
			}
			return v, nil
		case ActionNavUp:
			if v.cursor > 0 {
				v.cursor--
			}
			return v, nil
		case ActionNavDown:
			if v.cursor < len(v.results)-1 {
				v.cursor++
			}
			return v, nil
		}

		// Pass to text input
		var cmd tea.Cmd
		v.input, cmd = v.input.Update(msg)
		v.filter()
		return v, cmd
	}

	// Non-key messages (blink, etc.)
	var cmd tea.Cmd
	v.input, cmd = v.input.Update(msg)
	return v, cmd
}

func (v SearchView) View(width, height int) string {
	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true).
		Render("Search")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Input
	v.input.Width = width - 8
	b.WriteString("  " + v.input.View())
	b.WriteString("\n")

	query := strings.TrimSpace(v.input.Value())

	if query == "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorTextDim).
			Italic(true).
			Padding(0, 2).
			Render("Type to search tasks and lists"))
	} else if len(v.results) == 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorTextDim).
			Italic(true).
			Padding(0, 2).
			Render("No matches"))
	} else {
		b.WriteString("\n")
		maxVisible := height - 11
		if maxVisible < 1 {
			maxVisible = 1
		}

		// Scroll
		start := 0
		if v.cursor >= maxVisible {
			start = v.cursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(v.results) {
			end = len(v.results)
		}

		// Track section headers
		prevKind := searchResultKind(-1)
		assigneeNames := v.repo.GetAssigneeNameMap()

		for i := start; i < end; i++ {
			r := v.results[i]
			selected := i == v.cursor

			// Section headers on kind transitions
			if r.kind != prevKind {
				if prevKind >= 0 {
					b.WriteString("\n")
				}
				switch r.kind {
				case searchResultTask:
					b.WriteString("  " + sectionStyle.Render("Tasks"))
				case searchResultProject:
					b.WriteString("  " + sectionStyle.Render("Lists"))
				}
				b.WriteString("\n")
				prevKind = r.kind
			}

			var line string
			switch r.kind {
			case searchResultTask:
				content := truncate(r.task.Content, width-20)
				if selected {
					content = highlightMatchPlain(content, query)
					line = "  ○ " + content
					if r.projectName != "" {
						line += "  " + r.projectName
					}
					if r.task.Due != nil {
						due := formatDue(r.task.Due)
						if due != "" {
							line += "  " + due
						}
					}
					if deadline := formatDeadline(r.task.Deadline); deadline != "" {
						line += "  " + deadline
					}
					if assignee := formatAssignee(r.task, assigneeNames); assignee != "" {
						line += "  " + assignee
					}
					line = lipgloss.NewStyle().
						Background(colorBgHL).
						Foreground(colorBright).
						Bold(true).
						Width(width - 4).
						Render(line)
				} else {
					content = highlightMatch(content, query, taskContentStyle, searchMatchStyle)
					line = "  " + styledCheckbox(false, r.task.Priority) + "  " + content
					if r.projectName != "" {
						line += "  " + todayProjectTagStyle.Render(r.projectName)
					}
					if r.task.Due != nil {
						due := formatDue(r.task.Due)
						if due != "" {
							if isOverdue(r.task.Due) {
								line += "  " + dueOverdueStyle.Render(due)
							} else if isDueToday(r.task.Due) {
								line += "  " + dueTodayStyle.Render(due)
							} else {
								line += "  " + dueUpcomingStyle.Render(due)
							}
						}
					}
					if deadline := formatDeadline(r.task.Deadline); deadline != "" {
						if isDeadlineOverdue(r.task.Deadline) {
							line += "  " + dueOverdueStyle.Render(deadline)
						} else if isDeadlineToday(r.task.Deadline) {
							line += "  " + dueTodayStyle.Render(deadline)
						} else {
							line += "  " + deadlineStyle.Render(deadline)
						}
					}
					if assignee := formatAssignee(r.task, assigneeNames); assignee != "" {
						line += "  " + assigneeStyle.Render(assignee)
					}
				}

			case searchResultProject:
				name := truncate(r.project.Name, width-10)
				if selected {
					name = highlightMatchPlain(name, query)
					line = "  ● " + name
					line = lipgloss.NewStyle().
						Background(colorBgHL).
						Foreground(colorBright).
						Bold(true).
						Width(width - 4).
						Render(line)
				} else {
					dot := lipgloss.NewStyle().Foreground(projectColor(r.project.Color)).Render("●")
					name = highlightMatch(name, query, taskContentStyle, searchMatchStyle)
					line = "  " + dot + "  " + name
				}
			}

			b.WriteString(line)
			b.WriteString("\n")
		}

		b.WriteString(lipgloss.NewStyle().
			Foreground(colorTextDim).
			Padding(0, 2).
			Render(fmt.Sprintf("%d results", len(v.results))))
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(footerKeyStyle.Render("↑/↓") + " " + footerDescStyle.Render("navigate") + "  ")
	if len(v.results) > 0 {
		b.WriteString(footerKeyStyle.Render("enter") + " " + footerDescStyle.Render("go to") + "  ")
		b.WriteString(footerKeyStyle.Render("alt+enter") + " " + footerDescStyle.Render("new task") + "  ")
	} else if query != "" {
		b.WriteString(footerKeyStyle.Render("enter") + " " + footerDescStyle.Render("new task") + "  ")
	}
	b.WriteString(footerKeyStyle.Render("esc") + " " + footerDescStyle.Render("close"))

	return helpStyle.
		Width(width).
		Height(height).
		Render(b.String())
}
