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

// TaskSpec defines the desired state of Task
type TaskSpec struct {
	// projectRef is the name of the Project resource, in the same namespace,
	// this task belongs to. The instance to create the task on is taken from
	// the referenced Project.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectRef string `json:"projectRef"`

	// title of the task.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Title string `json:"title"`

	// description is an optional longer description of the task.
	// +optional
	Description string `json:"description,omitempty"`

	// done marks the task as completed.
	// +optional
	Done bool `json:"done,omitempty"`

	// priority of the task: 0 = unset, 1 = low, 2 = medium, 3 = high, 4 = urgent, 5 = DO NOW.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	// +optional
	Priority int64 `json:"priority,omitempty"`

	// hexColor is the hex color code of this task, without the leading '#',
	// e.g. "ff8800".
	// +kubebuilder:validation:Pattern=`^[0-9a-fA-F]{6}$`
	// +optional
	HexColor string `json:"hexColor,omitempty"`

	// dueDate is when the task is due.
	// +optional
	DueDate *metav1.Time `json:"dueDate,omitempty"`

	// startDate is when the task starts.
	// +optional
	StartDate *metav1.Time `json:"startDate,omitempty"`

	// endDate is when the task ends.
	// +optional
	EndDate *metav1.Time `json:"endDate,omitempty"`

	// labelRefs is a list of names of Label resources, in the same namespace, to
	// attach to this task. The operator adds and removes labels on the task to
	// match this list exactly.
	// +optional
	LabelRefs []string `json:"labelRefs,omitempty"`

	// assignees is a list of Vikunja usernames to assign to this task. The
	// operator adds and removes assignees on the task to match this list exactly.
	// Usernames must already exist on the Vikunja instance and have access to
	// the task's project.
	// +optional
	Assignees []string `json:"assignees,omitempty"`
}

// TaskStatus defines the observed state of Task.
type TaskStatus struct {
	// id is the numeric id Vikunja assigned to this task.
	// +optional
	ID int64 `json:"id,omitempty"`

	// identifier is the textual task identifier Vikunja assigned, derived from
	// the project identifier and the task index (e.g. "PROJ-12").
	// +optional
	Identifier string `json:"identifier,omitempty"`

	// index is the per-project task index Vikunja assigned.
	// +optional
	Index int64 `json:"index,omitempty"`

	// conditions represent the current state of the Task resource.
	//
	// The "Ready" condition is True when the task, its labels and its assignees
	// have been created or updated on the target VikunjaInstance to match spec.
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
// +kubebuilder:printcolumn:name="Identifier",type=string,JSONPath=".status.identifier"
// +kubebuilder:printcolumn:name="Done",type=boolean,JSONPath=".spec.done"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// Task is the Schema for the tasks API
type Task struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Task
	// +required
	Spec TaskSpec `json:"spec"`

	// status defines the observed state of Task
	// +optional
	Status TaskStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// TaskList contains a list of Task
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Task `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Task{}, &TaskList{})
		return nil
	})
}
