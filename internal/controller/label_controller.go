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

// LabelReconciler reconciles a Label object
type LabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=labels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=labels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=labels/finalizers,verbs=update
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=vikunjainstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile creates, updates or deletes the Vikunja label backing this Label
// resource so that it matches spec.
func (r *LabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var label vikunjav1alpha1.Label
	if err := r.Get(ctx, req.NamespacedName, &label); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !label.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &label)
	}

	if !controllerutil.ContainsFinalizer(&label, vikunjav1alpha1.Finalizer) {
		controllerutil.AddFinalizer(&label, vikunjav1alpha1.Finalizer)
		if err := r.Update(ctx, &label); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	resolved, reason, message, err := resolveInstance(ctx, r.Client, label.Namespace, label.Spec.InstanceRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	if reason != "" {
		return r.setNotReady(ctx, &label, reason, message)
	}

	desired := &vikunja.Label{
		Title:       label.Spec.Title,
		Description: label.Spec.Description,
		HexColor:    label.Spec.HexColor,
	}

	remote, err := r.upsertLabel(ctx, resolved.Client, label.Status.ID, desired)
	if err != nil {
		log.Error(err, "Failed to reconcile label on Vikunja instance", "instance", resolved.Instance.Name)
		return r.setNotReady(ctx, &label, vikunjav1alpha1.ReasonReconcileError, err.Error())
	}

	label.Status.ID = remote.ID
	return r.setReady(ctx, &label)
}

func (r *LabelReconciler) upsertLabel(ctx context.Context, vc *vikunja.Client, id int64, desired *vikunja.Label) (*vikunja.Label, error) {
	if id == 0 {
		return vc.CreateLabel(ctx, desired)
	}
	remote, err := vc.UpdateLabel(ctx, id, desired)
	if vikunja.IsNotFound(err) {
		return vc.CreateLabel(ctx, desired)
	}
	return remote, err
}

func (r *LabelReconciler) reconcileDelete(ctx context.Context, label *vikunjav1alpha1.Label) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(label, vikunjav1alpha1.Finalizer) {
		return ctrl.Result{}, nil
	}

	if label.Status.ID != 0 {
		resolved, reason, _, err := resolveInstance(ctx, r.Client, label.Namespace, label.Spec.InstanceRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		if reason != "" {
			log.Info("Skipping remote delete because the instance is unavailable", "reason", reason)
		} else if err := resolved.Client.DeleteLabel(ctx, label.Status.ID); err != nil && !vikunja.IsNotFound(err) {
			log.Error(err, "Failed to delete label on Vikunja instance")
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(label, vikunjav1alpha1.Finalizer)
	if err := r.Update(ctx, label); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *LabelReconciler) setReady(ctx context.Context, label *vikunjav1alpha1.Label) (ctrl.Result, error) {
	label.Status.ObservedGeneration = label.Generation
	setReadyCondition(&label.Status.Conditions, label.Generation, metav1.ConditionTrue,
		vikunjav1alpha1.ReasonReconcileSuccess, "Label matches the desired state on the Vikunja instance")
	if err := r.Status().Update(ctx, label); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: driftRequeueInterval}, nil
}

func (r *LabelReconciler) setNotReady(ctx context.Context, label *vikunjav1alpha1.Label, reason, message string) (ctrl.Result, error) {
	label.Status.ObservedGeneration = label.Generation
	setReadyCondition(&label.Status.Conditions, label.Generation, metav1.ConditionFalse, reason, message)
	if err := r.Status().Update(ctx, label); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: notReadyRequeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &vikunjav1alpha1.Label{}, instanceRefField,
		func(obj client.Object) []string {
			return []string{obj.(*vikunjav1alpha1.Label).Spec.InstanceRef.Name}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&vikunjav1alpha1.Label{}).
		Watches(
			&vikunjav1alpha1.VikunjaInstance{},
			handler.EnqueueRequestsFromMapFunc(mapToDependentsByField(r.Client, instanceRefField, &vikunjav1alpha1.LabelList{})),
		).
		Named("label").
		Complete(r)
}
