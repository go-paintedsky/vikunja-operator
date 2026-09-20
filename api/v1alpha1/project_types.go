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

// ProjectSpec defines the desired state of Project
type ProjectSpec struct {
	// instanceRef references the VikunjaInstance this project should be created on.
	// +kubebuilder:validation:Required
	InstanceRef InstanceReference `json:"instanceRef"`

	// title is the name of the project which does not need to be DNS compliant,
	// which is why we can't simply infer it from the project CR's metadata.name
	// value, which must be DNS compliant.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Title string `json:"title"`

	// description is an optional longer description of the project.
	// +optional
	Description string `json:"description,omitempty"`

	// identifier is the unique project short identifier used to build task
	// identifiers (e.g. "PROJ-123"). Left empty, Vikunja assigns one automatically.
	// +kubebuilder:validation:MaxLength=10
	// +optional
	Identifier string `json:"identifier,omitempty"`

	// hexColor is the hex color code of this project, without the leading '#',
	// e.g. "ff8800".
	// +kubebuilder:validation:Pattern=`^[0-9a-fA-F]{6}$`
	// +optional
	HexColor string `json:"hexColor,omitempty"`

	// isArchived marks the project as archived (read-only) in Vikunja.
	// +optional
	IsArchived bool `json:"isArchived,omitempty"`

	// parentProjectRef is the name of another Project resource in the same
	// namespace that should be the parent of this project. Omit for a
	// top-level project.
	// +optional
	ParentProjectRef *string `json:"parentProjectRef,omitempty"`
}

// ProjectStatus defines the observed state of Project.
type ProjectStatus struct {
	// id is the numeric id Vikunja assigned to this project.
	// +optional
	ID int64 `json:"id,omitempty"`

	// parentProjectID is the numeric id of the resolved parent project, or 0 if
	// this is a top-level project.
	// +optional
	ParentProjectID int64 `json:"parentProjectID,omitempty"`

	// conditions represent the current state of the Project resource.
	//
	// The "Ready" condition is True when the project has been created or updated
	// on the referenced VikunjaInstance to match spec.
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
// +kubebuilder:printcolumn:name="Title",type=string,JSONPath=".spec.title"
// +kubebuilder:printcolumn:name="ID",type=integer,JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// Project is the Schema for the projects API
type Project struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Project
	// +required
	Spec ProjectSpec `json:"spec"`

	// status defines the observed state of Project
	// +optional
	Status ProjectStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Project `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Project{}, &ProjectList{})
		return nil
	})
}
