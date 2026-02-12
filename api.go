package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

const baseURL = "https://api.todoist.com/api/v1"

// Client is the Todoist API client
type Client struct {
	token string
	http  *http.Client
}

type directoryUser struct {
	ID   string
	Name string
}

// NewClient creates a new Todoist API client
func NewClient(token string) *Client {
	return &Client{
		token: token,
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// --- Projects ---

func (c *Client) GetProjects(ctx context.Context) ([]Project, error) {
	var all []Project
	var cursor *string

	for {
		path := "/projects?limit=200"
		if cursor != nil {
			path += "&cursor=" + url.QueryEscape(*cursor)
		}

		data, err := c.doRequest(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}

		var resp PaginatedResponse[Project]
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode projects: %w", err)
		}

		all = append(all, resp.Results...)
		if resp.NextCursor == nil || *resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	return all, nil
}

// --- Sections ---

func (c *Client) GetSections(ctx context.Context, projectID string) ([]Section, error) {
	var all []Section
	var cursor *string

	for {
		path := "/sections?limit=200&project_id=" + url.QueryEscape(projectID)
		if cursor != nil {
			path += "&cursor=" + url.QueryEscape(*cursor)
		}

		data, err := c.doRequest(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}

		var resp PaginatedResponse[Section]
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode sections: %w", err)
		}

		all = append(all, resp.Results...)
		if resp.NextCursor == nil || *resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	return all, nil
}

// --- Tasks ---

func (c *Client) GetTasks(ctx context.Context, projectID string) ([]Task, error) {
	var all []Task
	var cursor *string

	for {
		path := "/tasks?limit=200"
		if projectID != "" {
			path += "&project_id=" + url.QueryEscape(projectID)
		}
		if cursor != nil {
			path += "&cursor=" + url.QueryEscape(*cursor)
		}

		data, err := c.doRequest(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}

		var resp PaginatedResponse[Task]
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode tasks: %w", err)
		}

		all = append(all, resp.Results...)
		if resp.NextCursor == nil || *resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	return all, nil
}

type createTaskRequest struct {
	Content      string   `json:"content"`
	Description  string   `json:"description,omitempty"`
	ProjectID    string   `json:"project_id,omitempty"`
	SectionID    string   `json:"section_id,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	DueString    string   `json:"due_string,omitempty"`
	DeadlineDate string   `json:"deadline_date,omitempty"`
	Labels       []string `json:"labels,omitempty"`
}

func (c *Client) CreateTask(ctx context.Context, req createTaskRequest) (Task, error) {
	data, err := c.doRequest(ctx, "POST", "/tasks", req)
	if err != nil {
		return Task{}, err
	}

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return Task{}, fmt.Errorf("decode task: %w", err)
	}
	return task, nil
}

type updateTaskRequest struct {
	Content       *string  `json:"content,omitempty"`
	Description   *string  `json:"description,omitempty"`
	Priority      *int     `json:"priority,omitempty"`
	DueString     *string  `json:"due_string,omitempty"`
	DeadlineDate  *string  `json:"deadline_date,omitempty"`
	Labels        []string `json:"labels,omitempty"`
	ClearDeadline bool     `json:"-"`
}

func (r updateTaskRequest) MarshalJSON() ([]byte, error) {
	payload := map[string]any{}
	if r.Content != nil {
		payload["content"] = *r.Content
	}
	if r.Description != nil {
		payload["description"] = *r.Description
	}
	if r.Priority != nil {
		payload["priority"] = *r.Priority
	}
	if r.DueString != nil {
		payload["due_string"] = *r.DueString
	}
	if r.ClearDeadline {
		payload["deadline_date"] = nil
	} else if r.DeadlineDate != nil {
		payload["deadline_date"] = *r.DeadlineDate
	}
	if r.Labels != nil {
		payload["labels"] = r.Labels
	}
	return json.Marshal(payload)
}

func (r *updateTaskRequest) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if v, ok := raw["content"]; ok && string(bytes.TrimSpace(v)) != "null" {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return err
		}
		r.Content = &s
	}
	if v, ok := raw["description"]; ok && string(bytes.TrimSpace(v)) != "null" {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return err
		}
		r.Description = &s
	}
	if v, ok := raw["priority"]; ok && string(bytes.TrimSpace(v)) != "null" {
		var p int
		if err := json.Unmarshal(v, &p); err != nil {
			return err
		}
		r.Priority = &p
	}
	if v, ok := raw["due_string"]; ok && string(bytes.TrimSpace(v)) != "null" {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return err
		}
		r.DueString = &s
	}
	if v, ok := raw["deadline_date"]; ok {
		if string(bytes.TrimSpace(v)) == "null" {
			r.ClearDeadline = true
			r.DeadlineDate = nil
		} else {
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				return err
			}
			r.DeadlineDate = &s
		}
	}
	if v, ok := raw["labels"]; ok {
		if string(bytes.TrimSpace(v)) == "null" {
			r.Labels = nil
		} else {
			var labels []string
			if err := json.Unmarshal(v, &labels); err != nil {
				return err
			}
			r.Labels = labels
		}
	}
	return nil
}

func (c *Client) UpdateTask(ctx context.Context, taskID string, req updateTaskRequest) (Task, error) {
	data, err := c.doRequest(ctx, "POST", "/tasks/"+taskID, req)
	if err != nil {
		return Task{}, err
	}

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return Task{}, fmt.Errorf("decode task: %w", err)
	}
	return task, nil
}

func (c *Client) GetTask(ctx context.Context, taskID string) (Task, error) {
	data, err := c.doRequest(ctx, "GET", "/tasks/"+taskID, nil)
	if err != nil {
		return Task{}, err
	}
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return Task{}, fmt.Errorf("decode task: %w", err)
	}
	return task, nil
}

func isNotFoundError(err error) bool {
	code, ok := apiErrorStatusCode(err)
	return ok && code == http.StatusNotFound
}

var apiErrorPattern = regexp.MustCompile(`API error ([0-9]{3}):`)

func apiErrorStatusCode(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	matches := apiErrorPattern.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return 0, false
	}
	code, convErr := strconv.Atoi(matches[1])
	if convErr != nil {
		return 0, false
	}
	return code, true
}

func isRetriableMutationError(err error) bool {
	if err == nil {
		return false
	}
	if code, ok := apiErrorStatusCode(err); ok {
		return code == http.StatusTooManyRequests || code == http.StatusRequestTimeout || code >= 500
	}
	// Non-HTTP failures (timeouts/network) are generally retriable.
	return true
}

func (c *Client) CloseTask(ctx context.Context, taskID string) error {
	_, err := c.doRequest(ctx, "POST", "/tasks/"+taskID+"/close", nil)
	return err
}

func (c *Client) ReopenTask(ctx context.Context, taskID string) error {
	_, err := c.doRequest(ctx, "POST", "/tasks/"+taskID+"/reopen", nil)
	return err
}

func (c *Client) DeleteTask(ctx context.Context, taskID string) error {
	_, err := c.doRequest(ctx, "DELETE", "/tasks/"+taskID, nil)
	return err
}

type quickAddRequest struct {
	Text string `json:"text"`
}

func (c *Client) QuickAdd(ctx context.Context, text string) (Task, error) {
	data, err := c.doRequest(ctx, "POST", "/tasks/quick", quickAddRequest{Text: text})
	if err != nil {
		return Task{}, err
	}
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return Task{}, fmt.Errorf("decode quick add task: %w", err)
	}
	return task, nil
}

// --- Labels ---

func (c *Client) GetLabels(ctx context.Context) ([]Label, error) {
	var all []Label
	var cursor *string

	for {
		path := "/labels?limit=200"
		if cursor != nil {
			path += "&cursor=" + url.QueryEscape(*cursor)
		}

		data, err := c.doRequest(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}

		var resp PaginatedResponse[Label]
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode labels: %w", err)
		}

		all = append(all, resp.Results...)
		if resp.NextCursor == nil || *resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	return all, nil
}

// --- User directory ---

func (c *Client) GetCurrentUserDirectoryEntry(ctx context.Context) (*directoryUser, error) {
	data, err := c.doRequest(ctx, "GET", "/user", nil)
	if err != nil {
		return nil, err
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}

	id := anyToString(obj["id"])
	name := firstNonEmpty(
		anyToString(obj["full_name"]),
		anyToString(obj["name"]),
		anyToString(obj["email"]),
	)
	if id == "" || name == "" {
		return nil, nil
	}
	return &directoryUser{ID: id, Name: name}, nil
}

func (c *Client) GetWorkspaceUsers(ctx context.Context) ([]directoryUser, error) {
	seen := make(map[string]directoryUser)
	var cursor string

	for {
		path := "/workspaces/users?limit=100"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		data, err := c.doRequest(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}

		var resp map[string]any
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode workspace users: %w", err)
		}

		for _, row := range toObjectSlice(resp["workspace_users"]) {
			id, name := parseDirectoryUser(row)
			if id == "" || name == "" {
				continue
			}
			seen[id] = directoryUser{ID: id, Name: name}
		}

		hasMore, _ := resp["has_more"].(bool)
		next := anyToString(resp["next_cursor"])
		if !hasMore || next == "" {
			break
		}
		cursor = next
	}

	return mapValues(seen), nil
}

func (c *Client) GetProjectCollaborators(ctx context.Context, projectID string) ([]directoryUser, error) {
	data, err := c.doRequest(ctx, "GET", "/projects/"+url.PathEscape(projectID)+"/collaborators", nil)
	if err != nil {
		return nil, err
	}

	rows, err := decodeObjectList(data, "results", "collaborators")
	if err != nil {
		return nil, fmt.Errorf("decode collaborators: %w", err)
	}

	seen := make(map[string]directoryUser)
	for _, row := range rows {
		id, name := parseDirectoryUser(row)
		if id == "" || name == "" {
			continue
		}
		seen[id] = directoryUser{ID: id, Name: name}
	}
	return mapValues(seen), nil
}

func decodeObjectList(data []byte, keys ...string) ([]map[string]any, error) {
	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	for _, key := range keys {
		if raw, ok := obj[key]; ok {
			return toObjectSlice(raw), nil
		}
	}
	return nil, nil
}

func parseDirectoryUser(row map[string]any) (string, string) {
	id := firstNonEmpty(
		anyToString(row["user_id"]),
		anyToString(row["id"]),
		anyToString(row["uid"]),
	)
	name := firstNonEmpty(
		anyToString(row["full_name"]),
		anyToString(row["name"]),
		anyToString(row["user_email"]),
		anyToString(row["email"]),
	)
	return id, name
}

func toObjectSlice(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			out = append(out, row)
		}
	}
	return out
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case float32:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case uint:
		return strconv.FormatUint(uint64(t), 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case uint32:
		return strconv.FormatUint(uint64(t), 10)
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func mapValues[T any](m map[string]T) []T {
	out := make([]T, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// --- Project mutations ---

type createProjectRequest struct {
	Name string `json:"name"`
}

func (c *Client) CreateProject(ctx context.Context, req createProjectRequest) (Project, error) {
	data, err := c.doRequest(ctx, "POST", "/projects", req)
	if err != nil {
		return Project{}, err
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return Project{}, fmt.Errorf("decode project: %w", err)
	}
	return p, nil
}

func (c *Client) ArchiveProject(ctx context.Context, projectID string) error {
	_, err := c.doRequest(ctx, "POST", "/projects/"+projectID+"/archive", nil)
	return err
}

func (c *Client) UnarchiveProject(ctx context.Context, projectID string) error {
	_, err := c.doRequest(ctx, "POST", "/projects/"+projectID+"/unarchive", nil)
	return err
}

// --- Comments ---

func (c *Client) GetComments(ctx context.Context, taskID string) ([]Comment, error) {
	var all []Comment
	var cursor *string

	for {
		path := "/comments?task_id=" + url.QueryEscape(taskID) + "&limit=200"
		if cursor != nil {
			path += "&cursor=" + url.QueryEscape(*cursor)
		}

		data, err := c.doRequest(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}

		var resp PaginatedResponse[Comment]
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode comments: %w", err)
		}

		all = append(all, resp.Results...)
		if resp.NextCursor == nil || *resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	return all, nil
}
