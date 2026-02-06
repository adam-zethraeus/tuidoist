package main

import (
	"context"

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

// --- Two-phase reads ---

// FetchProjects returns cached projects instantly if available (stale or not),
// then triggers a background refresh. If the cache is fresh, returns projectsMsg directly.
func (r *Repository) FetchProjects() tea.Cmd {
	return func() tea.Msg {
		if r.store != nil {
			if !r.store.IsStale("projects", "") {
				projects, err := r.store.GetProjects()
				if err == nil && len(projects) > 0 {
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
				if err == nil && len(tasks) > 0 {
					return tasksMsg{tasks: tasks}
				}
			}

			tasks, err := r.store.GetTasks(projectID)
			if err == nil && len(tasks) > 0 {
				return cachedTasksMsg{tasks: tasks}
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
		return tasksMsg{err: err}
	}
	if r.store != nil {
		_ = r.store.ReplaceTasks(projectID, tasks)
	}
	return tasksMsg{tasks: tasks}
}

// FetchSections returns cached sections instantly if available.
func (r *Repository) FetchSections(projectID string) tea.Cmd {
	return func() tea.Msg {
		if r.store != nil {
			if !r.store.IsStale("sections", projectID) {
				sections, err := r.store.GetSections(projectID)
				if err == nil && len(sections) > 0 {
					return sectionsMsg{sections: sections}
				}
			}

			sections, err := r.store.GetSections(projectID)
			if err == nil && len(sections) > 0 {
				return cachedSectionsMsg{sections: sections}
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
		return sectionsMsg{err: err}
	}
	if r.store != nil {
		_ = r.store.ReplaceSections(projectID, sections)
	}
	return sectionsMsg{sections: sections}
}

// --- Write-through mutations ---

// CloseTask completes a task via API, then removes it from cache.
func (r *Repository) CloseTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		err := r.client.CloseTask(context.Background(), taskID)
		if err == nil && r.store != nil {
			_ = r.store.DeleteTask(taskID)
		}
		return taskClosedMsg{taskID: taskID, err: err}
	}
}

// DeleteTask deletes a task via API, then removes it from cache.
func (r *Repository) DeleteTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		err := r.client.DeleteTask(context.Background(), taskID)
		if err == nil && r.store != nil {
			_ = r.store.DeleteTask(taskID)
		}
		return taskDeletedMsg{taskID: taskID, err: err}
	}
}

// CreateTask creates a task via API, then upserts it in cache.
func (r *Repository) CreateTask(req createTaskRequest) tea.Cmd {
	return func() tea.Msg {
		task, err := r.client.CreateTask(context.Background(), req)
		if err == nil && r.store != nil {
			_ = r.store.UpsertTask(task)
		}
		return taskCreatedMsg{task: task, err: err}
	}
}

// UpdateTask updates a task via API, then upserts it in cache.
func (r *Repository) UpdateTask(taskID string, req updateTaskRequest) tea.Cmd {
	return func() tea.Msg {
		task, err := r.client.UpdateTask(context.Background(), taskID, req)
		if err == nil && r.store != nil {
			_ = r.store.UpsertTask(task)
		}
		return taskUpdatedMsg{task: task, err: err}
	}
}

// QuickAdd creates a task via natural language, no cache update (unpredictable project).
func (r *Repository) QuickAdd(text string) tea.Cmd {
	return func() tea.Msg {
		err := r.client.QuickAdd(context.Background(), text)
		return quickAddMsg{err: err}
	}
}
