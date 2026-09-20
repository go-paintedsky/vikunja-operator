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

// TeamMemberSpec is a single desired member of a Team.
type TeamMemberSpec struct {
	// username of the Vikunja user to add to the team.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Username string `json:"username"`

	// admin grants the member admin rights on the team (add/remove members,
	// toggle other members' admin status).
	// +optional
	Admin bool `json:"admin,omitempty"`
}

// TeamSpec defines the desired state of Team
type TeamSpec struct {
	// instanceRef references the VikunjaInstance this team should be created on.
	// +kubebuilder:validation:Required
	InstanceRef InstanceReference `json:"instanceRef"`

	// name is the name of the team.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=250
	Name string `json:"name"`

	// description is an optional longer description of the team.
	// +optional
	Description string `json:"description,omitempty"`

	// isPublic marks the team as publicly discoverable when sharing a project,
	// if the instance has public teams enabled.
	// +optional
	IsPublic bool `json:"isPublic,omitempty"`

	// members is the desired list of team members. The operator adds and removes
	// members to match this list exactly, except that it never removes the user
	// the VikunjaInstance's API token belongs to.
	// +optional
	Members []TeamMemberSpec `json:"members,omitempty"`
}

// TeamStatus defines the observed state of Team.
type TeamStatus struct {
	// id is the numeric id Vikunja assigned to this team.
	// +optional
	ID int64 `json:"id,omitempty"`

	// conditions represent the current state of the Team resource.
	//
	// The "Ready" condition is True when the team and its membership have been
	// created or updated on the referenced VikunjaInstance to match spec.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="ID",type=integer,JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// Team is the Schema for the teams API
type Team struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Team
	// +required
	Spec TeamSpec `json:"spec"`

	// status defines the observed state of Team
	// +optional
	Status TeamStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// TeamList contains a list of Team
type TeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Team `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Team{}, &TeamList{})
		return nil
	})
}
