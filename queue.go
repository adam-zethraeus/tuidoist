package main

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// QueueView displays pending and conflicted mutations
type QueueView struct {
	repo         *Repository
	mutations    []Mutation
	cursor       int
	confirmClear string // "", "conflicts", "all"
}

func NewQueueView(repo *Repository) QueueView {
	return QueueView{repo: repo}
}

func (v *QueueView) Refresh() {
	v.mutations = v.repo.GetAllMutations()
	if v.cursor >= len(v.mutations) {
		v.cursor = len(v.mutations) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
}

func (v QueueView) Update(msg tea.Msg) (QueueView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		m := tea.MouseEvent(msg)
		if m.Action == tea.MouseActionMotion || m.Action == tea.MouseActionRelease {
			return v, nil
		}
		// Scroll wheel
		if m.Button == tea.MouseButtonWheelDown {
			if v.cursor < len(v.mutations)-1 {
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
		if m.Button == tea.MouseButtonLeft {
			// helpStyle has Padding(1,2): 1 row top pad, then title, MarginBottom(1), blank line
			contentY := m.Y - 4
			if contentY < 0 {
				return v, nil
			}

			// Walk groups same as View() to build line-to-index map
			var pending, conflicted []indexedMutation
			for i, mt := range v.mutations {
				im := indexedMutation{index: i, mutation: mt}
				if mt.Status == MutationConflicted {
					conflicted = append(conflicted, im)
				} else {
					pending = append(pending, im)
				}
			}

			line := 0
			if len(pending) > 0 {
				line++ // section header
				for _, im := range pending {
					if line == contentY {
						v.cursor = im.index
						return v, nil
					}
					line++ // item line
					if im.index == v.cursor {
						// selected items don't have extra lines in pending
					}
				}
				line++ // blank line after section
			}
			if len(conflicted) > 0 {
				line++ // section header
				for _, im := range conflicted {
					if line == contentY {
						v.cursor = im.index
						return v, nil
					}
					line++ // item line
					if im.mutation.Conflict != "" {
						line++ // conflict detail line
					}
					if im.index == v.cursor {
						line++ // action hints line
					}
				}
			}
		}
		return v, nil

	case tea.KeyMsg:
		if v.confirmClear != "" {
			switch ResolveAction(ContextMainSidebarDialog, msg.String()) {
			case ActionConfirm:
				mode := v.confirmClear
				v.confirmClear = ""
				switch mode {
				case "conflicts":
					v.clearConflictsLocal()
					return v, v.repo.DismissConflictedMutations()
				case "all":
					v.clearAllLocal()
					return v, v.repo.DismissAllMutations()
				}
				return v, nil
			case ActionCancel:
				v.confirmClear = ""
				return v, nil
			}
			return v, nil
		}
		switch ResolveAction(ContextQueueOverlay, msg.String()) {
		case ActionNavDown:
			if v.cursor < len(v.mutations)-1 {
				v.cursor++
			}
			return v, nil
		case ActionNavUp:
			if v.cursor > 0 {
				v.cursor--
			}
			return v, nil
		case ActionRetry:
			if v.cursor >= 0 && v.cursor < len(v.mutations) {
				m := v.mutations[v.cursor]
				if m.Status == MutationConflicted || m.Status == MutationFlushing {
					return v, v.repo.RetryMutation(m.ID)
				}
			}
			return v, nil
		case ActionDismiss:
			if v.cursor >= 0 && v.cursor < len(v.mutations) {
				m := v.mutations[v.cursor]
				cmd := v.repo.DismissMutation(m.ID)
				v.mutations = append(v.mutations[:v.cursor], v.mutations[v.cursor+1:]...)
				if v.cursor >= len(v.mutations) {
					v.cursor = len(v.mutations) - 1
				}
				if v.cursor < 0 {
					v.cursor = 0
				}
				return v, cmd
			}
			return v, nil
		case ActionClearConflicts:
			v.confirmClear = "conflicts"
			return v, nil
		case ActionClearAll:
			v.confirmClear = "all"
			return v, nil
		}
	}
	return v, nil
}

func (v QueueView) View(width, height int) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true).
		MarginBottom(1).
		Render("Sync Queue"))
	b.WriteString("\n\n")

	if len(v.mutations) == 0 {
		b.WriteString(emptyStyle.Render("No pending mutations"))
		b.WriteString("\n\n")
		b.WriteString(footerKeyStyle.Render("Q") + " " + footerDescStyle.Render("close"))
		return helpStyle.Width(width).Height(height).Render(b.String())
	}

	// Group by status
	var pending, conflicted []indexedMutation
	for i, m := range v.mutations {
		im := indexedMutation{index: i, mutation: m}
		switch m.Status {
		case MutationConflicted:
			conflicted = append(conflicted, im)
		default:
			pending = append(pending, im)
		}
	}

	if len(pending) > 0 {
		b.WriteString(queueTitleStyle.Render(fmt.Sprintf("━━ Pending (%d)", len(pending))))
		b.WriteString("\n")
		for _, im := range pending {
			selected := im.index == v.cursor
			line := "  " + renderMutationLine(im.mutation)
			if selected {
				b.WriteString(queueSelectedStyle.Width(width - 4).Render(line))
			} else {
				b.WriteString(queueItemStyle.Render(line))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(conflicted) > 0 {
		b.WriteString(queueTitleStyle.Render(fmt.Sprintf("━━ Conflicts (%d)", len(conflicted))))
		b.WriteString("\n")
		for _, im := range conflicted {
			selected := im.index == v.cursor
			line := "  " + renderMutationLine(im.mutation)
			if selected {
				b.WriteString(queueSelectedStyle.Width(width - 4).Render(line))
			} else {
				b.WriteString(queueConflictStyle.Render(line))
			}
			b.WriteString("\n")
			if im.mutation.Conflict != "" {
				b.WriteString(queueConflictStyle.Render("    " + im.mutation.Conflict))
				b.WriteString("\n")
			}
			if selected {
				b.WriteString("    " + footerKeyStyle.Render("r") + " retry  " + footerKeyStyle.Render("d") + " dismiss")
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	if v.confirmClear != "" {
		label := "Clear conflicted mutations?"
		if v.confirmClear == "all" {
			label = "Clear all mutations?"
		}
		b.WriteString(queueConflictStyle.Render(label))
		b.WriteString("\n")
		b.WriteString(footerKeyStyle.Render("enter") + " yes  " +
			footerKeyStyle.Render("esc") + " no")
	} else {
		b.WriteString(footerKeyStyle.Render("j/k") + " nav  " +
			footerKeyStyle.Render("r") + " retry  " +
			footerKeyStyle.Render("d") + " dismiss  " +
			footerKeyStyle.Render("x") + " clear conflicts  " +
			footerKeyStyle.Render("X") + " clear all  " +
			footerKeyStyle.Render("Q") + " close")
	}

	return helpStyle.Width(width).Height(height).Render(b.String())
}

func (v *QueueView) clearConflictsLocal() {
	filtered := v.mutations[:0]
	for _, m := range v.mutations {
		if m.Status == MutationConflicted {
			continue
		}
		filtered = append(filtered, m)
	}
	v.mutations = filtered
	if v.cursor >= len(v.mutations) {
		v.cursor = len(v.mutations) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
}

func (v *QueueView) clearAllLocal() {
	v.mutations = nil
	v.cursor = 0
}

type indexedMutation struct {
	index    int
	mutation Mutation
}

func renderMutationLine(m Mutation) string {
	icon := "↑"
	if m.Status == MutationConflicted {
		icon = "⚠"
	}

	var desc string
	switch m.Action {
	case MutationCreate:
		var req createTaskRequest
		if json.Unmarshal([]byte(m.Payload), &req) == nil {
			desc = fmt.Sprintf("Create %q", truncate(req.Content, 40))
		} else {
			desc = "Create task"
		}
	case MutationUpdate:
		desc = describeUpdate(m)
	case MutationClose:
		desc = describeFromSnapshot(m, "Close")
	case MutationDelete:
		desc = describeFromSnapshot(m, "Delete")
	case MutationReopen:
		desc = describeFromSnapshot(m, "Reopen")
	case MutationQuickAdd:
		var req quickAddMutationPayload
		if json.Unmarshal([]byte(m.Payload), &req) == nil {
			desc = fmt.Sprintf("Quick add %q", truncate(req.Text, 40))
		} else {
			desc = "Quick add task"
		}
	}

	return fmt.Sprintf("%s %s", icon, desc)
}

func describeUpdate(m Mutation) string {
	var req updateTaskRequest
	if err := json.Unmarshal([]byte(m.Payload), &req); err != nil {
		return "Update task"
	}

	name := taskNameFromSnapshot(m)
	var changes []string
	if req.Content != nil {
		changes = append(changes, fmt.Sprintf("content→%q", truncate(*req.Content, 20)))
	}
	if req.Priority != nil {
		changes = append(changes, fmt.Sprintf("priority→%d", *req.Priority))
	}
	if req.DueString != nil {
		changes = append(changes, fmt.Sprintf("due→%q", *req.DueString))
	}
	if req.ClearDeadline {
		changes = append(changes, "deadline→clear")
	} else if req.DeadlineDate != nil {
		changes = append(changes, fmt.Sprintf("deadline→%q", *req.DeadlineDate))
	}
	if req.Description != nil {
		changes = append(changes, "description")
	}
	if req.Labels != nil {
		changes = append(changes, "labels")
	}

	if len(changes) > 0 {
		return fmt.Sprintf("Update %q — %s", truncate(name, 30), strings.Join(changes, ", "))
	}
	return fmt.Sprintf("Update %q", truncate(name, 40))
}

func describeFromSnapshot(m Mutation, verb string) string {
	name := taskNameFromSnapshot(m)
	if name != "" {
		return fmt.Sprintf("%s %q", verb, truncate(name, 40))
	}
	return fmt.Sprintf("%s task %s", verb, m.EntityID)
}

func taskNameFromSnapshot(m Mutation) string {
	if m.Snapshot == "" {
		return ""
	}
	var t Task
	if json.Unmarshal([]byte(m.Snapshot), &t) == nil {
		return t.Content
	}
	return ""
}
