package resources

import (
	"fmt"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getObjectMeta(namespace, name string, labels map[string]string) v1.ObjectMeta {
	return v1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels}
}

func defaultAuthorinoLabels(name string) map[string]string {
	return map[string]string{
		"control-plane":      "controller-manager",
		"authorino-resource": name,
	}
}

func authorinoServiceAccountName(crName string) string {
	return fmt.Sprintf("%s-authorino", crName)
}

func authorinoRoleBindingName(crName, roleBindingNameSuffix string) string {
	return fmt.Sprintf("%s-%s", crName, roleBindingNameSuffix)
}

// authorinoClusterRoleBindingName builds the name for a cluster-scoped
// ClusterRoleBinding. ClusterRoleBindings are not namespaced, so the CR
// namespace is included in the name to disambiguate Authorino instances that
// share the same CR name across different namespaces.
//
// The namespace and CR name are separated by a "." rather than a "-". A
// Kubernetes namespace is a DNS-1123 label and cannot contain a ".", so the
// segment before the first "." is always exactly the namespace. This keeps the
// name human-readable while staying unambiguous: e.g. namespace "a-b"/name "c"
// yields "a-b.c-<suffix>" and namespace "a"/name "b-c" yields "a.b-c-<suffix>",
// which are distinct. ("." is a valid character in a ClusterRoleBinding name,
// which is an RFC 1123 DNS subdomain.)
func authorinoClusterRoleBindingName(namespace, crName, clusterRoleBindingNameSuffix string) string {
	return fmt.Sprintf("%s.%s-%s", namespace, crName, clusterRoleBindingNameSuffix)
}
