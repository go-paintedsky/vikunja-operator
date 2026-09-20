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

// Package controller contains the reconcilers for every vikunja.paintedsky.io
// kind, plus a handful of helpers shared between them (this file) for
// resolving a VikunjaInstance/Project/Label reference into either a usable
// object or a reason a caller can surface as a status condition.
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vikunjav1alpha1 "github.com/go-paintedsky/vikunja-operator/api/v1alpha1"
	"github.com/go-paintedsky/vikunja-operator/internal/vikunja"
)

const (
	// notReadyRequeueInterval is how soon a resource is requeued after finding
	// a dependency (VikunjaInstance, Project, Label) not yet ready. Short,
	// since the dependency is usually expected to become ready shortly.
	notReadyRequeueInterval = 15 * time.Second

	// driftRequeueInterval is how often an already-Ready resource is requeued
	// to detect and correct drift from changes made directly against the
	// Vikunja API, outside of this operator.
	driftRequeueInterval = 10 * time.Minute

	// instanceRefField and projectRefField are field indexer keys used to look
	// up dependents of a VikunjaInstance/Project by spec reference, so their
	// controllers can watch for changes without polling.
	instanceRefField = ".spec.instanceRef.name"
	projectRefField  = ".spec.projectRef"
)

// resolvedInstance is a VikunjaInstance that is Ready, along with a client
// authenticated against it.
type resolvedInstance struct {
	Client   *vikunja.Client
	Instance *vikunjav1alpha1.VikunjaInstance
}

// resolveInstance looks up the VikunjaInstance referenced by ref in namespace
// ns and, if it is Ready, builds a client for it.
//
// It returns a non-empty reason/message pair (and a nil error) when the
// instance can't be used yet for an expected, likely-transient cause (not
// found, not Ready, secret missing) -- callers should surface that as a
// status condition and requeue rather than treat it as a Reconcile error.
// A non-nil error indicates an unexpected failure talking to the Kubernetes
// API and should be returned from Reconcile as-is.
func resolveInstance(ctx context.Context, c client.Client, ns string, ref vikunjav1alpha1.InstanceReference) (*resolvedInstance, string, string, error) {
	var instance vikunjav1alpha1.VikunjaInstance
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &instance); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, vikunjav1alpha1.ReasonInstanceNotFound, fmt.Sprintf("VikunjaInstance %q not found", ref.Name), nil
		}
		return nil, "", "", err
	}

	if !meta.IsStatusConditionTrue(instance.Status.Conditions, vikunjav1alpha1.ConditionTypeReady) {
		return nil, vikunjav1alpha1.ReasonInstanceNotReady, fmt.Sprintf("VikunjaInstance %q is not Ready", ref.Name), nil
	}

	token, err := readSecretKey(ctx, c, ns, instance.Spec.APITokenSecretRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, vikunjav1alpha1.ReasonInstanceNotReady, fmt.Sprintf(
				"secret %q for VikunjaInstance %q not found", instance.Spec.APITokenSecretRef.Name, ref.Name), nil
		}
		return nil, "", "", err
	}

	return &resolvedInstance{
		Client:   vikunja.NewClient(instance.Spec.BaseURL, token, instance.Spec.InsecureSkipVerify),
		Instance: &instance,
	}, "", "", nil
}

// readSecretKey reads a single key out of a Secret in namespace ns.
func readSecretKey(ctx context.Context, c client.Client, ns string, ref vikunjav1alpha1.SecretKeyReference) (string, error) {
	key := ref.Key
	if key == "" {
		key = "token"
	}
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &secret); err != nil {
		return "", err
	}
	value, ok := secret.Data[key]
	if !ok {
		return "", apierrors.NewNotFound(corev1.Resource("secrets"), fmt.Sprintf("%s (missing key %q)", ref.Name, key))
	}
	return string(value), nil
}

// getReadyProject looks up the Project referenced by name in namespace ns and
// checks that it has already been assigned a Vikunja id. See resolveInstance
// for the reason/message/error contract.
func getReadyProject(ctx context.Context, c client.Client, ns, name string) (*vikunjav1alpha1.Project, string, string, error) {
	var project vikunjav1alpha1.Project
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &project); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, vikunjav1alpha1.ReasonDependencyNotReady, fmt.Sprintf("Project %q not found", name), nil
		}
		return nil, "", "", err
	}
	if project.Status.ID == 0 {
		return nil, vikunjav1alpha1.ReasonDependencyNotReady, fmt.Sprintf("Project %q is not yet Ready", name), nil
	}
	return &project, "", "", nil
}

// getReadyLabel looks up the Label referenced by name in namespace ns and
// checks that it has already been assigned a Vikunja id. See resolveInstance
// for the reason/message/error contract.
func getReadyLabel(ctx context.Context, c client.Client, ns, name string) (*vikunjav1alpha1.Label, string, string, error) {
	var label vikunjav1alpha1.Label
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &label); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, vikunjav1alpha1.ReasonDependencyNotReady, fmt.Sprintf("Label %q not found", name), nil
		}
		return nil, "", "", err
	}
	if label.Status.ID == 0 {
		return nil, vikunjav1alpha1.ReasonDependencyNotReady, fmt.Sprintf("Label %q is not yet Ready", name), nil
	}
	return &label, "", "", nil
}

// mapToDependentsByField returns a handler.MapFunc that, given a changed
// object (e.g. a VikunjaInstance or Project), lists objects of listPrototype's
// type in the same namespace whose field index equals the changed object's
// name, and enqueues a reconcile request for each. It's used so a dependent
// resource (Project, Task, Team, Label) is reconciled promptly when the thing
// it references (spec.instanceRef.name, spec.projectRef) changes, instead of
// waiting for its next periodic drift check.
func mapToDependentsByField(c client.Client, field string, listPrototype client.ObjectList) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		list := listPrototype.DeepCopyObject().(client.ObjectList) //nolint:forcetypeassert // DeepCopyObject returns the same concrete type
		if err := c.List(ctx, list,
			client.InNamespace(obj.GetNamespace()),
			client.MatchingFields{field: obj.GetName()},
		); err != nil {
			return nil
		}

		items, err := meta.ExtractList(list)
		if err != nil {
			return nil
		}

		requests := make([]reconcile.Request, 0, len(items))
		for _, item := range items {
			o, ok := item.(client.Object)
			if !ok {
				continue
			}
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: o.GetNamespace(), Name: o.GetName()},
			})
		}
		return requests
	}
}

// setReadyCondition records the Ready condition in conditions.
func setReadyCondition(conditions *[]metav1.Condition, generation int64, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               vikunjav1alpha1.ConditionTypeReady,
		Status:             status,
		ObservedGeneration: generation,
		Reason:             reason,
		Message:            message,
	})
}
