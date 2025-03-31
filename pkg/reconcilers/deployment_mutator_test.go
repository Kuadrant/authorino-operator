package reconcilers

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestDeploymentMutatorFunctions(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": "authorino"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "authorino-pod"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "authorino",
							Image: "authorino/image:latest",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "vol1", MountPath: "/path1"},
							},
						},
					},
					ServiceAccountName: "authorino-sa",
					Volumes: []corev1.Volume{
						{Name: "vol1"},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		mutator  DeploymentMutateFn
		desired  *appsv1.Deployment
		existing *appsv1.Deployment
		want     bool
		verify   func(t *testing.T, existing *appsv1.Deployment)
	}{
		{
			name:    "ReplicasMutator - no change",
			mutator: DeploymentReplicasMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Replicas = ptr.To[int32](1)
				return d
			}(),
			want: false,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if *existing.Spec.Replicas != 1 {
					t.Errorf("expected replicas to remain 1, got %d", *existing.Spec.Replicas)
				}
			},
		},
		{
			name:    "ReplicasMutator - change",
			mutator: DeploymentReplicasMutator,
			desired: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Replicas = ptr.To[int32](3)
				return d
			}(),
			existing: deployment.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if *existing.Spec.Replicas != 3 {
					t.Errorf("expected replicas to be updated to 3, got %d", *existing.Spec.Replicas)
				}
			},
		},
		{
			name:    "ContainerListMutator - no change",
			mutator: DeploymentContainerListMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Containers = []corev1.Container{
					{Name: "authorino", Image: "authorino/image:latest"},
				}
				return d
			}(),
			want: false,
		},
		{
			name:    "ContainerListMutator - change",
			mutator: DeploymentContainerListMutator,
			desired: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Containers = []corev1.Container{
					{Name: "authorino", Image: "authorino/image:new"},
					{Name: "sidecar", Image: "sidecar/image"},
				}
				return d
			}(),
			existing: deployment.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if len(existing.Spec.Template.Spec.Containers) != 2 {
					t.Errorf("expected 2 containers, got %d", len(existing.Spec.Template.Spec.Containers))
				}
			},
		},
		{
			name:    "ImageMutator - no change",
			mutator: DeploymentImageMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Containers[0].Image = "authorino/image:latest"
				return d
			}(),
			want: false,
		},
		{
			name:    "ImageMutator - change",
			mutator: DeploymentImageMutator,
			desired: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Containers[0].Image = "authorino/image:new"
				return d
			}(),
			existing: deployment.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if existing.Spec.Template.Spec.Containers[0].Image != "authorino/image:new" {
					t.Errorf("expected image to be updated, got %s", existing.Spec.Template.Spec.Containers[0].Image)
				}
			},
		},
		{
			name:    "VolumesMutator - no change",
			mutator: DeploymentVolumesMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Volumes = []corev1.Volume{
					{Name: "vol1"},
				}
				return d
			}(),
			want: false,
		},
		{
			name:    "VolumesMutator - change",
			mutator: DeploymentVolumesMutator,
			desired: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Volumes = []corev1.Volume{
					{Name: "vol1"},
					{Name: "vol2"},
				}
				return d
			}(),
			existing: deployment.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if len(existing.Spec.Template.Spec.Volumes) != 2 {
					t.Errorf("expected 2 volumes, got %d", len(existing.Spec.Template.Spec.Volumes))
				}
			},
		},
		{
			name:    "VolumeMountsMutator - no change",
			mutator: DeploymentVolumeMountsMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
					{Name: "vol1", MountPath: "/path1"},
				}
				return d
			}(),
			want: false,
		},
		{
			name:    "VolumeMountsMutator - change",
			mutator: DeploymentVolumeMountsMutator,
			desired: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
					{Name: "vol1", MountPath: "/newpath"},
				}
				return d
			}(),
			existing: deployment.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath != "/newpath" {
					t.Errorf("expected mount path to be updated, got %s",
						existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath)
				}
			},
		},
		{
			name:    "ServiceAccountMutator - no change",
			mutator: DeploymentServiceAccountMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.ServiceAccountName = "authorino-sa"
				return d
			}(),
			want: false,
		},
		{
			name:    "ServiceAccountMutator - change",
			mutator: DeploymentServiceAccountMutator,
			desired: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.ServiceAccountName = "new-sa"
				return d
			}(),
			existing: deployment.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if existing.Spec.Template.Spec.ServiceAccountName != "new-sa" {
					t.Errorf("expected service account to be updated, got %s",
						existing.Spec.Template.Spec.ServiceAccountName)
				}
			},
		},
		{
			name:    "LabelsMutator - no change",
			mutator: DeploymentLabelsMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.ObjectMeta.Labels = map[string]string{"app": "authorino"}
				return d
			}(),
			want: false,
		},
		{
			name:    "LabelsMutator - change",
			mutator: DeploymentLabelsMutator,
			desired: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.ObjectMeta.Labels = map[string]string{
					"app": "authorino",
					"new": "label",
				}
				return d
			}(),
			existing: deployment.DeepCopy(),
			want:     true,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if _, exists := existing.ObjectMeta.Labels["new"]; !exists {
					t.Error("expected new label to be added")
				}
			},
		},
		{
			name:    "SpecTemplateLabelsMutator - no change",
			mutator: DeploymentSpecTemplateLabelsMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Labels = map[string]string{"app": "authorino-pod"}
				return d
			}(),
			want: false,
		},
		{
			name:    "SpecTemplateLabelsMutator - change with merge",
			mutator: DeploymentSpecTemplateLabelsMutator,
			desired: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Labels = map[string]string{
					"app": "authorino-pod-new",
					"new": "pod-label",
				}
				return d
			}(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Labels = map[string]string{
					"app":      "authorino-pod",
					"existing": "label",
				}
				return d
			}(),
			want: true,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				labels := existing.Spec.Template.Labels
				if labels["app"] != "authorino-pod" {
					t.Errorf("expected app label not to be updated, got %s", labels["app"])
				}
				if labels["new"] != "pod-label" {
					t.Error("expected new label to be added")
				}
				if labels["existing"] != "label" {
					t.Error("expected existing label to be preserved")
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

func TestDeploymentMutator(t *testing.T) {
	t.Run("invalid object types", func(t *testing.T) {
		mutator := DeploymentMutator(DeploymentReplicasMutator)

		_, err := mutator(&corev1.Pod{}, &appsv1.Deployment{})
		if err == nil {
			t.Error("expected error for invalid desired type")
		}

		_, err = mutator(&appsv1.Deployment{}, &corev1.Pod{})
		if err == nil {
			t.Error("expected error for invalid existing type")
		}
	})

	t.Run("multiple mutators", func(t *testing.T) {
		desired := &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](3),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Image: "new-image"},
						},
					},
				},
			},
		}
		existing := &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](1),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Image: "old-image"},
						},
					},
				},
			},
		}

		mutator := DeploymentMutator(
			DeploymentReplicasMutator,
			DeploymentImageMutator,
		)

		updated, err := mutator(desired, existing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Error("expected update to be required")
		}

		if *existing.Spec.Replicas != 3 {
			t.Errorf("expected replicas to be updated to 3, got %d", *existing.Spec.Replicas)
		}
		if existing.Spec.Template.Spec.Containers[0].Image != "new-image" {
			t.Errorf("expected image to be updated to 'new-image', got %s",
				existing.Spec.Template.Spec.Containers[0].Image)
		}
	})
}
