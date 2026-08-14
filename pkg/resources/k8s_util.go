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
// Note: the name is a plain "-" join of namespace, CR name and suffix and is not
// strictly collision-proof, since the separator can also appear inside the
// namespace or CR name (e.g. namespace "a-b"/name "c" and namespace "a"/name
// "b-c" both yield "a-b-c-<suffix>"). This is sufficient for the common case but
// not a guarantee of global uniqueness.
func authorinoClusterRoleBindingName(namespace, crName, clusterRoleBindingNameSuffix string) string {
	return fmt.Sprintf("%s-%s-%s", namespace, crName, clusterRoleBindingNameSuffix)
}
