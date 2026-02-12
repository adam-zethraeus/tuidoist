package main

type appMode int

const (
	appModeMain appMode = iota
	appModeHelp
	appModeSearch
	appModeActions
	appModeQueue
	appModeCompleted
	appModeTriage
)

func (m appMode) isOverlay() bool {
	return m != appModeMain
}
