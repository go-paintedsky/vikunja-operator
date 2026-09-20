/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vikunja

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIError represents an error response returned by the Vikunja API.
type APIError struct {
	StatusCode int
	Detail     string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("vikunja api error (status %d): %s", e.StatusCode, e.Detail)
}

// IsNotFound reports whether err is an APIError with a 404 status code.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// Client is a minimal HTTP client for the Vikunja API v2.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient returns a Client for the Vikunja instance at baseURL (e.g.
// "https://try.vikunja.io/api/v2"), authenticating with the given API token.
func NewClient(baseURL, token string, insecureSkipVerify bool) *Client {
	transport := http.DefaultTransport
	if insecureSkipVerify {
		//nolint:gosec // explicitly opted into via VikunjaInstance.spec.insecureSkipVerify
		transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		apiErr := &APIError{StatusCode: resp.StatusCode, Detail: string(data)}
		var errModel struct {
			Detail string `json:"detail"`
		}
		if len(data) > 0 && json.Unmarshal(data, &errModel) == nil && errModel.Detail != "" {
			apiErr.Detail = errModel.Detail
		}
		return apiErr
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
}

// GetInfo returns the unauthenticated instance info from GET /info.
func (c *Client) GetInfo(ctx context.Context) (*Info, error) {
	var info Info
	if err := c.do(ctx, http.MethodGet, "/info", nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GetCurrentUser returns the user the configured API token belongs to.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	var u User
	if err := c.do(ctx, http.MethodGet, "/user", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// FindUserByUsername resolves a username to a User via GET /users?q=, since
// task assignment requires a numeric user id. It returns an APIError with a
// 404 status if no exact match is found.
func (c *Client) FindUserByUsername(ctx context.Context, username string) (*User, error) {
	var page paginatedUsers
	path := fmt.Sprintf("/users?q=%s", url.QueryEscape(username))
	if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	for _, u := range page.Items {
		if u.Username == username {
			return &u, nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Detail: fmt.Sprintf("no user found with username %q", username)}
}

// CreateProject creates a project via POST /projects.
func (c *Client) CreateProject(ctx context.Context, p *Project) (*Project, error) {
	var out Project
	if err := c.do(ctx, http.MethodPost, "/projects", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetProject fetches a project via GET /projects/{id}.
func (c *Client) GetProject(ctx context.Context, id int64) (*Project, error) {
	var out Project
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/projects/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateProject replaces a project's fields via PUT /projects/{id}.
func (c *Client) UpdateProject(ctx context.Context, id int64, p *Project) (*Project, error) {
	var out Project
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/projects/%d", id), p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteProject deletes a project via DELETE /projects/{id}.
func (c *Client) DeleteProject(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/projects/%d", id), nil, nil)
}

// CreateLabel creates a label via POST /labels.
func (c *Client) CreateLabel(ctx context.Context, l *Label) (*Label, error) {
	var out Label
	if err := c.do(ctx, http.MethodPost, "/labels", l, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLabel fetches a label via GET /labels/{id}.
func (c *Client) GetLabel(ctx context.Context, id int64) (*Label, error) {
	var out Label
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/labels/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateLabel replaces a label's fields via PUT /labels/{id}.
func (c *Client) UpdateLabel(ctx context.Context, id int64, l *Label) (*Label, error) {
	var out Label
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/labels/%d", id), l, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteLabel deletes a label via DELETE /labels/{id}.
func (c *Client) DeleteLabel(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/labels/%d", id), nil, nil)
}

// CreateTeam creates a team via POST /teams.
func (c *Client) CreateTeam(ctx context.Context, t *Team) (*Team, error) {
	var out Team
	if err := c.do(ctx, http.MethodPost, "/teams", t, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTeam fetches a team via GET /teams/{id}.
func (c *Client) GetTeam(ctx context.Context, id int64) (*Team, error) {
	var out Team
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/teams/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTeam replaces a team's fields (except membership) via PUT /teams/{id}.
func (c *Client) UpdateTeam(ctx context.Context, id int64, t *Team) (*Team, error) {
	var out Team
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/teams/%d", id), t, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTeam deletes a team via DELETE /teams/{id}.
func (c *Client) DeleteTeam(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/teams/%d", id), nil, nil)
}

// AddTeamMember adds a member to a team via POST /teams/{team}/members.
func (c *Client) AddTeamMember(ctx context.Context, teamID int64, username string, admin bool) (*TeamMember, error) {
	var out TeamMember
	body := &TeamMember{Username: username, Admin: admin}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/teams/%d/members", teamID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveTeamMember removes a member from a team via DELETE /teams/{team}/members/{user}.
func (c *Client) RemoveTeamMember(ctx context.Context, teamID int64, username string) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/teams/%d/members/%s", teamID, url.PathEscape(username)), nil, nil)
}

// ToggleTeamMemberAdmin flips a member's admin flag via POST /teams/{team}/members/{user}/admin.
func (c *Client) ToggleTeamMemberAdmin(ctx context.Context, teamID int64, username string) (*TeamMember, error) {
	var out TeamMember
	path := fmt.Sprintf("/teams/%d/members/%s/admin", teamID, url.PathEscape(username))
	if err := c.do(ctx, http.MethodPost, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateTask creates a task under a project via POST /projects/{project}/tasks.
func (c *Client) CreateTask(ctx context.Context, projectID int64, t *Task) (*Task, error) {
	var out Task
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/projects/%d/tasks", projectID), t, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTask fetches a task via GET /tasks/{task}.
func (c *Client) GetTask(ctx context.Context, id int64) (*Task, error) {
	var out Task
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/tasks/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTask replaces a task's fields via PUT /tasks/{task}.
func (c *Client) UpdateTask(ctx context.Context, id int64, t *Task) (*Task, error) {
	var out Task
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/tasks/%d", id), t, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTask deletes a task via DELETE /tasks/{task}.
func (c *Client) DeleteTask(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/tasks/%d", id), nil, nil)
}

// AddTaskLabel attaches a label to a task via POST /tasks/{task}/labels.
func (c *Client) AddTaskLabel(ctx context.Context, taskID, labelID int64) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/tasks/%d/labels", taskID), &labelTask{LabelID: labelID}, nil)
}

// RemoveTaskLabel detaches a label from a task via DELETE /tasks/{task}/labels/{label}.
func (c *Client) RemoveTaskLabel(ctx context.Context, taskID, labelID int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/tasks/%d/labels/%d", taskID, labelID), nil, nil)
}

// AddTaskAssignee assigns a user to a task via POST /tasks/{task}/assignees.
func (c *Client) AddTaskAssignee(ctx context.Context, taskID, userID int64) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/tasks/%d/assignees", taskID), &taskAssignee{UserID: userID}, nil)
}

// RemoveTaskAssignee unassigns a user from a task via DELETE /tasks/{task}/assignees/{user}.
func (c *Client) RemoveTaskAssignee(ctx context.Context, taskID, userID int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/tasks/%d/assignees/%d", taskID, userID), nil, nil)
}
