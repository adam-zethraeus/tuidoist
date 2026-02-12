package main

type appMode int

const (
	appModeMain appMode = iota
	appModeHelp
	appModeSearch
	appModeQueue
	appModeCompleted
	appModeTriage
)

func (m appMode) isOverlay() bool {
	return m != appModeMain
}
