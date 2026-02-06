package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.todoist.com/api/v1"

// Client is the Todoist API client
type Client struct {
	token  string
	http   *http.Client
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
	Content     string   `json:"content"`
	Description string   `json:"description,omitempty"`
	ProjectID   string   `json:"project_id,omitempty"`
	SectionID   string   `json:"section_id,omitempty"`
	Priority    int      `json:"priority,omitempty"`
	DueString   string   `json:"due_string,omitempty"`
	Labels      []string `json:"labels,omitempty"`
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
	Content     *string  `json:"content,omitempty"`
	Description *string  `json:"description,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	DueString   *string  `json:"due_string,omitempty"`
	Labels      []string `json:"labels,omitempty"`
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

func (c *Client) QuickAdd(ctx context.Context, text string) error {
	_, err := c.doRequest(ctx, "POST", "/tasks/quick", quickAddRequest{Text: text})
	return err
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
