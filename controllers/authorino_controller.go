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

	"github.com/go-logr/logr"
	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
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
func (r *AuthorinoReconciler) Reconcile(eventCtx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("authorino", req.NamespacedName)
	logger.V(1).Info("Reconciling authorino")
	ctx := logr.NewContext(eventCtx, logger)

	// Fetch the instance
	authorinoInstance := &api.Authorino{}
	err := r.Get(ctx, req.NamespacedName, authorinoInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			logger.Info("resource not found. Ignoring since object must have been deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Unable to get Authorino CR")
		return ctrl.Result{}, err
	}

	// authorino has been marked for deletion
	if authorinoInstance.GetDeletionTimestamp() != nil && controllerutil.ContainsFinalizer(authorinoInstance, authorinoFinalizer) {
		r.cleanupClusterScopedPermissions(ctx, req.NamespacedName, authorinoInstance.Labels)

		controllerutil.RemoveFinalizer(authorinoInstance, authorinoFinalizer)
		err = r.Client.Update(ctx, authorinoInstance)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Ignore deleted resource, this can happen when foregroundDeletion is enabled
	// https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/#foreground-cascading-deletion
	if authorinoInstance.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(authorinoInstance, authorinoFinalizer) {
		controllerutil.AddFinalizer(authorinoInstance, authorinoFinalizer)
		err = r.Client.Update(ctx, authorinoInstance)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if err := r.installationPreflightCheck(authorinoInstance); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	if err := r.ReconcileAuthorinoServices(ctx, authorinoInstance); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ReconcileAuthorinoServiceAccount(ctx, authorinoInstance); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ReconcileAuthorinoPermissions(ctx, authorinoInstance); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ReconcileAuthorinoDeployment(ctx, authorinoInstance); err != nil {
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

// TODO: this method should return error
func (r *AuthorinoReconciler) cleanupClusterScopedPermissions(ctx context.Context, crNamespacedName types.NamespacedName, labels map[string]string) {
	crName := crNamespacedName.Name
	sa := authorinoResources.GetAuthorinoServiceAccount(crNamespacedName.Namespace, crName, labels)

	// we only care about cluster-scoped role bindings for the cleanup
	// namespaced ones are garbage collected automatically by k8s because of the owner reference
	r.UnboundAuthorinoServiceAccountFromClusterRole(ctx, reconcilers.AuthorinoManagerClusterRoleBindingName, sa)
	r.UnboundAuthorinoServiceAccountFromClusterRole(ctx, reconcilers.AuthorinoK8sAuthClusterRoleBindingName, sa)
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
