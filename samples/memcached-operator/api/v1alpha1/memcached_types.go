/*
Copyright 2023.

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
	"github.com/disaster37/operator-sdk-extra/v2/pkg/apis"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MemcachedAnnotationKey = "cache.example.com"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MemcachedSpec defines the desired state of Memcached
type MemcachedSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	// +kubebuilder:validation:ExclusiveMaximum=false

	// Size defines the number of Memcached instances
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Size int32 `json:"size,omitempty"`

	// Port defines the port that will be used to init the container with the image
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ContainerPort int32 `json:"containerPort,omitempty"`
}

type MemcachedStatus struct {
	apis.BasicMultiPhaseObjectStatus `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1,memcached-deployment},{ConfigMap,v1,memcached-configmap}}
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Cluster deployment status"
// +kubebuilder:printcolumn:name="Error",type="boolean",JSONPath=".status.isOnError",description="Is on error"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='MemcachedReady')].status",description="Cluster health"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Memcached is the Schema for the memcacheds API
type Memcached struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemcachedSpec   `json:"spec,omitempty"`
	Status MemcachedStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MemcachedList contains a list of Memcached
type MemcachedList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Memcached `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Memcached{}, &MemcachedList{})
}
