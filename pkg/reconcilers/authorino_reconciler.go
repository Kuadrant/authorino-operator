package reconcilers

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8smeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	authorinoResources "github.com/kuadrant/authorino-operator/pkg/resources"
)

type AuthorinoReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *AuthorinoReconciler) ReconcileAuthorinoDeployment(ctx context.Context, authorinoInstance *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	deployment := AuthorinoDeployment(authorinoInstance)

	err = ctrl.SetControllerReference(authorinoInstance, deployment, r.Scheme)
	if err != nil {
		return r.WrapErrorWithStatusUpdate(logger, authorinoInstance, r.SetStatusFailed(StatusUnableToBuildDeploymentObject),
			fmt.Errorf("failed to set owner reference for Authorino Deployment: %s, err: %v", authorinoInstance.Name, err),
		)
	}

	err = r.reconcileDeployment(ctx, logger, deployment, authorinoInstance)
	if err != nil {
		return fmt.Errorf("failed to reconcile %s Deployment resource, err: %v", authorinoInstance.Name, err)
	}
	return nil
}

func (r *AuthorinoReconciler) ReconcileAuthorinoServices(ctx context.Context, authorinoInstance *api.Authorino) error {
	authorinoInstanceName := authorinoInstance.Name
	authorinoInstanceNamespace := authorinoInstance.Namespace

	var desiredServices []*k8score.Service
	var grpcPort, httpPort int32

	// auth service
	if p := authorinoInstance.Spec.Listener.Ports.GRPC; p != nil {
		grpcPort = *p
	} else if p := authorinoInstance.Spec.Listener.Port; p != nil { // deprecated
		grpcPort = *p
	} else {
		grpcPort = DefaultAuthGRPCServicePort
	}
	if p := authorinoInstance.Spec.Listener.Ports.HTTP; p != nil {
		httpPort = *p
	} else {
		httpPort = DefaultAuthHTTPServicePort
	}
	desiredServices = append(desiredServices, authorinoResources.NewAuthService(
		authorinoInstanceName,
		authorinoInstanceNamespace,
		grpcPort,
		httpPort,
		authorinoInstance.Labels,
	))

	// oidc service
	if p := authorinoInstance.Spec.OIDCServer.Port; p != nil {
		httpPort = *p
	} else {
		httpPort = DefaultOIDCServicePort
	}
	desiredServices = append(desiredServices, authorinoResources.NewOIDCService(
		authorinoInstanceName,
		authorinoInstanceNamespace,
		httpPort,
		authorinoInstance.Labels,
	))

	// metrics service
	if p := authorinoInstance.Spec.Metrics.Port; p != nil {
		httpPort = *p
	} else {
		httpPort = DefaultMetricsServicePort
	}
	desiredServices = append(desiredServices, authorinoResources.NewMetricsService(
		authorinoInstanceName,
		authorinoInstanceNamespace,
		httpPort,
		authorinoInstance.Labels,
	))

	for _, desiredService := range desiredServices {
		_ = ctrl.SetControllerReference(authorinoInstance, desiredService, r.Scheme)

		err := r.reconcileService(ctx, desiredService, authorinoInstance)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *AuthorinoReconciler) ReconcileAuthorinoPermissions(ctx context.Context, authorinoInstance *api.Authorino) error {

	// ClusterRoleBinding for the authorino-manager-role cluster role
	if err := r.reconcileManagerClusterRoleBinding(ctx, authorinoInstance); err != nil {
		return err
	}

	// RoleBinding for the authorino-manager-role cluster role
	if err := r.reconcileManagerRoleBinding(ctx, authorinoInstance); err != nil {
		return err
	}

	// ClusterRoleBinding for the authorino-manager-k8s-auth-role cluster role
	// for Authorino's Kubernetes TokenReview and SubjectAccessReview features
	// Disclaimer: this has nothing to do with kube-rbac-proxy, but to authn/authz features of Authorino that also require cluster scope role bindings
	if err := r.reconcileManagerAuthClusterRoleBinding(ctx, authorinoInstance); err != nil {
		return err
	}

	// authorino-leader-election-role role
	if err := r.reconcileLeaderElectionRole(ctx, authorinoInstance); err != nil {
		return err
	}

	// RoleBinding for the authorino-leader-election-role role
	if err := r.reconcileLeaderElectionRoleBinding(ctx, authorinoInstance); err != nil {
		return err
	}

	// Best-effort migration: remove ClusterRoleBindings created with the legacy
	// name scheme (<crName>-<suffix>). Those names omit the namespace, so
	// instances sharing a CR name in different namespaces collided on a single
	// cluster-scoped binding. This runs after the namespace-qualified bindings
	// are reconciled, so this instance's own permissions are never dropped
	// mid-reconcile. Note that when several instances shared a single legacy
	// binding, deleting it here can briefly remove another instance's permissions
	// until that instance reconciles and provisions its own namespace-qualified
	// binding.
	r.cleanupLegacyClusterRoleBindings(ctx, authorinoInstance)

	return nil
}

// cleanupLegacyClusterRoleBindings deletes the pre-namespace-qualified
// ClusterRoleBindings (named <crName>-<suffix>) for this instance, if present.
// It is best-effort: a missing binding is not an error, and other failures are
// logged rather than surfaced so they don't block reconciliation.
//
// The legacy name is <crName>-<suffix>, and crName is fully controlled by
// whoever creates the Authorino CR. Deleting purely by name would let a tenant
// name a CR so that the operator deletes a legacy binding owned by an instance
// in another namespace. To avoid that, each binding is read and only deleted
// once it is confirmed to belong to this instance: it must reference this
// instance's ServiceAccount as a subject. Bindings that cannot be confirmed are
// left in place.
func (r *AuthorinoReconciler) cleanupLegacyClusterRoleBindings(ctx context.Context, authorinoInstance *api.Authorino) {
	logger, _ := logr.FromContext(ctx)

	sa := authorinoResources.GetAuthorinoServiceAccount(authorinoInstance.Namespace, authorinoInstance.Name, authorinoInstance.Labels)
	ownSubject := authorinoResources.GetSubjectForRoleBinding(sa)

	for _, suffix := range []string{AuthorinoManagerClusterRoleBindingName, AuthorinoK8sAuthClusterRoleBindingName} {
		legacyName := fmt.Sprintf("%s-%s", authorinoInstance.Name, suffix)

		binding := &k8srbac.ClusterRoleBinding{}
		if err := r.Client.Get(ctx, client.ObjectKey{Name: legacyName}, binding); err != nil {
			if !errors.IsNotFound(err) {
				logger.Error(err, "failed to get legacy ClusterRoleBinding", "name", legacyName)
			}
			continue
		}

		if !authorinoResources.SubjectIncluded(binding.Subjects, ownSubject) {
			// The binding does not reference this instance's ServiceAccount, so
			// it was created for a different Authorino instance (e.g. one sharing
			// the CR name in another namespace). Leave it untouched.
			logger.Info("skipping legacy ClusterRoleBinding not owned by this instance", "name", legacyName)
			continue
		}

		// Delete only the exact object that was validated: the resourceVersion and
		// UID preconditions make the delete fail rather than remove a binding that
		// changed between the Get and the Delete.
		preconditions := client.Preconditions{
			UID:             &binding.UID,
			ResourceVersion: &binding.ResourceVersion,
		}
		if err := r.Client.Delete(ctx, binding, preconditions); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "failed to delete legacy ClusterRoleBinding", "name", legacyName)
		}
	}
}

func (r *AuthorinoReconciler) reconcileManagerClusterRoleBinding(ctx context.Context, authorinoInstance *api.Authorino) error {
	clusterRoleKey := client.ObjectKey{Name: AuthorinoManagerClusterRoleName}
	if err := r.checkClusterRoleExists(ctx, clusterRoleKey, authorinoInstance); err != nil {
		return err
	}

	sa := authorinoResources.GetAuthorinoServiceAccount(authorinoInstance.Namespace, authorinoInstance.Name, authorinoInstance.Labels)

	// if cluster scoped, ensure service account is in the binding
	if authorinoInstance.Spec.ClusterWide {
		binding := authorinoResources.GetAuthorinoClusterRoleBinding(authorinoInstance.Namespace, authorinoInstance.Name, AuthorinoManagerClusterRoleBindingName, AuthorinoManagerClusterRoleName, sa, authorinoInstance.Labels)
		return r.reconcileClusterRoleBinding(ctx, binding, authorinoInstance)
	}

	// local namespace scope
	// if switching from cluster-wide to namespaced, delete the ClusterRoleBinding
	binding := authorinoResources.GetAuthorinoClusterRoleBinding(authorinoInstance.Namespace, authorinoInstance.Name, AuthorinoManagerClusterRoleBindingName, AuthorinoManagerClusterRoleName, sa, authorinoInstance.Labels)
	TagObjectToDelete(binding)
	r.reconcileClusterRoleBinding(ctx, binding, authorinoInstance)

	return nil
}

func (r *AuthorinoReconciler) reconcileManagerRoleBinding(ctx context.Context, authorinoInstance *api.Authorino) error {
	clusterRoleKey := client.ObjectKey{Name: AuthorinoManagerClusterRoleName}
	if err := r.checkClusterRoleExists(ctx, clusterRoleKey, authorinoInstance); err != nil {
		return err
	}

	sa := authorinoResources.GetAuthorinoServiceAccount(authorinoInstance.Namespace, authorinoInstance.Name, authorinoInstance.Labels)

	binding := authorinoResources.GetAuthorinoRoleBinding(authorinoInstance.Namespace, authorinoInstance.Name, AuthorinoManagerClusterRoleBindingName, "ClusterRole", AuthorinoManagerClusterRoleName, sa, authorinoInstance.Labels)

	if authorinoInstance.Spec.ClusterWide {
		TagObjectToDelete(binding)
	}

	return r.reconcileRoleBinding(ctx, binding, authorinoInstance)
}

func (r *AuthorinoReconciler) reconcileManagerAuthClusterRoleBinding(ctx context.Context, authorinoInstance *api.Authorino) error {
	clusterRoleKey := client.ObjectKey{Name: AuthorinoK8sAuthClusterRoleName}
	if err := r.checkClusterRoleExists(ctx, clusterRoleKey, authorinoInstance); err != nil {
		return err
	}

	sa := authorinoResources.GetAuthorinoServiceAccount(authorinoInstance.Namespace, authorinoInstance.Name, authorinoInstance.Labels)

	binding := authorinoResources.GetAuthorinoClusterRoleBinding(authorinoInstance.Namespace, authorinoInstance.Name, AuthorinoK8sAuthClusterRoleBindingName, AuthorinoK8sAuthClusterRoleName, sa, authorinoInstance.Labels)
	return r.reconcileClusterRoleBinding(ctx, binding, authorinoInstance)
}

func (r *AuthorinoReconciler) reconcileLeaderElectionRole(ctx context.Context, authorinoInstance *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	role := &k8srbac.Role{
		TypeMeta:   k8smeta.TypeMeta{APIVersion: k8srbac.SchemeGroupVersion.String(), Kind: "Role"},
		ObjectMeta: k8smeta.ObjectMeta{Name: AuthorinoLeaderElectionRoleName, Namespace: authorinoInstance.Namespace},
		Rules:      authorinoResources.GetLeaderElectionRules(),
	}

	if err := ctrl.SetControllerReference(authorinoInstance, role, r.Scheme); err != nil {
		return err
	}

	crud, _, err := r.reconcileResource(ctx, &k8srbac.Role{}, role)
	if err != nil {
		if crud == "read" {
			return r.WrapErrorWithStatusUpdate(
				logger, authorinoInstance, r.SetStatusFailed(statusUnableToGetLeaderElectionRole),
				fmt.Errorf("failed to get %s role, err: %v", role.Name, err),
			)
		}

		// With create only mutator, update not happening

		if crud == "create" {
			return r.WrapErrorWithStatusUpdate(
				logger, authorinoInstance, r.SetStatusFailed(statusUnableToCreateLeaderElectionRole),
				fmt.Errorf("failed to create %s role, err: %v", role.Name, err),
			)
		}
	}

	return nil
}

func (r *AuthorinoReconciler) reconcileLeaderElectionRoleBinding(ctx context.Context, authorinoInstance *api.Authorino) error {
	sa := authorinoResources.GetAuthorinoServiceAccount(authorinoInstance.Namespace, authorinoInstance.Name, authorinoInstance.Labels)

	binding := authorinoResources.GetAuthorinoRoleBinding(
		authorinoInstance.Namespace,
		authorinoInstance.Name,
		authorinoLeaderElectionRoleBindingName,
		"Role",
		AuthorinoLeaderElectionRoleName,
		sa,
		authorinoInstance.Labels,
	)

	return r.reconcileRoleBinding(ctx, binding, authorinoInstance)
}

func (r *AuthorinoReconciler) checkClusterRoleExists(ctx context.Context, key client.ObjectKey, authorino *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	clusterRole := &k8srbac.ClusterRole{}
	if err := r.Client.Get(ctx, key, clusterRole); err != nil {
		if errors.IsNotFound(err) {
			return r.WrapErrorWithStatusUpdate(logger, authorino, r.SetStatusFailed(statusClusterRoleNotFound), fmt.Errorf("failed to find authorino ClusterRole %s: %v", key, err))
		} else {
			return r.WrapErrorWithStatusUpdate(logger, authorino, r.SetStatusFailed(statusUnableToGetClusterRole), fmt.Errorf("failed to get authorino ClusterRole %s: %v", key, err))
		}
	}

	return nil
}

func (r *AuthorinoReconciler) reconcileResource(ctx context.Context, obj, desired client.Object) (string, client.Object, error) {
	key := client.ObjectKeyFromObject(desired)

	if err := r.Client.Get(ctx, key, obj); err != nil {
		if !errors.IsNotFound(err) {
			return "read", nil, err
		}

		// Not found
		if !IsObjectTaggedToDelete(desired) {
			if err = r.CreateResource(ctx, desired); err != nil {
				return "create", nil, err
			}
			return "create", desired, nil
		}

		// Marked for deletion and not found. Nothing to do.
		return "read", nil, nil
	}

	if IsObjectTaggedToDelete(desired) {
		if err := r.DeleteResource(ctx, desired); err != nil {
			return "delete", desired, err
		}
		return "delete", desired, nil
	}

	// Apply the desired state using Server-Side Apply
	if err := r.ApplyResource(ctx, desired); err != nil {
		return "update", desired, err
	}

	return "", obj, nil
}

func (r *AuthorinoReconciler) CreateResource(ctx context.Context, obj client.Object) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	logger.Info("create object", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
	return r.Client.Create(ctx, obj)
}

func (r *AuthorinoReconciler) UpdateResource(ctx context.Context, obj client.Object) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	logger.Info("update object", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
	return r.Client.Update(ctx, obj)
}

func (r *AuthorinoReconciler) ApplyResource(ctx context.Context, obj client.Object) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	logger.Info("apply object", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
	return r.Client.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner("authorino-operator"))
}

func (r *AuthorinoReconciler) DeleteResource(ctx context.Context, obj client.Object, options ...client.DeleteOption) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	logger.Info("delete object", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
	return r.Client.Delete(ctx, obj, options...)
}

func (r *AuthorinoReconciler) reconcileDeployment(ctx context.Context, logger logr.Logger, desired *k8sapps.Deployment, authorino *api.Authorino) error {
	crud, obj, err := r.reconcileResource(ctx, &k8sapps.Deployment{}, desired)

	if crud == "read" && err != nil {
		return r.WrapErrorWithStatusUpdate(logger, authorino, r.SetStatusFailed(statusUnableToGetDeployment),
			fmt.Errorf("failed to get %s Deployment resource, err: %v", authorino.Name, err))
	}

	if crud == "create" && err != nil {
		return r.WrapErrorWithStatusUpdate(logger, authorino, r.SetStatusFailed(statusUnableToCreateDeployment),
			fmt.Errorf("failed to create %s Deployment resource, err: %v", desired.Name, err))
	}

	if crud == "update" && err != nil {
		return r.WrapErrorWithStatusUpdate(logger, authorino, r.SetStatusFailed(StatusUnableToBuildDeploymentObject),
			fmt.Errorf("failed to build %s Deployment resource for updating, err: %v", authorino.Name, err))
	}

	if err != nil {
		return r.WrapErrorWithStatusUpdate(logger, authorino, r.SetStatusFailed(statusUnableToUpdateDeployment), fmt.Errorf("failed to reconcile %s Deployment resource, err: %v", desired.Name, err))
	}

	if crud == "update" {
		if err = r.updateStatusConditions(authorino, statusNotReady(statusUpdated, "Authorino Deployment resource updated")); err != nil {
			return err
		}
		return nil
	}

	deployment, ok := obj.(*k8sapps.Deployment)
	if !ok {
		return fmt.Errorf("failed to cast object to Deployment")
	}

	if !DeploymentAvailable(deployment) {
		if err = r.updateStatusConditions(authorino, statusNotReady(statusDeploymentNotReady, "Authorino Deployment resource not ready")); err != nil {
			return err
		}
		return nil
	}

	if err = r.updateStatusConditions(authorino, statusReady()); err != nil {
		return err
	}
	return nil
}

func (r *AuthorinoReconciler) reconcileService(ctx context.Context, desired *k8score.Service, authorino *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	crud, _, err := r.reconcileResource(ctx, &k8score.Service{}, desired)

	if crud == "read" && err != nil {
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToGetServices), fmt.Errorf("failed to get %s service, err: %v", desired.Name, err))
	}

	if crud == "create" && err != nil {
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToCreateServices),
			fmt.Errorf("failed to create %s service, err: %v", desired.Name, err),
		)
	}

	if crud == "update" && err != nil {
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToGetServices),
			fmt.Errorf("failed to update %s service, err: %v", desired.Name, err),
		)
	}

	return nil
}

func (r *AuthorinoReconciler) reconcileRoleBinding(ctx context.Context, desired *k8srbac.RoleBinding, authorino *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	if err := ctrl.SetControllerReference(authorino, desired, r.Scheme); err != nil {
		return err
	}

	crud, _, err := r.reconcileResource(ctx, &k8srbac.RoleBinding{}, desired)

	if err = r.clusterRoleStatus(logger, authorino, crud, desired.Name, err); err != nil {
		return err
	}

	return nil
}

func (r *AuthorinoReconciler) reconcileClusterRoleBinding(ctx context.Context, desired *k8srbac.ClusterRoleBinding, authorino *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	crud, _, err := r.reconcileResource(ctx, &k8srbac.ClusterRoleBinding{}, desired)

	if err = r.clusterRoleStatus(logger, authorino, crud, desired.Name, err); err != nil {
		return err
	}

	return nil
}

func (r *AuthorinoReconciler) clusterRoleStatus(logger logr.Logger, authorino *api.Authorino, crud, name string, err error) error {
	if crud == "read" && err != nil {
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToGetBindingForClusterRole),
			fmt.Errorf("failed to get %s binding for authorino ClusterRole, err: %v", name, err),
		)
	}

	if crud == "create" && err != nil {
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToCreateBindingForClusterRole),
			fmt.Errorf("failed to create %s binding for authorino ClusterRole, err: %v", name, err),
		)
	}

	if crud == "update" && err != nil {
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToCreateBindingForClusterRole),
			fmt.Errorf("failed to update %s binding for authorino ClusterRole, err: %v", name, err),
		)
	}

	return nil
}

func (r *AuthorinoReconciler) ReconcileAuthorinoServiceAccount(ctx context.Context, authorino *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	sa := authorinoResources.GetAuthorinoServiceAccount(authorino.Namespace, authorino.Name, authorino.Labels)

	if err := ctrl.SetControllerReference(authorino, sa, r.Scheme); err != nil {
		return err
	}

	crud, _, err := r.reconcileResource(ctx, &k8score.ServiceAccount{}, sa)
	if err != nil {
		switch crud {
		case "read":
			return r.WrapErrorWithStatusUpdate(
				logger, authorino, r.SetStatusFailed(StatusUnableToGetServiceAccount),
				fmt.Errorf("failed to get %s ServiceAccount, err: %v", sa.Name, err),
			)
		case "create":
			return r.WrapErrorWithStatusUpdate(
				logger, authorino, r.SetStatusFailed(StatusUnableToCreateServiceAccount),
				fmt.Errorf("failed to create %s ServiceAccount, err: %v", sa.Name, err),
			)
			// update cannot happen as mutator is CreateOnly
		default:
			return r.WrapErrorWithStatusUpdate(
				logger, authorino, r.SetStatusFailed(StatusUnableToCreateServiceAccount),
				fmt.Errorf("failed to create %s ServiceAccount, err: %v", sa.Name, err),
			)
		}
	}

	return nil
}
