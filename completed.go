package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// completedGroup holds completed tasks for one project.
type completedGroup struct {
	projectID   string
	projectName string
	tasks       []Task
	collapsed   bool
}

type completedItemKind int

const (
	ciGroupHeader completedItemKind = iota
	ciTask
	ciSectionHeader
	ciArchivedProject
)

type completedViewItem struct {
	kind     completedItemKind
	groupIdx int
	task     *Task
	project  *Project
}

// CompletedView displays recently completed tasks grouped by project,
// and archived projects with unarchive option.
type CompletedView struct {
	repo         *Repository
	groups       []completedGroup
	archived     []Project
	items        []completedViewItem
	cursor       int
	scrollOffset int
	height       int
}

func NewCompletedView(repo *Repository) CompletedView {
	return CompletedView{repo: repo}
}

func (v *CompletedView) Refresh() {
	// Load recently completed tasks from store
	rows := v.repo.GetRecentlyCompleted(200)

	// Group by project
	groupMap := make(map[string]*completedGroup)
	var groupOrder []string
	for _, row := range rows {
		g, ok := groupMap[row.ProjectID]
		if !ok {
			g = &completedGroup{
				projectID:   row.ProjectID,
				projectName: row.ProjectName,
				collapsed:   true,
			}
			groupMap[row.ProjectID] = g
			groupOrder = append(groupOrder, row.ProjectID)
		}
		g.tasks = append(g.tasks, row.Task)
	}

	v.groups = nil
	for _, pid := range groupOrder {
		v.groups = append(v.groups, *groupMap[pid])
	}

	// Load archived projects
	v.archived = v.repo.GetArchivedProjects()

	v.rebuildItems()
	if v.cursor >= len(v.items) {
		v.cursor = len(v.items) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
}

func (v *CompletedView) rebuildItems() {
	v.items = nil

	for i := range v.groups {
		g := &v.groups[i]
		v.items = append(v.items, completedViewItem{kind: ciGroupHeader, groupIdx: i})
		if !g.collapsed {
			for j := range g.tasks {
				v.items = append(v.items, completedViewItem{kind: ciTask, groupIdx: i, task: &g.tasks[j]})
			}
		}
	}

	if len(v.archived) > 0 {
		v.items = append(v.items, completedViewItem{kind: ciSectionHeader})
		for i := range v.archived {
			v.items = append(v.items, completedViewItem{kind: ciArchivedProject, project: &v.archived[i]})
		}
	}
}

func (v CompletedView) Update(msg tea.Msg) (CompletedView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		m := tea.MouseEvent(msg)
		if m.Action == tea.MouseActionMotion || m.Action == tea.MouseActionRelease {
			return v, nil
		}
		// Scroll wheel
		if m.Button == tea.MouseButtonWheelDown {
			if v.cursor < len(v.items)-1 {
				v.cursor++
				if v.cursor < len(v.items) && v.items[v.cursor].kind == ciSectionHeader {
					if v.cursor < len(v.items)-1 {
						v.cursor++
					}
				}
			}
			v.ensureVisible(v.height)
			return v, nil
		}
		if m.Button == tea.MouseButtonWheelUp {
			if v.cursor > 0 {
				v.cursor--
				if v.cursor >= 0 && v.items[v.cursor].kind == ciSectionHeader {
					if v.cursor > 0 {
						v.cursor--
					}
				}
			}
			v.ensureVisible(v.height)
			return v, nil
		}
		// Left click
		if m.Button == tea.MouseButtonLeft {
			// helpStyle has Padding(1,2): 1 row top pad, then title, MarginBottom(1), blank line
			contentY := m.Y - 4
			if contentY < 0 {
				return v, nil
			}

			// Walk visible items tracking rendered lines
			visibleHeight := v.height - 8
			if visibleHeight < 1 {
				visibleHeight = 1
			}
			end := v.scrollOffset + visibleHeight
			if end > len(v.items) {
				end = len(v.items)
			}

			line := 0
			for i := v.scrollOffset; i < end; i++ {
				item := v.items[i]
				if item.kind == ciSectionHeader {
					// Section headers render as blank line + header text = 2 lines
					if line == contentY || line+1 == contentY {
						// Don't select section headers, but consume the lines
						line += 2
						continue
					}
					line += 2
					continue
				}
				if line == contentY {
					v.cursor = i
					v.ensureVisible(v.height)
					return v, nil
				}
				line++
			}
		}
		return v, nil

	case tea.KeyMsg:
		switch ResolveAction(ContextCompletedOverlay, msg.String()) {
		case ActionNavDown:
			if v.cursor < len(v.items)-1 {
				v.cursor++
				// Skip section headers
				if v.cursor < len(v.items) && v.items[v.cursor].kind == ciSectionHeader {
					if v.cursor < len(v.items)-1 {
						v.cursor++
					}
				}
			}
			v.ensureVisible(v.height)
			return v, nil
		case ActionNavUp:
			if v.cursor > 0 {
				v.cursor--
				if v.cursor >= 0 && v.items[v.cursor].kind == ciSectionHeader {
					if v.cursor > 0 {
						v.cursor--
					}
				}
			}
			v.ensureVisible(v.height)
			return v, nil
		case ActionConfirm:
			if v.cursor >= 0 && v.cursor < len(v.items) {
				item := v.items[v.cursor]
				switch item.kind {
				case ciGroupHeader:
					v.groups[item.groupIdx].collapsed = !v.groups[item.groupIdx].collapsed
					v.rebuildItems()
					return v, nil
				case ciTask:
					if item.task != nil {
						return v, v.repo.ReopenTask(*item.task)
					}
				case ciArchivedProject:
					if item.project != nil {
						return v, v.repo.UnarchiveProject(item.project.ID)
					}
				}
			}
			return v, nil
		case ActionUnarchive:
			if v.cursor >= 0 && v.cursor < len(v.items) {
				item := v.items[v.cursor]
				if item.kind == ciArchivedProject && item.project != nil {
					return v, v.repo.UnarchiveProject(item.project.ID)
				}
			}
			return v, nil
		}
	}
	return v, nil
}

func (v *CompletedView) ensureVisible(height int) {
	visibleHeight := height - 8
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	listEnsureVisible(v.cursor, &v.scrollOffset, visibleHeight)
}

func (v *CompletedView) SetSize(height int) {
	v.height = height
	v.ensureVisible(height)
}

func (v CompletedView) View(width, height int) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true).
		MarginBottom(1).
		Render("Recently Completed"))
	b.WriteString("\n\n")

	if len(v.items) == 0 {
		b.WriteString(emptyStyle.Render("No recently completed tasks"))
		b.WriteString("\n\n")
		b.WriteString(footerKeyStyle.Render("C") + " " + footerDescStyle.Render("close"))
		return helpStyle.Width(width).Height(height).Render(b.String())
	}

	visibleHeight := height - 8
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	end := v.scrollOffset + visibleHeight
	if end > len(v.items) {
		end = len(v.items)
	}

	for i := v.scrollOffset; i < end; i++ {
		item := v.items[i]
		selected := i == v.cursor

		switch item.kind {
		case ciGroupHeader:
			g := v.groups[item.groupIdx]
			arrow := "▸"
			if !g.collapsed {
				arrow = "▾"
			}
			name := g.projectName
			if name == "" {
				name = "Unknown"
			}
			header := fmt.Sprintf("%s %s (%d)", arrow, name, len(g.tasks))
			if selected {
				b.WriteString(queueSelectedStyle.Width(width - 4).Render(header))
			} else {
				b.WriteString(queueTitleStyle.Render(header))
			}
			b.WriteString("\n")

		case ciTask:
			if selected {
				// Plain text avoids inner ANSI resets breaking the selection background
				line := "    ✓  " + truncate(item.task.Content, width-20)
				b.WriteString(lipgloss.NewStyle().
					Background(colorBgHL).
					Foreground(colorBright).
					Width(width - 4).
					Render(line))
			} else {
				line := "    " + styledCheckbox(true, item.task.Priority) + "  " + taskCompletedStyle.Render(truncate(item.task.Content, width-20))
				b.WriteString(line)
			}
			b.WriteString("\n")

		case ciSectionHeader:
			b.WriteString("\n")
			b.WriteString(sectionStyle.Render("━━ Archived Lists"))
			b.WriteString("\n")

		case ciArchivedProject:
			if item.project != nil {
				line := "  📦 " + item.project.Name
				if selected {
					b.WriteString(queueSelectedStyle.Width(width - 4).Render(line))
				} else {
					b.WriteString(queueItemStyle.Render(line))
				}
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(footerKeyStyle.Render("j/k") + " nav  " +
		footerKeyStyle.Render("space") + " toggle/reopen  " +
		footerKeyStyle.Render("u") + " unarchive  " +
		footerKeyStyle.Render("C") + " close")

	return helpStyle.Width(width).Height(height).Render(b.String())
}
