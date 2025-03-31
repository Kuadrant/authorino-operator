package reconcilers

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"

	api "github.com/kuadrant/authorino-operator/api/v1beta1"
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

func TestReconcileService(t *testing.T) {
	t.Run("update existing service", func(t *testing.T) {
		existingService := &k8score.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-authorino",
				Namespace: namespace,
			},
			Spec: k8score.ServiceSpec{
				Selector: map[string]string{"old-label": "old-value"},
			},
		}

		r, ctx := setupTestEnvironment(t, []client.Object{authorinoInstance, existingService})
		logger := log.FromContext(ctx)

		newLabels := map[string]string{"new-label": "new-value"}
		desiredService := existingService.DeepCopy()
		desiredService.Spec.Selector = newLabels
		desiredService.Labels = newLabels
		err := r.reconcileService(ctx, logger, desiredService, ServiceMutator(LabelsMutator, PortMutator, SelectorMutator), authorinoInstance)
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
		err := r.reconcileDeployment(ctx, logger, deployment, DeploymentMutator(DeploymentReplicasMutator), authorinoInstance)
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
		err := r.reconcileDeployment(ctx, logger, desiredDeployment, DeploymentMutator(DeploymentReplicasMutator, DeploymentLabelsMutator), authorinoInstance)
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
		err := r.reconcileDeployment(ctx, logger, desiredDeployment, DeploymentMutator(DeploymentLabelsMutator), authorinoInstance)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated := &appsv1.Deployment{}
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(desiredDeployment), updated)
		if err != nil {
			t.Fatalf("expected deployment to exist: %v", err)
		}

		if *updated.Spec.Replicas != *existingDeployment.Spec.Replicas {
			t.Errorf("expected replicas not to be updated, got %d", *updated.Spec.Replicas)
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
		err := r.reconcileDeployment(ctx, logger, desiredDeployment, DeploymentMutator(DeploymentLabelsMutator), authorinoInstance)
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
