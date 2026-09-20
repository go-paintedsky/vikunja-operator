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

// LabelSpec defines the desired state of Label
type LabelSpec struct {
	// instanceRef references the VikunjaInstance this label should be created on.
	// +kubebuilder:validation:Required
	InstanceRef InstanceReference `json:"instanceRef"`

	// title of the label. You'll see this one on tasks associated with it.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=250
	Title string `json:"title"`

	// description is an optional longer description of the label.
	// +optional
	Description string `json:"description,omitempty"`

	// hexColor is the hex color code of this label, without the leading '#',
	// e.g. "ff8800".
	// +kubebuilder:validation:Pattern=`^[0-9a-fA-F]{6}$`
	// +optional
	HexColor string `json:"hexColor,omitempty"`
}

// LabelStatus defines the observed state of Label.
type LabelStatus struct {
	// id is the numeric id Vikunja assigned to this label.
	// +optional
	ID int64 `json:"id,omitempty"`

	// conditions represent the current state of the Label resource.
	//
	// The "Ready" condition is True when the label has been created or updated
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

// Label is the Schema for the labels API
type Label struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Label
	// +required
	Spec LabelSpec `json:"spec"`

	// status defines the observed state of Label
	// +optional
	Status LabelStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// LabelList contains a list of Label
type LabelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Label `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Label{}, &LabelList{})
		return nil
	})
}
