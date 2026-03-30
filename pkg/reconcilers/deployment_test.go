package reconcilers

import (
	"testing"

	k8score "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	api "github.com/kuadrant/authorino-operator/api/v1beta1"
)

func TestAuthorinoDeployment_VolumeMounts(t *testing.T) {
	tests := []struct {
		name                 string
		volumeSpec           api.VolumesSpec
		tlsEnabled           bool
		oidcTlsEnabled       bool
		expectedVolumeMounts []k8score.VolumeMount
		expectedVolumes      []k8score.Volume
	}{
		{
			name: "volume without items mounts entire volume to path",
			volumeSpec: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:      "config-volume",
						MountPath: "/etc/config",
						ConfigMaps: []string{"my-config"},
					},
				},
			},
			tlsEnabled:     false,
			oidcTlsEnabled: false,
			expectedVolumeMounts: []k8score.VolumeMount{
				{
					Name:      "config-volume",
					MountPath: "/etc/config",
				},
			},
		},
		{
			name: "volume with single item and directory mountPath",
			volumeSpec: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:      "ca-volume",
						MountPath: "/etc/ssl/certs/",
						Secrets:   []string{"ca-secret"},
						Items: []k8score.KeyToPath{
							{Key: "ca.crt", Path: "ca.crt"},
						},
					},
				},
			},
			tlsEnabled:     false,
			oidcTlsEnabled: false,
			expectedVolumeMounts: []k8score.VolumeMount{
				{
					Name:      "ca-volume",
					MountPath: "/etc/ssl/certs/ca.crt",
					SubPath:   "ca.crt",
				},
			},
		},
		{
			name: "volume with single item and directory mountPath without trailing slash",
			volumeSpec: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:      "ca-volume",
						MountPath: "/etc/ssl/certs",
						Secrets:   []string{"ca-secret"},
						Items: []k8score.KeyToPath{
							{Key: "ca.crt", Path: "ca.crt"},
						},
					},
				},
			},
			tlsEnabled:     false,
			oidcTlsEnabled: false,
			expectedVolumeMounts: []k8score.VolumeMount{
				{
					Name:      "ca-volume",
					MountPath: "/etc/ssl/certs/ca.crt",
					SubPath:   "ca.crt",
				},
			},
		},
		{
			name: "volume with single item and full-path mountPath (backward compatible)",
			volumeSpec: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:      "ca-volume",
						MountPath: "/etc/ssl/certs/custom-ca.crt",
						Secrets:   []string{"ca-secret"},
						Items: []k8score.KeyToPath{
							{Key: "ca.crt", Path: "custom-ca.crt"},
						},
					},
				},
			},
			tlsEnabled:     false,
			oidcTlsEnabled: false,
			expectedVolumeMounts: []k8score.VolumeMount{
				{
					Name:      "ca-volume",
					MountPath: "/etc/ssl/certs/custom-ca.crt",
					SubPath:   "custom-ca.crt",
				},
			},
		},
		{
			name: "volume with multiple items from single secret",
			volumeSpec: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:      "multi-cert-volume",
						MountPath: "/etc/ssl/certs/custom/",
						Secrets:   []string{"cert-bundle"},
						Items: []k8score.KeyToPath{
							{Key: "cert1.crt", Path: "cert1.crt"},
							{Key: "cert2.crt", Path: "cert2.crt"},
						},
					},
				},
			},
			tlsEnabled:     false,
			oidcTlsEnabled: false,
			expectedVolumeMounts: []k8score.VolumeMount{
				{
					Name:      "multi-cert-volume",
					MountPath: "/etc/ssl/certs/custom/cert1.crt",
					SubPath:   "cert1.crt",
				},
				{
					Name:      "multi-cert-volume",
					MountPath: "/etc/ssl/certs/custom/cert2.crt",
					SubPath:   "cert2.crt",
				},
			},
		},
		{
			name: "multiple volumes mounting to same directory without clash",
			volumeSpec: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:      "ca1-volume",
						MountPath: "/etc/ssl/certs/",
						Secrets:   []string{"ca1-secret"},
						Items: []k8score.KeyToPath{
							{Key: "ca.crt", Path: "ca1.crt"},
						},
					},
					{
						Name:      "ca2-volume",
						MountPath: "/etc/ssl/certs/",
						Secrets:   []string{"ca2-secret"},
						Items: []k8score.KeyToPath{
							{Key: "ca.crt", Path: "ca2.crt"},
						},
					},
				},
			},
			tlsEnabled:     false,
			oidcTlsEnabled: false,
			expectedVolumeMounts: []k8score.VolumeMount{
				{
					Name:      "ca1-volume",
					MountPath: "/etc/ssl/certs/ca1.crt",
					SubPath:   "ca1.crt",
				},
				{
					Name:      "ca2-volume",
					MountPath: "/etc/ssl/certs/ca2.crt",
					SubPath:   "ca2.crt",
				},
			},
		},
		{
			name: "volume with item using key as path when path is empty",
			volumeSpec: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:      "ca-volume",
						MountPath: "/etc/ssl/certs/",
						Secrets:   []string{"ca-secret"},
						Items: []k8score.KeyToPath{
							{Key: "tls.crt", Path: ""},
						},
					},
				},
			},
			tlsEnabled:     false,
			oidcTlsEnabled: false,
			expectedVolumeMounts: []k8score.VolumeMount{
				{
					Name:      "ca-volume",
					MountPath: "/etc/ssl/certs/tls.crt",
					SubPath:   "tls.crt",
				},
			},
		},
		{
			name: "custom volumes with TLS enabled should not clash",
			volumeSpec: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:      "ca-volume",
						MountPath: "/etc/ssl/certs/",
						Secrets:   []string{"ca-secret"},
						Items: []k8score.KeyToPath{
							{Key: "ca.crt", Path: "custom-ca.crt"},
						},
					},
				},
			},
			tlsEnabled:     true,
			oidcTlsEnabled: true,
			expectedVolumeMounts: []k8score.VolumeMount{
				{
					Name:      "ca-volume",
					MountPath: "/etc/ssl/certs/custom-ca.crt",
					SubPath:   "custom-ca.crt",
				},
				{
					Name:      AuthorinoTlsCertVolumeName,
					MountPath: DefaultTlsCertPath,
					SubPath:   "tls.crt",
					ReadOnly:  true,
				},
				{
					Name:      AuthorinoTlsCertVolumeName,
					MountPath: DefaultTlsCertKeyPath,
					SubPath:   "tls.key",
					ReadOnly:  true,
				},
				{
					Name:      AuthorinoOidcTlsCertVolumeName,
					MountPath: DefaultOidcTlsCertPath,
					SubPath:   "tls.crt",
					ReadOnly:  true,
				},
				{
					Name:      AuthorinoOidcTlsCertVolumeName,
					MountPath: DefaultOidcTlsCertKeyPath,
					SubPath:   "tls.key",
					ReadOnly:  true,
				},
			},
		},
		{
			name: "volume with configmap and items",
			volumeSpec: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:       "config-volume",
						MountPath:  "/etc/config/",
						ConfigMaps: []string{"my-config"},
						Items: []k8score.KeyToPath{
							{Key: "config.yaml", Path: "app-config.yaml"},
						},
					},
				},
			},
			tlsEnabled:     false,
			oidcTlsEnabled: false,
			expectedVolumeMounts: []k8score.VolumeMount{
				{
					Name:      "config-volume",
					MountPath: "/etc/config/app-config.yaml",
					SubPath:   "app-config.yaml",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorino := &api.Authorino{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-authorino",
					Namespace: "test-namespace",
				},
				Spec: api.AuthorinoSpec{
					Image:   "quay.io/kuadrant/authorino:latest",
					Volumes: tt.volumeSpec,
					Listener: api.Listener{
						Tls: api.Tls{
							Enabled: pointer.Bool(tt.tlsEnabled),
							CertSecret: &k8score.LocalObjectReference{
								Name: "server-tls",
							},
						},
					},
					OIDCServer: api.OIDCServer{
						Tls: api.Tls{
							Enabled: pointer.Bool(tt.oidcTlsEnabled),
							CertSecret: &k8score.LocalObjectReference{
								Name: "oidc-tls",
							},
						},
					},
				},
			}

			deployment := AuthorinoDeployment(authorino)

			if len(deployment.Spec.Template.Spec.Containers) != 1 {
				t.Fatalf("expected 1 container, got %d", len(deployment.Spec.Template.Spec.Containers))
			}

			container := deployment.Spec.Template.Spec.Containers[0]

			if len(container.VolumeMounts) != len(tt.expectedVolumeMounts) {
				t.Fatalf("expected %d volume mounts, got %d\nExpected: %+v\nActual: %+v",
					len(tt.expectedVolumeMounts), len(container.VolumeMounts),
					tt.expectedVolumeMounts, container.VolumeMounts)
			}

			for i, expected := range tt.expectedVolumeMounts {
				actual := container.VolumeMounts[i]
				if actual.Name != expected.Name {
					t.Errorf("volumeMount[%d].Name: expected %q, got %q", i, expected.Name, actual.Name)
				}
				if actual.MountPath != expected.MountPath {
					t.Errorf("volumeMount[%d].MountPath: expected %q, got %q", i, expected.MountPath, actual.MountPath)
				}
				if actual.SubPath != expected.SubPath {
					t.Errorf("volumeMount[%d].SubPath: expected %q, got %q", i, expected.SubPath, actual.SubPath)
				}
				if actual.ReadOnly != expected.ReadOnly {
					t.Errorf("volumeMount[%d].ReadOnly: expected %v, got %v", i, expected.ReadOnly, actual.ReadOnly)
				}
			}
		})
	}
}

func TestAuthorinoDeployment_VolumeProjections(t *testing.T) {
	tests := []struct {
		name                   string
		volumeSpec             api.VolumeSpec
		expectedProjectedCount int
		validateProjection     func(*testing.T, []k8score.VolumeProjection)
	}{
		{
			name: "volume with single secret creates one projection",
			volumeSpec: api.VolumeSpec{
				Name:      "secret-volume",
				MountPath: "/etc/secrets/",
				Secrets:   []string{"my-secret"},
				Items: []k8score.KeyToPath{
					{Key: "key1", Path: "file1"},
				},
			},
			expectedProjectedCount: 1,
			validateProjection: func(t *testing.T, projections []k8score.VolumeProjection) {
				if projections[0].Secret == nil {
					t.Error("expected secret projection, got nil")
					return
				}
				if projections[0].Secret.Name != "my-secret" {
					t.Errorf("expected secret name 'my-secret', got %q", projections[0].Secret.Name)
				}
				if len(projections[0].Secret.Items) != 1 {
					t.Errorf("expected 1 item, got %d", len(projections[0].Secret.Items))
				}
			},
		},
		{
			name: "volume with multiple secrets creates multiple projections",
			volumeSpec: api.VolumeSpec{
				Name:      "multi-secret-volume",
				MountPath: "/etc/secrets/",
				Secrets:   []string{"secret1", "secret2"},
				Items: []k8score.KeyToPath{
					{Key: "key1", Path: "file1"},
				},
			},
			expectedProjectedCount: 2,
			validateProjection: func(t *testing.T, projections []k8score.VolumeProjection) {
				if projections[0].Secret == nil || projections[1].Secret == nil {
					t.Error("expected secret projections, got nil")
					return
				}
				if projections[0].Secret.Name != "secret1" {
					t.Errorf("expected first secret name 'secret1', got %q", projections[0].Secret.Name)
				}
				if projections[1].Secret.Name != "secret2" {
					t.Errorf("expected second secret name 'secret2', got %q", projections[1].Secret.Name)
				}
			},
		},
		{
			name: "volume with single configmap creates one projection",
			volumeSpec: api.VolumeSpec{
				Name:       "config-volume",
				MountPath:  "/etc/config/",
				ConfigMaps: []string{"my-config"},
				Items: []k8score.KeyToPath{
					{Key: "config.yaml", Path: "app.yaml"},
				},
			},
			expectedProjectedCount: 1,
			validateProjection: func(t *testing.T, projections []k8score.VolumeProjection) {
				if projections[0].ConfigMap == nil {
					t.Error("expected configmap projection, got nil")
					return
				}
				if projections[0].ConfigMap.Name != "my-config" {
					t.Errorf("expected configmap name 'my-config', got %q", projections[0].ConfigMap.Name)
				}
			},
		},
		{
			name: "volume with multiple configmaps creates multiple projections",
			volumeSpec: api.VolumeSpec{
				Name:       "multi-config-volume",
				MountPath:  "/etc/config/",
				ConfigMaps: []string{"config1", "config2"},
				Items: []k8score.KeyToPath{
					{Key: "key1", Path: "file1"},
				},
			},
			expectedProjectedCount: 2,
			validateProjection: func(t *testing.T, projections []k8score.VolumeProjection) {
				if projections[0].ConfigMap == nil || projections[1].ConfigMap == nil {
					t.Error("expected configmap projections, got nil")
					return
				}
				if projections[0].ConfigMap.Name != "config1" {
					t.Errorf("expected first configmap name 'config1', got %q", projections[0].ConfigMap.Name)
				}
				if projections[1].ConfigMap.Name != "config2" {
					t.Errorf("expected second configmap name 'config2', got %q", projections[1].ConfigMap.Name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorino := &api.Authorino{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-authorino",
					Namespace: "test-namespace",
				},
				Spec: api.AuthorinoSpec{
					Image: "quay.io/kuadrant/authorino:latest",
					Volumes: api.VolumesSpec{
						Items: []api.VolumeSpec{tt.volumeSpec},
					},
					Listener: api.Listener{
						Tls: api.Tls{
							Enabled: pointer.Bool(false),
						},
					},
					OIDCServer: api.OIDCServer{
						Tls: api.Tls{
							Enabled: pointer.Bool(false),
						},
					},
				},
			}

			deployment := AuthorinoDeployment(authorino)

			if len(deployment.Spec.Template.Spec.Volumes) == 0 {
				t.Fatal("expected at least 1 volume, got 0")
			}

			volume := deployment.Spec.Template.Spec.Volumes[0]
			if volume.Projected == nil {
				t.Fatal("expected projected volume, got nil")
			}

			if len(volume.Projected.Sources) != tt.expectedProjectedCount {
				t.Fatalf("expected %d projected sources, got %d",
					tt.expectedProjectedCount, len(volume.Projected.Sources))
			}

			tt.validateProjection(t, volume.Projected.Sources)
		})
	}
}

func TestAuthorinoDeployment_DefaultMode(t *testing.T) {
	defaultMode := int32(0640)
	authorino := &api.Authorino{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-authorino",
			Namespace: "test-namespace",
		},
		Spec: api.AuthorinoSpec{
			Image: "quay.io/kuadrant/authorino:latest",
			Volumes: api.VolumesSpec{
				DefaultMode: &defaultMode,
				Items: []api.VolumeSpec{
					{
						Name:      "test-volume",
						MountPath: "/etc/test/",
						Secrets:   []string{"test-secret"},
						Items: []k8score.KeyToPath{
							{Key: "key1", Path: "file1"},
						},
					},
				},
			},
			Listener: api.Listener{
				Tls: api.Tls{
					Enabled: pointer.Bool(false),
				},
			},
			OIDCServer: api.OIDCServer{
				Tls: api.Tls{
					Enabled: pointer.Bool(false),
				},
			},
		},
	}

	deployment := AuthorinoDeployment(authorino)

	if len(deployment.Spec.Template.Spec.Volumes) == 0 {
		t.Fatal("expected at least 1 volume, got 0")
	}

	volume := deployment.Spec.Template.Spec.Volumes[0]
	if volume.Projected == nil {
		t.Fatal("expected projected volume, got nil")
	}

	if volume.Projected.DefaultMode == nil {
		t.Fatal("expected DefaultMode to be set, got nil")
	}

	if *volume.Projected.DefaultMode != defaultMode {
		t.Errorf("expected DefaultMode %d, got %d", defaultMode, *volume.Projected.DefaultMode)
	}
}
