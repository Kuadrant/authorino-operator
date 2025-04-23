package reconcilers

import (
	"testing"

	k8srbac "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var ClusterRoleBinding = &k8srbac.ClusterRoleBinding{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-rolebinding",
		Namespace: "default",
		Labels:    map[string]string{"app": "authorino"},
	},
	Subjects: []k8srbac.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      "authorino-sa",
			Namespace: "default",
		},
	},
	RoleRef: k8srbac.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     "authorino-role",
	},
}

func TestClusterRoleBindingMutatorFunctions(t *testing.T) {
	tests := []struct {
		name     string
		mutator  ClusterRoleBindingMutateFn
		desired  *k8srbac.ClusterRoleBinding
		existing *k8srbac.ClusterRoleBinding
		want     bool
		verify   func(t *testing.T, existing *k8srbac.ClusterRoleBinding)
	}{
		{
			name:    "LabelsMutator - no change",
			mutator: ClusterRoleBindingLabelsMutator,
			desired: ClusterRoleBinding.DeepCopy(),
			existing: func() *k8srbac.ClusterRoleBinding {
				rb := ClusterRoleBinding.DeepCopy()
				rb.Labels = map[string]string{"app": "authorino"}
				return rb
			}(),
			want: false,
		},
		{
			name:    "LabelsMutator - change",
			mutator: ClusterRoleBindingLabelsMutator,
			desired: func() *k8srbac.ClusterRoleBinding {
				rb := ClusterRoleBinding.DeepCopy()
				rb.Labels = map[string]string{
					"app": "authorino",
					"new": "label",
				}
				return rb
			}(),
			existing: ClusterRoleBinding.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8srbac.ClusterRoleBinding) {
				if _, exists := existing.Labels["new"]; !exists {
					t.Error("expected 'new' label to be added")
				}
				if existing.Labels["app"] != "authorino" {
					t.Error("expected 'app' label to remain unchanged")
				}
			},
		},
		{
			name:    "SubjectMutator - no change",
			mutator: ClusterRoleBindingSubjectMutator,
			desired: ClusterRoleBinding.DeepCopy(),
			existing: func() *k8srbac.ClusterRoleBinding {
				rb := ClusterRoleBinding.DeepCopy()
				rb.Subjects = []k8srbac.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "authorino-sa",
						Namespace: "default",
					},
				}
				return rb
			}(),
			want: false,
		},
		{
			name:    "SubjectMutator - change",
			mutator: ClusterRoleBindingSubjectMutator,
			desired: func() *k8srbac.ClusterRoleBinding {
				rb := ClusterRoleBinding.DeepCopy()
				rb.Subjects = []k8srbac.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "new-sa",
						Namespace: "default",
					},
				}
				return rb
			}(),
			existing: ClusterRoleBinding.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8srbac.ClusterRoleBinding) {
				if len(existing.Subjects) != 2 {
					t.Fatalf("expected 2 subject, got %d", len(existing.Subjects))
				}
				if existing.Subjects[0].Name != "authorino-sa" {
					t.Errorf("expected subject name 'authorino-sa', got '%s'", existing.Subjects[0].Name)
				}
				if existing.Subjects[1].Name != "new-sa" {
					t.Errorf("expected subject name 'new-sa', got '%s'", existing.Subjects[1].Name)
				}
			},
		},
		{
			name:    "SubjectMutator - multiple subjects",
			mutator: ClusterRoleBindingSubjectMutator,
			desired: func() *k8srbac.ClusterRoleBinding {
				rb := ClusterRoleBinding.DeepCopy()
				rb.Subjects = []k8srbac.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "sa1",
						Namespace: "default",
					},
					{
						Kind:      "ServiceAccount",
						Name:      "sa2",
						Namespace: "default",
					},
				}
				return rb
			}(),
			existing: ClusterRoleBinding.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8srbac.ClusterRoleBinding) {
				if len(existing.Subjects) != 3 {
					t.Fatalf("expected 3 subjects, got %d", len(existing.Subjects))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := tt.desired.DeepCopy()
			existing := tt.existing.DeepCopy()

			got := tt.mutator(desired, existing)
			if got != tt.want {
				t.Errorf("expected update %v, got %v", tt.want, got)
			}

			if tt.verify != nil {
				tt.verify(t, existing)
			}
		})
	}
}

func TestClusterRoleBindingMutator(t *testing.T) {
	t.Run("invalid object types", func(t *testing.T) {
		mutator := ClusterRoleBindingMutator(ClusterRoleBindingLabelsMutator)

		_, err := mutator(&k8srbac.ClusterRoleBinding{}, &k8srbac.RoleBinding{})
		if err == nil {
			t.Error("expected error for invalid desired type")
		}

		_, err = mutator(&k8srbac.RoleBinding{}, &k8srbac.ClusterRoleBinding{})
		if err == nil {
			t.Error("expected error for invalid existing type")
		}
	})

	t.Run("multiple mutators", func(t *testing.T) {
		desired := ClusterRoleBinding.DeepCopy()
		desired.Labels = map[string]string{"new": "label"}
		desired.Subjects = []k8srbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "new-sa",
				Namespace: "new-ns",
			},
		}
		existing := ClusterRoleBinding.DeepCopy()

		mutator := ClusterRoleBindingMutator(
			ClusterRoleBindingLabelsMutator,
			ClusterRoleBindingSubjectMutator,
		)

		updated, err := mutator(desired, existing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Error("expected update to be required")
		}

		if _, exists := existing.Labels["new"]; !exists {
			t.Error("expected 'new' label to be added")
		}
		if len(existing.Subjects) != 2 || existing.Subjects[1].Name != "new-sa" {
			t.Error("expected subjects to be updated")
		}
	})
}
