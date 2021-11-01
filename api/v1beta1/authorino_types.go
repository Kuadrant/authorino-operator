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
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ConditionType string

const (
	// ConditionReady specifies that the resource is ready
	ConditionReady         ConditionType = "Ready"
	AuthorinoContainerName string        = "authorino"

	// Authorino EnvVars
	WatchNamespace          string = "WATCH_NAMESPACE"
	ExtAuthGRPCPort         string = "EXT_AUTH_GRPC_PORT"
	TLSCertPath             string = "TLS_CERT"
	TLSCertKeyPath          string = "TLS_CERT_KEY"
	OIDCHTTPPort            string = "OIDC_HTTP_PORT"
	OIDCTLSCertPath         string = "OIDC_TLS_CERT"
	OIDCTLSCertKeyPath      string = "OIDC_TLS_CERT_KEY"
	AuthConfigLabelSelector string = "AUTH_CONFIG_LABEL_SELECTOR"
	SecretLabelSelector     string = "SECRET_LABEL_SELECTOR"
)

type Condition struct {
	// Type of condition
	Type ConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status apiv1.ConditionStatus `json:"status"`
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

	Image                    string     `json:"image,omitempty"`
	Replicas                 *int32     `json:"replicas,omitempty"`
	ImagePullPolicy          string     `json:"imagePullPolicy,omitempty"`
	ClusterWide              bool       `json:"clusterWide,omitempty"`
	Listener                 Listener   `json:"listener,omitempty"`
	OIDCServer               OIDCServer `json:"oidcServer,omitempty"`
	AuthConfigLabelSelectors string     `json:"authConfigLabelSelectors,omitempty"`
	SecretLabelSelectors     string     `json:"secretLabelSelectors,omitempty"`
}

type Listener struct {
	Port        *int32 `json:"port,omitempty"`
	Tls         bool   `json:"tsl,omitempty"`
	CertPath    string `json:"certPath,omitempty"`
	CertKeyPath string `json:"certKeyPath,omitempty"`
}

type OIDCServer struct {
	Port        *int32 `json:"port,omitempty"`
	CertPath    string `json:"certPath,omitempty"`
	CertKeyPath string `json:"certKeyPath,omitempty"`
}

// AuthorinoStatus defines the observed state of Authorino
type AuthorinoStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	//  Defines if the authorino intance is ready
	Ready bool `json:"ready"`

	// Reports an error during the deployment of an instance
	LastError string `json:"lastError"`

	// Conditions is an array of the current Authorino's CR conditions
	// Supported condition types: ConditionReady
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
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
