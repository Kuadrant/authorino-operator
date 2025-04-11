package reconcilers

import (
	"testing"

	k8score "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceMutatorFunctions(t *testing.T) {
	service := &k8score.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
			Labels:    map[string]string{"app": "authorino"},
		},
		Spec: k8score.ServiceSpec{
			Ports: []k8score.ServicePort{
				{
					Name:     "http",
					Port:     8080,
					Protocol: k8score.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "authorino",
			},
		},
	}

	tests := []struct {
		name     string
		mutator  ServiceMutateFn
		desired  *k8score.Service
		existing *k8score.Service
		want     bool
		verify   func(t *testing.T, existing *k8score.Service)
	}{
		{
			name:    "LabelsMutator - no change",
			mutator: LabelsMutator,
			desired: service.DeepCopy(),
			existing: func() *k8score.Service {
				s := service.DeepCopy()
				s.Labels = map[string]string{"app": "authorino"}
				return s
			}(),
			want: false,
		},
		{
			name:    "LabelsMutator - change",
			mutator: LabelsMutator,
			desired: func() *k8score.Service {
				s := service.DeepCopy()
				s.Labels = map[string]string{
					"app": "authorino",
					"new": "label",
				}
				return s
			}(),
			existing: service.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8score.Service) {
				if _, exists := existing.Labels["new"]; !exists {
					t.Error("expected 'new' label to be added")
				}
			},
		},
		{
			name:    "PortMutator - no change",
			mutator: PortMutator,
			desired: service.DeepCopy(),
			existing: func() *k8score.Service {
				s := service.DeepCopy()
				s.Spec.Ports = []k8score.ServicePort{
					{
						Name:     "http",
						Port:     8080,
						Protocol: k8score.ProtocolTCP,
					},
				}
				return s
			}(),
			want: false,
		},
		{
			name:    "PortMutator - change port",
			mutator: PortMutator,
			desired: func() *k8score.Service {
				s := service.DeepCopy()
				s.Spec.Ports = []k8score.ServicePort{
					{
						Name:     "http",
						Port:     9090,
						Protocol: k8score.ProtocolTCP,
					},
				}
				return s
			}(),
			existing: service.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8score.Service) {
				if existing.Spec.Ports[0].Port != 9090 {
					t.Errorf("expected port to be 9090, got %d", existing.Spec.Ports[0].Port)
				}
			},
		},
		{
			name:    "PortMutator - add port",
			mutator: PortMutator,
			desired: func() *k8score.Service {
				s := service.DeepCopy()
				s.Spec.Ports = []k8score.ServicePort{
					{
						Name:     "http",
						Port:     8080,
						Protocol: k8score.ProtocolTCP,
					},
					{
						Name:     "metrics",
						Port:     8081,
						Protocol: k8score.ProtocolTCP,
					},
				}
				return s
			}(),
			existing: service.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8score.Service) {
				if len(existing.Spec.Ports) != 2 {
					t.Errorf("expected 2 ports, got %d", len(existing.Spec.Ports))
				}
			},
		},
		{
			name:    "SelectorMutator - no change",
			mutator: SelectorMutator,
			desired: service.DeepCopy(),
			existing: func() *k8score.Service {
				s := service.DeepCopy()
				s.Spec.Selector = map[string]string{"app": "authorino"}
				return s
			}(),
			want: false,
		},
		{
			name:    "SelectorMutator - change",
			mutator: SelectorMutator,
			desired: func() *k8score.Service {
				s := service.DeepCopy()
				s.Spec.Selector = map[string]string{
					"app": "authorino-v2",
				}
				return s
			}(),
			existing: service.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *k8score.Service) {
				if existing.Spec.Selector["app"] != "authorino-v2" {
					t.Errorf("expected selector to be 'authorino-v2', got '%s'", existing.Spec.Selector["app"])
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

func TestServiceMutator(t *testing.T) {
	service := &k8score.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
			Labels:    map[string]string{"app": "authorino"},
		},
		Spec: k8score.ServiceSpec{
			Ports: []k8score.ServicePort{
				{
					Name:     "http",
					Port:     8080,
					Protocol: k8score.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "authorino",
			},
		},
	}

	t.Run("invalid object types", func(t *testing.T) {
		mutator := ServiceMutator(LabelsMutator)

		_, err := mutator(&k8score.Pod{}, &k8score.Service{})
		if err == nil {
			t.Error("expected error for invalid desired type")
		}

		_, err = mutator(&k8score.Service{}, &k8score.Pod{})
		if err == nil {
			t.Error("expected error for invalid existing type")
		}
	})

	t.Run("multiple mutators", func(t *testing.T) {
		desired := &k8score.Service{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app": "authorino",
					"new": "label",
				},
			},
			Spec: k8score.ServiceSpec{
				Ports: []k8score.ServicePort{
					{
						Name:     "http",
						Port:     9090,
						Protocol: k8score.ProtocolTCP,
					},
				},
				Selector: map[string]string{
					"app": "authorino-v2",
				},
			},
		}
		existing := service.DeepCopy()

		mutator := ServiceMutator(
			LabelsMutator,
			PortMutator,
			SelectorMutator,
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
		if existing.Spec.Ports[0].Port != 9090 {
			t.Errorf("expected port to be updated to 9090, got %d", existing.Spec.Ports[0].Port)
		}
		if existing.Spec.Selector["app"] != "authorino-v2" {
			t.Errorf("expected selector to be updated to 'authorino-v2', got '%s'", existing.Spec.Selector["app"])
		}
	})
}
