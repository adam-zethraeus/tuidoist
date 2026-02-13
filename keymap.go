package main

import "strings"

// Action is a semantic user intent derived from raw key presses.
type Action int

const (
	ActionNone Action = iota
	ActionQuit
	ActionToggleHelp
	ActionOpenQueue
	ActionOpenCompleted
	ActionOpenTriage
	ActionOpenSearch
	ActionOpenActions
	ActionToggleFocus
	ActionFocusTasks
	ActionNewTask
	ActionRefresh
	ActionNavDown
	ActionNavUp
	ActionNavTop
	ActionNavBottom
	ActionConfirm
	ActionCancel
	ActionSearchLocal
	ActionSearchNext
	ActionSearchPrev
	ActionClearSearch
	ActionToggleDone
	ActionEditTask
	ActionSetDue
	ActionSetDeadline
	ActionClearDates
	ActionDeleteTask
	ActionAddProject
	ActionArchiveProject
	ActionSetPriority1
	ActionSetPriority2
	ActionSetPriority3
	ActionSetPriority4
	ActionClearPriority
	ActionSetLabels
	ActionMarkReviewed
	ActionRetry
	ActionDismiss
	ActionClearConflicts
	ActionClearAll
	ActionUnarchive
	ActionSearchCreate
)

// InputContext defines where key input is currently routed.
type InputContext int

const (
	ContextMainSidebar InputContext = iota
	ContextMainSidebarDialog
	ContextMainTasks
	ContextMainTasksDialog
	ContextMainTasksSearch
	ContextMainToday
	ContextMainTodayDialog
	ContextMainTodaySearch
	ContextHelp
	ContextSearchOverlay
	ContextQueueOverlay
	ContextCompletedOverlay
	ContextTriageOverlay
	ContextTriageDialog
)

type KeyBinding struct {
	Action Action
	Keys   []string
	Hint   string
	Desc   string
}

var contextBindings = map[InputContext][]KeyBinding{
	ContextHelp: {
		{Action: ActionToggleHelp, Keys: []string{"?", "esc"}, Hint: "?", Desc: "close help"},
		{Action: ActionQuit, Keys: []string{"q", "ctrl+c"}, Hint: "q", Desc: "quit"},
	},
	ContextSearchOverlay: {
		{Action: ActionCancel, Keys: []string{"esc"}, Hint: "esc", Desc: "close"},
		{Action: ActionConfirm, Keys: []string{"enter"}, Hint: "enter", Desc: "go to"},
		{Action: ActionSearchCreate, Keys: []string{"alt+enter"}, Hint: "alt+enter", Desc: "new task"},
		{Action: ActionNavUp, Keys: []string{"up", "ctrl+p"}, Hint: "up", Desc: "prev"},
		{Action: ActionNavDown, Keys: []string{"down", "ctrl+n"}, Hint: "down", Desc: "next"},
	},
	ContextQueueOverlay: {
		{Action: ActionCancel, Keys: []string{"Q", "esc"}, Hint: "Q", Desc: "close"},
		{Action: ActionOpenActions, Keys: []string{"."}, Hint: ".", Desc: "actions"},
		{Action: ActionNavDown, Keys: []string{"j", "down"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavUp, Keys: []string{"k", "up"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionRetry, Keys: []string{"r"}, Hint: "r", Desc: "retry"},
		{Action: ActionDismiss, Keys: []string{"d"}, Hint: "d", Desc: "dismiss"},
		{Action: ActionClearConflicts, Keys: []string{"x"}, Hint: "x", Desc: "clear conflicts"},
		{Action: ActionClearAll, Keys: []string{"X"}, Hint: "X", Desc: "clear all"},
	},
	ContextCompletedOverlay: {
		{Action: ActionCancel, Keys: []string{"C", "esc"}, Hint: "C", Desc: "close"},
		{Action: ActionOpenActions, Keys: []string{"."}, Hint: ".", Desc: "actions"},
		{Action: ActionNavDown, Keys: []string{"j", "down"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavUp, Keys: []string{"k", "up"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionConfirm, Keys: []string{"enter", " "}, Hint: "enter", Desc: "open/reopen"},
		{Action: ActionUnarchive, Keys: []string{"u"}, Hint: "u", Desc: "unarchive"},
	},
	ContextTriageOverlay: {
		{Action: ActionCancel, Keys: []string{"T", "esc"}, Hint: "T", Desc: "close"},
		{Action: ActionOpenActions, Keys: []string{"."}, Hint: ".", Desc: "actions"},
		{Action: ActionNavDown, Keys: []string{"j", "down"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavUp, Keys: []string{"k", "up"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavTop, Keys: []string{"g"}, Hint: "g", Desc: "top"},
		{Action: ActionNavBottom, Keys: []string{"G"}, Hint: "G", Desc: "bottom"},
		{Action: ActionSetPriority1, Keys: []string{"1"}, Hint: "1", Desc: "do"},
		{Action: ActionSetPriority2, Keys: []string{"2"}, Hint: "2", Desc: "sched"},
		{Action: ActionSetPriority3, Keys: []string{"3"}, Hint: "3", Desc: "deleg"},
		{Action: ActionClearPriority, Keys: []string{"0", "4"}, Hint: "0", Desc: "clear"},
		{Action: ActionToggleDone, Keys: []string{"x"}, Hint: "x", Desc: "done"},
		{Action: ActionDeleteTask, Keys: []string{"d", "backspace"}, Hint: "d", Desc: "del"},
		{Action: ActionSetDue, Keys: []string{"s"}, Hint: "s", Desc: "due"},
		{Action: ActionSetDeadline, Keys: []string{"S"}, Hint: "S", Desc: "deadline"},
		{Action: ActionClearDates, Keys: []string{"-"}, Hint: "-", Desc: "clear dates"},
		{Action: ActionEditTask, Keys: []string{"e"}, Hint: "e", Desc: "edit"},
		{Action: ActionSetLabels, Keys: []string{"l"}, Hint: "l", Desc: "labels"},
		{Action: ActionNewTask, Keys: []string{"n"}, Hint: "n", Desc: "new"},
		{Action: ActionMarkReviewed, Keys: []string{"enter", " "}, Hint: "enter", Desc: "skip"},
	},
	ContextTriageDialog: {
		{Action: ActionConfirm, Keys: []string{"enter"}, Hint: "enter", Desc: "confirm"},
		{Action: ActionCancel, Keys: []string{"esc", "n"}, Hint: "esc", Desc: "cancel"},
	},
	ContextMainSidebar: {
		{Action: ActionQuit, Keys: []string{"q", "ctrl+c"}, Hint: "q", Desc: "quit"},
		{Action: ActionOpenActions, Keys: []string{"."}, Hint: ".", Desc: "actions"},
		{Action: ActionToggleHelp, Keys: []string{"?"}, Hint: "?", Desc: "help"},
		{Action: ActionOpenSearch, Keys: []string{"ctrl+p", "alt+p"}, Hint: "^P", Desc: "search"},
		{Action: ActionOpenQueue, Keys: []string{"Q"}, Hint: "Q", Desc: "queue"},
		{Action: ActionOpenCompleted, Keys: []string{"C"}, Hint: "C", Desc: "completed"},
		{Action: ActionOpenTriage, Keys: []string{"T"}, Hint: "T", Desc: "triage"},
		{Action: ActionToggleFocus, Keys: []string{"tab"}, Hint: "tab", Desc: "tasks"},
		{Action: ActionFocusTasks, Keys: []string{"enter"}, Hint: "enter", Desc: "tasks"},
		{Action: ActionNavDown, Keys: []string{"j", "down"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavUp, Keys: []string{"k", "up"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavTop, Keys: []string{"g"}, Hint: "g", Desc: "top"},
		{Action: ActionNavBottom, Keys: []string{"G"}, Hint: "G", Desc: "bottom"},
		{Action: ActionNewTask, Keys: []string{"n"}, Hint: "n", Desc: "new task"},
		{Action: ActionAddProject, Keys: []string{"a"}, Hint: "a", Desc: "add list"},
		{Action: ActionArchiveProject, Keys: []string{"d"}, Hint: "d", Desc: "archive"},
		{Action: ActionRefresh, Keys: []string{"r"}, Hint: "r", Desc: "refresh"},
	},
	ContextMainSidebarDialog: {
		{Action: ActionConfirm, Keys: []string{"enter", "y"}, Hint: "enter", Desc: "confirm"},
		{Action: ActionCancel, Keys: []string{"esc", "n"}, Hint: "esc", Desc: "cancel"},
	},
	ContextMainTasks: {
		{Action: ActionQuit, Keys: []string{"q", "ctrl+c"}, Hint: "q", Desc: "quit"},
		{Action: ActionOpenActions, Keys: []string{"."}, Hint: ".", Desc: "actions"},
		{Action: ActionToggleHelp, Keys: []string{"?"}, Hint: "?", Desc: "help"},
		{Action: ActionOpenSearch, Keys: []string{"ctrl+p", "alt+p"}, Hint: "^P", Desc: "search all"},
		{Action: ActionOpenQueue, Keys: []string{"Q"}, Hint: "Q", Desc: "queue"},
		{Action: ActionOpenCompleted, Keys: []string{"C"}, Hint: "C", Desc: "completed"},
		{Action: ActionOpenTriage, Keys: []string{"T"}, Hint: "T", Desc: "triage"},
		{Action: ActionToggleFocus, Keys: []string{"tab"}, Hint: "tab", Desc: "projects"},
		{Action: ActionNavDown, Keys: []string{"j", "down"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavUp, Keys: []string{"k", "up"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavTop, Keys: []string{"g"}, Hint: "g", Desc: "top"},
		{Action: ActionNavBottom, Keys: []string{"G"}, Hint: "G", Desc: "bottom"},
		{Action: ActionToggleDone, Keys: []string{"x", " "}, Hint: "x/space", Desc: "toggle"},
		{Action: ActionNewTask, Keys: []string{"n"}, Hint: "n", Desc: "new"},
		{Action: ActionEditTask, Keys: []string{"e"}, Hint: "e", Desc: "edit"},
		{Action: ActionSetDue, Keys: []string{"s"}, Hint: "s", Desc: "due"},
		{Action: ActionSetDeadline, Keys: []string{"S"}, Hint: "S", Desc: "deadline"},
		{Action: ActionClearDates, Keys: []string{"-"}, Hint: "-", Desc: "clear dates"},
		{Action: ActionDeleteTask, Keys: []string{"d"}, Hint: "d", Desc: "del"},
		{Action: ActionSetPriority1, Keys: []string{"1"}, Hint: "1-4", Desc: "prio"},
		{Action: ActionSetPriority2, Keys: []string{"2"}, Hint: "1-4", Desc: "prio"},
		{Action: ActionSetPriority3, Keys: []string{"3"}, Hint: "1-4", Desc: "prio"},
		{Action: ActionSetPriority4, Keys: []string{"4"}, Hint: "1-4", Desc: "prio"},
		{Action: ActionSearchLocal, Keys: []string{"/"}, Hint: "/", Desc: "search"},
		{Action: ActionSearchNext, Keys: []string{"n"}, Hint: "n/N", Desc: "next/prev"},
		{Action: ActionSearchPrev, Keys: []string{"N"}, Hint: "n/N", Desc: "next/prev"},
		{Action: ActionClearSearch, Keys: []string{"esc"}, Hint: "esc", Desc: "clear search"},
		{Action: ActionRefresh, Keys: []string{"r"}, Hint: "r", Desc: "refresh"},
	},
	ContextMainTasksDialog: {
		{Action: ActionConfirm, Keys: []string{"enter", "y"}, Hint: "enter", Desc: "confirm"},
		{Action: ActionCancel, Keys: []string{"esc", "n"}, Hint: "esc", Desc: "cancel"},
	},
	ContextMainTasksSearch: {
		{Action: ActionConfirm, Keys: []string{"enter"}, Hint: "enter", Desc: "search"},
		{Action: ActionCancel, Keys: []string{"esc"}, Hint: "esc", Desc: "cancel"},
	},
	ContextMainToday: {
		{Action: ActionQuit, Keys: []string{"q", "ctrl+c"}, Hint: "q", Desc: "quit"},
		{Action: ActionOpenActions, Keys: []string{"."}, Hint: ".", Desc: "actions"},
		{Action: ActionToggleHelp, Keys: []string{"?"}, Hint: "?", Desc: "help"},
		{Action: ActionOpenSearch, Keys: []string{"ctrl+p", "alt+p"}, Hint: "^P", Desc: "search all"},
		{Action: ActionOpenQueue, Keys: []string{"Q"}, Hint: "Q", Desc: "queue"},
		{Action: ActionOpenCompleted, Keys: []string{"C"}, Hint: "C", Desc: "completed"},
		{Action: ActionOpenTriage, Keys: []string{"T"}, Hint: "T", Desc: "triage"},
		{Action: ActionToggleFocus, Keys: []string{"tab"}, Hint: "tab", Desc: "projects"},
		{Action: ActionNavDown, Keys: []string{"j", "down"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavUp, Keys: []string{"k", "up"}, Hint: "j/k", Desc: "nav"},
		{Action: ActionNavTop, Keys: []string{"g"}, Hint: "g", Desc: "top"},
		{Action: ActionNavBottom, Keys: []string{"G"}, Hint: "G", Desc: "bottom"},
		{Action: ActionToggleDone, Keys: []string{"x", " "}, Hint: "x/space", Desc: "toggle"},
		{Action: ActionSetDue, Keys: []string{"s"}, Hint: "s", Desc: "due"},
		{Action: ActionSetDeadline, Keys: []string{"S"}, Hint: "S", Desc: "deadline"},
		{Action: ActionClearDates, Keys: []string{"-"}, Hint: "-", Desc: "clear dates"},
		{Action: ActionNewTask, Keys: []string{"n"}, Hint: "n", Desc: "new"},
		{Action: ActionSearchLocal, Keys: []string{"/"}, Hint: "/", Desc: "search"},
		{Action: ActionSearchNext, Keys: []string{"n"}, Hint: "n/N", Desc: "next/prev"},
		{Action: ActionSearchPrev, Keys: []string{"N"}, Hint: "n/N", Desc: "next/prev"},
		{Action: ActionClearSearch, Keys: []string{"esc"}, Hint: "esc", Desc: "clear search"},
		{Action: ActionRefresh, Keys: []string{"r"}, Hint: "r", Desc: "refresh"},
	},
	ContextMainTodayDialog: {
		{Action: ActionConfirm, Keys: []string{"enter"}, Hint: "enter", Desc: "confirm"},
		{Action: ActionCancel, Keys: []string{"esc"}, Hint: "esc", Desc: "cancel"},
	},
	ContextMainTodaySearch: {
		{Action: ActionConfirm, Keys: []string{"enter"}, Hint: "enter", Desc: "search"},
		{Action: ActionCancel, Keys: []string{"esc"}, Hint: "esc", Desc: "cancel"},
	},
}

type DiscoverableAction struct {
	Action Action
	Desc   string
	Keys   []string
}

func DiscoverableActions(ctx InputContext) []DiscoverableAction {
	bindings := contextBindings[ctx]
	order := make([]Action, 0, len(bindings))
	items := make(map[Action]DiscoverableAction)
	for _, b := range bindings {
		if b.Desc == "" {
			continue
		}
		if b.Action == ActionOpenActions || b.Action == ActionOpenSearch {
			continue
		}
		if _, ok := items[b.Action]; !ok {
			order = append(order, b.Action)
			items[b.Action] = DiscoverableAction{
				Action: b.Action,
				Desc:   b.Desc,
				Keys:   append([]string{}, b.Keys...),
			}
			continue
		}
		cur := items[b.Action]
		seen := make(map[string]bool, len(cur.Keys))
		for _, k := range cur.Keys {
			seen[k] = true
		}
		for _, k := range b.Keys {
			if !seen[k] {
				cur.Keys = append(cur.Keys, k)
				seen[k] = true
			}
		}
		items[b.Action] = cur
	}
	out := make([]DiscoverableAction, 0, len(order))
	for _, a := range order {
		out = append(out, items[a])
	}
	return out
}

func KeysForAction(ctx InputContext, action Action) []string {
	for _, b := range contextBindings[ctx] {
		if b.Action == action {
			return append([]string{}, b.Keys...)
		}
	}
	return nil
}

func ResolveAction(ctx InputContext, key string) Action {
	bindings := contextBindings[ctx]
	for _, b := range bindings {
		for _, candidate := range b.Keys {
			if key == candidate {
				return b.Action
			}
		}
	}
	return ActionNone
}

func HintsForContext(ctx InputContext) []string {
	bindings := contextBindings[ctx]
	seen := make(map[string]bool)
	var hints []string
	for _, b := range bindings {
		if b.Hint == "" || b.Desc == "" {
			continue
		}
		id := b.Hint + "::" + b.Desc
		if seen[id] {
			continue
		}
		seen[id] = true
		hints = append(hints, keyHint(b.Hint, b.Desc))
	}
	return hints
}

func HelpSections() []helpSection {
	return []helpSection{
		{Title: "Navigation", Context: ContextMainTasks, ActionFilter: map[Action]bool{ActionNavDown: true, ActionNavUp: true, ActionNavTop: true, ActionNavBottom: true, ActionToggleFocus: true, ActionFocusTasks: true}},
		{Title: "Tasks", Context: ContextMainTasks, ActionFilter: map[Action]bool{ActionToggleDone: true, ActionNewTask: true, ActionEditTask: true, ActionSetDue: true, ActionSetDeadline: true, ActionClearDates: true, ActionDeleteTask: true, ActionSetPriority1: true}},
		{Title: "Projects", Context: ContextMainSidebar, ActionFilter: map[Action]bool{ActionAddProject: true, ActionArchiveProject: true}},
		{Title: "Search", Context: ContextMainTasks, ActionFilter: map[Action]bool{ActionOpenSearch: true, ActionSearchLocal: true, ActionSearchNext: true, ActionClearSearch: true}},
		{Title: "Triage", Context: ContextTriageOverlay, ActionFilter: map[Action]bool{ActionOpenTriage: true, ActionSetPriority1: true, ActionSetPriority2: true, ActionSetPriority3: true, ActionClearPriority: true, ActionSetLabels: true, ActionMarkReviewed: true}},
		{Title: "General", Context: ContextMainTasks, ActionFilter: map[Action]bool{ActionOpenActions: true, ActionRefresh: true, ActionOpenCompleted: true, ActionOpenQueue: true, ActionToggleHelp: true, ActionQuit: true}},
	}
}

type helpSection struct {
	Title        string
	Context      InputContext
	ActionFilter map[Action]bool
}

func HelpItemsFromKeymap() []struct{ key, desc string } {
	sections := HelpSections()
	var items []struct{ key, desc string }

	for _, s := range sections {
		items = append(items, struct{ key, desc string }{key: s.Title})
		for _, b := range contextBindings[s.Context] {
			if !s.ActionFilter[b.Action] || b.Desc == "" {
				continue
			}
			keyText := strings.Join(b.Keys, " / ")
			items = append(items, struct{ key, desc string }{key: keyText, desc: b.Desc})
		}
		items = append(items, struct{ key, desc string }{})
	}

	if len(items) > 0 {
		items = items[:len(items)-1]
	}
	return items
}
