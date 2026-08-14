package resources

import (
	"slices"
	"strings"
	"testing"

	k8srbac "k8s.io/api/rbac/v1"
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

func TestMergeBindingSubject(t *testing.T) {
	subjectFoo := k8srbac.Subject{Kind: "Foo"}
	subjectBar := k8srbac.Subject{Kind: "Bar"}

	emptySlice := make([]k8srbac.Subject, 0)
	var nilSlice []k8srbac.Subject

	type args struct {
		existing *[]k8srbac.Subject
		desired  []k8srbac.Subject
	}
	tests := []struct {
		name       string
		args       args
		wantUpdate bool
	}{
		{
			name: "nil pointer to slice",
			args: args{
				existing: nil,
				desired:  []k8srbac.Subject{subjectFoo, subjectBar},
			},
			wantUpdate: false,
		},
		{
			name: "nil slice",
			args: args{
				existing: &nilSlice,
				desired:  []k8srbac.Subject{subjectFoo, subjectBar},
			},
			wantUpdate: true,
		},
		{
			name: "empty slice",
			args: args{
				existing: &emptySlice,
				desired:  []k8srbac.Subject{subjectFoo, subjectBar},
			},
			wantUpdate: true,
		},
		{
			name: "desired subjects not in existing",
			args: args{
				existing: &[]k8srbac.Subject{subjectFoo},
				desired:  []k8srbac.Subject{subjectBar},
			},
			wantUpdate: true,
		},
		{
			name: "same slices",
			args: args{
				existing: &[]k8srbac.Subject{subjectFoo, subjectBar},
				desired:  []k8srbac.Subject{subjectFoo, subjectBar},
			},
			wantUpdate: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			got := MergeBindingSubject(tt.args.desired, tt.args.existing)
			if got != tt.wantUpdate {
				subT.Errorf("MergeBindingSubject() got = %v, wantUpdate %v", got, tt.wantUpdate)
			}

			if tt.args.existing == nil {
				return
			}

			if len(*tt.args.existing) < len(tt.args.desired) {
				subT.Error("existing has less subjects than desired")
			}

			for idx := range tt.args.desired {
				if !slices.Contains(*tt.args.existing, tt.args.desired[idx]) {
					t.Errorf("MergeBindingSubject() desired subject not in existing: %v", tt.args.desired[idx])
				}
			}
		})
	}
}
