/*
Copyright 2025.

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
)

// ProjectSpec defines the desired state of Project
// +kubebuilder:validation:XValidation:rule="self.cluster == oldSelf.cluster",message="cluster is immutable"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.tenantId) || oldSelf.tenantId.size() == 0 || (has(self.tenantId) && self.tenantId == oldSelf.tenantId)",message="tenantId can be generated once, but cannot be changed after it is set"
type ProjectSpec struct {
	// Name of the cluster where the project will be created.
	// +kubebuilder:validation:MinLength:=1
	ClusterName string `json:"cluster"`

	// Will be generated unless specified.
	// Has to be a 32 character alphanumeric string.
	// +optional
	// +kubebuilder:validation:Pattern:=`^$|^[0-9a-f]{32}$`
	TenantID string `json:"tenantId"`

	// PostgreSQL version to use for the project.
	// +optional
	// +kubebuilder:validation:Enum=14;15;16;17
	// +kubebuilder:default:=17
	PGVersion int `json:"pgVersion"`
}

// ProjectStatus defines the observed state of Project.
type ProjectStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=".status.conditions[?(@.type==\"Available\")].status"
// +kubebuilder:printcolumn:name="Progressing",type=string,JSONPath=".status.conditions[?(@.type==\"Progressing\")].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// Project is the Schema for the projects API
type Project struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Project
	// +required
	Spec ProjectSpec `json:"spec"`

	// status defines the observed state of Project
	// +optional
	Status ProjectStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
