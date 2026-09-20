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

// TeamReconciler reconciles a Team object
type TeamReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=teams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=teams/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=teams/finalizers,verbs=update
// +kubebuilder:rbac:groups=vikunja.paintedsky.io,resources=vikunjainstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile creates, updates or deletes the Vikunja team backing this Team
// resource, and adds, removes and promotes/demotes its members so that both
// match spec.
func (r *TeamReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var team vikunjav1alpha1.Team
	if err := r.Get(ctx, req.NamespacedName, &team); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !team.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &team)
	}

	if !controllerutil.ContainsFinalizer(&team, vikunjav1alpha1.Finalizer) {
		controllerutil.AddFinalizer(&team, vikunjav1alpha1.Finalizer)
		if err := r.Update(ctx, &team); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	resolved, reason, message, err := resolveInstance(ctx, r.Client, team.Namespace, team.Spec.InstanceRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	if reason != "" {
		return r.setNotReady(ctx, &team, reason, message)
	}

	desired := &vikunja.Team{
		Name:        team.Spec.Name,
		Description: team.Spec.Description,
		IsPublic:    team.Spec.IsPublic,
	}

	remote, err := r.upsertTeam(ctx, resolved.Client, team.Status.ID, desired)
	if err != nil {
		log.Error(err, "Failed to reconcile team on Vikunja instance", "instance", resolved.Instance.Name)
		return r.setNotReady(ctx, &team, vikunjav1alpha1.ReasonReconcileError, err.Error())
	}

	if err := r.reconcileMembers(ctx, resolved.Client, remote, team.Spec.Members, resolved.Instance.Status.TokenOwner); err != nil {
		log.Error(err, "Failed to reconcile team members on Vikunja instance", "instance", resolved.Instance.Name)
		team.Status.ID = remote.ID
		return r.setNotReady(ctx, &team, vikunjav1alpha1.ReasonReconcileError, err.Error())
	}

	team.Status.ID = remote.ID
	return r.setReady(ctx, &team)
}

func (r *TeamReconciler) upsertTeam(ctx context.Context, vc *vikunja.Client, id int64, desired *vikunja.Team) (*vikunja.Team, error) {
	if id == 0 {
		return vc.CreateTeam(ctx, desired)
	}
	remote, err := vc.UpdateTeam(ctx, id, desired)
	if vikunja.IsNotFound(err) {
		return vc.CreateTeam(ctx, desired)
	}
	return remote, err
}

// reconcileMembers adds, removes and toggles the admin flag of remote's
// members so they match desired exactly, except that tokenOwner (the user the
// VikunjaInstance's API token belongs to) is never removed: Vikunja always
// makes the team's creator an admin member, and removing the operator's own
// access would strand the team without a way to manage it further.
func (r *TeamReconciler) reconcileMembers(ctx context.Context, vc *vikunja.Client, remote *vikunja.Team, desired []vikunjav1alpha1.TeamMemberSpec, tokenOwner string) error {
	desiredByUsername := make(map[string]bool, len(desired))
	for _, m := range desired {
		desiredByUsername[m.Username] = m.Admin
	}

	currentByUsername := make(map[string]vikunja.TeamMember, len(remote.Members))
	for _, m := range remote.Members {
		currentByUsername[m.Username] = m
	}

	for username, admin := range desiredByUsername {
		if _, ok := currentByUsername[username]; !ok {
			if _, err := vc.AddTeamMember(ctx, remote.ID, username, admin); err != nil {
				return fmt.Errorf("add member %q: %w", username, err)
			}
		}
	}

	for username, current := range currentByUsername {
		wantAdmin, wanted := desiredByUsername[username]
		switch {
		case !wanted:
			if username == tokenOwner {
				continue
			}
			if err := vc.RemoveTeamMember(ctx, remote.ID, username); err != nil {
				return fmt.Errorf("remove member %q: %w", username, err)
			}
		case wantAdmin != current.Admin:
			if _, err := vc.ToggleTeamMemberAdmin(ctx, remote.ID, username); err != nil {
				return fmt.Errorf("toggle admin for member %q: %w", username, err)
			}
		}
	}

	return nil
}

func (r *TeamReconciler) reconcileDelete(ctx context.Context, team *vikunjav1alpha1.Team) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(team, vikunjav1alpha1.Finalizer) {
		return ctrl.Result{}, nil
	}

	if team.Status.ID != 0 {
		resolved, reason, _, err := resolveInstance(ctx, r.Client, team.Namespace, team.Spec.InstanceRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		if reason != "" {
			log.Info("Skipping remote delete because the instance is unavailable", "reason", reason)
		} else if err := resolved.Client.DeleteTeam(ctx, team.Status.ID); err != nil && !vikunja.IsNotFound(err) {
			log.Error(err, "Failed to delete team on Vikunja instance")
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(team, vikunjav1alpha1.Finalizer)
	if err := r.Update(ctx, team); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *TeamReconciler) setReady(ctx context.Context, team *vikunjav1alpha1.Team) (ctrl.Result, error) {
	team.Status.ObservedGeneration = team.Generation
	setReadyCondition(&team.Status.Conditions, team.Generation, metav1.ConditionTrue,
		vikunjav1alpha1.ReasonReconcileSuccess, "Team and its members match the desired state on the Vikunja instance")
	if err := r.Status().Update(ctx, team); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: driftRequeueInterval}, nil
}

func (r *TeamReconciler) setNotReady(ctx context.Context, team *vikunjav1alpha1.Team, reason, message string) (ctrl.Result, error) {
	team.Status.ObservedGeneration = team.Generation
	setReadyCondition(&team.Status.Conditions, team.Generation, metav1.ConditionFalse, reason, message)
	if err := r.Status().Update(ctx, team); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: notReadyRequeueInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TeamReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &vikunjav1alpha1.Team{}, instanceRefField,
		func(obj client.Object) []string {
			return []string{obj.(*vikunjav1alpha1.Team).Spec.InstanceRef.Name}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&vikunjav1alpha1.Team{}).
		Watches(
			&vikunjav1alpha1.VikunjaInstance{},
			handler.EnqueueRequestsFromMapFunc(mapToDependentsByField(r.Client, instanceRefField, &vikunjav1alpha1.TeamList{})),
		).
		Named("team").
		Complete(r)
}
