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

package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"

	"github.com/go-paintedsky/vikunja-operator/internal/vikunja"
)

// fakeVikunjaServer is a minimal, in-memory stand-in for a Vikunja API v2
// instance, covering just enough of the surface (projects, labels, teams and
// their members, tasks and their labels/assignees) for the controller suite
// to exercise real HTTP round-trips end to end instead of mocking the client.
type fakeVikunjaServer struct {
	mu     sync.Mutex
	nextID int64
	token  string

	projects map[int64]*vikunja.Project
	labels   map[int64]*vikunja.Label
	teams    map[int64]*vikunja.Team
	tasks    map[int64]*vikunja.Task

	usersByUsername map[string]*vikunja.User
	currentUser     *vikunja.User
}

func newFakeVikunjaServer() (*httptest.Server, *fakeVikunjaServer) {
	f := &fakeVikunjaServer{
		nextID:   1,
		token:    testToken,
		projects: map[int64]*vikunja.Project{},
		labels:   map[int64]*vikunja.Label{},
		teams:    map[int64]*vikunja.Team{},
		tasks:    map[int64]*vikunja.Task{},
		usersByUsername: map[string]*vikunja.User{
			testUsernameOperator: {ID: 1, Username: testUsernameOperator, Name: "Operator"},
			testUsernameAlice:    {ID: 2, Username: testUsernameAlice, Name: "Alice"},
			testUsernameBob:      {ID: 3, Username: testUsernameBob, Name: "Bob"},
		},
	}
	f.currentUser = f.usersByUsername[testUsernameOperator]

	mux := http.NewServeMux()
	mux.HandleFunc("GET /info", f.handleInfo)
	mux.HandleFunc("GET /user", f.handleCurrentUser)
	mux.HandleFunc("GET /users", f.handleSearchUsers)

	mux.HandleFunc("POST /projects", f.handleCreateProject)
	mux.HandleFunc("GET /projects/{id}", f.handleGetProject)
	mux.HandleFunc("PUT /projects/{id}", f.handleUpdateProject)
	mux.HandleFunc("DELETE /projects/{id}", f.handleDeleteProject)

	mux.HandleFunc("POST /labels", f.handleCreateLabel)
	mux.HandleFunc("GET /labels/{id}", f.handleGetLabel)
	mux.HandleFunc("PUT /labels/{id}", f.handleUpdateLabel)
	mux.HandleFunc("DELETE /labels/{id}", f.handleDeleteLabel)

	mux.HandleFunc("POST /teams", f.handleCreateTeam)
	mux.HandleFunc("GET /teams/{id}", f.handleGetTeam)
	mux.HandleFunc("PUT /teams/{id}", f.handleUpdateTeam)
	mux.HandleFunc("DELETE /teams/{id}", f.handleDeleteTeam)
	mux.HandleFunc("POST /teams/{id}/members", f.handleAddTeamMember)
	mux.HandleFunc("DELETE /teams/{id}/members/{user}", f.handleRemoveTeamMember)
	mux.HandleFunc("POST /teams/{id}/members/{user}/admin", f.handleToggleTeamMemberAdmin)

	mux.HandleFunc("POST /projects/{id}/tasks", f.handleCreateTask)
	mux.HandleFunc("GET /tasks/{id}", f.handleGetTask)
	mux.HandleFunc("PUT /tasks/{id}", f.handleUpdateTask)
	mux.HandleFunc("DELETE /tasks/{id}", f.handleDeleteTask)
	mux.HandleFunc("POST /tasks/{id}/labels", f.handleAddTaskLabel)
	mux.HandleFunc("DELETE /tasks/{id}/labels/{label}", f.handleRemoveTaskLabel)
	mux.HandleFunc("POST /tasks/{id}/assignees", f.handleAddTaskAssignee)
	mux.HandleFunc("DELETE /tasks/{id}/assignees/{user}", f.handleRemoveTaskAssignee)

	server := httptest.NewServer(f.withAuth(mux))
	return server, f
}

func (f *fakeVikunjaServer) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+f.token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{detailKey: "invalid token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

const detailKey = "detail"

func writeNotFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]string{detailKey: "not found"})
}

func writeBadRequest(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]string{detailKey: err.Error()})
}

func pathInt64(r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return v, err == nil
}

func (f *fakeVikunjaServer) allocID() int64 {
	id := f.nextID
	f.nextID++
	return id
}

func (f *fakeVikunjaServer) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, vikunja.Info{Version: "test"})
}

func (f *fakeVikunjaServer) handleCurrentUser(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, f.currentUser)
}

func (f *fakeVikunjaServer) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	f.mu.Lock()
	defer f.mu.Unlock()

	var items []vikunja.User
	for username, u := range f.usersByUsername {
		if q == "" || strings.Contains(username, q) {
			items = append(items, *u)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// -- projects --

func (f *fakeVikunjaServer) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var p vikunja.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeBadRequest(w, err)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p.ID = f.allocID()
	f.projects[p.ID] = &p
	writeJSON(w, http.StatusCreated, p)
}

func (f *fakeVikunjaServer) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	p, found := f.projects[id]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (f *fakeVikunjaServer) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, found := f.projects[id]; !ok || !found {
		writeNotFound(w)
		return
	}
	var p vikunja.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeBadRequest(w, err)
		return
	}
	p.ID = id
	f.projects[id] = &p
	writeJSON(w, http.StatusOK, p)
}

func (f *fakeVikunjaServer) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, found := f.projects[id]; !ok || !found {
		writeNotFound(w)
		return
	}
	delete(f.projects, id)
	writeJSON(w, http.StatusOK, map[string]string{})
}

// -- labels --

func (f *fakeVikunjaServer) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	var l vikunja.Label
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		writeBadRequest(w, err)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	l.ID = f.allocID()
	f.labels[l.ID] = &l
	writeJSON(w, http.StatusCreated, l)
}

func (f *fakeVikunjaServer) handleGetLabel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	l, found := f.labels[id]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (f *fakeVikunjaServer) handleUpdateLabel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, found := f.labels[id]; !ok || !found {
		writeNotFound(w)
		return
	}
	var l vikunja.Label
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		writeBadRequest(w, err)
		return
	}
	l.ID = id
	f.labels[id] = &l
	writeJSON(w, http.StatusOK, l)
}

func (f *fakeVikunjaServer) handleDeleteLabel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, found := f.labels[id]; !ok || !found {
		writeNotFound(w)
		return
	}
	delete(f.labels, id)
	writeJSON(w, http.StatusOK, map[string]string{})
}

// -- teams --

func (f *fakeVikunjaServer) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var t vikunja.Team
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeBadRequest(w, err)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	t.ID = f.allocID()
	// Vikunja always makes the creator an admin member of a new team.
	t.Members = []vikunja.TeamMember{{ID: f.allocID(), Username: f.currentUser.Username, Admin: true}}
	f.teams[t.ID] = &t
	writeJSON(w, http.StatusCreated, t)
}

func (f *fakeVikunjaServer) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	t, found := f.teams[id]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (f *fakeVikunjaServer) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, found := f.teams[id]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	var t vikunja.Team
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeBadRequest(w, err)
		return
	}
	t.ID = id
	t.Members = existing.Members // membership is managed via the members endpoints, not PUT
	f.teams[id] = &t
	writeJSON(w, http.StatusOK, t)
}

func (f *fakeVikunjaServer) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, found := f.teams[id]; !ok || !found {
		writeNotFound(w)
		return
	}
	delete(f.teams, id)
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (f *fakeVikunjaServer) handleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	t, found := f.teams[id]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	var m vikunja.TeamMember
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeBadRequest(w, err)
		return
	}
	m.ID = f.allocID()
	t.Members = append(t.Members, m)
	writeJSON(w, http.StatusCreated, m)
}

func (f *fakeVikunjaServer) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	username := r.PathValue("user")
	f.mu.Lock()
	defer f.mu.Unlock()
	t, found := f.teams[id]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	kept := t.Members[:0]
	for _, m := range t.Members {
		if m.Username != username {
			kept = append(kept, m)
		}
	}
	t.Members = kept
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (f *fakeVikunjaServer) handleToggleTeamMemberAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	username := r.PathValue("user")
	f.mu.Lock()
	defer f.mu.Unlock()
	t, found := f.teams[id]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	for i := range t.Members {
		if t.Members[i].Username == username {
			t.Members[i].Admin = !t.Members[i].Admin
			writeJSON(w, http.StatusOK, t.Members[i])
			return
		}
	}
	writeNotFound(w)
}

// -- tasks --

func (f *fakeVikunjaServer) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathInt64(r, "id")
	var t vikunja.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeBadRequest(w, err)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, found := f.projects[projectID]; !ok || !found {
		writeNotFound(w)
		return
	}
	t.ID = f.allocID()
	t.ProjectID = projectID
	t.Index = t.ID
	t.Identifier = "TASK-" + strconv.FormatInt(t.Index, 10)
	f.tasks[t.ID] = &t
	writeJSON(w, http.StatusCreated, t)
}

func (f *fakeVikunjaServer) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	t, found := f.tasks[id]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (f *fakeVikunjaServer) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, found := f.tasks[id]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	var t vikunja.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeBadRequest(w, err)
		return
	}
	t.ID = id
	t.Index = existing.Index
	t.Identifier = existing.Identifier
	t.Labels = existing.Labels       // managed via the label endpoints, not PUT
	t.Assignees = existing.Assignees // managed via the assignee endpoints, not PUT
	if t.ProjectID == 0 {
		t.ProjectID = existing.ProjectID
	}
	f.tasks[id] = &t
	writeJSON(w, http.StatusOK, t)
}

func (f *fakeVikunjaServer) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, found := f.tasks[id]; !ok || !found {
		writeNotFound(w)
		return
	}
	delete(f.tasks, id)
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (f *fakeVikunjaServer) handleAddTaskLabel(w http.ResponseWriter, r *http.Request) {
	taskID, ok := pathInt64(r, "id")
	var body struct {
		LabelID int64 `json:"label_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, err)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	t, found := f.tasks[taskID]
	label, labelFound := f.labels[body.LabelID]
	if !ok || !found || !labelFound {
		writeNotFound(w)
		return
	}
	t.Labels = append(t.Labels, *label)
	writeJSON(w, http.StatusCreated, map[string]string{})
}

func (f *fakeVikunjaServer) handleRemoveTaskLabel(w http.ResponseWriter, r *http.Request) {
	taskID, ok := pathInt64(r, "id")
	labelID, lok := pathInt64(r, "label")
	f.mu.Lock()
	defer f.mu.Unlock()
	t, found := f.tasks[taskID]
	if !ok || !lok || !found {
		writeNotFound(w)
		return
	}
	kept := t.Labels[:0]
	for _, l := range t.Labels {
		if l.ID != labelID {
			kept = append(kept, l)
		}
	}
	t.Labels = kept
	writeJSON(w, http.StatusOK, map[string]string{})
}

func (f *fakeVikunjaServer) handleAddTaskAssignee(w http.ResponseWriter, r *http.Request) {
	taskID, ok := pathInt64(r, "id")
	var body struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, err)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	t, found := f.tasks[taskID]
	if !ok || !found {
		writeNotFound(w)
		return
	}
	var user *vikunja.User
	for _, u := range f.usersByUsername {
		if u.ID == body.UserID {
			user = u
			break
		}
	}
	if user == nil {
		writeNotFound(w)
		return
	}
	t.Assignees = append(t.Assignees, *user)
	writeJSON(w, http.StatusCreated, map[string]string{})
}

func (f *fakeVikunjaServer) handleRemoveTaskAssignee(w http.ResponseWriter, r *http.Request) {
	taskID, ok := pathInt64(r, "id")
	userID, uok := pathInt64(r, "user")
	f.mu.Lock()
	defer f.mu.Unlock()
	t, found := f.tasks[taskID]
	if !ok || !uok || !found {
		writeNotFound(w)
		return
	}
	kept := t.Assignees[:0]
	for _, a := range t.Assignees {
		if a.ID != userID {
			kept = append(kept, a)
		}
	}
	t.Assignees = kept
	writeJSON(w, http.StatusOK, map[string]string{})
}
