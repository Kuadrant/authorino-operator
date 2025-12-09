package reconcilers

import (
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestFindContainerByName(t *testing.T) {
	tests := []struct {
		name          string
		containers    []corev1.Container
		containerName string
		wantNil       bool
		wantImage     string
	}{
		{
			name: "find authorino container at index 0",
			containers: []corev1.Container{
				{Name: "authorino", Image: "authorino:latest"},
			},
			containerName: "authorino",
			wantNil:       false,
			wantImage:     "authorino:latest",
		},
		{
			name: "find authorino container with sidecar at index 0",
			containers: []corev1.Container{
				{Name: "sidecar", Image: "sidecar:latest"},
				{Name: "authorino", Image: "authorino:v1"},
			},
			containerName: "authorino",
			wantNil:       false,
			wantImage:     "authorino:v1",
		},
		{
			name: "container not found",
			containers: []corev1.Container{
				{Name: "other", Image: "other:latest"},
			},
			containerName: "authorino",
			wantNil:       true,
		},
		{
			name:          "empty container list",
			containers:    []corev1.Container{},
			containerName: "authorino",
			wantNil:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findContainerByName(tt.containers, tt.containerName)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got container with name %s", got.Name)
				}
			} else {
				if got == nil {
					t.Fatal("expected container, got nil")
				}
				if got.Name != tt.containerName {
					t.Errorf("expected container name %s, got %s", tt.containerName, got.Name)
				}
				if got.Image != tt.wantImage {
					t.Errorf("expected image %s, got %s", tt.wantImage, got.Image)
				}
			}
		})
	}
}

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
			name:    "ContainerListMutator - empty existing containers",
			mutator: DeploymentContainerListMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Containers = []corev1.Container{}
				return d
			}(),
			want: true,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if len(existing.Spec.Template.Spec.Containers) != 1 {
					t.Errorf("expected 1 container, got %d", len(existing.Spec.Template.Spec.Containers))
				}
				if existing.Spec.Template.Spec.Containers[0].Name != "authorino" {
					t.Errorf("expected authorino container, got %s", existing.Spec.Template.Spec.Containers[0].Name)
				}
			},
		},
		{
			name:    "ContainerListMutator - preserves sidecars",
			mutator: DeploymentContainerListMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Containers = []corev1.Container{
					{Name: "authorino", Image: "authorino/image:latest"},
					{Name: "sidecar", Image: "sidecar/image"},
				}
				return d
			}(),
			want: false,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if len(existing.Spec.Template.Spec.Containers) != 2 {
					t.Errorf("expected 2 containers (sidecar should be preserved), got %d", len(existing.Spec.Template.Spec.Containers))
				}
				if existing.Spec.Template.Spec.Containers[0].Name != "authorino" {
					t.Errorf("expected authorino container at index 0, got %s", existing.Spec.Template.Spec.Containers[0].Name)
				}
				if existing.Spec.Template.Spec.Containers[1].Name != "sidecar" {
					t.Errorf("expected sidecar container at index 1, got %s", existing.Spec.Template.Spec.Containers[1].Name)
				}
			},
		},
		{
			name:    "ContainerListMutator - sidecar at index 0",
			mutator: DeploymentContainerListMutator,
			desired: deployment.DeepCopy(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Spec.Containers = []corev1.Container{
					{Name: "sidecar", Image: "sidecar/image"},
					{Name: "authorino", Image: "authorino/image:latest"},
				}
				return d
			}(),
			want: false,
			verify: func(t *testing.T, existing *appsv1.Deployment) {
				if len(existing.Spec.Template.Spec.Containers) != 2 {
					t.Errorf("expected 2 containers (both should be preserved), got %d", len(existing.Spec.Template.Spec.Containers))
				}
				// Order should be preserved - sidecar stays at index 0
				if existing.Spec.Template.Spec.Containers[0].Name != "sidecar" {
					t.Errorf("expected sidecar container at index 0, got %s", existing.Spec.Template.Spec.Containers[0].Name)
				}
				if existing.Spec.Template.Spec.Containers[1].Name != "authorino" {
					t.Errorf("expected authorino container at index 1, got %s", existing.Spec.Template.Spec.Containers[1].Name)
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
			name:    "SpecTemplateLabelsMutator - existing has few extra",
			mutator: DeploymentSpecTemplateLabelsMutator,
			desired: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Labels = map[string]string{
					"foo":  "bar",
					"foo2": "bar2",
				}
				return d
			}(),
			existing: func() *appsv1.Deployment {
				d := deployment.DeepCopy()
				d.Spec.Template.Labels = map[string]string{
					"foo":       "bar",
					"foo2":      "bar2",
					"new-label": "new-value",
				}
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
				if len(labels) != 3 {
					t.Errorf("expected num label to be 3, got %d", len(labels))
				}
				if labels["app"] != "authorino-pod-new" {
					t.Errorf("expected app label to be updated, got %s", labels["app"])
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
							{Name: "authorino", Image: "new-image"},
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
							{Name: "authorino", Image: "old-image"},
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

func TestDetectEnvVarAuthorinoVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{
			version:  "v0.9.0",
			expected: true,
		},
		{
			version:  "v0.10.0",
			expected: true,
		},
		{
			version:  "v0.10.11",
			expected: true,
		},
		{
			version:  "v0.11.0",
			expected: false,
		},
		{
			version:  "latest",
			expected: false,
		},
		{
			version:  "3ba0baa64b9b86a0a197e28fcb269a07cbae8e04",
			expected: false,
		},
		{
			version:  "git-ref-name",
			expected: false,
		},
		{
			version:  "very.weird.version",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(subT *testing.T) {
			res := detectEnvVarAuthorinoVersion(tt.version)
			if res != tt.expected {
				subT.Errorf("expected: %t, got: %t", tt.expected, res)
			}
		})
	}
}

func TestDeploymentImagePullPolicyMutator(t *testing.T) {
	t.Run("same imagepullpolicy", func(subT *testing.T) {
		desired := &appsv1.Deployment{}
		desired.Spec.Template.Spec.Containers = []corev1.Container{
			{
				ImagePullPolicy: corev1.PullAlways,
			},
		}
		existing := &appsv1.Deployment{}
		existing.Spec.Template.Spec.Containers = []corev1.Container{
			{
				ImagePullPolicy: corev1.PullAlways,
			},
		}

		update := DeploymentImagePullPolicyMutator(desired, existing)
		if update {
			subT.Error("expected no update")
		}
	})

	t.Run("diff imagepullpolicy", func(subT *testing.T) {
		desired := &appsv1.Deployment{}
		desired.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:            "authorino",
				ImagePullPolicy: corev1.PullAlways,
			},
		}
		existing := &appsv1.Deployment{}
		existing.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:            "authorino",
				ImagePullPolicy: corev1.PullNever,
			},
		}

		update := DeploymentImagePullPolicyMutator(desired, existing)
		if !update {
			subT.Error("expected update")
		}

		if existing.Spec.Template.Spec.Containers[0].ImagePullPolicy != corev1.PullAlways {
			subT.Error("expected pullalways")
		}
	})
}

func TestDeploymentContainerArgsMutator(t *testing.T) {
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "authorino"}}},
			},
		},
	}

	tests := []struct {
		name           string
		expectedUpdate bool
		existingArgs   []string
		desiredArgs    []string
		expectedArgs   []string
	}{
		{
			name:           "nil args",
			expectedUpdate: false,
			existingArgs:   nil,
			desiredArgs:    nil,
			expectedArgs:   nil,
		},
		{
			name:           "empty args",
			expectedUpdate: false,
			existingArgs:   []string{},
			desiredArgs:    []string{},
			expectedArgs:   []string{},
		},
		{
			name:           "same args",
			expectedUpdate: false,
			existingArgs:   []string{"a", "b", "c"},
			desiredArgs:    []string{"a", "b", "c"},
			expectedArgs:   []string{"a", "b", "c"},
		},
		{
			name:           "different args",
			expectedUpdate: true,
			existingArgs:   []string{"a", "b"},
			desiredArgs:    []string{"c"},
			expectedArgs:   []string{"c"},
		},
		{
			name:           "same args different order",
			expectedUpdate: false,
			existingArgs:   []string{"a", "b"},
			desiredArgs:    []string{"b", "a"},
			expectedArgs:   []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(subT *testing.T) {
			existing := deployment.DeepCopy()
			existing.Spec.Template.Spec.Containers[0].Args = tt.existingArgs
			desired := deployment.DeepCopy()
			desired.Spec.Template.Spec.Containers[0].Args = tt.desiredArgs

			update := DeploymentContainerArgsMutator(desired, existing)
			if update != tt.expectedUpdate {
				subT.Fatalf("expected: %t, got: %t", tt.expectedUpdate, update)
			}

			existingArgs := existing.Spec.Template.Spec.Containers[0].Args
			if !reflect.DeepEqual(existingArgs, tt.expectedArgs) {
				subT.Fatalf("expected: %v, got: %v", tt.expectedArgs, existingArgs)
			}
		})
	}
}
