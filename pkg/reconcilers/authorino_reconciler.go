package reconcilers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	deploymentMutators := make([]DeploymentMutateFn, 0)
	if authorinoInstance.Spec.Replicas != nil {
		deploymentMutators = append(deploymentMutators, DeploymentReplicasMutator)
	}

	deploymentMutators = append(deploymentMutators,
		DeploymentContainerListMutator,
		DeploymentImageMutator,
		DeploymentServiceAccountMutator,
		DeploymentLabelsMutator,
		DeploymentSpecTemplateLabelsMutator,
		DeploymentVolumesMutator,
		DeploymentVolumeMountsMutator,
	)

	deployment := AuthorinoDeployment(authorinoInstance)

	err = ctrl.SetControllerReference(authorinoInstance, deployment, r.Scheme)
	if err != nil {
		return r.WrapErrorWithStatusUpdate(logger, authorinoInstance, r.SetStatusFailed(StatusUnableToBuildDeploymentObject),
			fmt.Errorf("failed to set owner reference for Authorino Deployment: %s, err: %v", authorinoInstance.Name, err),
		)
	}

	err = r.reconcileDeployment(ctx, logger, deployment, DeploymentMutator(deploymentMutators...), authorinoInstance)
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

		serviceMutators := make([]ServiceMutateFn, 0)

		serviceMutators = append(serviceMutators,
			LabelsMutator,
			PortMutator,
			SelectorMutator,
		)

		err := r.reconcileService(ctx, desiredService, ServiceMutator(serviceMutators...), authorinoInstance)
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

	return nil
}

func (r *AuthorinoReconciler) reconcileManagerClusterRoleBinding(ctx context.Context, authorinoInstance *api.Authorino) error {
	clusterRoleKey := client.ObjectKey{Name: AuthorinoManagerClusterRoleName}
	if err := r.checkClusterRoleExists(ctx, clusterRoleKey, authorinoInstance); err != nil {
		return err
	}

	sa := authorinoResources.GetAuthorinoServiceAccount(authorinoInstance.Namespace, authorinoInstance.Name, authorinoInstance.Labels)

	// if cluster scoped, ensure service account is in the binding
	if authorinoInstance.Spec.ClusterWide {
		binding := authorinoResources.GetAuthorinoClusterRoleBinding(AuthorinoManagerClusterRoleBindingName, AuthorinoManagerClusterRoleName, sa)
		return r.reconcileClusterRoleBinding(
			ctx, binding,
			ClusterRoleBindingMutator(ClusterRoleBindingLabelsMutator, ClusterRoleBindingSubjectMutator),
			authorinoInstance,
		)
	}

	// local namespace scope
	// ensure the service account is NOT in the binding
	r.UnboundAuthorinoServiceAccountFromClusterRole(ctx, AuthorinoManagerClusterRoleBindingName, sa)

	return nil
}

func (r *AuthorinoReconciler) reconcileManagerRoleBinding(ctx context.Context, authorinoInstance *api.Authorino) error {
	// TODO
	return nil
}

func (r *AuthorinoReconciler) reconcileManagerAuthClusterRoleBinding(ctx context.Context, authorinoInstance *api.Authorino) error {
	// TODO
	return nil
}

func (r *AuthorinoReconciler) reconcileLeaderElectionRole(ctx context.Context, authorinoInstance *api.Authorino) error {
	// TODO
	return nil
}

func (r *AuthorinoReconciler) reconcileLeaderElectionRoleBinding(ctx context.Context, authorinoInstance *api.Authorino) error {
	// TODO
	return nil
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

func (r *AuthorinoReconciler) ReconcileAuthorinoPermissionsOLD(ctx context.Context, authorinoInstance *api.Authorino) error {
	sa := authorinoResources.GetAuthorinoServiceAccount(authorinoInstance.Namespace, authorinoInstance.Name, authorinoInstance.Labels)

	roleBindingMutators := []RoleBindingMutateFn{
		RoleBindingLabelsMutator,
		RoleBindingSubjectMutator,
	}

	clusterRoleBindingMutators := []ClusterRoleBindingMutateFn{
		ClusterRoleBindingLabelsMutator,
		ClusterRoleBindingSubjectMutator,
	}

	// creates the manager ClusterRoleBinding/RoleBinding depending on type of installation
	rb, err := r.bindAuthorinoServiceAccountToClusterRole(AuthorinoManagerClusterRoleBindingName, authorinoInstance.Spec.ClusterWide, AuthorinoManagerClusterRoleName, sa, authorinoInstance)
	if err != nil {
		return err
	}

	switch binding := rb.(type) {
	case *k8srbac.RoleBinding:
		err := r.reconcileRoleBinding(ctx, binding, RoleBindingMutator(roleBindingMutators...), authorinoInstance)
		if err != nil {
			return fmt.Errorf("failed to reconcile %s RoleBinding resource, err: %v", authorinoInstance.Name, err)
		}
	case *k8srbac.ClusterRoleBinding:
		err := r.reconcileClusterRoleBinding(ctx, binding, ClusterRoleBindingMutator(clusterRoleBindingMutators...), authorinoInstance)
		if err != nil {
			return fmt.Errorf("failed to reconcile %s ClusterRoleBinding resource, err: %v", authorinoInstance.Name, err)
		}
	}

	// creates the K8s Auth ClusterRoleBinding (for Authorino's Kubernetes TokenReview and SubjectAccessReview features)
	// Disclaimer: this has nothing to do with kube-rbac-proxy, but to authn/authz features of Authorino that also require cluster scope role bindings
	rb, err = r.bindAuthorinoServiceAccountToClusterRole(AuthorinoK8sAuthClusterRoleBindingName, true, AuthorinoK8sAuthClusterRoleName, sa, authorinoInstance)
	if err != nil {
		return err
	}

	if roleBinding, ok := rb.(*k8srbac.ClusterRoleBinding); ok {
		err := r.reconcileClusterRoleBinding(ctx, roleBinding, RoleBindingMutator(roleBindingMutators...), authorinoInstance)
		if err != nil {
			return fmt.Errorf("failed to reconcile %s ClusterRoleBinding resource, err: %v", authorinoInstance.Name, err)
		}
	}
	// creates leader election role (for the replicas of the Auhtorino instance to choose the one replica responsible for updating the status of the reconciled AuthConfig CRs)
	return r.bindAuthorinoServiceAccountToLeaderElectionRole(authorinoInstance, sa)
}

func (r *AuthorinoReconciler) bindAuthorinoServiceAccountToClusterRole(roleBindingName string, clusterScoped bool, clusterRoleName string, serviceAccount *k8score.ServiceAccount, authorino *api.Authorino) (client.Object, error) {
	var ctx = context.TODO()
	var logger = r.Log

	// check if clusterrole exists
	clusterRole := &k8srbac.ClusterRole{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: clusterRoleName}, clusterRole); err != nil {
		if errors.IsNotFound(err) {
			return nil, r.WrapErrorWithStatusUpdate(logger, authorino, r.SetStatusFailed(statusClusterRoleNotFound), fmt.Errorf("failed to find authorino ClusterRole %s: %v", clusterRoleName, err))
		} else {
			return nil, r.WrapErrorWithStatusUpdate(logger, authorino, r.SetStatusFailed(statusUnableToGetClusterRole), fmt.Errorf("failed to get authorino ClusterRole %s: %v", clusterRoleName, err))
		}
	}

	var roleBinding client.Object

	if clusterScoped {
		roleBinding = authorinoResources.GetAuthorinoClusterRoleBinding(roleBindingName, clusterRoleName, serviceAccount)
	} else {
		roleBinding = authorinoResources.GetAuthorinoRoleBinding(authorino.Namespace, authorino.Name, roleBindingName, "ClusterRole", clusterRoleName, serviceAccount, authorino.Labels)
		roleBinding.SetNamespace(authorino.Namespace)
	}

	roleBinding = authorinoResources.AppendSubjectToRoleBinding(roleBinding, serviceAccount)

	return roleBinding, nil
}

func (r *AuthorinoReconciler) bindAuthorinoServiceAccountToLeaderElectionRole(authorino *api.Authorino, serviceAccount *k8score.ServiceAccount) error {
	var logger = r.Log

	leaderElectionRole := &k8srbac.Role{}
	leaderElectionNsdName := namespacedName(authorino.Namespace, AuthorinoLeaderElectionRoleName)
	if err := r.Get(context.TODO(), leaderElectionNsdName, leaderElectionRole); err != nil {
		if errors.IsNotFound(err) {
			// leader election Role doesn't exist then create
			leaderElectionRole.Name = AuthorinoLeaderElectionRoleName
			leaderElectionRole.Namespace = authorino.Namespace
			leaderElectionRole.Rules = authorinoResources.GetLeaderElectionRules()
			_ = ctrl.SetControllerReference(authorino, leaderElectionRole, r.Scheme)
			if err := r.Client.Create(context.TODO(), leaderElectionRole); err != nil {
				return r.WrapErrorWithStatusUpdate(
					logger, authorino, r.SetStatusFailed(statusUnableToCreateLeaderElectionRole),
					fmt.Errorf("failed to create %s role, err: %v", leaderElectionRole, err),
				)
			}
		}
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToGetLeaderElectionRole),
			fmt.Errorf("failed to get %s Role, err: %v", AuthorinoLeaderElectionRoleName, err),
		)
	}

	leRoleBinding := authorinoResources.GetAuthorinoRoleBinding(
		authorino.Namespace,
		authorino.Name,
		authorinoLeaderElectionRoleBindingName,
		"Role",
		AuthorinoLeaderElectionRoleName,
		serviceAccount,
		authorino.Labels,
	)
	if err := r.Get(context.TODO(), namespacedName(leRoleBinding.Namespace, leRoleBinding.Name), leRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			_ = ctrl.SetControllerReference(authorino, leRoleBinding, r.Scheme)
			// doesn't exist - create one
			if err := r.Client.Create(context.TODO(), leRoleBinding); err != nil {
				return r.WrapErrorWithStatusUpdate(
					logger, authorino, r.SetStatusFailed(statusUnableToCreateLeaderElectionRoleBinding),
					fmt.Errorf("failed to create %s RoleBinding, err: %v", leRoleBinding.Name, err),
				)
			}
		}
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToGetLeaderElectionRoleBinding),
			fmt.Errorf("failed to get %s RoleBinding, err: %v", leRoleBinding.Name, err),
		)
	}
	return nil
}

func (r *AuthorinoReconciler) reconcileResource(ctx context.Context, obj, desired client.Object, mutateFn MutateFn) (string, client.Object, error) {
	key := client.ObjectKeyFromObject(desired)

	if err := r.Client.Get(ctx, key, obj); err != nil {
		if !errors.IsNotFound(err) {
			return "read", nil, err
		}

		// Not found
		if !IsObjectTaggedToDelete(desired) {
			if err = r.Client.Create(ctx, desired); err != nil {
				return "create", nil, err
			}
			return "create", desired, nil
		}

		// Marked for deletion and not found. Nothing to do.
		return "read", nil, nil
	}

	if IsObjectTaggedToDelete(desired) {
		if err := r.Client.Delete(ctx, desired); err != nil {
			return "delete", desired, err
		}
		return "delete", desired, nil
	}

	update, err := mutateFn(desired, obj)
	if err != nil {
		return "", obj, err
	}

	if update {
		if err = r.Client.Update(ctx, obj); err != nil {
			return "update", obj, err
		}
		return "update", obj, nil
	}

	return "", obj, nil
}

func (r *AuthorinoReconciler) reconcileDeployment(ctx context.Context, logger logr.Logger, desired *k8sapps.Deployment, mutatefn MutateFn, authorino *api.Authorino) error {
	crud, obj, err := r.reconcileResource(ctx, &k8sapps.Deployment{}, desired, mutatefn)

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
		if err = r.updateStatusConditions(logger, authorino, statusNotReady(statusUpdated, "Authorino Deployment resource updated")); err != nil {
			return err
		}
		return nil
	}

	deployment, ok := obj.(*k8sapps.Deployment)
	if !ok {
		return fmt.Errorf("failed to cast object to Deployment")
	}

	if !DeploymentAvailable(deployment) {
		if err = r.updateStatusConditions(logger, authorino, statusNotReady(statusDeploymentNotReady, "Authorino Deployment resource not ready")); err != nil {
			return err
		}
		return nil
	}

	if err = r.updateStatusConditions(logger, authorino, statusReady()); err != nil {
		return err
	}
	return nil
}

func (r *AuthorinoReconciler) reconcileService(ctx context.Context, desired *k8score.Service, mutatefn MutateFn, authorino *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	crud, _, err := r.reconcileResource(ctx, &k8score.Service{}, desired, mutatefn)

	if crud == "read" && err != nil {
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToGetServices), fmt.Errorf("failed to get %s service, err: %v", desired.Name, err))
	}

	if crud == "create" && err != nil {
		return r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToGetLeaderElectionRoleBinding),
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

func (r *AuthorinoReconciler) reconcileRoleBinding(ctx context.Context, desired *k8srbac.RoleBinding, mutatefn MutateFn, authorino *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	if err := ctrl.SetControllerReference(authorino, desired, r.Scheme); err != nil {
		return err
	}

	crud, _, err := r.reconcileResource(ctx, &k8srbac.RoleBinding{}, desired, mutatefn)

	if err = r.clusterRoleStatus(logger, authorino, crud, desired.Name, err); err != nil {
		return err
	}

	return nil
}

func (r *AuthorinoReconciler) reconcileClusterRoleBinding(ctx context.Context, desired *k8srbac.ClusterRoleBinding, mutatefn MutateFn, authorino *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	crud, _, err := r.reconcileResource(ctx, &k8srbac.ClusterRoleBinding{}, desired, mutatefn)

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

func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

func (r *AuthorinoReconciler) ReconcileAuthorinoServiceAccount(ctx context.Context, authorino *api.Authorino) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return err
	}

	sa := authorinoResources.GetAuthorinoServiceAccount(authorino.Namespace, authorino.Name, authorino.Labels)

	crud, _, err := r.reconcileResource(ctx, &k8score.ServiceAccount{}, sa, CreateOnlyMutator)
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

// remove SA from list of subjects of the clusterrolebinding
func (r *AuthorinoReconciler) UnboundAuthorinoServiceAccountFromClusterRole(ctx context.Context, roleBindingName string, sa *k8score.ServiceAccount) {
	// TODO: should return error for error handling
	logger, _ := logr.FromContext(ctx)
	roleBinding := &k8srbac.ClusterRoleBinding{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: roleBindingName}, roleBinding); err == nil {
		staleSubject := authorinoResources.GetSubjectForRoleBinding(sa)
		var subjects []k8srbac.Subject
		for _, subject := range roleBinding.Subjects {
			if subject.Kind != staleSubject.Kind || subject.Name != staleSubject.Name || subject.Namespace != staleSubject.Namespace {
				subjects = append(subjects, subject)
			}
		}

		if len(subjects) == 0 {
			if err = r.Client.Delete(ctx, roleBinding); err != nil {
				logger.Error(err, "failed to delete authorino role binding", "roleBinding", roleBinding, "subject", staleSubject)
			}
		} else {
			// FIXME: This is subject to race condition. The list of subjects may be outdated under concurrent updates
			roleBinding.Subjects = subjects
			if err = r.Client.Update(ctx, roleBinding); err != nil {
				logger.Error(err, "failed to cleanup subject from authorino role binding", "roleBinding", roleBinding, "subject", staleSubject)
			}
		}
	}
}
