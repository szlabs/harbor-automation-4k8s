/*


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
	"sigs.k8s.io/kustomize/kstatus/status"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HarborServerConfigurationSpec defines the desired state of HarborServerConfiguration
type HarborServerConfigurationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="https?://.*"
	ServerURL string `json:"serverURL"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*"
	AccessSecretRef string `json:"access_secret_ref"`

	// The version of the Harbor server
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="(0|[1-9]\\d*)\\.(0|[1-9]\\d*)\\.(0|[1-9]\\d*)(?:-((?:0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\\.(?:0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\\+([0-9a-zA-Z-]+(?:\\.[0-9a-zA-Z-]+)*))?"
	Version string `json:"version"`
}

// HarborServerConfigurationStatus defines the observed state of HarborServerConfiguration
type HarborServerConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Indicate if the server is healthy
	Status string `json:"status"`

	// Conditions list of extracted conditions from Resource
	// Add the health status of harbor components into condition list
	// +listType:map
	// +listMapKey:type
	Conditions []Condition `json:"conditions"`
}

// Condition defines the general format for conditions on Kubernetes resources.
// In practice, each kubernetes resource defines their own format for conditions, but
// most (maybe all) follows this structure.
type Condition struct {
	// +kubebuilder:validation:Required
	// Type condition type
	Type status.ConditionType `json:"type"`

	// +kubebuilder:validation:Required
	// Status String that describes the condition status
	Status corev1.ConditionStatus `json:"status"`

	// +kubebuilder:validation:Optional
	// Reason one work CamelCase reason
	Reason string `json:"reason,omitempty"`

	// +kubebuilder:validation:Optional
	// Message Human readable reason string
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true

// HarborServerConfiguration is the Schema for the harborserverconfigurations API
type HarborServerConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HarborServerConfigurationSpec   `json:"spec,omitempty"`
	Status HarborServerConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HarborServerConfigurationList contains a list of HarborServerConfiguration
type HarborServerConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HarborServerConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HarborServerConfiguration{}, &HarborServerConfigurationList{})
}
