/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	"github.com/kuadrant/authorino-operator/pkg/reconcilers"
	authorinoResources "github.com/kuadrant/authorino-operator/pkg/resources"
)

// AuthorinoReconciler reconciles a Authorino object
type AuthorinoReconciler struct {
	*reconcilers.AuthorinoReconciler
}

//+kubebuilder:rbac:groups=operator.authorino.kuadrant.io,resources=authorinos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.authorino.kuadrant.io,resources=authorinos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.authorino.kuadrant.io,resources=authorinos/finalizers,verbs=update

// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="authentication.k8s.io",resources=tokenreviews,verbs=create;
// +kubebuilder:rbac:groups="authorization.k8s.io",resources=subjectaccessreviews,verbs=create;
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;delete;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="authorino.kuadrant.io",resources=authconfigs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="authorino.kuadrant.io",resources=authconfigs/status,verbs=get;patch;update
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=get;list;create;update;

// Reconcile deploys an instance of authorino depending on the settings
// defined in the API, any change applied to the existing CRs will trigger
// a new reconciliation to apply the required changes
func (r *AuthorinoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("authorino", req.NamespacedName)

	// Retrieve Authorino instance
	authorinoInstance, err := r.getAuthorinoInstance(req.NamespacedName)
	if err != nil {
		logger.Error(err, "Unable to get Authorino CR")
		return ctrl.Result{}, err
	}

	// If the Authorino instance is not found, returns the reconcile request.
	if authorinoInstance == nil {
		logger.Info("Authorino instance not found. returning the reconciler")
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Found an instance of authorino", "authorinoInstanceName", authorinoInstance.Name)

	if err := r.installationPreflightCheck(authorinoInstance); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	if err := r.ReconcileAuthorinoServices(ctx, logger, authorinoInstance); err != nil {
		return ctrl.Result{}, err
	}

	sa, err := r.createAuthorinoServiceAccount(authorinoInstance)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ReconcileAuthorinoPermissions(ctx, logger, authorinoInstance, *sa); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ReconcileAuthorinoDeployment(ctx, logger, authorinoInstance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuthorinoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Owns(&k8sapps.Deployment{}).
		For(&api.Authorino{}).
		Complete(r)
}

func (r *AuthorinoReconciler) getAuthorinoInstance(namespacedName types.NamespacedName) (*api.Authorino, error) {
	authorinoInstance := &api.Authorino{}
	err := r.Get(context.TODO(), namespacedName, authorinoInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("Authorino CR not found.")
			r.cleanupClusterScopedPermissions(context.Background(), namespacedName, authorinoInstance.Labels)
			return nil, nil
		}
		return nil, err
	}
	return authorinoInstance, nil
}

func (r *AuthorinoReconciler) createAuthorinoServiceAccount(authorino *api.Authorino) (*k8score.ServiceAccount, error) {
	var logger = r.Log
	sa := authorinoResources.GetAuthorinoServiceAccount(authorino.Namespace, authorino.Name, authorino.Labels)
	if err := r.Get(context.TODO(), namespacedName(sa.Namespace, sa.Name), sa); err != nil {
		if errors.IsNotFound(err) {
			// ServiceAccount doesn't exit - create one
			_ = ctrl.SetControllerReference(authorino, sa, r.Scheme)
			if err := r.Client.Create(context.TODO(), sa); err != nil {
				return nil, r.WrapErrorWithStatusUpdate(
					logger, authorino, r.SetStatusFailed(statusUnableToCreateServiceAccount),
					fmt.Errorf("failed to create %s ServiceAccount, err: %v", sa.Name, err),
				)
			}
		}
		return nil, r.WrapErrorWithStatusUpdate(
			logger, authorino, r.SetStatusFailed(statusUnableToGetServiceAccount),
			fmt.Errorf("failed to get %s ServiceAccount, err: %v", sa.Name, err),
		)
	}
	// ServiceAccount exists
	return sa, nil
}

func (r *AuthorinoReconciler) cleanupClusterScopedPermissions(ctx context.Context, crNamespacedName types.NamespacedName, labels map[string]string) {
	crName := crNamespacedName.Name
	sa := authorinoResources.GetAuthorinoServiceAccount(crNamespacedName.Namespace, crName, labels)

	// we only care about cluster-scoped role bindings for the cleanup
	// namespaced ones are garbage collected automatically by k8s because of the owner reference
	r.unboundAuthorinoServiceAccountFromClusterRole(ctx, authorinoManagerClusterRoleBindingName, sa)
	r.unboundAuthorinoServiceAccountFromClusterRole(ctx, authorinoK8sAuthClusterRoleBindingName, sa)
}

// remove SA from list of subjects of the clusterrolebinding
func (r *AuthorinoReconciler) unboundAuthorinoServiceAccountFromClusterRole(ctx context.Context, roleBindingName string, sa *k8score.ServiceAccount) {
	var logger = r.Log
	roleBinding := &k8srbac.ClusterRoleBinding{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: roleBindingName}, roleBinding); err == nil {
		staleSubject := authorinoResources.GetSubjectForRoleBinding(*sa)
		var subjects []k8srbac.Subject
		for _, subject := range roleBinding.Subjects {
			if subject.Kind != staleSubject.Kind || subject.Name != staleSubject.Name || subject.Namespace != staleSubject.Namespace {
				subjects = append(subjects, subject)
			}
		}
		// FIXME: This is subject to race condition. The list of subjects may be outdated under concurrent updates
		roleBinding.Subjects = subjects
		if err = r.Client.Update(ctx, roleBinding); err != nil {
			logger.Error(err, "failed to cleanup subject from authorino role binding", "roleBinding", roleBinding, "subject", staleSubject)
		}
	}
}

func (r *AuthorinoReconciler) installationPreflightCheck(authorino *api.Authorino) error {

	// When tls is enabled, checks if the secret with the certs exists
	// if not, installation of the authorino instance won't progress until the
	// secret is created
	tlsCerts := map[string]api.Tls{
		"listener": authorino.Spec.Listener.Tls,
		"oidc":     authorino.Spec.OIDCServer.Tls,
	}

	for authServerName, tlsCert := range tlsCerts {
		tlsEnabled := tlsCert.Enabled == nil || *tlsCert.Enabled
		if tlsEnabled {
			if tlsCert.CertSecret == nil {
				return r.WrapErrorWithStatusUpdate(
					r.Log, authorino, r.SetStatusFailed(statusTlsSecretNotProvided),
					fmt.Errorf("%s secret with tls cert not provided", authServerName),
				)
			}

			secretName := tlsCert.CertSecret.Name
			nsdName := namespacedName(authorino.Namespace, secretName)
			if err := r.Get(context.TODO(), nsdName, &k8score.Secret{}); err != nil {
				errorMessage := fmt.Errorf("failed to get %s secret name %s , err: %v",
					authServerName, secretName, err)
				if errors.IsNotFound(err) {
					errorMessage = fmt.Errorf("%s secret name %s not found, err: %v",
						authServerName, secretName, err)
				}
				return r.WrapErrorWithStatusUpdate(
					r.Log, authorino, r.SetStatusFailed(statusTlsSecretNotProvided),
					errorMessage,
				)
			}
		}
	}
	return nil
}

func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

// Detects possible old Authorino version (<= v0.10.x) configurable with deprecated environemnt variables (instead of command-line args)
func detectEnvVarAuthorinoVersion(version string) bool {
	if match, err := regexp.MatchString(`v0\.(\d)+\..+`, version); err != nil || !match {
		return false
	}

	parts := strings.Split(version, ".")
	minor, err := strconv.Atoi(parts[1])
	return err == nil && minor <= 10
}
