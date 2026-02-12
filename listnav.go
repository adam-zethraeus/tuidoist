package main

// listSkipFn reports whether an index is non-selectable and should be skipped.
type listSkipFn func(idx int) bool

func listClampCursor(cursor *int, total int, skip listSkipFn) {
	if total <= 0 {
		*cursor = 0
		return
	}
	if *cursor < 0 {
		*cursor = 0
	}
	if *cursor >= total {
		*cursor = total - 1
	}
	if skip != nil && skip(*cursor) {
		listSkip(cursor, total, 1, skip)
	}
}

func listMoveDown(cursor *int, total int, skip listSkipFn) {
	if total <= 0 {
		*cursor = 0
		return
	}
	if *cursor < total-1 {
		*cursor = *cursor + 1
		if skip != nil && skip(*cursor) {
			if *cursor < total-1 {
				*cursor = *cursor + 1
			}
		}
	}
}

func listMoveUp(cursor *int, total int, skip listSkipFn) {
	if total <= 0 {
		*cursor = 0
		return
	}
	if *cursor > 0 {
		*cursor = *cursor - 1
		if skip != nil && skip(*cursor) {
			if *cursor > 0 {
				*cursor = *cursor - 1
			}
		}
	}
}

func listJumpTop(cursor, scroll *int, total int, skip listSkipFn) {
	if total <= 0 {
		*cursor = 0
		*scroll = 0
		return
	}
	*cursor = 0
	*scroll = 0
	if skip != nil {
		listSkip(cursor, total, 1, skip)
	}
}

func listJumpBottom(cursor *int, total int, skip listSkipFn) {
	if total <= 0 {
		*cursor = 0
		return
	}
	*cursor = total - 1
	if skip != nil {
		listSkip(cursor, total, -1, skip)
	}
}

func listSkip(cursor *int, total, dir int, skip listSkipFn) {
	if total <= 0 {
		*cursor = 0
		return
	}
	if dir == 0 {
		dir = 1
	}
	for *cursor >= 0 && *cursor < total && skip(*cursor) {
		*cursor += dir
	}
	if *cursor < 0 {
		*cursor = 0
	}
	if *cursor >= total {
		*cursor = total - 1
	}
}

func listEnsureVisible(cursor int, scroll *int, visibleHeight int) {
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	if cursor < *scroll {
		*scroll = cursor
	}
	if cursor >= *scroll+visibleHeight {
		*scroll = cursor - visibleHeight + 1
	}
	if *scroll < 0 {
		*scroll = 0
	}
}
