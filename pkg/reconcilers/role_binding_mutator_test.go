package reconcilers

import (
	"testing"

	k8srbac "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var RoleBinding = &k8srbac.RoleBinding{
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

func TestRoleBindingMutatorFunctions(t *testing.T) {
	tests := []struct {
		name     string
		mutator  RoleBindingMutateFn
		desired  *k8srbac.RoleBinding
		existing *k8srbac.RoleBinding
		want     bool
		verify   func(t *testing.T, existing *k8srbac.RoleBinding)
	}{
		{
			name:    "NamespaceMutator - no change",
			mutator: RoleBindingNamespaceMutator,
			desired: RoleBinding.DeepCopy(),
			existing: func() *k8srbac.RoleBinding {
				rb := RoleBinding.DeepCopy()
				rb.Namespace = "default"
				return rb
			}(),
			want: false,
		},
		{
			name:    "NamespaceMutator - change",
			mutator: RoleBindingNamespaceMutator,
			desired: func() *k8srbac.RoleBinding {
				rb := RoleBinding.DeepCopy()
				rb.Namespace = "new-namespace"
				return rb
			}(),
			existing: RoleBinding.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8srbac.RoleBinding) {
				if existing.Namespace != "new-namespace" {
					t.Errorf("expected namespace to be 'new-namespace', got '%s'", existing.Namespace)
				}
			},
		},
		{
			name:    "NameMutator - no change",
			mutator: RoleBindingNameMutator,
			desired: RoleBinding.DeepCopy(),
			existing: func() *k8srbac.RoleBinding {
				rb := RoleBinding.DeepCopy()
				rb.Name = "test-rolebinding"
				return rb
			}(),
			want: false,
		},
		{
			name:    "NameMutator - change",
			mutator: RoleBindingNameMutator,
			desired: func() *k8srbac.RoleBinding {
				rb := RoleBinding.DeepCopy()
				rb.Name = "new-rolebinding"
				return rb
			}(),
			existing: RoleBinding.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8srbac.RoleBinding) {
				if existing.Name != "new-rolebinding" {
					t.Errorf("expected name to be 'new-rolebinding', got '%s'", existing.Name)
				}
			},
		},
		{
			name:    "LabelsMutator - no change",
			mutator: RoleBindingLabelsMutator,
			desired: RoleBinding.DeepCopy(),
			existing: func() *k8srbac.RoleBinding {
				rb := RoleBinding.DeepCopy()
				rb.Labels = map[string]string{"app": "authorino"}
				return rb
			}(),
			want: false,
		},
		{
			name:    "LabelsMutator - change",
			mutator: RoleBindingLabelsMutator,
			desired: func() *k8srbac.RoleBinding {
				rb := RoleBinding.DeepCopy()
				rb.Labels = map[string]string{
					"app": "authorino",
					"new": "label",
				}
				return rb
			}(),
			existing: RoleBinding.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8srbac.RoleBinding) {
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
			mutator: RoleBindingSubjectMutator,
			desired: RoleBinding.DeepCopy(),
			existing: func() *k8srbac.RoleBinding {
				rb := RoleBinding.DeepCopy()
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
			mutator: RoleBindingSubjectMutator,
			desired: func() *k8srbac.RoleBinding {
				rb := RoleBinding.DeepCopy()
				rb.Subjects = []k8srbac.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "new-sa",
						Namespace: "default",
					},
				}
				return rb
			}(),
			existing: RoleBinding.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8srbac.RoleBinding) {
				if len(existing.Subjects) != 1 {
					t.Fatalf("expected 1 subject, got %d", len(existing.Subjects))
				}
				if existing.Subjects[0].Name != "new-sa" {
					t.Errorf("expected subject name 'new-sa', got '%s'", existing.Subjects[0].Name)
				}
			},
		},
		{
			name:    "SubjectMutator - multiple subjects",
			mutator: RoleBindingSubjectMutator,
			desired: func() *k8srbac.RoleBinding {
				rb := RoleBinding.DeepCopy()
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
			existing: RoleBinding.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8srbac.RoleBinding) {
				if len(existing.Subjects) != 2 {
					t.Fatalf("expected 2 subjects, got %d", len(existing.Subjects))
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

func TestRoleBindingMutator(t *testing.T) {
	t.Run("invalid object types", func(t *testing.T) {
		mutator := RoleBindingMutator(RoleBindingNameMutator)

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
		desired := &k8srbac.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-name",
				Namespace: "new-ns",
				Labels:    map[string]string{"new": "label"},
			},
			Subjects: []k8srbac.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "new-sa",
					Namespace: "new-ns",
				},
			},
		}
		existing := RoleBinding.DeepCopy()

		mutator := RoleBindingMutator(
			RoleBindingNameMutator,
			RoleBindingNamespaceMutator,
			RoleBindingLabelsMutator,
			RoleBindingSubjectMutator,
		)

		updated, err := mutator(desired, existing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Error("expected update to be required")
		}

		if existing.Name != "new-name" {
			t.Errorf("expected name to be 'new-name', got '%s'", existing.Name)
		}
		if existing.Namespace != "new-ns" {
			t.Errorf("expected namespace to be 'new-ns', got '%s'", existing.Namespace)
		}
		if _, exists := existing.Labels["new"]; !exists {
			t.Error("expected 'new' label to be added")
		}
		if len(existing.Subjects) != 1 || existing.Subjects[0].Name != "new-sa" {
			t.Error("expected subjects to be updated")
		}
	})
}
