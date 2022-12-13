/*
Copyright 2021.

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

package v1beta1

import (
	k8score "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ConditionType string

const (
	// ConditionReady specifies that the resource is ready
	ConditionReady ConditionType = "Ready"
)

type Condition struct {
	// Type of condition
	Type ConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown.
	Status k8score.ConditionStatus `json:"status"`

	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`

	// Last time the condition was updated
	// +optional
	LastUpdatedTime *metav1.Time `json:"lastUpdatedTime,omitempty"`
}

// AuthorinoSpec defines the desired state of Authorino
type AuthorinoSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Image                    string      `json:"image,omitempty"`
	ImagePullPolicy          string      `json:"imagePullPolicy,omitempty"`
	Replicas                 *int32      `json:"replicas,omitempty"`
	Volumes                  VolumesSpec `json:"volumes,omitempty"`
	LogLevel                 string      `json:"logLevel,omitempty"`
	LogMode                  string      `json:"logMode,omitempty"`
	ClusterWide              bool        `json:"clusterWide,omitempty"`
	Listener                 Listener    `json:"listener"`
	OIDCServer               OIDCServer  `json:"oidcServer"`
	AuthConfigLabelSelectors string      `json:"authConfigLabelSelectors,omitempty"`
	SecretLabelSelectors     string      `json:"secretLabelSelectors,omitempty"`
	EvaluatorCacheSize       *int        `json:"evaluatorCacheSize,omitempty"`
	Metrics                  Metrics     `json:"metrics,omitempty"`
}

type Listener struct {
	// Port number of the GRPC interface.
	// DEPRECATED: use 'ports.grpc' instead.
	Port *int32 `json:"port,omitempty"`
	// Port numbers of the GRPC and HTTP auth interfaces.
	Ports Ports `json:"ports,omitempty"`
	// TLS configuration of the auth service (GRPC and HTTP interfaces).
	Tls Tls `json:"tls"`
	// Timeout of the auth service (GRPC and HTTP interfaces), in milliseconds.
	Timeout *int `json:"timeout,omitempty"`
	// Maximum payload (request body) size for the auth service (HTTP interface), in bytes.
	MaxHttpRequestBodySize *int `json:"maxHttpRequestBodySize,omitempty"`
}

type OIDCServer struct {
	Port *int32 `json:"port,omitempty"`
	Tls  Tls    `json:"tls"`
}

type Ports struct {
	GRPC *int32 `json:"grpc,omitempty"`
	HTTP *int32 `json:"http,omitempty"`
}

type Metrics struct {
	Port               *int32 `json:"port,omitempty"`
	DeepMetricsEnabled *bool  `json:"deep,omitempty"`
}

type Tls struct {
	Enabled    *bool                         `json:"enabled,omitempty"`
	CertSecret *k8score.LocalObjectReference `json:"certSecretRef,omitempty"`
}

type VolumesSpec struct {
	Items []VolumeSpec `json:"items,omitempty"`
	// Permissions mode.
	// +optional
	DefaultMode *int32 `json:"defaultMode,omitempty"`
}

type VolumeSpec struct {
	// Volume name
	Name string `json:"name,omitempty"`
	// An absolute path where to mount it
	MountPath string `json:"mountPath"`
	// Allow multiple configmaps to mount to the same directory
	// +optional
	ConfigMaps []string `json:"configMaps,omitempty"`
	// Secret mount
	// +optional
	Secrets []string `json:"secrets,omitempty"`
	// Mount details
	// +optional
	Items []k8score.KeyToPath `json:"items,omitempty" protobuf:"bytes,2,rep,name=items"`
}

// AuthorinoStatus defines the observed state of Authorino
type AuthorinoStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions is an array of the current Authorino's CR conditions
	// Supported condition types: ConditionReady
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

func (status *AuthorinoStatus) Ready() bool {
	for _, condition := range status.Conditions {
		switch condition.Type {
		case ConditionReady:
			return condition.Status == k8score.ConditionTrue
		}
	}
	return false
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path="authorinos"

// Authorino is the Schema for the authorinos API
type Authorino struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthorinoSpec   `json:"spec,omitempty"`
	Status AuthorinoStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AuthorinoList contains a list of Authorino
type AuthorinoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Authorino `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Authorino{}, &AuthorinoList{})
}
