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

// Package vikunja is a minimal client for the Vikunja API v2
// (https://vikunja.io/docs/api-v2/), covering only the operations the
// vikunja-operator needs to reconcile Projects, Tasks, Teams and Labels.
package vikunja

// Info is the subset of VikunjaInfos (GET /info) the operator cares about.
type Info struct {
	Version string `json:"version"`
}

// User is the subset of the Vikunja User schema the operator cares about.
type User struct {
	ID       int64  `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	Name     string `json:"name,omitempty"`
}

// paginatedUsers mirrors PaginatedUser.
type paginatedUsers struct {
	Items []User `json:"items"`
}

// Project mirrors the fields of the Vikunja Project schema the operator reads and writes.
type Project struct {
	ID              int64  `json:"id,omitempty"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Identifier      string `json:"identifier"`
	HexColor        string `json:"hex_color"`
	IsArchived      bool   `json:"is_archived"`
	ParentProjectID int64  `json:"parent_project_id"`
}

// Label mirrors the fields of the Vikunja Label schema the operator reads and writes.
type Label struct {
	ID          int64  `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	HexColor    string `json:"hex_color"`
}

// labelTask mirrors LabelTask, the body used to attach a label to a task.
type labelTask struct {
	LabelID int64 `json:"label_id"`
}

// Team mirrors the fields of the Vikunja Team schema the operator reads and writes.
type Team struct {
	ID          int64        `json:"id,omitempty"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	IsPublic    bool         `json:"is_public"`
	Members     []TeamMember `json:"members,omitempty"`
}

// TeamMember mirrors the fields of the Vikunja TeamMember schema.
type TeamMember struct {
	ID       int64  `json:"id,omitempty"`
	Username string `json:"username"`
	Admin    bool   `json:"admin"`
}

// Task mirrors the fields of the Vikunja Task schema the operator reads and writes.
type Task struct {
	ID          int64   `json:"id,omitempty"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Done        bool    `json:"done"`
	Priority    int64   `json:"priority"`
	HexColor    string  `json:"hex_color"`
	DueDate     string  `json:"due_date,omitempty"`
	StartDate   string  `json:"start_date,omitempty"`
	EndDate     string  `json:"end_date,omitempty"`
	ProjectID   int64   `json:"project_id,omitempty"`
	Identifier  string  `json:"identifier,omitempty"`
	Index       int64   `json:"index,omitempty"`
	Labels      []Label `json:"labels,omitempty"`
	Assignees   []User  `json:"assignees,omitempty"`
}

// taskAssignee mirrors TaskAssginee, the body used to assign a user to a task.
type taskAssignee struct {
	UserID int64 `json:"user_id"`
}
