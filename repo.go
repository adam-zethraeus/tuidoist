package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Repository orchestrates cache-first reads and write-through mutations.
type Repository struct {
	client *Client
	store  *Store
}

// NewRepository creates a Repository. store may be nil (falls back to direct API).
func NewRepository(client *Client, store *Store) *Repository {
	return &Repository{client: client, store: store}
}

// --- Synchronous cache access ---

// GetCachedTasks returns tasks from cache synchronously. Returns nil if unavailable.
func (r *Repository) GetCachedTasks(projectID string) []Task {
	if r.store == nil {
		return nil
	}
	tasks, _ := r.store.GetTasks(projectID)
	return tasks
}

// GetCachedSections returns sections from cache synchronously. Returns nil if unavailable.
func (r *Repository) GetCachedSections(projectID string) []Section {
	if r.store == nil {
		return nil
	}
	sections, _ := r.store.GetSections(projectID)
	return sections
}

// GetCachedProjects returns all projects from cache synchronously.
func (r *Repository) GetCachedProjects() []Project {
	if r.store == nil {
		return nil
	}
	projects, _ := r.store.GetProjects()
	return projects
}

// GetAllCachedTasks returns all tasks from cache across all projects.
func (r *Repository) GetAllCachedTasks() []Task {
	if r.store == nil {
		return nil
	}
	tasks, _ := r.store.GetAllTasks()
	return tasks
}

// GetProjectNameMap returns a map of project ID to project name.
func (r *Repository) GetProjectNameMap() map[string]string {
	if r.store == nil {
		return nil
	}
	projects, _ := r.store.GetProjects()
	m := make(map[string]string, len(projects))
	for _, p := range projects {
		m[p.ID] = p.Name
	}
	return m
}

// GetAssigneeNameMap returns cached assignee names keyed by user ID.
func (r *Repository) GetAssigneeNameMap() map[string]string {
	if r.store == nil {
		return nil
	}
	names, _ := r.store.GetUserNames()
	return names
}

// --- Two-phase reads ---

// FetchProjects returns cached projects instantly if available (stale or not),
// then triggers a background refresh. If the cache is fresh, returns projectsMsg directly.
func (r *Repository) FetchProjects() tea.Cmd {
	return func() tea.Msg {
		if r.store != nil {
			stale := r.store.IsStale("projects", "")
			lastSynced, _ := r.store.LastSynced("projects", "")
			if !stale {
				projects, err := r.store.GetProjects()
				if err == nil {
					return projectsMsg{
						projects:   projects,
						fromCache:  true,
						stale:      false,
						lastSynced: lastSynced,
					}
				}
			}

			// Stale but has data — return cached, view will trigger RefreshProjects
			projects, err := r.store.GetProjects()
			if err == nil && len(projects) > 0 {
				return cachedProjectsMsg{
					projects:   projects,
					stale:      true,
					lastSynced: lastSynced,
				}
			}
		}

		// Empty cache — block on API
		return r.fetchProjectsFromAPI()
	}
}

// RefreshProjects forces an API fetch and updates the cache.
func (r *Repository) RefreshProjects() tea.Cmd {
	return func() tea.Msg {
		return r.fetchProjectsFromAPI()
	}
}

func (r *Repository) fetchProjectsFromAPI() projectsMsg {
	projects, err := r.client.GetProjects(context.Background())
	if err != nil {
		return projectsMsg{err: err}
	}
	if r.store != nil {
		_ = r.store.ReplaceProjects(projects)
	}
	now := time.Now()
	return projectsMsg{projects: projects, fromCache: false, stale: false, lastSynced: &now}
}

// FetchTasks returns cached tasks instantly if available, then triggers refresh.
func (r *Repository) FetchTasks(projectID string) tea.Cmd {
	return func() tea.Msg {
		if r.store != nil {
			stale := r.store.IsStale("tasks", projectID)
			lastSynced, _ := r.store.LastSynced("tasks", projectID)
			if !stale {
				tasks, err := r.store.GetTasks(projectID)
				if err == nil {
					return tasksMsg{
						projectID:  projectID,
						tasks:      tasks,
						fromCache:  true,
						stale:      false,
						lastSynced: lastSynced,
					}
				}
			}

			tasks, err := r.store.GetTasks(projectID)
			if err == nil && len(tasks) > 0 {
				return cachedTasksMsg{
					projectID:  projectID,
					tasks:      tasks,
					stale:      true,
					lastSynced: lastSynced,
				}
			}
		}

		return r.fetchTasksFromAPI(projectID)
	}
}

// RefreshTasks forces an API fetch for tasks.
func (r *Repository) RefreshTasks(projectID string) tea.Cmd {
	return func() tea.Msg {
		return r.fetchTasksFromAPI(projectID)
	}
}

func (r *Repository) fetchTasksFromAPI(projectID string) tasksMsg {
	tasks, err := r.client.GetTasks(context.Background(), projectID)
	if err != nil {
		return tasksMsg{projectID: projectID, err: err}
	}
	if r.store != nil {
		_ = r.store.ReplaceTasks(projectID, tasks)
	}
	now := time.Now()
	return tasksMsg{projectID: projectID, tasks: tasks, fromCache: false, stale: false, lastSynced: &now}
}

// FetchSections returns cached sections instantly if available.
func (r *Repository) FetchSections(projectID string) tea.Cmd {
	return func() tea.Msg {
		if r.store != nil {
			stale := r.store.IsStale("sections", projectID)
			lastSynced, _ := r.store.LastSynced("sections", projectID)
			if !stale {
				sections, err := r.store.GetSections(projectID)
				if err == nil {
					return sectionsMsg{
						projectID:  projectID,
						sections:   sections,
						fromCache:  true,
						stale:      false,
						lastSynced: lastSynced,
					}
				}
			}

			sections, err := r.store.GetSections(projectID)
			if err == nil && len(sections) > 0 {
				return cachedSectionsMsg{
					projectID:  projectID,
					sections:   sections,
					stale:      true,
					lastSynced: lastSynced,
				}
			}
		}

		return r.fetchSectionsFromAPI(projectID)
	}
}

// RefreshSections forces an API fetch for sections.
func (r *Repository) RefreshSections(projectID string) tea.Cmd {
	return func() tea.Msg {
		return r.fetchSectionsFromAPI(projectID)
	}
}

func (r *Repository) fetchSectionsFromAPI(projectID string) sectionsMsg {
	sections, err := r.client.GetSections(context.Background(), projectID)
	if err != nil {
		return sectionsMsg{projectID: projectID, err: err}
	}
	if r.store != nil {
		_ = r.store.ReplaceSections(projectID, sections)
	}
	now := time.Now()
	return sectionsMsg{projectID: projectID, sections: sections, fromCache: false, stale: false, lastSynced: &now}
}

// --- Optimistic mutations ---

// CloseTask optimistically removes a task from cache, saves to completed, and enqueues a close mutation.
func (r *Repository) CloseTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		if IsPendingID(taskID) {
			return toastMsg{text: "Task is still syncing, please wait", isError: true}
		}
		snapshot := r.snapshotTask(taskID)
		if r.store != nil {
			// Save to completed_tasks before deleting
			if task, err := r.store.GetTaskByID(taskID); err == nil && task != nil {
				projectName := r.projectNameForID(task.ProjectID)
				_ = r.store.SaveCompletedTask(*task, projectName)
			}
			_ = r.store.DeleteTask(taskID)
			_, _ = r.store.EnqueueMutation(Mutation{
				EntityType: "task",
				EntityID:   taskID,
				Action:     MutationClose,
				Snapshot:   snapshot,
				Status:     MutationPending,
				CreatedAt:  time.Now(),
			})
		}
		return taskClosedMsg{taskID: taskID, err: nil}
	}
}

// ReopenTask optimistically moves a task from completed back to active cache and enqueues a reopen mutation.
func (r *Repository) ReopenTask(task Task) tea.Cmd {
	return func() tea.Msg {
		if IsPendingID(task.ID) {
			return toastMsg{text: "Task is still syncing, please wait", isError: true}
		}
		snapshotBlob, _ := json.Marshal(task)
		if r.store != nil {
			_ = r.store.UpsertTask(task)
			_ = r.store.DeleteCompletedTask(task.ID)
			_, _ = r.store.EnqueueMutation(Mutation{
				EntityType: "task",
				EntityID:   task.ID,
				Action:     MutationReopen,
				Snapshot:   string(snapshotBlob),
				Status:     MutationPending,
				CreatedAt:  time.Now(),
			})
		}
		return taskReopenedMsg{task: task, err: nil}
	}
}

// DeleteTask optimistically removes a task from cache and enqueues a delete mutation.
func (r *Repository) DeleteTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		if IsPendingID(taskID) {
			return toastMsg{text: "Task is still syncing, please wait", isError: true}
		}
		snapshot := r.snapshotTask(taskID)
		if r.store != nil {
			_ = r.store.DeleteTask(taskID)
			_, _ = r.store.EnqueueMutation(Mutation{
				EntityType: "task",
				EntityID:   taskID,
				Action:     MutationDelete,
				Snapshot:   snapshot,
				Status:     MutationPending,
				CreatedAt:  time.Now(),
			})
		}
		return taskDeletedMsg{taskID: taskID, err: nil}
	}
}

// CreateTask optimistically inserts a temp task into cache and enqueues a create mutation.
func (r *Repository) CreateTask(req createTaskRequest) tea.Cmd {
	return func() tea.Msg {
		tempID := NewPendingID()
		tempTask := Task{
			ID:        tempID,
			Content:   req.Content,
			ProjectID: req.ProjectID,
			SectionID: req.SectionID,
			Priority:  req.Priority,
			Labels:    req.Labels,
		}
		if req.DueString != "" {
			tempTask.Due = &Due{String: req.DueString}
		}
		if req.DeadlineDate != "" {
			tempTask.Deadline = &Deadline{Date: req.DeadlineDate}
		}
		if r.store != nil {
			_ = r.store.UpsertTask(tempTask)
			payload, _ := json.Marshal(req)
			_, _ = r.store.EnqueueMutation(Mutation{
				EntityType: "task",
				EntityID:   tempID,
				Action:     MutationCreate,
				Payload:    string(payload),
				Status:     MutationPending,
				CreatedAt:  time.Now(),
			})
		}
		return taskCreatedMsg{task: tempTask, err: nil}
	}
}

// UpdateTask optimistically updates cache and enqueues an update mutation.
func (r *Repository) UpdateTask(taskID string, req updateTaskRequest) tea.Cmd {
	return func() tea.Msg {
		if IsPendingID(taskID) {
			return toastMsg{text: "Task is still syncing, please wait", isError: true}
		}
		snapshot := r.snapshotTask(taskID)
		updated := r.applyUpdateToCache(taskID, req)
		payload, _ := json.Marshal(req)
		if r.store != nil {
			_, _ = r.store.EnqueueMutation(Mutation{
				EntityType: "task",
				EntityID:   taskID,
				Action:     MutationUpdate,
				Payload:    string(payload),
				Snapshot:   snapshot,
				Status:     MutationPending,
				CreatedAt:  time.Now(),
			})
		}
		return taskUpdatedMsg{task: updated, err: nil}
	}
}

type quickAddMutationPayload struct {
	Text      string `json:"text"`
	TempID    string `json:"temp_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// QuickAdd creates a task via natural language and optimistically inserts a temp task when project context is known.
func (r *Repository) QuickAdd(text string, defaultProjectID string) tea.Cmd {
	return func() tea.Msg {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return quickAddMsg{err: nil}
		}

		if r.store == nil {
			_, err := r.client.QuickAdd(context.Background(), trimmed)
			return quickAddMsg{err: err, projectID: defaultProjectID}
		}

		payload := quickAddMutationPayload{Text: trimmed, ProjectID: defaultProjectID}
		var tempTask *Task
		if defaultProjectID != "" {
			temp := Task{
				ID:        NewPendingID(),
				Content:   trimmed,
				ProjectID: defaultProjectID,
				Priority:  4,
			}
			_ = r.store.UpsertTask(temp)
			payload.TempID = temp.ID
			tempTask = &temp
		}

		body, _ := json.Marshal(payload)
		_, _ = r.store.EnqueueMutation(Mutation{
			EntityType: "task",
			EntityID:   payload.TempID,
			Action:     MutationQuickAdd,
			Payload:    string(body),
			Status:     MutationPending,
			CreatedAt:  time.Now(),
		})

		return quickAddMsg{err: nil, task: tempTask, projectID: defaultProjectID}
	}
}

// --- Sync count helpers ---

func (r *Repository) PendingCount() int {
	if r.store == nil {
		return 0
	}
	return r.store.PendingCount() + r.store.FlushingCount()
}

func (r *Repository) ConflictCount() int {
	if r.store == nil {
		return 0
	}
	return r.store.ConflictCount()
}

// --- Flush logic ---

// FlushNext picks the oldest pending mutation and flushes it to the API.
func (r *Repository) FlushNext() tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return noopMsg{}
		}
		m, err := r.store.NextPendingMutation()
		if m == nil || err != nil {
			return noopMsg{}
		}

		_ = r.store.UpdateMutationStatus(m.ID, MutationFlushing, "")
		_ = r.store.IncrementMutationAttempts(m.ID)

		switch m.Action {
		case MutationCreate:
			return r.flushCreate(*m)
		case MutationQuickAdd:
			return r.flushQuickAdd(*m)
		case MutationUpdate:
			return r.flushUpdate(*m)
		case MutationClose:
			return r.flushClose(*m)
		case MutationDelete:
			return r.flushDelete(*m)
		case MutationReopen:
			return r.flushReopen(*m)
		}
		return noopMsg{}
	}
}

func (r *Repository) flushCreate(m Mutation) tea.Msg {
	var req createTaskRequest
	if err := json.Unmarshal([]byte(m.Payload), &req); err != nil {
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "invalid payload: "+err.Error())
		return mutationConflictMsg{mutation: m, conflict: "invalid payload"}
	}

	task, err := r.client.CreateTask(context.Background(), req)
	if err != nil {
		if isRetriableMutationError(err) {
			_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
			return mutationFlushedMsg{mutation: m, err: err}
		}
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "API error: "+err.Error())
		return mutationConflictMsg{mutation: m, conflict: err.Error()}
	}

	// Replace temp task with real task in cache
	if r.store != nil {
		_ = r.store.DeleteTask(m.EntityID) // remove temp
		_ = r.store.UpsertTask(task)       // insert real
	}
	_ = r.store.DeleteMutation(m.ID)
	return mutationFlushedMsg{mutation: m, err: nil}
}

func (r *Repository) flushQuickAdd(m Mutation) tea.Msg {
	var payload quickAddMutationPayload
	if err := json.Unmarshal([]byte(m.Payload), &payload); err != nil {
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "invalid payload: "+err.Error())
		return mutationConflictMsg{mutation: m, conflict: "invalid payload"}
	}

	task, err := r.client.QuickAdd(context.Background(), payload.Text)
	if err != nil {
		if isRetriableMutationError(err) {
			_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
			return mutationFlushedMsg{mutation: m, err: err}
		}
		// For quick add, promote to conflict so the user gets explicit visibility and can retry/dismiss.
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "API error: "+err.Error())
		// Remove temp placeholder if we inserted one.
		if payload.TempID != "" && r.store != nil {
			_ = r.store.DeleteTask(payload.TempID)
		}
		return mutationConflictMsg{mutation: m, conflict: err.Error()}
	}

	if r.store != nil {
		if payload.TempID != "" {
			_ = r.store.DeleteTask(payload.TempID)
		}
		_ = r.store.UpsertTask(task)
	}
	_ = r.store.DeleteMutation(m.ID)
	return mutationFlushedMsg{mutation: m, err: nil}
}

func (r *Repository) flushUpdate(m Mutation) tea.Msg {
	var req updateTaskRequest
	if err := json.Unmarshal([]byte(m.Payload), &req); err != nil {
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "invalid payload: "+err.Error())
		return mutationConflictMsg{mutation: m, conflict: "invalid payload"}
	}

	// Get current server state for conflict detection
	serverTask, err := r.client.GetTask(context.Background(), m.EntityID)
	if err != nil {
		if isNotFoundError(err) {
			_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "task deleted on server")
			return mutationConflictMsg{mutation: m, conflict: "task deleted on server"}
		}
		if isRetriableMutationError(err) {
			_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
			return mutationFlushedMsg{mutation: m, err: err}
		}
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "API error: "+err.Error())
		return mutationConflictMsg{mutation: m, conflict: err.Error()}
	}

	// Snapshot-based conflict detection
	var snapshotTask Task
	if m.Snapshot != "" {
		_ = json.Unmarshal([]byte(m.Snapshot), &snapshotTask)
	}

	if conflict := r.detectConflict(snapshotTask, serverTask, req); conflict != nil {
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, *conflict)
		return mutationConflictMsg{mutation: m, conflict: *conflict}
	}

	// No conflict — apply update
	task, err := r.client.UpdateTask(context.Background(), m.EntityID, req)
	if err != nil {
		if isRetriableMutationError(err) {
			_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
			return mutationFlushedMsg{mutation: m, err: err}
		}
		_ = r.restoreTaskFromSnapshot(m)
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "API error: "+err.Error())
		return mutationConflictMsg{mutation: m, conflict: err.Error()}
	}

	if r.store != nil {
		_ = r.store.UpsertTask(task)
	}
	_ = r.store.DeleteMutation(m.ID)
	return mutationFlushedMsg{mutation: m, err: nil}
}

func (r *Repository) flushClose(m Mutation) tea.Msg {
	err := r.client.CloseTask(context.Background(), m.EntityID)
	if err != nil {
		if isNotFoundError(err) {
			// Task already gone — not a conflict
			_ = r.store.DeleteMutation(m.ID)
			return mutationFlushedMsg{mutation: m, err: nil}
		}
		if isRetriableMutationError(err) {
			_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
			return mutationFlushedMsg{mutation: m, err: err}
		}
		// Close is user-visible state. Roll back and mark conflicted so it is explicit.
		_ = r.restoreTaskFromSnapshot(m)
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "API error: "+err.Error())
		return mutationConflictMsg{mutation: m, conflict: err.Error()}
	}
	_ = r.store.DeleteMutation(m.ID)
	return mutationFlushedMsg{mutation: m, err: nil}
}

func (r *Repository) flushDelete(m Mutation) tea.Msg {
	err := r.client.DeleteTask(context.Background(), m.EntityID)
	if err != nil {
		if isNotFoundError(err) {
			_ = r.store.DeleteMutation(m.ID)
			return mutationFlushedMsg{mutation: m, err: nil}
		}
		if isRetriableMutationError(err) {
			_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
			return mutationFlushedMsg{mutation: m, err: err}
		}
		_ = r.restoreTaskFromSnapshot(m)
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "API error: "+err.Error())
		return mutationConflictMsg{mutation: m, conflict: err.Error()}
	}
	_ = r.store.DeleteMutation(m.ID)
	return mutationFlushedMsg{mutation: m, err: nil}
}

func (r *Repository) flushReopen(m Mutation) tea.Msg {
	err := r.client.ReopenTask(context.Background(), m.EntityID)
	if err != nil {
		if isNotFoundError(err) {
			_ = r.store.DeleteMutation(m.ID)
			return mutationFlushedMsg{mutation: m, err: nil}
		}
		if isRetriableMutationError(err) {
			_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
			return mutationFlushedMsg{mutation: m, err: err}
		}
		_ = r.rollbackReopen(m)
		_ = r.store.UpdateMutationStatus(m.ID, MutationConflicted, "API error: "+err.Error())
		return mutationConflictMsg{mutation: m, conflict: err.Error()}
	}
	_ = r.store.DeleteMutation(m.ID)
	return mutationFlushedMsg{mutation: m, err: nil}
}

// --- Snapshot and conflict helpers ---

func (r *Repository) snapshotTask(taskID string) string {
	if r.store == nil {
		return ""
	}
	task, err := r.store.GetTaskByID(taskID)
	if err != nil || task == nil {
		return ""
	}
	blob, _ := json.Marshal(task)
	return string(blob)
}

func (r *Repository) restoreTaskFromSnapshot(m Mutation) error {
	if r.store == nil || m.Snapshot == "" {
		return nil
	}
	var t Task
	if err := json.Unmarshal([]byte(m.Snapshot), &t); err != nil {
		return err
	}
	_ = r.store.DeleteCompletedTask(t.ID)
	return r.store.UpsertTask(t)
}

func (r *Repository) rollbackReopen(m Mutation) error {
	if r.store == nil {
		return nil
	}
	// Remove from active list if present.
	_ = r.store.DeleteTask(m.EntityID)

	// Try to restore completed row from snapshot when available.
	if m.Snapshot == "" {
		return nil
	}
	var t Task
	if err := json.Unmarshal([]byte(m.Snapshot), &t); err != nil {
		return err
	}
	projectName := r.projectNameForID(t.ProjectID)
	return r.store.SaveCompletedTask(t, projectName)
}

func (r *Repository) applyUpdateToCache(taskID string, req updateTaskRequest) Task {
	var task Task
	if r.store != nil {
		if t, err := r.store.GetTaskByID(taskID); err == nil && t != nil {
			task = *t
		}
	}

	if req.Content != nil {
		task.Content = *req.Content
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.DueString != nil {
		if *req.DueString == "" {
			task.Due = nil
		} else {
			task.Due = &Due{String: *req.DueString}
		}
	}
	if req.ClearDeadline {
		task.Deadline = nil
	} else if req.DeadlineDate != nil {
		if *req.DeadlineDate == "" {
			task.Deadline = nil
		} else {
			task.Deadline = &Deadline{Date: *req.DeadlineDate}
		}
	}
	if req.Labels != nil {
		task.Labels = req.Labels
	}

	if r.store != nil {
		_ = r.store.UpsertTask(task)
	}
	return task
}

func (r *Repository) detectConflict(snapshot, server Task, req updateTaskRequest) *string {
	var conflicts []string

	if req.Content != nil && snapshot.Content != server.Content {
		conflicts = append(conflicts, fmt.Sprintf(
			"content: you changed %q→%q, server has %q",
			snapshot.Content, *req.Content, server.Content))
	}
	if req.Priority != nil && snapshot.Priority != server.Priority {
		conflicts = append(conflicts, fmt.Sprintf(
			"priority: you changed %d→%d, server has %d",
			snapshot.Priority, *req.Priority, server.Priority))
	}
	if req.Description != nil && snapshot.Description != server.Description {
		conflicts = append(conflicts, fmt.Sprintf(
			"description: you changed %q→%q, server has %q",
			snapshot.Description, *req.Description, server.Description))
	}
	if req.DueString != nil {
		snapshotDue := ""
		serverDue := ""
		if snapshot.Due != nil {
			snapshotDue = snapshot.Due.String
		}
		if server.Due != nil {
			serverDue = server.Due.String
		}
		if snapshotDue != serverDue {
			conflicts = append(conflicts, fmt.Sprintf(
				"due: you changed %q→%q, server has %q",
				snapshotDue, *req.DueString, serverDue))
		}
	}
	if req.ClearDeadline || req.DeadlineDate != nil {
		snapshotDeadline := ""
		serverDeadline := ""
		targetDeadline := ""
		if snapshot.Deadline != nil {
			snapshotDeadline = snapshot.Deadline.Date
		}
		if server.Deadline != nil {
			serverDeadline = server.Deadline.Date
		}
		if req.ClearDeadline {
			targetDeadline = "<clear>"
		} else if req.DeadlineDate != nil {
			targetDeadline = *req.DeadlineDate
		}
		if snapshotDeadline != serverDeadline {
			conflicts = append(conflicts, fmt.Sprintf(
				"deadline: you changed %q→%q, server has %q",
				snapshotDeadline, targetDeadline, serverDeadline))
		}
	}
	if req.Labels != nil {
		snapshotLabels := strings.Join(snapshot.Labels, ",")
		serverLabels := strings.Join(server.Labels, ",")
		if snapshotLabels != serverLabels {
			conflicts = append(conflicts, fmt.Sprintf(
				"labels: snapshot=%q, server=%q",
				snapshotLabels, serverLabels))
		}
	}

	if len(conflicts) == 0 {
		return nil
	}
	result := strings.Join(conflicts, "; ")
	return &result
}

// --- Mutation queue access for queue view ---

func (r *Repository) GetAllMutations() []Mutation {
	if r.store == nil {
		return nil
	}
	mutations, _ := r.store.GetAllMutations()
	return mutations
}

// TaskMutationStatusMap returns the strongest mutation status for each task entity ID.
func (r *Repository) TaskMutationStatusMap() map[string]MutationStatus {
	out := make(map[string]MutationStatus)
	if r.store == nil {
		return out
	}
	mutations, err := r.store.GetAllMutations()
	if err != nil {
		return out
	}
	weight := func(s MutationStatus) int {
		switch s {
		case MutationConflicted:
			return 3
		case MutationFlushing:
			return 2
		case MutationPending:
			return 1
		default:
			return 0
		}
	}
	for _, m := range mutations {
		if m.EntityType != "task" || m.EntityID == "" {
			continue
		}
		prev, ok := out[m.EntityID]
		if !ok || weight(m.Status) >= weight(prev) {
			out[m.EntityID] = m.Status
		}
	}
	return out
}

func (r *Repository) RetryMutation(id int64) tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return noopMsg{}
		}
		if muts, err := r.store.GetAllMutations(); err == nil {
			for _, m := range muts {
				if m.ID != id {
					continue
				}
				switch m.Action {
				case MutationClose:
					_ = r.restoreTaskFromSnapshot(m)
				case MutationReopen:
					_ = r.rollbackReopen(m)
				}
				break
			}
		}
		_ = r.store.UpdateMutationStatus(id, MutationPending, "")
		return flushNextMsg{}
	}
}

func (r *Repository) restoreForDismiss(m Mutation) {
	switch m.Action {
	case MutationClose:
		_ = r.restoreTaskFromSnapshot(m)
	case MutationReopen:
		_ = r.rollbackReopen(m)
	}
}

func (r *Repository) DismissMutation(id int64) tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return noopMsg{}
		}
		if muts, err := r.store.GetAllMutations(); err == nil {
			for _, m := range muts {
				if m.ID != id {
					continue
				}
				r.restoreForDismiss(m)
				break
			}
		}
		_ = r.store.DeleteMutation(id)
		return mutationEnqueuedMsg{count: r.store.PendingCount()}
	}
}

func (r *Repository) DismissConflictedMutations() tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return noopMsg{}
		}
		muts, err := r.store.GetConflictedMutations()
		if err != nil {
			return noopMsg{}
		}
		for _, m := range muts {
			r.restoreForDismiss(m)
			_ = r.store.DeleteMutation(m.ID)
		}
		return mutationEnqueuedMsg{count: r.store.PendingCount()}
	}
}

func (r *Repository) DismissAllMutations() tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return noopMsg{}
		}
		muts, err := r.store.GetAllMutations()
		if err != nil {
			return noopMsg{}
		}
		for _, m := range muts {
			r.restoreForDismiss(m)
			_ = r.store.DeleteMutation(m.ID)
		}
		return mutationEnqueuedMsg{count: r.store.PendingCount()}
	}
}

// --- Project operations (direct API, no mutation queue) ---

func (r *Repository) CreateProject(name string) tea.Cmd {
	return func() tea.Msg {
		project, err := r.client.CreateProject(context.Background(), createProjectRequest{Name: name})
		if err != nil {
			return projectCreatedMsg{err: err}
		}
		return projectCreatedMsg{project: project}
	}
}

func (r *Repository) ArchiveProject(projectID string) tea.Cmd {
	return func() tea.Msg {
		// Save project data before archiving
		if r.store != nil {
			projects, _ := r.store.GetProjects()
			for _, p := range projects {
				if p.ID == projectID {
					_ = r.store.SaveArchivedProject(p)
					break
				}
			}
		}
		err := r.client.ArchiveProject(context.Background(), projectID)
		if err != nil {
			// Remove from archived on failure
			if r.store != nil {
				_ = r.store.DeleteArchivedProject(projectID)
			}
			return projectArchivedMsg{projectID: projectID, err: err}
		}
		// Remove from active projects cache
		if r.store != nil {
			_ = r.store.DeleteProject(projectID)
		}
		return projectArchivedMsg{projectID: projectID, err: nil}
	}
}

func (r *Repository) UnarchiveProject(projectID string) tea.Cmd {
	return func() tea.Msg {
		err := r.client.UnarchiveProject(context.Background(), projectID)
		if err != nil {
			return projectUnarchivedMsg{err: err}
		}
		if r.store != nil {
			_ = r.store.DeleteArchivedProject(projectID)
		}
		// Fetch the unarchived project to return it
		projects, _ := r.client.GetProjects(context.Background())
		var found Project
		for _, p := range projects {
			if p.ID == projectID {
				found = p
				break
			}
		}
		return projectUnarchivedMsg{project: found}
	}
}

// --- Completed/Archived access ---

func (r *Repository) GetRecentlyCompleted(limit int) []CompletedTaskRow {
	if r.store == nil {
		return nil
	}
	rows, _ := r.store.GetRecentlyCompleted(limit)
	return rows
}

func (r *Repository) GetArchivedProjects() []Project {
	if r.store == nil {
		return nil
	}
	projects, _ := r.store.GetArchivedProjects()
	return projects
}

func (r *Repository) projectNameForID(projectID string) string {
	if r.store == nil {
		return ""
	}
	projects, _ := r.store.GetProjects()
	for _, p := range projects {
		if p.ID == projectID {
			return p.Name
		}
	}
	return ""
}

// RefreshAssigneeDirectory updates the local assignee name cache from Todoist user directories.
func (r *Repository) RefreshAssigneeDirectory() tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return assigneeDirectoryMsg{}
		}

		names, _ := r.store.GetUserNames()
		if names == nil {
			names = make(map[string]string)
		}
		updated := 0
		var errs []string

		if me, err := r.client.GetCurrentUserDirectoryEntry(context.Background()); err != nil {
			errs = append(errs, "user lookup failed: "+err.Error())
		} else if me != nil && me.ID != "" && me.Name != "" {
			if names[me.ID] != me.Name {
				updated++
			}
			names[me.ID] = me.Name
		}

		workspaceLookupOK := false
		if users, err := r.client.GetWorkspaceUsers(context.Background()); err != nil {
			errs = append(errs, "workspace users lookup failed: "+err.Error())
		} else {
			workspaceLookupOK = true
			for _, u := range users {
				if u.ID == "" || u.Name == "" {
					continue
				}
				if names[u.ID] != u.Name {
					updated++
				}
				names[u.ID] = u.Name
			}
		}

		unresolved := r.unresolvedAssigneeIDs(names)
		if len(unresolved) > 0 {
			for _, projectID := range r.projectsWithAssignees(unresolved) {
				users, err := r.client.GetProjectCollaborators(context.Background(), projectID)
				if err != nil {
					// Avoid noisy errors when workspace lookup already succeeded.
					if !workspaceLookupOK {
						errs = append(errs, "project collaborators lookup failed: "+err.Error())
					}
					continue
				}
				for _, u := range users {
					if u.ID == "" || u.Name == "" {
						continue
					}
					if names[u.ID] != u.Name {
						updated++
					}
					names[u.ID] = u.Name
				}
			}
		}

		if err := r.store.UpsertUserNames(names); err != nil {
			return assigneeDirectoryMsg{err: err}
		}

		var err error
		if len(errs) > 0 && updated == 0 {
			err = errors.New(strings.Join(errs, "; "))
		}
		return assigneeDirectoryMsg{updated: updated, err: err}
	}
}

func (r *Repository) unresolvedAssigneeIDs(names map[string]string) map[string]bool {
	out := make(map[string]bool)
	if r.store == nil {
		return out
	}
	tasks, err := r.store.GetAllTasks()
	if err != nil {
		return out
	}
	for _, t := range tasks {
		if t.ResponsibleUID == nil || *t.ResponsibleUID == "" {
			continue
		}
		if _, ok := names[*t.ResponsibleUID]; ok {
			continue
		}
		out[*t.ResponsibleUID] = true
	}
	return out
}

func (r *Repository) projectsWithAssignees(assigneeIDs map[string]bool) []string {
	if r.store == nil || len(assigneeIDs) == 0 {
		return nil
	}
	tasks, err := r.store.GetAllTasks()
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	for _, t := range tasks {
		if t.ResponsibleUID == nil || *t.ResponsibleUID == "" {
			continue
		}
		if !assigneeIDs[*t.ResponsibleUID] || t.ProjectID == "" {
			continue
		}
		seen[t.ProjectID] = true
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

// --- Background cache warming ---

// FindStaleProjects returns a list of project IDs whose task or section cache is past TTL.
func (r *Repository) FindStaleProjects() tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return noopMsg{}
		}
		projects, err := r.store.GetProjects()
		if err != nil || len(projects) == 0 {
			return noopMsg{}
		}
		var stale []string
		for _, p := range projects {
			if r.store.IsStale("tasks", p.ID) || r.store.IsStale("sections", p.ID) {
				stale = append(stale, p.ID)
			}
		}
		if len(stale) == 0 {
			return noopMsg{}
		}
		return backgroundRefreshMsg{staleProjects: stale}
	}
}

// BackgroundRefreshProject warms the cache for a single project.
// It skips projects that have become fresh since queueing (e.g. user navigated there).
func (r *Repository) BackgroundRefreshProject(projectID string, remaining []string) tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return backgroundRefreshDoneMsg{remaining: remaining}
		}
		taskStale := r.store.IsStale("tasks", projectID)
		sectionStale := r.store.IsStale("sections", projectID)

		// Skip if both are already fresh
		if !taskStale && !sectionStale {
			return backgroundRefreshDoneMsg{remaining: remaining}
		}

		// Refresh whichever is stale
		if taskStale {
			r.fetchTasksFromAPI(projectID)
		}
		if sectionStale {
			r.fetchSectionsFromAPI(projectID)
		}
		return backgroundRefreshDoneMsg{remaining: remaining}
	}
}
