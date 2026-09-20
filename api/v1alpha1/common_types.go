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

package v1alpha1

// InstanceReference references a VikunjaInstance resource in the same namespace
// as the resource that embeds it.
type InstanceReference struct {
	// name is the name of the VikunjaInstance resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// SecretKeyReference references a key within a Secret in the same namespace as
// the resource that embeds it.
type SecretKeyReference struct {
	// name is the name of the referenced Secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// key is the key of the Secret to select the token from.
	// +kubebuilder:default="token"
	// +optional
	Key string `json:"key,omitempty"`
}

// Condition types used across all vikunja.paintedsky.io resources.
const (
	// ConditionTypeReady indicates whether the resource has been successfully
	// reconciled against the Vikunja API and reflects its current state.
	ConditionTypeReady = "Ready"
)

// Condition reasons used across all vikunja.paintedsky.io resources.
const (
	// ReasonReconcileSuccess is used when a resource was successfully reconciled.
	ReasonReconcileSuccess = "ReconcileSuccess"
	// ReasonReconcileError is used when reconciling against the Vikunja API failed.
	ReasonReconcileError = "ReconcileError"
	// ReasonInvalidSpec is used when the resource's spec is invalid or references
	// another resource that does not exist or is not ready.
	ReasonInvalidSpec = "InvalidSpec"
	// ReasonInstanceNotFound is used when spec.instanceRef does not resolve to an
	// existing VikunjaInstance.
	ReasonInstanceNotFound = "InstanceNotFound"
	// ReasonInstanceNotReady is used when the referenced VikunjaInstance is not Ready.
	ReasonInstanceNotReady = "InstanceNotReady"
	// ReasonDependencyNotReady is used when a referenced Project or Label has not
	// yet been reconciled (i.e. has no Vikunja id assigned).
	ReasonDependencyNotReady = "DependencyNotReady"
)

// Finalizer is attached to every resource that owns an object in Vikunja, so
// that resource's Reconcile can delete the remote object before the Kubernetes
// object is removed.
const Finalizer = "vikunja.paintedsky.io/finalizer"
