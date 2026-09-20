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
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vikunjav1alpha1 "github.com/go-paintedsky/vikunja-operator/api/v1alpha1"
	"github.com/go-paintedsky/vikunja-operator/internal/vikunja"
)

// TaskReconciler reconciles a Task object
type TaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=tasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=tasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=projects,verbs=get;list;watch
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=labels,verbs=get;list;watch
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=vikunjainstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile creates, updates or deletes the Vikunja task backing this Task
// resource, and adds/removes its labels and assignees, so that all three
// match spec.
func (r *TaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var task vikunjav1alpha1.Task
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !task.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &task)
	}

	if !controllerutil.ContainsFinalizer(&task, vikunjav1alpha1.Finalizer) {
		controllerutil.AddFinalizer(&task, vikunjav1alpha1.Finalizer)
		if err := r.Update(ctx, &task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	project, reason, message, err := getReadyProject(ctx, r.Client, task.Namespace, task.Spec.ProjectRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	if reason != "" {
		return r.setNotReady(ctx, &task, reason, message)
	}

	resolved, reason, message, err := resolveInstance(ctx, r.Client, task.Namespace, project.Spec.InstanceRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	if reason != "" {
		return r.setNotReady(ctx, &task, reason, message)
	}

	labelIDs := make([]int64, 0, len(task.Spec.LabelRefs))
	for _, name := range task.Spec.LabelRefs {
		label, lreason, lmessage, err := getReadyLabel(ctx, r.Client, task.Namespace, name)
		if err != nil {
			return ctrl.Result{}, err
		}
		if lreason != "" {
			return r.setNotReady(ctx, &task, lreason, lmessage)
		}
		labelIDs = append(labelIDs, label.Status.ID)
	}

	desired := &vikunja.Task{
		Title:       task.Spec.Title,
		Description: task.Spec.Description,
		Done:        task.Spec.Done,
		Priority:    task.Spec.Priority,
		HexColor:    task.Spec.HexColor,
		DueDate:     formatTaskTime(task.Spec.DueDate),
		StartDate:   formatTaskTime(task.Spec.StartDate),
		EndDate:     formatTaskTime(task.Spec.EndDate),
		ProjectID:   project.Status.ID,
	}

	remote, err := r.upsertTask(ctx, resolved.Client, task.Status.ID, project.Status.ID, desired)
	if err != nil {
		log.Error(err, "Failed to reconcile task on Vikunja instance", "instance", resolved.Instance.Name)
		return r.setNotReady(ctx, &task, vikunjav1alpha1.ReasonReconcileError, err.Error())
	}
	task.Status.ID = remote.ID

	if err := r.reconcileLabels(ctx, resolved.Client, remote, labelIDs); err != nil {
		log.Error(err, "Failed to reconcile task labels on Vikunja instance")
		return r.setNotReady(ctx, &task, vikunjav1alpha1.ReasonReconcileError, err.Error())
	}

	if err := r.reconcileAssignees(ctx, resolved.Client, remote, task.Spec.Assignees); err != nil {
		log.Error(err, "Failed to reconcile task assignees on Vikunja instance")
		return r.setNotReady(ctx, &task, vikunjav1alpha1.ReasonReconcileError, err.Error())
	}

	task.Status.Identifier = remote.Identifier
	task.Status.Index = remote.Index
	return r.setReady(ctx, &task)
}

func formatTaskTime(t *metav1.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (r *TaskReconciler) upsertTask(ctx context.Context, vc *vikunja.Client, id, projectID int64, desired *vikunja.Task) (*vikunja.Task, error) {
	if id == 0 {
		return vc.CreateTask(ctx, projectID, desired)
	}
	remote, err := vc.UpdateTask(ctx, id, desired)
	if vikunja.IsNotFound(err) {
		return vc.CreateTask(ctx, projectID, desired)
	}
	return remote, err
}

// reconcileLabels adds and removes labels on remote so its label set exactly
// matches desiredIDs (the resolved Vikunja ids of task.spec.labelRefs).
func (r *TaskReconciler) reconcileLabels(ctx context.Context, vc *vikunja.Client, remote *vikunja.Task, desiredIDs []int64) error {
	desired := make(map[int64]bool, len(desiredIDs))
	for _, id := range desiredIDs {
		desired[id] = true
	}
	current := make(map[int64]bool, len(remote.Labels))
	for _, l := range remote.Labels {
		current[l.ID] = true
	}

	for id := range desired {
		if !current[id] {
			if err := vc.AddTaskLabel(ctx, remote.ID, id); err != nil {
				return fmt.Errorf("add label %d: %w", id, err)
			}
		}
	}
	for id := range current {
		if !desired[id] {
			if err := vc.RemoveTaskLabel(ctx, remote.ID, id); err != nil {
				return fmt.Errorf("remove label %d: %w", id, err)
			}
		}
	}
	return nil
}

// reconcileAssignees adds and removes assignees on remote so its assignee set
// exactly matches desiredUsernames, resolving each new username to a user id
// as needed.
func (r *TaskReconciler) reconcileAssignees(ctx context.Context, vc *vikunja.Client, remote *vikunja.Task, desiredUsernames []string) error {
	desired := make(map[string]bool, len(desiredUsernames))
	for _, u := range desiredUsernames {
		desired[u] = true
	}
	current := make(map[string]int64, len(remote.Assignees))
	for _, a := range remote.Assignees {
		current[a.Username] = a.ID
	}

	for username := range desired {
		if _, ok := current[username]; ok {
			continue
		}
		user, err := vc.FindUserByUsername(ctx, username)
		if err != nil {
			return fmt.Errorf("resolve assignee %q: %w", username, err)
		}
		if err := vc.AddTaskAssignee(ctx, remote.ID, user.ID); err != nil {
			return fmt.Errorf("assign %q: %w", username, err)
		}
	}
	for username, id := range current {
		if !desired[username] {
			if err := vc.RemoveTaskAssignee(ctx, remote.ID, id); err != nil {
				return fmt.Errorf("unassign %q: %w", username, err)
			}
		}
	}
	return nil
}

func (r *TaskReconciler) reconcileDelete(ctx context.Context, task *vikunjav1alpha1.Task) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(task, vikunjav1alpha1.Finalizer) {
		return ctrl.Result{}, nil
	}

	if task.Status.ID != 0 {
		if err := r.deleteRemoteTask(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(task, vikunjav1alpha1.Finalizer)
	if err := r.Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// deleteRemoteTask deletes the task on its Vikunja instance, skipping (rather
// than blocking finalization on) the delete when its Project or
// VikunjaInstance is no longer available to resolve.
func (r *TaskReconciler) deleteRemoteTask(ctx context.Context, task *vikunjav1alpha1.Task) error {
	log := logf.FromContext(ctx)

	project, reason, _, err := getReadyProject(ctx, r.Client, task.Namespace, task.Spec.ProjectRef)
	if err != nil {
		return err
	}
	if reason != "" {
		log.Info("Skipping remote delete because the project is unavailable", "reason", reason)
		return nil
	}

	resolved, reason, _, err := resolveInstance(ctx, r.Client, task.Namespace, project.Spec.InstanceRef)
	if err != nil {
		return err
	}
	if reason != "" {
		log.Info("Skipping remote delete because the instance is unavailable", "reason", reason)
		return nil
	}

	if err := resolved.Client.DeleteTask(ctx, task.Status.ID); err != nil && !vikunja.IsNotFound(err) {
		log.Error(err, "Failed to delete task on Vikunja instance")
		return err
	}
	return nil
}

func (r *TaskReconciler) setReady(ctx context.Context, task *vikunjav1alpha1.Task) (ctrl.Result, error) {
	task.Status.ObservedGeneration = task.Generation
	setReadyCondition(&task.Status.Conditions, task.Generation, metav1.ConditionTrue,
		vikunjav1alpha1.ReasonReconcileSuccess, "Task, its labels and its assignees match the desired state on the Vikunja instance")
	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: driftRequeueInterval}, nil
}

func (r *TaskReconciler) setNotReady(ctx context.Context, task *vikunjav1alpha1.Task, reason, message string) (ctrl.Result, error) {
	task.Status.ObservedGeneration = task.Generation
	setReadyCondition(&task.Status.Conditions, task.Generation, metav1.ConditionFalse, reason, message)
	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: notReadyRequeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &vikunjav1alpha1.Task{}, projectRefField,
		func(obj client.Object) []string {
			return []string{obj.(*vikunjav1alpha1.Task).Spec.ProjectRef}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&vikunjav1alpha1.Task{}).
		Watches(
			&vikunjav1alpha1.Project{},
			handler.EnqueueRequestsFromMapFunc(mapToDependentsByField(r.Client, projectRefField, &vikunjav1alpha1.TaskList{})),
		).
		Named("task").
		Complete(r)
}
