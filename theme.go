package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// --- Color palette ---
var (
	// Base colors
	colorBg        = lipgloss.Color("#1A1B26")
	colorBgDark    = lipgloss.Color("#16161E")
	colorBgLight   = lipgloss.Color("#24283B")
	colorBgHL      = lipgloss.Color("#292E42")
	colorBgOverlay = lipgloss.Color("#1F2335")

	// Todoist brand
	colorTodoistRed = lipgloss.Color("#E44332")

	// Text
	colorText    = lipgloss.Color("#C0CAF5")
	colorTextDim = lipgloss.Color("#565F89")
	colorBright  = lipgloss.Color("#FFFFFF")
	colorSubtext = lipgloss.Color("#9AA5CE")

	// Accent
	colorBlue   = lipgloss.Color("#7AA2F7")
	colorGreen  = lipgloss.Color("#9ECE6A")
	colorYellow = lipgloss.Color("#E0AF68")
	colorOrange = lipgloss.Color("#FF9E64")
	colorRed    = lipgloss.Color("#F7768E")
	colorPurple = lipgloss.Color("#BB9AF7")
	colorCyan   = lipgloss.Color("#7DCFFF")

	// Priority colors (Todoist: 1=highest/red, 4=lowest/none)
	colorP1 = lipgloss.Color("#FF6B6B")
	colorP2 = lipgloss.Color("#FF9E64")
	colorP3 = lipgloss.Color("#E0AF68")
	colorP4 = lipgloss.Color("#565F89")

	// Borders
	colorBorder    = lipgloss.Color("#3B4261")
	colorBorderDim = lipgloss.Color("#292E42")
)

// --- Styles ---
var (
	appStyle = lipgloss.NewStyle().
			Background(colorBg)

	// Header
	headerStyle = lipgloss.NewStyle().
			Foreground(colorTodoistRed).
			Bold(true).
			Padding(0, 1)

	headerUserStyle = lipgloss.NewStyle().
			Foreground(colorTextDim).
			Padding(0, 1)

	// Sidebar
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(colorBorderDim).
			Padding(0, 1)

	sidebarTitleStyle = lipgloss.NewStyle().
				Foreground(colorTextDim).
				Bold(true).
				MarginBottom(1)

	projectSelectedStyle = lipgloss.NewStyle().
				Foreground(colorBright).
				Bold(true).
				Background(colorBgHL).
				Padding(0, 1)

	projectNormalStyle = lipgloss.NewStyle().
				Foreground(colorText).
				Padding(0, 1)

	projectFavStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	// Task list
	taskContentStyle = lipgloss.NewStyle().
				Foreground(colorText)

	taskSelectedBg = lipgloss.NewStyle().
			Background(colorBgHL)

	taskDescStyle = lipgloss.NewStyle().
			Foreground(colorTextDim).
			Italic(true)

	taskCheckbox = lipgloss.NewStyle().
			Foreground(colorTextDim)

	taskCheckboxDone = lipgloss.NewStyle().
				Foreground(colorGreen)

	// Due dates
	dueTodayStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	dueUpcomingStyle = lipgloss.NewStyle().
				Foreground(colorPurple)

	dueOverdueStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	// Priority styles
	p1Style = lipgloss.NewStyle().Foreground(colorP1).Bold(true)
	p2Style = lipgloss.NewStyle().Foreground(colorP2).Bold(true)
	p3Style = lipgloss.NewStyle().Foreground(colorP3).Bold(true)
	p4Style = lipgloss.NewStyle().Foreground(colorP4)

	// Labels
	labelStyle = lipgloss.NewStyle().
			Foreground(colorCyan)

	// Section headers
	sectionStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			Bold(true).
			MarginTop(1).
			MarginBottom(0)

	// Input/dialog
	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(1, 2).
			Background(colorBgOverlay)

	dialogTitleStyle = lipgloss.NewStyle().
				Foreground(colorBlue).
				Bold(true).
				MarginBottom(1)

	inputLabelStyle = lipgloss.NewStyle().
			Foreground(colorSubtext)

	inputStyle = lipgloss.NewStyle().
			Foreground(colorBright)

	// Footer
	footerStyle = lipgloss.NewStyle().
			Foreground(colorTextDim).
			Padding(0, 1)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)

	footerDescStyle = lipgloss.NewStyle().
			Foreground(colorTextDim)

	// Toast
	toastSuccessStyle = lipgloss.NewStyle().
				Foreground(colorGreen).
				Bold(true).
				Padding(0, 1)

	toastErrorStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true).
			Padding(0, 1)

	// Misc
	emptyStyle = lipgloss.NewStyle().
			Foreground(colorTextDim).
			Italic(true).
			Padding(1, 2)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Padding(1, 2)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true).
			Width(16)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorSubtext)

	// Recurring indicator
	recurringStyle = lipgloss.NewStyle().
			Foreground(colorCyan)
)

// keyHint renders a styled key shortcut for footer
func keyHint(key, desc string) string {
	return footerKeyStyle.Render(key) + " " + footerDescStyle.Render(desc)
}

// priorityStyle returns the appropriate style for a priority level
func priorityStyle(p int) lipgloss.Style {
	switch p {
	case 1:
		return p1Style
	case 2:
		return p2Style
	case 3:
		return p3Style
	default:
		return p4Style
	}
}

// styledCheckbox renders a task checkbox
func styledCheckbox(checked bool, priority int) string {
	if checked {
		return taskCheckboxDone.Render("✓")
	}
	switch priority {
	case 1:
		return p1Style.Render("○")
	case 2:
		return p2Style.Render("○")
	case 3:
		return p3Style.Render("○")
	default:
		return taskCheckbox.Render("○")
	}
}

// padLines pads every line of s to the given width with background color
func padLines(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth < width {
			lines[i] = line + strings.Repeat(" ", width-lineWidth)
		}
	}
	return strings.Join(lines, "\n")
}

