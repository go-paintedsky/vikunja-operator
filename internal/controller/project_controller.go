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

// ProjectReconciler reconciles a Project object
type ProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=projects/finalizers,verbs=update
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=vikunjainstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile creates, updates or deletes the Vikunja project backing this
// Project resource so that it matches spec.
func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var project vikunjav1alpha1.Project
	if err := r.Get(ctx, req.NamespacedName, &project); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !project.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &project)
	}

	if !controllerutil.ContainsFinalizer(&project, vikunjav1alpha1.Finalizer) {
		controllerutil.AddFinalizer(&project, vikunjav1alpha1.Finalizer)
		if err := r.Update(ctx, &project); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	resolved, reason, message, err := resolveInstance(ctx, r.Client, project.Namespace, project.Spec.InstanceRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	if reason != "" {
		return r.setNotReady(ctx, &project, reason, message)
	}

	var parentID int64
	if project.Spec.ParentProjectRef != nil {
		parent, preason, pmessage, err := getReadyProject(ctx, r.Client, project.Namespace, *project.Spec.ParentProjectRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		if preason != "" {
			return r.setNotReady(ctx, &project, preason, pmessage)
		}
		parentID = parent.Status.ID
	}

	desired := &vikunja.Project{
		Title:           project.Spec.Title,
		Description:     project.Spec.Description,
		Identifier:      project.Spec.Identifier,
		HexColor:        project.Spec.HexColor,
		IsArchived:      project.Spec.IsArchived,
		ParentProjectID: parentID,
	}

	remote, err := r.upsertProject(ctx, resolved.Client, project.Status.ID, desired)
	if err != nil {
		log.Error(err, "Failed to reconcile project on Vikunja instance", "instance", resolved.Instance.Name)
		return r.setNotReady(ctx, &project, vikunjav1alpha1.ReasonReconcileError, err.Error())
	}

	project.Status.ID = remote.ID
	project.Status.ParentProjectID = remote.ParentProjectID
	return r.setReady(ctx, &project)
}

// upsertProject creates the project if it has no known id yet, otherwise
// replaces its fields; if the remote project was deleted out of band it falls
// back to creating a new one.
func (r *ProjectReconciler) upsertProject(ctx context.Context, vc *vikunja.Client, id int64, desired *vikunja.Project) (*vikunja.Project, error) {
	if id == 0 {
		return vc.CreateProject(ctx, desired)
	}
	remote, err := vc.UpdateProject(ctx, id, desired)
	if vikunja.IsNotFound(err) {
		return vc.CreateProject(ctx, desired)
	}
	return remote, err
}

func (r *ProjectReconciler) reconcileDelete(ctx context.Context, project *vikunjav1alpha1.Project) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(project, vikunjav1alpha1.Finalizer) {
		return ctrl.Result{}, nil
	}

	if project.Status.ID != 0 {
		resolved, reason, _, err := resolveInstance(ctx, r.Client, project.Namespace, project.Spec.InstanceRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		if reason != "" {
			// The instance is gone or unreachable; there is nothing left to
			// clean up against it, so proceed with removing the finalizer.
			log.Info("Skipping remote delete because the instance is unavailable", "reason", reason)
		} else if err := resolved.Client.DeleteProject(ctx, project.Status.ID); err != nil && !vikunja.IsNotFound(err) {
			log.Error(err, "Failed to delete project on Vikunja instance")
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(project, vikunjav1alpha1.Finalizer)
	if err := r.Update(ctx, project); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ProjectReconciler) setReady(ctx context.Context, project *vikunjav1alpha1.Project) (ctrl.Result, error) {
	project.Status.ObservedGeneration = project.Generation
	setReadyCondition(&project.Status.Conditions, project.Generation, metav1.ConditionTrue,
		vikunjav1alpha1.ReasonReconcileSuccess, "Project matches the desired state on the Vikunja instance")
	if err := r.Status().Update(ctx, project); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: driftRequeueInterval}, nil
}

func (r *ProjectReconciler) setNotReady(ctx context.Context, project *vikunjav1alpha1.Project, reason, message string) (ctrl.Result, error) {
	project.Status.ObservedGeneration = project.Generation
	setReadyCondition(&project.Status.Conditions, project.Generation, metav1.ConditionFalse, reason, message)
	if err := r.Status().Update(ctx, project); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: notReadyRequeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &vikunjav1alpha1.Project{}, instanceRefField,
		func(obj client.Object) []string {
			return []string{obj.(*vikunjav1alpha1.Project).Spec.InstanceRef.Name}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&vikunjav1alpha1.Project{}).
		Watches(
			&vikunjav1alpha1.VikunjaInstance{},
			handler.EnqueueRequestsFromMapFunc(mapToDependentsByField(r.Client, instanceRefField, &vikunjav1alpha1.ProjectList{})),
		).
		Named("project").
		Complete(r)
}
