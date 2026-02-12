package main

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// API response wrapper for paginated endpoints
type PaginatedResponse[T any] struct {
	Results    []T     `json:"results"`
	NextCursor *string `json:"next_cursor"`
}

// Project represents a Todoist project
type Project struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Color        string  `json:"color"`
	ParentID     *string `json:"parent_id"`
	ChildOrder   int     `json:"child_order"`
	IsFavorite   bool    `json:"is_favorite"`
	IsArchived   bool    `json:"is_archived"`
	IsDeleted    bool    `json:"is_deleted"`
	ViewStyle    string  `json:"view_style"`
	InboxProject bool    `json:"inbox_project"`
	Description  string  `json:"description"`
}

// Section represents a Todoist section
type Section struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Name         string `json:"name"`
	SectionOrder int    `json:"section_order"`
	IsArchived   bool   `json:"is_archived"`
	IsDeleted    bool   `json:"is_deleted"`
}

// Due represents a task due date
type Due struct {
	Date        string  `json:"date"`
	Timezone    *string `json:"timezone"`
	String      string  `json:"string"`
	Lang        string  `json:"lang"`
	IsRecurring bool    `json:"is_recurring"`
}

// Deadline represents a task deadline date (non-recurring date only).
type Deadline struct {
	Date string  `json:"date"`
	Lang *string `json:"lang"`
}

// Task represents a Todoist task
type Task struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	ProjectID      string    `json:"project_id"`
	SectionID      string    `json:"section_id"`
	ParentID       *string   `json:"parent_id"`
	AssignedByUID  *string   `json:"assigned_by_uid"`
	ResponsibleUID *string   `json:"responsible_uid"`
	Content        string    `json:"content"`
	Description    string    `json:"description"`
	Priority       int       `json:"priority"`
	Due            *Due      `json:"due"`
	Deadline       *Deadline `json:"deadline"`
	Labels         []string  `json:"labels"`
	ChildOrder     int       `json:"child_order"`
	Checked        bool      `json:"checked"`
	IsDeleted      bool      `json:"is_deleted"`
	AddedAt        string    `json:"added_at"`
	CompletedAt    *string   `json:"completed_at"`
	NoteCount      int       `json:"note_count"`
}

// Label represents a Todoist label
type Label struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	Order      int    `json:"order"`
	IsFavorite bool   `json:"is_favorite"`
}

// Comment represents a Todoist comment
type Comment struct {
	ID        string  `json:"id"`
	TaskID    *string `json:"task_id"`
	ProjectID *string `json:"project_id"`
	Content   string  `json:"content"`
	PostedAt  string  `json:"posted_at"`
}

// --- Mutation types ---

type MutationAction string

const (
	MutationCreate   MutationAction = "create"
	MutationUpdate   MutationAction = "update"
	MutationClose    MutationAction = "close"
	MutationDelete   MutationAction = "delete"
	MutationReopen   MutationAction = "reopen"
	MutationQuickAdd MutationAction = "quick_add"
)

type MutationStatus string

const (
	MutationPending    MutationStatus = "pending"
	MutationFlushing   MutationStatus = "flushing"
	MutationConflicted MutationStatus = "conflicted"
)

type Mutation struct {
	ID         int64
	EntityType string // "task"
	EntityID   string // task ID (or temp ID for creates)
	Action     MutationAction
	Payload    string // JSON of the request (createTaskRequest or updateTaskRequest)
	Snapshot   string // JSON of entity state at mutation time (empty for creates)
	Status     MutationStatus
	Conflict   string // JSON description of conflict (empty if none)
	CreatedAt  time.Time
	Attempts   int
}

// NewPendingID returns a temporary ID for optimistically created entities.
func NewPendingID() string {
	return "pending-" + uuid.New().String()
}

// IsPendingID checks if an ID is a temporary pending ID.
func IsPendingID(id string) bool {
	return strings.HasPrefix(id, "pending-")
}

// --- Message types for async Bubbletea commands ---

type projectsMsg struct {
	projects   []Project
	err        error
	fromCache  bool
	stale      bool
	lastSynced *time.Time
}

type tasksMsg struct {
	projectID  string
	tasks      []Task
	err        error
	fromCache  bool
	stale      bool
	lastSynced *time.Time
}

type sectionsMsg struct {
	projectID  string
	sections   []Section
	err        error
	fromCache  bool
	stale      bool
	lastSynced *time.Time
}

type labelsMsg struct {
	labels []Label
	err    error
}

type taskClosedMsg struct {
	taskID string
	err    error
}

type taskReopenedMsg struct {
	task Task
	err  error
}

type taskDeletedMsg struct {
	taskID string
	err    error
}

type taskCreatedMsg struct {
	task Task
	err  error
}

type taskUpdatedMsg struct {
	task Task
	err  error
}

type quickAddMsg struct {
	err       error
	task      *Task
	projectID string
}

type cachedProjectsMsg struct {
	projects   []Project
	stale      bool
	lastSynced *time.Time
}

type cachedTasksMsg struct {
	projectID  string
	tasks      []Task
	stale      bool
	lastSynced *time.Time
}

type cachedSectionsMsg struct {
	projectID  string
	sections   []Section
	stale      bool
	lastSynced *time.Time
}

type noopMsg struct{}

type mutationEnqueuedMsg struct{ count int }
type mutationFlushedMsg struct {
	mutation Mutation
	err      error
}
type mutationConflictMsg struct {
	mutation Mutation
	conflict string
}
type flushNextMsg struct{}

type backgroundRefreshMsg struct {
	staleProjects []string
}
type backgroundRefreshDoneMsg struct {
	remaining []string
}

type assigneeDirectoryMsg struct {
	updated int
	err     error
}

type commentsMsg struct {
	comments []Comment
	err      error
}

type projectCreatedMsg struct {
	project Project
	err     error
}

type projectArchivedMsg struct {
	projectID string
	err       error
}

type projectUnarchivedMsg struct {
	project Project
	err     error
}

// CompletedTaskRow holds a completed task with its project context.
type CompletedTaskRow struct {
	Task        Task
	ProjectID   string
	ProjectName string
	CompletedAt time.Time
}

type toastMsg struct {
	text    string
	isError bool
}

type clearToastMsg struct{}

type navigateToTaskMsg struct {
	projectID string
	taskID    string
}

type navigateToProjectMsg struct {
	projectID string
}

type tickMsg struct{}

// --- Helper functions ---

// formatDue returns a human-readable due date string with color hints
func formatDue(due *Due) string {
	if due == nil {
		return ""
	}
	if due.String != "" {
		return due.String
	}
	return due.Date
}

// formatDeadline returns a short deadline tag.
func formatDeadline(deadline *Deadline) string {
	if deadline == nil || deadline.Date == "" {
		return ""
	}
	return "by " + deadline.Date
}

// isOverdue checks if a due date is in the past
func isOverdue(due *Due) bool {
	if due == nil {
		return false
	}
	dateStr := due.Date
	if dateStr == "" {
		return false
	}

	now := time.Now()
	// Try full-day date first
	if len(dateStr) == 10 {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return false
		}
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return t.Before(today)
	}

	// Try datetime
	for _, layout := range []string{
		"2006-01-02T15:04:05.000000Z",
		"2006-01-02T15:04:05.000000",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	} {
		t, err := time.Parse(layout, dateStr)
		if err == nil {
			return t.Before(now)
		}
	}
	return false
}

// isDeadlineOverdue checks if a deadline date is in the past.
func isDeadlineOverdue(deadline *Deadline) bool {
	if deadline == nil || deadline.Date == "" {
		return false
	}
	t, err := time.Parse("2006-01-02", deadline.Date)
	if err != nil {
		return false
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return t.Before(today)
}

// isDueToday checks if a due date is today
func isDueToday(due *Due) bool {
	if due == nil {
		return false
	}
	dateStr := due.Date
	if dateStr == "" {
		return false
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	return strings.HasPrefix(dateStr, today)
}

// isDeadlineToday checks if a deadline date is today.
func isDeadlineToday(deadline *Deadline) bool {
	if deadline == nil || deadline.Date == "" {
		return false
	}
	return deadline.Date == time.Now().Format("2006-01-02")
}

func isTaskOverdue(task *Task) bool {
	if task == nil {
		return false
	}
	return isOverdue(task.Due) || isDeadlineOverdue(task.Deadline)
}

func formatAssignee(task *Task, nameMap map[string]string) string {
	if task == nil || task.ResponsibleUID == nil || *task.ResponsibleUID == "" {
		return ""
	}
	uid := *task.ResponsibleUID
	label := uid
	if nameMap != nil {
		if name, ok := nameMap[uid]; ok && name != "" {
			label = name
		}
	}
	if len(label) > 18 {
		label = label[:18] + "..."
	}
	return "+" + label
}

// priorityLabel returns a display string for priority
func priorityLabel(p int) string {
	switch p {
	case 1:
		return "p1"
	case 2:
		return "p2"
	case 3:
		return "p3"
	default:
		return ""
	}
}

// truncate clips a string to maxLen with ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// colorHex maps Todoist color names to hex values
var colorHex = map[string]string{
	"berry_red":   "#B8255F",
	"red":         "#DC4C3E",
	"orange":      "#C77100",
	"yellow":      "#B29104",
	"olive_green": "#949C31",
	"lime_green":  "#65A33A",
	"green":       "#369307",
	"mint_green":  "#42A393",
	"teal":        "#148FAD",
	"sky_blue":    "#319DC0",
	"light_blue":  "#6988A4",
	"blue":        "#4180FF",
	"grape":       "#692EC2",
	"violet":      "#CA3FEE",
	"lavender":    "#A4698C",
	"magenta":     "#E05095",
	"salmon":      "#C9766F",
	"charcoal":    "#808080",
	"grey":        "#999999",
	"taupe":       "#8F7A69",
}
