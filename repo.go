package main

import (
	"context"
	"encoding/json"
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

// --- Two-phase reads ---

// FetchProjects returns cached projects instantly if available (stale or not),
// then triggers a background refresh. If the cache is fresh, returns projectsMsg directly.
func (r *Repository) FetchProjects() tea.Cmd {
	return func() tea.Msg {
		if r.store != nil {
			if !r.store.IsStale("projects", "") {
				projects, err := r.store.GetProjects()
				if err == nil {
					return projectsMsg{projects: projects}
				}
			}

			// Stale but has data — return cached, view will trigger RefreshProjects
			projects, err := r.store.GetProjects()
			if err == nil && len(projects) > 0 {
				return cachedProjectsMsg{projects: projects}
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
	return projectsMsg{projects: projects}
}

// FetchTasks returns cached tasks instantly if available, then triggers refresh.
func (r *Repository) FetchTasks(projectID string) tea.Cmd {
	return func() tea.Msg {
		if r.store != nil {
			if !r.store.IsStale("tasks", projectID) {
				tasks, err := r.store.GetTasks(projectID)
				if err == nil {
					return tasksMsg{projectID: projectID, tasks: tasks}
				}
			}

			tasks, err := r.store.GetTasks(projectID)
			if err == nil && len(tasks) > 0 {
				return cachedTasksMsg{projectID: projectID, tasks: tasks}
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
	return tasksMsg{projectID: projectID, tasks: tasks}
}

// FetchSections returns cached sections instantly if available.
func (r *Repository) FetchSections(projectID string) tea.Cmd {
	return func() tea.Msg {
		if r.store != nil {
			if !r.store.IsStale("sections", projectID) {
				sections, err := r.store.GetSections(projectID)
				if err == nil {
					return sectionsMsg{projectID: projectID, sections: sections}
				}
			}

			sections, err := r.store.GetSections(projectID)
			if err == nil && len(sections) > 0 {
				return cachedSectionsMsg{projectID: projectID, sections: sections}
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
	return sectionsMsg{projectID: projectID, sections: sections}
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
		if r.store != nil {
			_ = r.store.UpsertTask(task)
			_ = r.store.DeleteCompletedTask(task.ID)
			_, _ = r.store.EnqueueMutation(Mutation{
				EntityType: "task",
				EntityID:   task.ID,
				Action:     MutationReopen,
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

// QuickAdd creates a task via natural language, no cache update (unpredictable project).
func (r *Repository) QuickAdd(text string) tea.Cmd {
	return func() tea.Msg {
		err := r.client.QuickAdd(context.Background(), text)
		return quickAddMsg{err: err}
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
		// Network error — revert to pending for retry
		_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
		return mutationFlushedMsg{mutation: m, err: err}
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
		_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
		return mutationFlushedMsg{mutation: m, err: err}
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
		// Network error — revert to pending
		_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
		return mutationFlushedMsg{mutation: m, err: err}
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
		_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
		return mutationFlushedMsg{mutation: m, err: err}
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
		_ = r.store.UpdateMutationStatus(m.ID, MutationPending, "")
		return mutationFlushedMsg{mutation: m, err: err}
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

func (r *Repository) RetryMutation(id int64) tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return noopMsg{}
		}
		_ = r.store.UpdateMutationStatus(id, MutationPending, "")
		return flushNextMsg{}
	}
}

func (r *Repository) DismissMutation(id int64) tea.Cmd {
	return func() tea.Msg {
		if r.store == nil {
			return noopMsg{}
		}
		_ = r.store.DeleteMutation(id)
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

// --- Background cache warming ---

// FindStaleProjects returns a list of project IDs whose task cache is past TTL.
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
			if r.store.IsStale("tasks", p.ID) {
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
		// Skip if user already loaded this project (it's fresh now)
		if r.store != nil && !r.store.IsStale("tasks", projectID) {
			return backgroundRefreshDoneMsg{remaining: remaining}
		}
		// Warm cache — call API and update store, discard the returned msg structs
		r.fetchTasksFromAPI(projectID)
		r.fetchSectionsFromAPI(projectID)
		return backgroundRefreshDoneMsg{remaining: remaining}
	}
}
