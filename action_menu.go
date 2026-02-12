package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type actionMenuView struct {
	width   int
	height  int
	context InputContext
	query   textinput.Model
	items   []DiscoverableAction
	filter  []int
	cursor  int
}

func newActionMenuView() actionMenuView {
	ti := textinput.New()
	ti.Placeholder = "Filter actions..."
	ti.CharLimit = 80
	ti.Prompt = "› "
	return actionMenuView{query: ti}
}

func (v *actionMenuView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.query.Width = min(38, width-16)
}

func (v *actionMenuView) Open(ctx InputContext) tea.Cmd {
	v.context = ctx
	v.items = DiscoverableActions(ctx)
	v.cursor = 0
	v.query.Reset()
	v.query.Focus()
	v.refilter()
	return textinput.Blink
}

func (v actionMenuView) Context() InputContext {
	return v.context
}

func (v *actionMenuView) refilter() {
	q := strings.ToLower(strings.TrimSpace(v.query.Value()))
	v.filter = v.filter[:0]
	for i, item := range v.items {
		keys := strings.ToLower(strings.Join(item.Keys, " "))
		desc := strings.ToLower(item.Desc)
		if q == "" || strings.Contains(desc, q) || strings.Contains(keys, q) {
			v.filter = append(v.filter, i)
		}
	}
	if len(v.filter) == 0 {
		v.cursor = 0
		return
	}
	if v.cursor >= len(v.filter) {
		v.cursor = len(v.filter) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
}

func (v actionMenuView) Update(msg tea.Msg) (actionMenuView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return v, func() tea.Msg { return actionMenuCloseMsg{} }
		case "enter":
			if len(v.filter) == 0 {
				return v, nil
			}
			item := v.items[v.filter[v.cursor]]
			return v, func() tea.Msg {
				return actionMenuInvokeMsg{context: v.context, action: item.Action}
			}
		case "up", "k":
			if v.cursor > 0 {
				v.cursor--
			}
			return v, nil
		case "down", "j":
			if v.cursor < len(v.filter)-1 {
				v.cursor++
			}
			return v, nil
		}
	}

	var cmd tea.Cmd
	v.query, cmd = v.query.Update(msg)
	v.refilter()
	return v, cmd
}

func (v actionMenuView) View() string {
	panelW := 64
	if v.width < panelW+8 {
		panelW = v.width - 8
	}
	if panelW < 36 {
		panelW = 36
	}

	var b strings.Builder
	b.WriteString(dialogTitleStyle.Render("Actions"))
	b.WriteString("\n")
	b.WriteString(inputLabelStyle.Render("Type to filter, enter to run, esc to close"))
	b.WriteString("\n\n")
	b.WriteString(v.query.View())
	b.WriteString("\n\n")

	if len(v.filter) == 0 {
		b.WriteString(emptyStyle.Render("No matching actions"))
	} else {
		maxRows := 12
		start := 0
		if v.cursor >= maxRows {
			start = v.cursor - maxRows + 1
		}
		end := start + maxRows
		if end > len(v.filter) {
			end = len(v.filter)
		}

		for i := start; i < end; i++ {
			item := v.items[v.filter[i]]
			line := fmt.Sprintf("  %-16s  %s", item.Desc, strings.Join(item.Keys, " / "))
			if i == v.cursor {
				b.WriteString(queueSelectedStyle.Width(panelW - 4).Render(line))
			} else {
				b.WriteString(taskContentStyle.Render(line))
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	return dialogStyle.Width(panelW).Render(b.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func renderCenteredOverlay(base, overlay string, width, height int) string {
	ow := lipgloss.Width(overlay)
	oh := lipgloss.Height(overlay)
	x := (width - ow) / 2
	if x < 0 {
		x = 0
	}
	y := (height - oh) / 2
	if y < 0 {
		y = 0
	}
	return placeOverlay(base, overlay, x, y)
}
