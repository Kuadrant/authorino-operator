package resources

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

func TestAuthorinoClusterRoleBindingNameIsUnambiguous(t *testing.T) {
	const suffix = "authorino-k8s-auth"

	// Namespace "a-b"/name "c" and namespace "a"/name "b-c" must not collide:
	// a plain "-" join would produce "a-b-c-<suffix>" for both, causing the two
	// cluster-scoped instances to share a single ClusterRoleBinding.
	nameOne := authorinoClusterRoleBindingName("a-b", "c", suffix)
	nameTwo := authorinoClusterRoleBindingName("a", "b-c", suffix)

	if nameOne == nameTwo {
		t.Errorf("expected distinct ClusterRoleBinding names, both resolved to %q", nameOne)
	}
}

func TestAuthorinoClusterRoleBindingNameRespectsMaxLength(t *testing.T) {
	const suffix = "authorino-k8s-auth"

	longName := strings.Repeat("a", 253)
	name := authorinoClusterRoleBindingName("my-namespace", longName, suffix)

	if len(name) > validation.DNS1123SubdomainMaxLength {
		t.Errorf("expected name length <= %d, got %d (%q)", validation.DNS1123SubdomainMaxLength, len(name), name)
	}
	if !strings.HasPrefix(name, "my-namespace.") {
		t.Errorf("expected name to preserve namespace prefix, got %q", name)
	}
	if !strings.HasSuffix(name, "-"+suffix) {
		t.Errorf("expected name to preserve suffix, got %q", name)
	}
}

func TestAuthorinoClusterRoleBindingNameTruncationIsUnique(t *testing.T) {
	const suffix = "authorino-k8s-auth"

	// Two long CR names sharing a common prefix must still yield distinct names.
	prefix := strings.Repeat("a", 253)
	nameOne := authorinoClusterRoleBindingName("ns", prefix+"one", suffix)
	nameTwo := authorinoClusterRoleBindingName("ns", prefix+"two", suffix)

	if nameOne == nameTwo {
		t.Errorf("expected distinct ClusterRoleBinding names for distinct CR names, both resolved to %q", nameOne)
	}
}
