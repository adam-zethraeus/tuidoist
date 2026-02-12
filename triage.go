package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TriageView implements an Eisenhower Matrix triage overlay.
// Tasks are sorted into quadrants by priority: P1=Do First, P2=Schedule,
// P3=Delegate. Tasks with no priority are shown as "Needs Review".
type TriageView struct {
	repo         *Repository
	allTasks     []Task
	projectNames map[string]string
	items        []triageItem
	cursor       int
	width        int
	height       int
	viewport     viewport.Model

	// Session tracking
	reviewed map[string]bool
	changes  triageStats

	// Inline dialogs
	mode          string // "", "due", "deadline", "edit", "delete", "quick-add", "label"
	dueInput      textinput.Model
	deadlineInput textinput.Model
	editInput     textinput.Model
	quickInput    textinput.Model
	labelInput    textinput.Model
}

type triageItem struct {
	isSection   bool
	sectionName string
	task        *Task
}

type triageStats struct {
	prioritized int
	rescheduled int
	completed   int
	deleted     int
	added       int
	labeled     int
}

func NewTriageView(repo *Repository) TriageView {
	di := textinput.New()
	di.Placeholder = "e.g. tomorrow, next monday, every day"
	di.CharLimit = 200

	dli := textinput.New()
	dli.Placeholder = "YYYY-MM-DD (empty to clear)"
	dli.CharLimit = 32

	ei := textinput.New()
	ei.Placeholder = "Edit task..."
	ei.CharLimit = 500

	qi := textinput.New()
	qi.Placeholder = "Buy milk tomorrow #Work @urgent p1"
	qi.CharLimit = 500

	li := textinput.New()
	li.Placeholder = "urgent work (space-separated)"
	li.CharLimit = 200

	return TriageView{
		repo:          repo,
		reviewed:      make(map[string]bool),
		dueInput:      di,
		deadlineInput: dli,
		editInput:     ei,
		quickInput:    qi,
		labelInput:    li,
	}
}

func (v *TriageView) Open() {
	v.allTasks = v.repo.GetAllCachedTasks()
	v.projectNames = v.repo.GetProjectNameMap()
	v.reviewed = make(map[string]bool)
	v.changes = triageStats{}
	v.mode = ""
	v.cursor = 0
	v.viewport.YOffset = 0
	v.rebuildItems()
	v.skipToNextTask(1)
}

func (v *TriageView) SetSize(width, height int) {
	v.width = width
	v.height = height
	if v.viewport.Width == 0 || v.viewport.Height == 0 {
		v.viewport = viewport.New(width, height)
	} else {
		v.viewport.Width = width
		v.viewport.Height = height
	}
	v.dueInput.Width = width - 12
	v.deadlineInput.Width = width - 12
	v.editInput.Width = width - 12
	v.quickInput.Width = width - 12
	v.labelInput.Width = width - 12
	v.ensureVisible()
}

func (v TriageView) handlesInput() bool {
	return v.mode != ""
}

// --- Update ---

func (v TriageView) Update(msg tea.Msg) (TriageView, tea.Cmd) {
	switch msg := msg.(type) {
	case taskUpdatedMsg:
		if msg.err == nil {
			for i, t := range v.allTasks {
				if t.ID == msg.task.ID {
					v.allTasks[i] = msg.task
					break
				}
			}
			v.rebuildItems()
		}
		return v, nil

	case taskClosedMsg:
		if msg.err == nil {
			for i, t := range v.allTasks {
				if t.ID == msg.taskID {
					v.allTasks = append(v.allTasks[:i], v.allTasks[i+1:]...)
					break
				}
			}
			v.rebuildItems()
			v.clampCursor()
		}
		return v, nil

	case taskDeletedMsg:
		if msg.err == nil {
			for i, t := range v.allTasks {
				if t.ID == msg.taskID {
					v.allTasks = append(v.allTasks[:i], v.allTasks[i+1:]...)
					break
				}
			}
			v.rebuildItems()
			v.clampCursor()
		}
		return v, nil

	case taskCreatedMsg:
		if msg.err == nil {
			v.allTasks = append(v.allTasks, msg.task)
			v.rebuildItems()
		}
		return v, nil

	case quickAddMsg:
		if msg.err == nil {
			v.changes.added++
			if msg.task != nil {
				v.allTasks = append(v.allTasks, *msg.task)
				v.rebuildItems()
			}
		}
		return v, nil

	case tea.KeyMsg:
		return v.handleKey(msg)
	}

	// Route blink/tick to active inputs
	switch v.mode {
	case "due":
		var cmd tea.Cmd
		v.dueInput, cmd = v.dueInput.Update(msg)
		return v, cmd
	case "edit":
		var cmd tea.Cmd
		v.editInput, cmd = v.editInput.Update(msg)
		return v, cmd
	case "deadline":
		var cmd tea.Cmd
		v.deadlineInput, cmd = v.deadlineInput.Update(msg)
		return v, cmd
	case "quick-add":
		var cmd tea.Cmd
		v.quickInput, cmd = v.quickInput.Update(msg)
		return v, cmd
	case "label":
		var cmd tea.Cmd
		v.labelInput, cmd = v.labelInput.Update(msg)
		return v, cmd
	}

	return v, nil
}

func (v TriageView) handleKey(msg tea.KeyMsg) (TriageView, tea.Cmd) {
	// Dialog modes
	switch v.mode {
	case "quick-add":
		switch ResolveAction(ContextTriageDialog, msg.String()) {
		case ActionConfirm:
			text := strings.TrimSpace(v.quickInput.Value())
			if text == "" {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			return v, v.repo.QuickAdd(text, "")
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.quickInput, cmd = v.quickInput.Update(msg)
		return v, cmd

	case "edit":
		switch ResolveAction(ContextTriageDialog, msg.String()) {
		case ActionConfirm:
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
			v.reviewed[task.ID] = true
			return v, v.repo.UpdateTask(task.ID, updateTaskRequest{Content: &content})
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.editInput, cmd = v.editInput.Update(msg)
		return v, cmd

	case "due":
		switch ResolveAction(ContextTriageDialog, msg.String()) {
		case ActionConfirm:
			dueStr := strings.TrimSpace(v.dueInput.Value())
			task := v.selectedTask()
			if task == nil {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			v.reviewed[task.ID] = true
			v.changes.rescheduled++
			if dueStr == "" {
				empty := ""
				return v, v.repo.UpdateTask(task.ID, updateTaskRequest{DueString: &empty})
			}
			return v, v.repo.UpdateTask(task.ID, updateTaskRequest{DueString: &dueStr})
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.dueInput, cmd = v.dueInput.Update(msg)
		return v, cmd

	case "deadline":
		switch ResolveAction(ContextTriageDialog, msg.String()) {
		case ActionConfirm:
			deadlineStr := strings.TrimSpace(v.deadlineInput.Value())
			task := v.selectedTask()
			if task == nil {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			v.reviewed[task.ID] = true
			v.changes.rescheduled++
			if deadlineStr == "" {
				return v, v.repo.UpdateTask(task.ID, updateTaskRequest{ClearDeadline: true})
			}
			return v, v.repo.UpdateTask(task.ID, updateTaskRequest{DeadlineDate: &deadlineStr})
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.deadlineInput, cmd = v.deadlineInput.Update(msg)
		return v, cmd

	case "label":
		switch ResolveAction(ContextTriageDialog, msg.String()) {
		case ActionConfirm:
			labelStr := strings.TrimSpace(v.labelInput.Value())
			task := v.selectedTask()
			if task == nil {
				v.mode = ""
				return v, nil
			}
			v.mode = ""
			v.reviewed[task.ID] = true
			v.changes.labeled++
			// Parse labels: split on spaces, strip @ prefix
			var labels []string
			if labelStr != "" {
				for _, l := range strings.Fields(labelStr) {
					labels = append(labels, strings.TrimPrefix(l, "@"))
				}
			}
			return v, v.repo.UpdateTask(task.ID, updateTaskRequest{Labels: labels})
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		var cmd tea.Cmd
		v.labelInput, cmd = v.labelInput.Update(msg)
		return v, cmd

	case "delete":
		switch ResolveAction(ContextMainSidebarDialog, msg.String()) {
		case ActionConfirm:
			task := v.selectedTask()
			if task != nil {
				v.reviewed[task.ID] = true
				v.changes.deleted++
				v.mode = ""
				return v, v.repo.DeleteTask(task.ID)
			}
			v.mode = ""
			return v, nil
		case ActionCancel:
			v.mode = ""
			return v, nil
		}
		return v, nil
	}

	// Normal mode
	switch ResolveAction(ContextTriageOverlay, msg.String()) {
	case ActionNavDown:
		v.moveDown()
		v.ensureVisible()
		return v, nil
	case ActionNavUp:
		v.moveUp()
		v.ensureVisible()
		return v, nil
	case ActionNavTop:
		tmp := 0
		listJumpTop(&v.cursor, &tmp, len(v.items), func(idx int) bool { return v.items[idx].isSection })
		v.ensureVisible()
		return v, nil
	case ActionNavBottom:
		listJumpBottom(&v.cursor, len(v.items), func(idx int) bool { return v.items[idx].isSection })
		v.ensureVisible()
		return v, nil

	// Eisenhower quadrant assignment
	case ActionSetPriority1:
		return v.setPriority(1)
	case ActionSetPriority2:
		return v.setPriority(2)
	case ActionSetPriority3:
		return v.setPriority(3)
	case ActionClearPriority:
		return v.setPriority(4) // clear priority

	// Skip / mark reviewed
	case ActionMarkReviewed:
		task := v.selectedTask()
		if task != nil {
			v.reviewed[task.ID] = true
			v.advanceToNextUnreviewed()
		}
		return v, nil

	// Complete task
	case ActionToggleDone:
		task := v.selectedTask()
		if task != nil {
			v.reviewed[task.ID] = true
			v.changes.completed++
			return v, v.repo.CloseTask(task.ID)
		}
		return v, nil

	// Delete task
	case ActionDeleteTask:
		task := v.selectedTask()
		if task != nil {
			v.mode = "delete"
		}
		return v, nil

	// Set due date
	case ActionSetDue:
		task := v.selectedTask()
		if task != nil {
			v.mode = "due"
			v.dueInput.Reset()
			if task.Due != nil {
				if task.Due.String != "" {
					v.dueInput.SetValue(task.Due.String)
				} else {
					v.dueInput.SetValue(task.Due.Date)
				}
			}
			v.dueInput.Focus()
			return v, textinput.Blink
		}
		return v, nil

	case ActionSetDeadline:
		task := v.selectedTask()
		if task != nil {
			v.mode = "deadline"
			v.deadlineInput.Reset()
			if task.Deadline != nil {
				v.deadlineInput.SetValue(task.Deadline.Date)
			}
			v.deadlineInput.Focus()
			return v, textinput.Blink
		}
		return v, nil

	case ActionClearDates:
		task := v.selectedTask()
		if task == nil {
			return v, nil
		}
		if task.Due == nil && task.Deadline == nil {
			return v, func() tea.Msg {
				return toastMsg{text: "Task has no due/deadline to clear", isError: true}
			}
		}
		v.reviewed[task.ID] = true
		v.changes.rescheduled++
		empty := ""
		return v, v.repo.UpdateTask(task.ID, updateTaskRequest{
			DueString:     &empty,
			ClearDeadline: true,
		})

	// Edit content
	case ActionEditTask:
		task := v.selectedTask()
		if task != nil {
			v.mode = "edit"
			v.editInput.SetValue(task.Content)
			v.editInput.Focus()
			return v, textinput.Blink
		}
		return v, nil

	// Labels
	case ActionSetLabels:
		task := v.selectedTask()
		if task != nil {
			v.mode = "label"
			v.labelInput.Reset()
			if len(task.Labels) > 0 {
				v.labelInput.SetValue(strings.Join(task.Labels, " "))
			}
			v.labelInput.Focus()
			return v, textinput.Blink
		}
		return v, nil

	// Quick add
	case ActionNewTask:
		v.mode = "quick-add"
		v.quickInput.Reset()
		v.quickInput.Focus()
		return v, textinput.Blink
	}

	return v, nil
}

func (v *TriageView) setPriority(p int) (TriageView, tea.Cmd) {
	task := v.selectedTask()
	if task == nil {
		return *v, nil
	}
	v.reviewed[task.ID] = true
	v.changes.prioritized++
	cmd := v.repo.UpdateTask(task.ID, updateTaskRequest{Priority: &p})
	return *v, cmd
}

// --- View ---

func (v TriageView) View(width, height int) string {
	var b strings.Builder
	mutationStatus := v.repo.TaskMutationStatusMap()
	assigneeNames := v.repo.GetAssigneeNameMap()

	// Title + progress
	totalTasks := len(v.allTasks)
	reviewedCount := len(v.reviewed)
	title := triageTitleStyle.Render("Triage")
	progress := v.renderProgress(reviewedCount, totalTasks, width-lipgloss.Width(title)-8)
	titleLine := title + "  " + progress
	b.WriteString(titleLine)
	b.WriteString("\n\n")

	// Eisenhower Matrix
	b.WriteString(v.renderMatrix())
	b.WriteString("\n\n")

	// Task list
	if len(v.items) == 0 {
		b.WriteString(emptyStyle.Render("No tasks to triage"))
	} else {
		for i := 0; i < len(v.items); i++ {
			item := v.items[i]

			if item.isSection {
				b.WriteString(sectionStyle.Render("━━ " + item.sectionName))
				b.WriteString("\n")
				continue
			}

			task := item.task
			selected := i == v.cursor
			line := v.renderTask(task, selected, width-6, mutationStatus[task.ID], assigneeNames)
			b.WriteString(line)
			if i < len(v.items)-1 {
				b.WriteString("\n")
			}
		}
	}

	// Dialog overlay
	if v.mode != "" {
		b.WriteString("\n\n")
		b.WriteString(v.renderDialog(width))
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(v.renderFooter())

	vp := v.viewport
	if vp.Width != width || vp.Height != height {
		if vp.Width == 0 || vp.Height == 0 {
			vp = viewport.New(width, height)
		} else {
			vp.Width = width
			vp.Height = height
		}
	}
	vp.SetContent(b.String())
	return vp.View()
}

func (v TriageView) renderMatrix() string {
	// Count tasks per quadrant
	var q1, q2, q3, unsorted int
	for _, t := range v.allTasks {
		switch t.Priority {
		case 1:
			q1++
		case 2:
			q2++
		case 3:
			q3++
		default:
			unsorted++
		}
	}

	// Calculate cell widths
	totalW := v.width - 8
	if totalW < 30 {
		totalW = 30
	}
	halfW := totalW / 2

	bd := triageBorderStyle

	// Build each cell
	q1Label := triageQ1Style.Render("① DO FIRST")
	q2Label := triageQ2Style.Render("② SCHEDULE")
	q3Label := triageQ3Style.Render("③ DELEGATE")
	q4Label := triageUnsortedStyle.Render("✦ UNSORTED")

	q1Count := fmt.Sprintf("%d", q1)
	q2Count := fmt.Sprintf("%d", q2)
	q3Count := fmt.Sprintf("%d", q3)
	q4Count := fmt.Sprintf("%d", unsorted)

	q1Dots := triageQ1Style.Render(dotBar(q1))
	q2Dots := triageQ2Style.Render(dotBar(q2))
	q3Dots := triageQ3Style.Render(dotBar(q3))
	q4Dots := triageUnsortedStyle.Render(dotBar(unsorted))

	// Pad helpers
	pad := func(s string, w int) string {
		sw := lipgloss.Width(s)
		if sw >= w {
			return s
		}
		return s + strings.Repeat(" ", w-sw)
	}

	cellW := halfW - 2 // inner width per cell
	hLine := strings.Repeat("─", halfW-1)

	var lines []string
	lines = append(lines, bd.Render("  ┌─")+bd.Render(hLine)+bd.Render("┬─")+bd.Render(hLine)+bd.Render("┐"))
	lines = append(lines, bd.Render("  │")+" "+pad(q1Label+"  "+q1Count, cellW)+bd.Render("│")+" "+pad(q2Label+"  "+q2Count, cellW)+bd.Render("│"))
	lines = append(lines, bd.Render("  │")+" "+pad(q1Dots, cellW)+bd.Render("│")+" "+pad(q2Dots, cellW)+bd.Render("│"))
	lines = append(lines, bd.Render("  ├─")+bd.Render(hLine)+bd.Render("┼─")+bd.Render(hLine)+bd.Render("┤"))
	lines = append(lines, bd.Render("  │")+" "+pad(q3Label+"  "+q3Count, cellW)+bd.Render("│")+" "+pad(q4Label+"  "+q4Count, cellW)+bd.Render("│"))
	lines = append(lines, bd.Render("  │")+" "+pad(q3Dots, cellW)+bd.Render("│")+" "+pad(q4Dots, cellW)+bd.Render("│"))
	lines = append(lines, bd.Render("  └─")+bd.Render(hLine)+bd.Render("┴─")+bd.Render(hLine)+bd.Render("┘"))

	return strings.Join(lines, "\n")
}

func (v TriageView) renderProgress(reviewed, total, barWidth int) string {
	if total == 0 {
		return triageStatStyle.Render("No tasks")
	}
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 30 {
		barWidth = 30
	}

	filled := 0
	if total > 0 {
		filled = (reviewed * barWidth) / total
	}
	if filled > barWidth {
		filled = barWidth
	}

	bar := triageProgressFull.Render(strings.Repeat("█", filled)) +
		triageProgressEmpty.Render(strings.Repeat("░", barWidth-filled))

	label := fmt.Sprintf(" %d/%d", reviewed, total)
	return bar + triageStatStyle.Render(label)
}

func (v TriageView) renderTask(task *Task, selected bool, maxW int, syncStatus MutationStatus, assigneeNames map[string]string) string {
	maxContentWidth := maxW - 44
	if maxContentWidth < 20 {
		maxContentWidth = 20
	}

	projectName := v.projectNames[task.ProjectID]
	isReviewed := v.reviewed[task.ID]

	// Review indicator
	var reviewMark string
	if isReviewed {
		reviewMark = triageReviewedStyle.Render("✓")
	} else {
		reviewMark = lipgloss.NewStyle().Foreground(colorTextDim).Render("·")
	}

	if selected {
		check := "○"
		content := truncate(task.Content, maxContentWidth)
		var parts []string
		parts = append(parts, reviewMark, check, content)
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
		if len(task.Labels) > 0 {
			lbls := make([]string, len(task.Labels))
			for i, l := range task.Labels {
				lbls[i] = "@" + l
			}
			parts = append(parts, strings.Join(lbls, " "))
		}
		if badge := mutationBadgePlain(syncStatus); badge != "" {
			parts = append(parts, badge)
		}
		return lipgloss.NewStyle().
			Background(colorBgHL).
			Foreground(colorBright).
			Bold(true).
			Width(maxW).
			Render("  " + strings.Join(parts, "  "))
	}

	// Non-selected
	var parts []string
	parts = append(parts, reviewMark)
	parts = append(parts, styledCheckbox(false, task.Priority))
	parts = append(parts, taskContentStyle.Render(truncate(task.Content, maxContentWidth)))

	if projectName != "" {
		parts = append(parts, todayProjectTagStyle.Render(projectName))
	}

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
	if deadlineText := formatDeadline(task.Deadline); deadlineText != "" {
		if isDeadlineOverdue(task.Deadline) {
			deadlineText = dueOverdueStyle.Render(deadlineText)
		} else if isDeadlineToday(task.Deadline) {
			deadlineText = dueTodayStyle.Render(deadlineText)
		} else {
			deadlineText = deadlineStyle.Render(deadlineText)
		}
		parts = append(parts, deadlineText)
	}

	if task.Priority > 0 && task.Priority < 4 {
		parts = append(parts, priorityStyle(task.Priority).Render(priorityLabel(task.Priority)))
	}

	if assignee := formatAssignee(task, assigneeNames); assignee != "" {
		parts = append(parts, assigneeStyle.Render(assignee))
	}

	if len(task.Labels) > 0 {
		lbls := make([]string, len(task.Labels))
		for i, l := range task.Labels {
			lbls[i] = "@" + l
		}
		parts = append(parts, labelStyle.Render(strings.Join(lbls, " ")))
	}

	if badge := mutationBadgeStyled(syncStatus); badge != "" {
		parts = append(parts, badge)
	}

	return "  " + strings.Join(parts, "  ")
}

func (v TriageView) renderDialog(width int) string {
	dialogW := width - 8
	if dialogW < 30 {
		dialogW = 30
	}

	switch v.mode {
	case "quick-add":
		return dialogStyle.Width(dialogW).Render(
			dialogTitleStyle.Render("Quick Add") + "\n" +
				inputLabelStyle.Render("Supports: dates, #project, @label, p1-p4, //description") + "\n" +
				v.quickInput.View(),
		)
	case "edit":
		return dialogStyle.Width(dialogW).Render(
			dialogTitleStyle.Render("Edit Task") + "\n" +
				v.editInput.View(),
		)
	case "due":
		return dialogStyle.Width(dialogW).Render(
			dialogTitleStyle.Render("Set Due Date") + "\n" +
				inputLabelStyle.Render("e.g. today, tomorrow, next monday, every friday, (empty to clear)") + "\n" +
				v.dueInput.View(),
		)
	case "deadline":
		return dialogStyle.Width(dialogW).Render(
			dialogTitleStyle.Render("Set Deadline") + "\n" +
				inputLabelStyle.Render("YYYY-MM-DD (empty to clear)") + "\n" +
				v.deadlineInput.View(),
		)
	case "label":
		return dialogStyle.Width(dialogW).Render(
			dialogTitleStyle.Render("Set Labels") + "\n" +
				inputLabelStyle.Render("Space-separated labels (e.g. urgent work), empty to clear") + "\n" +
				v.labelInput.View(),
		)
	case "delete":
		task := v.selectedTask()
		name := ""
		if task != nil {
			name = task.Content
		}
		return dialogStyle.Width(dialogW).Render(
			dialogTitleStyle.Render("Delete Task?") + "\n" +
				taskContentStyle.Render("\""+truncate(name, 60)+"\"") + "\n\n" +
				footerKeyStyle.Render("y") + " confirm  " +
				footerKeyStyle.Render("n") + " cancel",
		)
	}
	return ""
}

func (v TriageView) renderFooter() string {
	if v.mode != "" {
		return footerKeyStyle.Render("enter") + " " + footerDescStyle.Render("confirm") + "  " +
			footerKeyStyle.Render("esc") + " " + footerDescStyle.Render("cancel")
	}

	// Show changes summary inline
	var stats []string
	if v.changes.prioritized > 0 {
		stats = append(stats, fmt.Sprintf("%d sorted", v.changes.prioritized))
	}
	if v.changes.rescheduled > 0 {
		stats = append(stats, fmt.Sprintf("%d dated", v.changes.rescheduled))
	}
	if v.changes.completed > 0 {
		stats = append(stats, fmt.Sprintf("%d done", v.changes.completed))
	}
	if v.changes.deleted > 0 {
		stats = append(stats, fmt.Sprintf("%d removed", v.changes.deleted))
	}
	if v.changes.labeled > 0 {
		stats = append(stats, fmt.Sprintf("%d tagged", v.changes.labeled))
	}
	if v.changes.added > 0 {
		stats = append(stats, fmt.Sprintf("%d added", v.changes.added))
	}

	var footer string
	if len(stats) > 0 {
		footer = triageStatStyle.Render(strings.Join(stats, " · ")) + "\n"
	}

	footer += keyHint("1", "do") + "  " +
		keyHint("2", "sched") + "  " +
		keyHint("3", "deleg") + "  " +
		keyHint("0", "clear") + "  " +
		keyHint("s", "due") + "  " +
		keyHint("S", "deadline") + "  " +
		keyHint("-", "clr dates") + "  " +
		keyHint("e", "edit") + "  " +
		keyHint("l", "labels") + "  " +
		keyHint("x", "done") + "  " +
		keyHint("d", "del") + "  " +
		keyHint("n", "new") + "  " +
		keyHint("enter", "skip") + "  " +
		keyHint("T", "close")

	return footer
}

// --- Item management ---

func (v *TriageView) rebuildItems() {
	// Save current task ID to restore position
	var currentTaskID string
	if v.cursor >= 0 && v.cursor < len(v.items) && v.items[v.cursor].task != nil {
		currentTaskID = v.items[v.cursor].task.ID
	}

	v.items = nil

	// Categorize
	var needsReview, q1, q2, q3 []Task
	for i := range v.allTasks {
		t := v.allTasks[i]
		switch t.Priority {
		case 1:
			q1 = append(q1, t)
		case 2:
			q2 = append(q2, t)
		case 3:
			q3 = append(q3, t)
		default:
			needsReview = append(needsReview, t)
		}
	}

	// Sort each: overdue first, then by date, then alphabetical
	triageSort := func(tasks []Task) {
		dateKey := func(t Task) string {
			if t.Due != nil && t.Due.Date != "" {
				return t.Due.Date
			}
			if t.Deadline != nil {
				return t.Deadline.Date
			}
			return ""
		}
		sort.Slice(tasks, func(i, j int) bool {
			iOver := isTaskOverdue(&tasks[i])
			jOver := isTaskOverdue(&tasks[j])
			if iOver != jOver {
				return iOver
			}
			di, dj := dateKey(tasks[i]), dateKey(tasks[j])
			if di != dj {
				if di == "" {
					return false
				}
				if dj == "" {
					return true
				}
				return di < dj
			}
			return tasks[i].Content < tasks[j].Content
		})
	}

	triageSort(needsReview)
	triageSort(q1)
	triageSort(q2)
	triageSort(q3)

	addSection := func(name string, tasks []Task) {
		if len(tasks) == 0 {
			return
		}
		v.items = append(v.items, triageItem{isSection: true, sectionName: fmt.Sprintf("%s (%d)", name, len(tasks))})
		for i := range tasks {
			v.items = append(v.items, triageItem{task: &tasks[i]})
		}
	}

	addSection("Needs Review", needsReview)
	addSection("① Do First", q1)
	addSection("② Schedule", q2)
	addSection("③ Delegate", q3)

	// Try to restore cursor position
	if currentTaskID != "" {
		for i, item := range v.items {
			if item.task != nil && item.task.ID == currentTaskID {
				v.cursor = i
				v.ensureVisible()
				return
			}
		}
	}

	// If task was removed (completed/deleted), try to find next unreviewed
	v.clampCursor()
}

// --- Navigation ---

func (v *TriageView) moveDown() {
	listMoveDown(&v.cursor, len(v.items), func(idx int) bool { return v.items[idx].isSection })
}

func (v *TriageView) moveUp() {
	listMoveUp(&v.cursor, len(v.items), func(idx int) bool { return v.items[idx].isSection })
}

func (v *TriageView) skipToNextTask(dir int) {
	listSkip(&v.cursor, len(v.items), dir, func(idx int) bool { return v.items[idx].isSection })
}

func (v *TriageView) clampCursor() {
	listClampCursor(&v.cursor, len(v.items), func(idx int) bool { return v.items[idx].isSection })
}

func (v *TriageView) ensureVisible() {
	line := v.selectedViewportLine()
	if line < 0 || v.viewport.Height <= 0 {
		return
	}
	if line < v.viewport.YOffset {
		v.viewport.YOffset = line
		return
	}
	if line >= v.viewport.YOffset+v.viewport.Height {
		v.viewport.YOffset = line - v.viewport.Height + 1
	}
	if v.viewport.YOffset < 0 {
		v.viewport.YOffset = 0
	}
}

func (v *TriageView) selectedViewportLine() int {
	if v.cursor < 0 || v.cursor >= len(v.items) {
		return -1
	}
	// Title+progress (1), blank (1), matrix (7), blank (1) => list starts at line 10.
	return 10 + v.cursor
}

func (v *TriageView) advanceToNextUnreviewed() {
	// Look forward from current position
	for i := v.cursor + 1; i < len(v.items); i++ {
		if v.items[i].task != nil && !v.reviewed[v.items[i].task.ID] {
			v.cursor = i
			v.ensureVisible()
			return
		}
	}
	// Wrap around from start
	for i := 0; i < v.cursor; i++ {
		if v.items[i].task != nil && !v.reviewed[v.items[i].task.ID] {
			v.cursor = i
			v.ensureVisible()
			return
		}
	}
	// All reviewed — stay in place
	v.moveDown()
	v.ensureVisible()
}

func (v TriageView) selectedTask() *Task {
	if v.cursor >= 0 && v.cursor < len(v.items) && !v.items[v.cursor].isSection {
		return v.items[v.cursor].task
	}
	return nil
}

// --- Helpers ---

func dotBar(count int) string {
	if count == 0 {
		return "─"
	}
	max := 15
	n := count
	if n > max {
		n = max
	}
	return strings.Repeat("●", n)
}
