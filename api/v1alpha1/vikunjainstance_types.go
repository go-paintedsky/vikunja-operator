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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// VikunjaInstanceSpec defines the desired state of VikunjaInstance
type VikunjaInstanceSpec struct {
	// baseURL is the base URL of the Vikunja API v2, e.g. "https://try.vikunja.io/api/v2".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	BaseURL string `json:"baseURL"`

	// apiTokenSecretRef references a Secret containing a Vikunja API token
	// (a "tk_"-prefixed personal access token created under Settings > API Tokens,
	// or a bot token) used to authenticate as a Bearer token against the instance.
	// The token must have permissions for whichever of projects, tasks, teams and
	// labels this instance's dependents manage.
	// +kubebuilder:validation:Required
	APITokenSecretRef SecretKeyReference `json:"apiTokenSecretRef"`

	// insecureSkipVerify disables TLS certificate verification when connecting to
	// the instance. Only use this for testing against instances with self-signed
	// certificates.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// VikunjaInstanceStatus defines the observed state of VikunjaInstance.
type VikunjaInstanceStatus struct {
	// conditions represent the current state of the VikunjaInstance resource.
	//
	// The "Ready" condition is True when the operator was able to reach baseURL
	// and authenticate with the referenced API token.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// version is the Vikunja server version reported by the instance's /info endpoint.
	// +optional
	Version string `json:"version,omitempty"`

	// tokenOwner is the username the configured API token authenticates as.
	// +optional
	TokenOwner string `json:"tokenOwner,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Base URL",type=string,JSONPath=".spec.baseURL"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=".status.version"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// VikunjaInstance is the Schema for the vikunjainstances API
type VikunjaInstance struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of VikunjaInstance
	// +required
	Spec VikunjaInstanceSpec `json:"spec"`

	// status defines the observed state of VikunjaInstance
	// +optional
	Status VikunjaInstanceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// VikunjaInstanceList contains a list of VikunjaInstance
type VikunjaInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []VikunjaInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &VikunjaInstance{}, &VikunjaInstanceList{})
		return nil
	})
}
