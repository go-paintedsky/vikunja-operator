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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vikunjav1alpha1 "github.com/go-paintedsky/vikunja-operator/api/v1alpha1"
	"github.com/go-paintedsky/vikunja-operator/internal/vikunja"
)

// VikunjaInstanceReconciler reconciles a VikunjaInstance object
type VikunjaInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=vikunjainstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=vikunjainstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=vikunjainstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile validates that the VikunjaInstance's baseURL is reachable and its
// API token is valid, and records the result (plus the instance's reported
// version) in status. It does not create or manage anything in Vikunja
// itself -- it exists so Project/Task/Team/Label controllers can wait on a
// single "is this instance usable" signal instead of each probing it.
func (r *VikunjaInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var instance vikunjav1alpha1.VikunjaInstance
	if err := r.Get(ctx, req.NamespacedName, &instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	token, err := readSecretKey(ctx, r.Client, instance.Namespace, instance.Spec.APITokenSecretRef)
	if err != nil {
		log.Info("API token secret not available", "secret", instance.Spec.APITokenSecretRef.Name, "error", err.Error())
		return r.setStatus(ctx, &instance, "", "", metav1.ConditionFalse,
			vikunjav1alpha1.ReasonInstanceNotReady, "Failed to read API token secret: "+err.Error())
	}

	vc := vikunja.NewClient(instance.Spec.BaseURL, token, instance.Spec.InsecureSkipVerify)

	info, err := vc.GetInfo(ctx)
	if err != nil {
		log.Info("Failed to reach Vikunja instance", "baseURL", instance.Spec.BaseURL, "error", err.Error())
		return r.setStatus(ctx, &instance, "", "", metav1.ConditionFalse,
			vikunjav1alpha1.ReasonInstanceNotReady, "Failed to reach instance: "+err.Error())
	}
	version := info.Version

	user, err := vc.GetCurrentUser(ctx)
	if err != nil {
		log.Info("API token rejected by Vikunja instance", "baseURL", instance.Spec.BaseURL, "error", err.Error())
		return r.setStatus(ctx, &instance, version, "", metav1.ConditionFalse,
			vikunjav1alpha1.ReasonInstanceNotReady, "API token rejected: "+err.Error())
	}

	log.Info("VikunjaInstance is ready", "baseURL", instance.Spec.BaseURL, "version", version, "tokenOwner", user.Username)
	return r.setStatus(ctx, &instance, version, user.Username, metav1.ConditionTrue,
		vikunjav1alpha1.ReasonReconcileSuccess, "Instance is reachable and the API token is valid")
}

func (r *VikunjaInstanceReconciler) setStatus(
	ctx context.Context,
	instance *vikunjav1alpha1.VikunjaInstance,
	version, tokenOwner string,
	status metav1.ConditionStatus,
	reason, message string,
) (ctrl.Result, error) {
	instance.Status.Version = version
	instance.Status.TokenOwner = tokenOwner
	instance.Status.ObservedGeneration = instance.Generation
	setReadyCondition(&instance.Status.Conditions, instance.Generation, status, reason, message)

	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	if status != metav1.ConditionTrue {
		return ctrl.Result{RequeueAfter: notReadyRequeueInterval}, nil
	}
	return ctrl.Result{RequeueAfter: driftRequeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VikunjaInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vikunjav1alpha1.VikunjaInstance{}).
		Named("vikunjainstance").
		Complete(r)
}
