package resources

import (
	"crypto/sha256"
	"fmt"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
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
//
// The generated name is capped at the RFC 1123 DNS subdomain max length. When a
// long CR name would exceed that limit, the CR name portion is truncated and a
// deterministic hash of the CR name is appended, so distinct long names remain
// uniquely identifiable while the namespace and suffix are always preserved.
func authorinoClusterRoleBindingName(namespace, crName, clusterRoleBindingNameSuffix string) string {
	name := fmt.Sprintf("%s.%s-%s", namespace, crName, clusterRoleBindingNameSuffix)
	if len(name) <= validation.DNS1123SubdomainMaxLength {
		return name
	}

	// Deterministic hash of the CR name to keep truncated names unique: two long
	// CR names can share a common prefix, so once truncated, uniqueness rests on
	// the hash. The namespace is preserved verbatim as the prefix (and cannot
	// contain a "."), so a collision is only possible within a single namespace
	// -- hashing the namespace as well would add nothing. 64 bits (16 hex chars)
	// makes an accidental collision negligible for any realistic set of names.
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(crName)))[:16]

	// Room left for the CR name once the always-preserved parts (namespace,
	// hash, suffix and their separators) are accounted for. The hash is
	// appended directly onto the truncated CR name, so this fixed portion is
	// exactly the final name minus the CR name.
	available := validation.DNS1123SubdomainMaxLength - len(fmt.Sprintf("%s.%s-%s", namespace, hash, clusterRoleBindingNameSuffix))
	if available < 0 {
		available = 0
	}

	truncatedName := crName
	if len(truncatedName) > available {
		truncatedName = truncatedName[:available]
	}

	return fmt.Sprintf("%s.%s%s-%s", namespace, truncatedName, hash, clusterRoleBindingNameSuffix)
}
