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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PageserverSpec defines the desired state of Pageserver
// +kubebuilder:validation:XValidation:rule="self.id == oldSelf.id",message="id is immutable"
// +kubebuilder:validation:XValidation:rule="self.cluster == oldSelf.cluster",message="cluster is immutable"
// +kubebuilder:validation:XValidation:rule="self.storageConfig.size == oldSelf.storageConfig.size",message="storageConfig.size is immutable"
// +kubebuilder:validation:XValidation:rule="has(self.storageConfig.storageClass) == has(oldSelf.storageConfig.storageClass) && (!has(self.storageConfig.storageClass) || self.storageConfig.storageClass == oldSelf.storageConfig.storageClass)",message="storageConfig.storageClass is immutable"
// +kubebuilder:validation:XValidation:rule="has(self.bucketCredentialsSecret.name) && self.bucketCredentialsSecret.name.size() > 0",message="bucketCredentialsSecret.name is required"
type PageserverSpec struct {
	// ID which the pageserver uses when registering with storage-controller
	// This ID must be unique within the cluster.
	ID uint64 `json:"id"`

	// Used to deterministically setup which storage controller and broker to communicate with
	// +kubebuilder:validation:MinLength:=1
	Cluster string `json:"cluster"`

	// Reference to a Secret containing credentials for accessing a storage bucket.
	BucketCredentialsSecret *corev1.SecretReference `json:"bucketCredentialsSecret"`

	// PVC configuration
	StorageConfig StorageConfig `json:"storageConfig"`
}

// PageserverStatus defines the observed state of Pageserver.
type PageserverStatus struct {
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

// Pageserver is the Schema for the pageservers API
type Pageserver struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Pageserver
	// +required
	Spec PageserverSpec `json:"spec"`

	// status defines the observed state of Pageserver
	// +optional
	Status PageserverStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PageserverList contains a list of Pageserver
type PageserverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Pageserver `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pageserver{}, &PageserverList{})
}
