package reconcilers

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	authorinoResources "github.com/kuadrant/authorino-operator/pkg/resources"
)

var namespace = "test-namespace"

var authorinoInstance = &api.Authorino{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-authorino",
		Namespace: namespace,
	},
	Spec: api.AuthorinoSpec{
		Replicas: pointer.Int32(2),
		Image:    "quay.io/kuadrant/authorino:latest",
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

func setupTestEnvironment(t *testing.T, objs []client.Object) (*AuthorinoReconciler, context.Context) {
	t.Helper()

	logger := zap.New(zap.UseDevMode(true))
	ctx := log.IntoContext(context.Background(), logger)

	s := scheme.Scheme
	if err := api.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&api.Authorino{}, &appsv1.Deployment{}).WithObjects(objs...).Build()

	return &AuthorinoReconciler{
		Client: cl,
		Log:    logger,
		Scheme: s,
	}, ctx
}

func hasArg(args []string, flag string) bool {
	prefix := "--" + flag
	for _, a := range args {
		if a == prefix || strings.HasPrefix(a, prefix+"=") {
			return true
		}
	}
	return false
}

func getArgValue(args []string, flag string) string {
	prefix := "--" + flag + "="
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return strings.TrimPrefix(a, prefix)
		}
	}
	return ""
}

func TestBuildAuthorinoArgs(t *testing.T) {
	t.Run("TLS disabled omits all TLS flags", func(t *testing.T) {
		a := &api.Authorino{
			Spec: api.AuthorinoSpec{
				Listener: api.Listener{
					Tls: api.Tls{
						Enabled:      pointer.Bool(false),
						MinVersion:   "1.2",
						CipherSuites: []string{"TLS_AES_128_GCM_SHA256"},
					},
				},
				OIDCServer: api.OIDCServer{
					Tls: api.Tls{
						Enabled:      pointer.Bool(false),
						MinVersion:   "1.3",
						CipherSuites: []string{"TLS_AES_256_GCM_SHA384"},
					},
				},
			},
		}
		args := buildAuthorinoArgs(a)
		for _, flag := range []string{
			FlagTlsCertPath, FlagTlsCertKeyPath, FlagTlsMinVersion, FlagTlsMaxVersion, FlagTlsCipherSuites,
			FlagOidcTLSCertPath, FlagOidcTLSCertKeyPath, FlagOidcTlsMinVersion, FlagOidcTlsMaxVersion, FlagOidcTlsCipherSuites,
		} {
			if hasArg(args, flag) {
				t.Errorf("expected --%s to be absent when TLS is disabled, but it was present", flag)
			}
		}
	})

	t.Run("TLS enabled includes cert and version flags", func(t *testing.T) {
		a := &api.Authorino{
			Spec: api.AuthorinoSpec{
				Listener: api.Listener{
					Tls: api.Tls{
						Enabled:      pointer.Bool(true),
						MinVersion:   "1.2",
						MaxVersion:   "1.3",
						CipherSuites: []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384"},
					},
				},
				OIDCServer: api.OIDCServer{
					Tls: api.Tls{
						Enabled:      pointer.Bool(true),
						MinVersion:   "1.3",
						CipherSuites: []string{"TLS_CHACHA20_POLY1305_SHA256"},
					},
				},
			},
		}
		args := buildAuthorinoArgs(a)

		if v := getArgValue(args, FlagTlsMinVersion); v != "1.2" {
			t.Errorf("expected --%s=1.2, got %q", FlagTlsMinVersion, v)
		}
		if v := getArgValue(args, FlagTlsMaxVersion); v != "1.3" {
			t.Errorf("expected --%s=1.3, got %q", FlagTlsMaxVersion, v)
		}
		if v := getArgValue(args, FlagTlsCipherSuites); v != "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384" {
			t.Errorf("expected --%s=TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384, got %q", FlagTlsCipherSuites, v)
		}
		if v := getArgValue(args, FlagOidcTlsMinVersion); v != "1.3" {
			t.Errorf("expected --%s=1.3, got %q", FlagOidcTlsMinVersion, v)
		}
		if hasArg(args, FlagOidcTlsMaxVersion) {
			t.Errorf("expected --%s to be absent when not set, but it was present", FlagOidcTlsMaxVersion)
		}
		if v := getArgValue(args, FlagOidcTlsCipherSuites); v != "TLS_CHACHA20_POLY1305_SHA256" {
			t.Errorf("expected --%s=TLS_CHACHA20_POLY1305_SHA256, got %q", FlagOidcTlsCipherSuites, v)
		}
		if !hasArg(args, FlagTlsCertPath) {
			t.Errorf("expected --%s to be present when TLS is enabled", FlagTlsCertPath)
		}
		if !hasArg(args, FlagOidcTLSCertPath) {
			t.Errorf("expected --%s to be present when TLS is enabled", FlagOidcTLSCertPath)
		}
	})

	t.Run("TLS enabled nil defaults to enabled", func(t *testing.T) {
		a := &api.Authorino{
			Spec: api.AuthorinoSpec{
				Listener: api.Listener{
					Tls: api.Tls{
						MinVersion: "1.2",
					},
				},
				OIDCServer: api.OIDCServer{
					Tls: api.Tls{
						MinVersion: "1.3",
					},
				},
			},
		}
		args := buildAuthorinoArgs(a)

		if !hasArg(args, FlagTlsCertPath) {
			t.Errorf("expected --%s when Enabled is nil (defaults to true)", FlagTlsCertPath)
		}
		if v := getArgValue(args, FlagTlsMinVersion); v != "1.2" {
			t.Errorf("expected --%s=1.2, got %q", FlagTlsMinVersion, v)
		}
		if !hasArg(args, FlagOidcTLSCertPath) {
			t.Errorf("expected --%s when Enabled is nil (defaults to true)", FlagOidcTLSCertPath)
		}
		if v := getArgValue(args, FlagOidcTlsMinVersion); v != "1.3" {
			t.Errorf("expected --%s=1.3, got %q", FlagOidcTlsMinVersion, v)
		}
	})

	t.Run("empty version and cipher fields are omitted", func(t *testing.T) {
		a := &api.Authorino{
			Spec: api.AuthorinoSpec{
				Listener: api.Listener{
					Tls: api.Tls{
						Enabled: pointer.Bool(true),
					},
				},
				OIDCServer: api.OIDCServer{
					Tls: api.Tls{
						Enabled: pointer.Bool(true),
					},
				},
			},
		}
		args := buildAuthorinoArgs(a)

		if !hasArg(args, FlagTlsCertPath) {
			t.Errorf("expected --%s when TLS is enabled", FlagTlsCertPath)
		}
		for _, flag := range []string{FlagTlsMinVersion, FlagTlsMaxVersion, FlagTlsCipherSuites, FlagOidcTlsMinVersion, FlagOidcTlsMaxVersion, FlagOidcTlsCipherSuites} {
			if hasArg(args, flag) {
				t.Errorf("expected --%s to be absent when value is empty, but it was present", flag)
			}
		}
	})
}

func TestReconcileService(t *testing.T) {
	t.Run("update existing service", func(t *testing.T) {
		existingService := &k8score.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-authorino",
				Namespace: namespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: k8score.SchemeGroupVersion.String(),
			},
			Spec: k8score.ServiceSpec{
				Selector: map[string]string{"old-label": "old-value"},
			},
		}

		r, ctx := setupTestEnvironment(t, []client.Object{authorinoInstance, existingService})

		newLabels := map[string]string{"new-label": "new-value"}
		desiredService := existingService.DeepCopy()
		desiredService.Spec.Selector = newLabels
		desiredService.Labels = newLabels
		err := r.reconcileService(ctx, desiredService, authorinoInstance)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated := &k8score.Service{}
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(desiredService), updated)
		if err != nil {
			t.Fatalf("expected service to exist: %v", err)
		}

		if updated.Spec.Selector["new-label"] != "new-value" {
			t.Errorf("expected selector 'new-label' to be 'new-value', got %s", updated.Spec.Selector["new-label"])
		}

		if updated.Labels["new-label"] != "new-value" {
			t.Errorf("expected label 'new-label' to be 'new-value', got %s", updated.Labels["new-label"])
		}
	})
}

func TestReconcileDeployment(t *testing.T) {
	t.Run("create new deployment", func(t *testing.T) {
		r, ctx := setupTestEnvironment(t, []client.Object{authorinoInstance})
		logger := log.FromContext(ctx)

		deployment := AuthorinoDeployment(authorinoInstance)
		err := r.reconcileDeployment(ctx, logger, deployment, authorinoInstance)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		created := &appsv1.Deployment{}
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(deployment), created)
		if err != nil {
			t.Fatalf("expected deployment to exist: %v", err)
		}
	})

	t.Run("update existing deployment", func(t *testing.T) {
		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-authorino",
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: pointer.Int32(1),
			},
		}

		r, ctx := setupTestEnvironment(t, []client.Object{authorinoInstance, existingDeployment})
		logger := log.FromContext(ctx)

		authorinoInstance.Spec.Replicas = pointer.Int32(3)
		authorinoInstance.Labels = map[string]string{"test": "test1"}
		desiredDeployment := AuthorinoDeployment(authorinoInstance)
		err := r.reconcileDeployment(ctx, logger, desiredDeployment, authorinoInstance)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated := &appsv1.Deployment{}
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(desiredDeployment), updated)
		if err != nil {
			t.Fatalf("expected deployment to exist: %v", err)
		}

		if *updated.Spec.Replicas != 3 {
			t.Errorf("expected 3 replicas after update, got %d", *updated.Spec.Replicas)
		}

		if updated.ObjectMeta.Labels["test"] != "test1" {
			t.Errorf("expected label 'test' to be 'test1', got %s", updated.Labels["test"])
		}
	})

	t.Run("update existing deployment with no replica mutators", func(t *testing.T) {
		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-authorino",
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: pointer.Int32(1),
			},
		}

		r, ctx := setupTestEnvironment(t, []client.Object{authorinoInstance, existingDeployment})
		logger := log.FromContext(ctx)

		authorinoInstance.Spec.Replicas = pointer.Int32(3)
		authorinoInstance.Labels = map[string]string{"test": "test1"}
		desiredDeployment := AuthorinoDeployment(authorinoInstance)
		err := r.reconcileDeployment(ctx, logger, desiredDeployment, authorinoInstance)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated := &appsv1.Deployment{}
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(desiredDeployment), updated)
		if err != nil {
			t.Fatalf("expected deployment to exist: %v", err)
		}

		// With Server-Side Apply, all fields in the desired state are applied
		// Even if ReplicasMutator is not included, replicas will be updated because
		// they are part of the desired deployment state
		if *updated.Spec.Replicas != *authorinoInstance.Spec.Replicas {
			t.Errorf("expected replicas to be updated to %d, got %d", *authorinoInstance.Spec.Replicas, *updated.Spec.Replicas)
		}

	})

	t.Run("deployment not ready", func(t *testing.T) {
		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-authorino",
				Namespace: namespace,
			},
			Status: appsv1.DeploymentStatus{},
		}

		r, ctx := setupTestEnvironment(t, []client.Object{authorinoInstance, existingDeployment})
		logger := log.FromContext(ctx)

		desiredDeployment := AuthorinoDeployment(authorinoInstance)
		err := r.reconcileDeployment(ctx, logger, desiredDeployment, authorinoInstance)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated := &api.Authorino{}
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(authorinoInstance), updated)
		if err != nil {
			t.Fatalf("expected deployment to exist: %v", err)
		}

		if updated.Status.Conditions[0].Type != api.ConditionReady || updated.Status.Conditions[0].Status != k8score.ConditionFalse {
			t.Errorf("expected condition type to be %s:%s, got %s", api.ConditionReady, k8score.ConditionFalse, updated.Status.Conditions[0])
		}
	})
}

func clusterRoleBindingSubject(namespace, crName string) k8srbac.Subject {
	sa := authorinoResources.GetAuthorinoServiceAccount(namespace, crName, nil)
	return authorinoResources.GetSubjectForRoleBinding(sa)
}

func TestCleanupLegacyClusterRoleBindings(t *testing.T) {
	const suffix = AuthorinoK8sAuthClusterRoleBindingName

	t.Run("deletes legacy binding owned by this instance", func(t *testing.T) {
		instance := &api.Authorino{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "team-a"},
		}
		legacyName := instance.Name + "-" + suffix
		binding := &k8srbac.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: legacyName},
			RoleRef:    k8srbac.RoleRef{APIGroup: k8srbac.GroupName, Kind: "ClusterRole", Name: AuthorinoK8sAuthClusterRoleName},
			Subjects:   []k8srbac.Subject{clusterRoleBindingSubject("team-a", "gateway")},
		}
		r, ctx := setupTestEnvironment(t, []client.Object{binding})

		r.cleanupLegacyClusterRoleBindings(ctx, instance)

		err := r.Client.Get(ctx, client.ObjectKey{Name: legacyName}, &k8srbac.ClusterRoleBinding{})
		if !apierrors.IsNotFound(err) {
			t.Fatalf("expected legacy binding %q to be deleted, got err=%v", legacyName, err)
		}
	})

	t.Run("preserves colliding legacy binding owned by another instance", func(t *testing.T) {
		// A legacy CR named "team-a.gateway" (in namespace "team") produced a
		// binding named "team-a.gateway-<suffix>", which is byte-identical to the
		// new canonical name for namespace "team-a" / CR "gateway". Reconciling
		// the "team-a.gateway" instance must not delete (or alter) a binding that
		// belongs to the "team-a"/"gateway" instance.
		instance := &api.Authorino{
			ObjectMeta: metav1.ObjectMeta{Name: "team-a.gateway", Namespace: "team"},
		}
		legacyName := instance.Name + "-" + suffix // "team-a.gateway-<suffix>"

		foreignSubject := clusterRoleBindingSubject("team-a", "gateway")
		binding := &k8srbac.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: legacyName},
			RoleRef:    k8srbac.RoleRef{APIGroup: k8srbac.GroupName, Kind: "ClusterRole", Name: AuthorinoK8sAuthClusterRoleName},
			Subjects:   []k8srbac.Subject{foreignSubject},
		}
		r, ctx := setupTestEnvironment(t, []client.Object{binding})

		r.cleanupLegacyClusterRoleBindings(ctx, instance)

		got := &k8srbac.ClusterRoleBinding{}
		if err := r.Client.Get(ctx, client.ObjectKey{Name: legacyName}, got); err != nil {
			t.Fatalf("expected colliding binding %q to be preserved, got err=%v", legacyName, err)
		}
		if len(got.Subjects) != 1 || got.Subjects[0] != foreignSubject {
			t.Errorf("expected legacy subject to be preserved, got %+v", got.Subjects)
		}
	})
}
