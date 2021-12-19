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
	"sort"
	"time"

	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	"github.com/kuadrant/authorino-operator/pkg/condition"
	authorinoResources "github.com/kuadrant/authorino-operator/pkg/resources"
)

// AuthorinoReconciler reconciles a Authorino object
type AuthorinoReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

const (
	tlsCertName     string = "tls-cert"
	oidcTlsCertName string = "oidc-cert"

	authorinoClusterRoleName string = "authorino-manager-role"
	leaderElectionRoleName   string = "authorino-leader-election-role"
)

//+kubebuilder:rbac:groups=operator.authorino.kuadrant.io,resources=authorinos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.authorino.kuadrant.io,resources=authorinos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.authorino.kuadrant.io,resources=authorinos/finalizers,verbs=update

// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="*",resources=services,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=clusterroles,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=rolebindings,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=clusterrolebindings,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=serviceaccounts,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=roles,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="*",resources=configmaps/status,verbs=get;update;delete;patch
// +kubebuilder:rbac:groups="*",resources=events,verbs=create;patch;
// +kubebuilder:rbac:groups="*",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="authorino.kuadrant.io",resources=authconfigs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="authorino.kuadrant.io",resources=authconfigs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="authorino.kuadrant.io",resources=authconfigs/status,verbs=get;patch;update
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=get;list;create;update;

// Reconcile deploys an instance of authorino depending on the settings
// defined in the API, any change applied to the existings CRs will trigger
// a new reconcilation to apply the required changes
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

	// Creates services required by authorino
	if err := r.createAuthorinoServices(authorinoInstance); err != nil {
		return ctrl.Result{}, err
	}

	// Creates RBAC permission required by authorino
	if err := r.createAuthorinoPermission(authorinoInstance, req.NamespacedName.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	// Gets Deployment resource for the authorino instance
	if existingDeployment, err := r.getAuthorinoDeployment(authorinoInstance); err != nil {
		return ctrl.Result{}, r.wrapErrorWithStatusUpdate(logger, authorinoInstance, r.setStatusFailed(api.AuthorinoUnableToGetDeployment),
			fmt.Errorf("failed to get %s Deployment resource, err: %v", authorinoInstance.Name, err),
		)
	} else if existingDeployment == nil {
		// Creates a new deployment resource to deploy the new authorino instance
		newDeployment := r.buildAuthorinoDeployment(authorinoInstance)
		if err := r.Client.Create(context.TODO(), newDeployment); err != nil {
			return ctrl.Result{}, r.wrapErrorWithStatusUpdate(
				logger, authorinoInstance, r.setStatusFailed(api.AuthorinoUnableToCreateDeployment),
				fmt.Errorf("failed to create %s Deployment resource, err: %v", newDeployment.Name, err),
			)
		}
		// Updates the status conditions to provisioning
		if err := updateStatusConditions(logger, authorinoInstance, r.Client, statusNotReady(api.AuthorinoProvisioningReason, "")); err != nil {
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else {

		// deployment already exists, then build a new resource with the desired changes
		// and compare them, if changes are encountered apply the desired changes
		desiredDeployment := r.buildAuthorinoDeployment(authorinoInstance)
		logger.Info("desiredDeployment", "deployment", desiredDeployment)
		logger.Info("existingDeployment", "deployment", existingDeployment)
		if changed := r.authorinoDeploymentChanges(existingDeployment, desiredDeployment); changed {
			if err := r.Update(ctx, desiredDeployment); err != nil {
				return ctrl.Result{}, r.wrapErrorWithStatusUpdate(
					logger, authorinoInstance, r.setStatusFailed(api.AuthorinoUnableToUpdateDeployment),
					fmt.Errorf("failed to update %s Deployment resource, err: %v", desiredDeployment.Name, err),
				)
			}

			err = updateStatusConditions(logger, authorinoInstance,
				r.Client, statusNotReady(api.AuthorinoUpdatedReason, "Authorino Deployment resource updated"))
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
	}

	// Updates the status conditions to provisioned
	return ctrl.Result{}, updateStatusConditions(logger, authorinoInstance, r.Client, statusReady())
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuthorinoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.Authorino{}).
		Complete(r)
}

func (r *AuthorinoReconciler) getAuthorinoInstance(namespacedName types.NamespacedName) (*api.Authorino, error) {
	authorinoInstance := &api.Authorino{}
	err := r.Get(context.TODO(), namespacedName, authorinoInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("Authorino CR not found.")
			return nil, nil
		}
		return nil, err
	}
	return authorinoInstance, nil
}

func (r *AuthorinoReconciler) getAuthorinoDeployment(authorino *api.Authorino) (*k8sapps.Deployment, error) {
	deployment := &k8sapps.Deployment{}
	namespacedName := namespacedName(authorino.Namespace, authorino.Name)
	if err := r.Get(context.TODO(), namespacedName, deployment); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return deployment, nil
}

func (r *AuthorinoReconciler) buildAuthorinoDeployment(authorino *api.Authorino) *k8sapps.Deployment {
	var containers []k8score.Container
	var saName = authorino.Name + "-authorino"

	if authorino.Spec.Image == "" {
		authorino.Spec.Image = fmt.Sprintf("quay.io/3scale/authorino:%s", api.AuthorinoVersion)
	}

	volumeMounts := authorino.Spec.VolumeMounts
	volumes := authorino.Spec.Volumes

	// if an external auth server is enabled mounts a volume to the container
	// by using the secret with the cert
	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		secretName := authorino.Spec.Listener.Tls.CertSecret.Name
		volumeMounts = append(volumeMounts, authorinoResources.GetTlsVolumeMount(tlsCertName, api.DefaultTlsCertPath, api.DefaultTlsCertKeyPath)...)
		volumes = append(volumes, authorinoResources.GetTlsVolume(tlsCertName, secretName))
	}

	// if an external OIDC server is enable mounts a volume to the container
	// by using the secret with the certs
	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		secretName := authorino.Spec.OIDCServer.Tls.CertSecret.Name
		volumeMounts = append(volumeMounts, authorinoResources.GetTlsVolumeMount(oidcTlsCertName, api.DefaultOidcTlsCertPath, api.DefaultOidcTlsCertKeyPath)...)
		volumes = append(volumes, authorinoResources.GetTlsVolume(oidcTlsCertName, secretName))
	}

	// generates the env variables
	envs := r.buildAuthorinoEnv(authorino)

	// generates the Container where authorino will be running
	// adds to the list of containers available in the deployment
	authorinoContainer := authorinoResources.GetContainer(authorino.Spec.Image, authorino.Spec.ImagePullPolicy, api.AuthorinoContainerName, envs, volumeMounts)
	containers = append(containers, authorinoContainer)

	// generate Deployment resource to deploy an authorino instance
	deployment := authorinoResources.GetDeployment(authorino.Name, authorino.Namespace, saName, authorino.Spec.Replicas, containers, volumes)

	ctrl.SetControllerReference(authorino, deployment, r.Scheme)
	return deployment
}

func (r *AuthorinoReconciler) buildAuthorinoEnv(authorino *api.Authorino) []k8score.EnvVar {
	envVar := []k8score.EnvVar{}

	if !authorino.Spec.ClusterWide {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.WatchNamespace,
			Value: authorino.GetNamespace(),
		})
	}

	if authorino.Spec.AuthConfigLabelSelectors != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.AuthConfigLabelSelector,
			Value: fmt.Sprint(authorino.Spec.AuthConfigLabelSelectors),
		})
	}

	if authorino.Spec.LogLevel != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvLogLevel,
			Value: fmt.Sprint(authorino.Spec.LogLevel),
		})
	}

	if authorino.Spec.LogMode != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvLogMode,
			Value: fmt.Sprint(authorino.Spec.LogMode),
		})
	}

	if authorino.Spec.SecretLabelSelectors != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.SecretLabelSelector,
			Value: fmt.Sprint(authorino.Spec.SecretLabelSelectors),
		})
	}

	// external auth service via GRPC
	if authorino.Spec.Listener.Port != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.ExtAuthGRPCPort,
			Value: fmt.Sprintf("%v", *authorino.Spec.Listener.Port),
		})
	}

	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvVarTlsCert,
			Value: api.DefaultTlsCertPath,
		})

		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvVarTlsCertKey,
			Value: api.DefaultTlsCertKeyPath,
		})
	}

	// OIDC service
	if authorino.Spec.OIDCServer.Port != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.OIDCHTTPPort,
			Value: fmt.Sprintf("%v", *authorino.Spec.OIDCServer.Port),
		})
	}
	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvVarOidcTlsCertPath,
			Value: api.DefaultOidcTlsCertPath,
		})
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvVarOidcTlsCertKeyPath,
			Value: api.DefaultOidcTlsCertKeyPath,
		})
	}

	return envVar
}

func (r *AuthorinoReconciler) authorinoDeploymentChanges(existingDeployment, desiredDeployment *k8sapps.Deployment) bool {
	if *existingDeployment.Spec.Replicas != *desiredDeployment.Spec.Replicas {
		return true
	}

	if len(desiredDeployment.Spec.Template.Spec.Containers) != 1 {
		// error
	}

	existingContainer := existingDeployment.Spec.Template.Spec.Containers[0]
	desiredContainer := desiredDeployment.Spec.Template.Spec.Containers[0]

	if existingContainer.Image != desiredContainer.Image {
		return true
	}

	if existingContainer.ImagePullPolicy != desiredContainer.ImagePullPolicy {
		return true
	}

	// checking envvars
	existingEnvvars := existingContainer.Env
	desiredEnvvars := desiredContainer.Env
	for _, desiredEnvvar := range desiredEnvvars {
		for _, existingEnvvar := range existingEnvvars {
			if existingEnvvar.Name == desiredEnvvar.Name && existingEnvvar.Value != desiredEnvvar.Value {
				return true
			}
		}
	}

	// checking volumes
	existingVolumes := existingDeployment.Spec.Template.Spec.Volumes
	desiredVolumes := desiredDeployment.Spec.Template.Spec.Volumes

	if len(existingVolumes) != len(desiredVolumes) {
		return true
	}

	sort.Slice(existingVolumes, func(i, j int) bool {
		return existingVolumes[i].Name < existingVolumes[j].Name
	})

	sort.Slice(desiredVolumes, func(i, j int) bool {
		return desiredVolumes[i].Name < desiredVolumes[j].Name
	})

	for i, desiredVolume := range desiredVolumes {
		if existingVolumes[i].Name != desiredVolume.Name { // comparing only the names has limitation, but more reliable than using reflect.DeepEqual or comparing the marshalled version of the resources
			return true
		}
	}

	// checking volumeMounts
	existingVolumeMounts := existingContainer.VolumeMounts
	desiredVolumeMounts := desiredContainer.VolumeMounts

	if len(existingVolumeMounts) != len(desiredVolumeMounts) {
		return true
	}

	sort.Slice(existingVolumeMounts, func(i, j int) bool {
		return existingVolumeMounts[i].Name < existingVolumeMounts[j].Name
	})

	sort.Slice(desiredVolumeMounts, func(i, j int) bool {
		return desiredVolumeMounts[i].Name < desiredVolumeMounts[j].Name
	})

	for i, desiredVolumeMount := range desiredVolumeMounts {
		if existingVolumeMounts[i].Name != desiredVolumeMount.Name { // comparing only the names has limitation, but more reliable than using reflect.DeepEqual or comparing the marshalled version of the resources
			return true
		}
	}

	return false
}

func (r *AuthorinoReconciler) createAuthorinoPermission(authorino *api.Authorino, operatorNamespace string) error {
	var logger = r.Log
	resourcePrefixName := authorino.Name

	clNsdName := namespacedName(authorino.Namespace, authorinoClusterRoleName)
	authorinoClusterRole := &k8srbac.ClusterRole{}
	if err := r.Get(context.TODO(), clNsdName, authorinoClusterRole); err != nil {
		if errors.IsNotFound(err) {
			// authorino ClusterRole has not being created
			return r.wrapErrorWithStatusUpdate(logger, authorino, r.setStatusFailed(api.AuthorinoClusterRoleNotFound),
				fmt.Errorf("failed to find authorino ClusterRole %v", err),
			)
		}
		return r.wrapErrorWithStatusUpdate(logger, authorino, r.setStatusFailed(api.AuthorinoUnableToGetClusterRole),
			fmt.Errorf("failed to get authorino ClusterRole %v", err),
		)
	}

	// get ServiceAccount from authorino instance namespace
	sa := &k8score.ServiceAccount{}
	saName := resourcePrefixName + "-authorino"
	saNsdName := namespacedName(authorino.Namespace, saName)
	if err := r.Get(context.TODO(), saNsdName, sa); err != nil {
		if errors.IsNotFound(err) {
			// ServiceAccount doesn't exit - create one
			ctrl.SetControllerReference(authorino, sa, r.Scheme)
			sa.Name = saName
			sa.Namespace = authorino.Namespace
			if err := r.Client.Create(context.TODO(), sa); err != nil {
				return r.wrapErrorWithStatusUpdate(
					logger, authorino, r.setStatusFailed(api.AuthorinoUnableToCreateServiceAccount),
					fmt.Errorf("failed to create %s ServiceAccount, err: %v", saName, err),
				)
			}
		}
		return r.wrapErrorWithStatusUpdate(
			logger, authorino, r.setStatusFailed(api.AuthorinoUnableToGetServiceAccount),
			fmt.Errorf("failed to get %s ServiceAccount, err: %v", saName, err),
		)
	}

	// creates the ClusterRoleBinding/RoleBinding depending on type of installation
	// if ClusterWide true - ClusterRoleBinding
	// if ClusterWide false - RoleBinding
	var binding client.Object = authorinoResources.GetAuthorinoClusterRoleBinding(authorinoClusterRoleName, sa.Name, sa.Namespace)
	if !authorino.Spec.ClusterWide {
		binding = authorinoResources.GetAuthorinoRoleBinding(authorinoClusterRoleName, sa.Name, sa.Namespace)
		binding.SetNamespace(authorino.Namespace)
	}
	bindingName := resourcePrefixName + "-authorino"
	bindingNsdName := namespacedName(authorino.Namespace, bindingName)
	if err := r.Get(context.TODO(), bindingNsdName, binding); err != nil {
		if errors.IsNotFound(err) {
			ctrl.SetControllerReference(authorino, binding, r.Scheme)
			// doesn't exist - create one
			binding.SetName(bindingName)
			if err := r.Client.Create(context.TODO(), binding); err != nil {
				return r.wrapErrorWithStatusUpdate(
					logger, authorino, r.setStatusFailed(api.AuthorinoUnableToCreateBindingForClusterRole),
					fmt.Errorf("failed to create %s binding for authorino ClusterRole, err: %v", bindingName, err),
				)
			}
		}
		return r.wrapErrorWithStatusUpdate(
			logger, authorino, r.setStatusFailed(api.AuthorinoUnableToGetBindingForClusterRole),
			fmt.Errorf("failed to get %s binding for authorino ClusterRole, err: %v", bindingName, err),
		)
	}

	// creates leader election role
	// TODO: describe what this leader election is
	return r.leaderElectionPermission(authorino, sa.GetName())
}

func (r *AuthorinoReconciler) leaderElectionPermission(authorino *api.Authorino, saName string) error {
	var logger = r.Log

	leaderElectionRole := &k8srbac.Role{}
	leaderElectionNsdName := namespacedName(authorino.Namespace, leaderElectionRoleName)
	if err := r.Get(context.TODO(), leaderElectionNsdName, leaderElectionRole); err != nil {
		if errors.IsNotFound(err) {
			// leader election Role doesn't exist then create
			ctrl.SetControllerReference(authorino, leaderElectionRole, r.Scheme)
			leaderElectionRole.Name = leaderElectionRoleName
			leaderElectionRole.Namespace = authorino.Namespace
			leaderElectionRole.Rules = authorinoResources.GetLeaderElectionRules()
			if err := r.Client.Create(context.TODO(), leaderElectionRole); err != nil {
				return r.wrapErrorWithStatusUpdate(
					logger, authorino, r.setStatusFailed(api.AuthorinoUnableToCreateLeaderElectionRole),
					fmt.Errorf("failed to create %s role, err: %v", leaderElectionRole, err),
				)
			}
		}
		return r.wrapErrorWithStatusUpdate(
			logger, authorino, r.setStatusFailed(api.AuthorinoUnableToGetLeaderElectionRole),
			fmt.Errorf("failed to get %s Role, err: %v", leaderElectionRoleName, err),
		)
	}

	leRoleBindingName := authorino.Name + "-authorino-leader-election"
	leRoleBinding := authorinoResources.GetAuthorinoLeaderElectionRoleBinding(leaderElectionRoleName, saName, authorino.Namespace)
	bindingNsdName := namespacedName(authorino.Namespace, leRoleBindingName)
	if err := r.Get(context.TODO(), bindingNsdName, leRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			ctrl.SetControllerReference(authorino, leRoleBinding, r.Scheme)
			leRoleBinding.Name = leRoleBindingName
			leRoleBinding.Namespace = authorino.Namespace
			// doesn't exist - create one
			if err := r.Client.Create(context.TODO(), leRoleBinding); err != nil {
				return r.wrapErrorWithStatusUpdate(
					logger, authorino, r.setStatusFailed(api.AuthorinoUnableToCreateLeaderElectionRoleBinding),
					fmt.Errorf("failed to create %s RoleBinding, err: %v", leRoleBindingName, err),
				)
			}
		}
		return r.wrapErrorWithStatusUpdate(
			logger, authorino, r.setStatusFailed(api.AuthorinoUnableToGetLeaderElectionRoleBinding),
			fmt.Errorf("failed to get %s RoleBinding, err: %v", leRoleBindingName, err),
		)
	}
	return nil
}

func (r *AuthorinoReconciler) createAuthorinoServices(authorino *api.Authorino) error {
	logger := r.Log

	var services = authorinoResources.GetAuthorinoServices(
		authorino.Name,
		authorino.Namespace,
	)

	for _, service := range services {
		// get services from an authorino instance
		nsdName := namespacedName(service.Namespace, service.Name)
		if err := r.Client.Get(context.TODO(), nsdName, service); err != nil {
			if errors.IsNotFound(err) {
				// service doesn't exist then create
				ctrl.SetControllerReference(authorino, service, r.Scheme)
				if err := r.Client.Create(context.TODO(), service); err != nil {
					return r.wrapErrorWithStatusUpdate(
						logger, authorino, r.setStatusFailed(api.AuthorinoUnableToGetLeaderElectionRoleBinding),
						fmt.Errorf("failed to create %s service, err: %v", service.Name, err),
					)
				}
			}
			return r.wrapErrorWithStatusUpdate(
				logger, authorino, r.setStatusFailed(api.AuthorinoUnableToGetServices),
				fmt.Errorf("failed to get %s service, err: %v", service.Name, err),
			)
		}
	}
	return nil
}

type statusUpdater func(logger logr.Logger, authorino *api.Authorino, message string) error

// wrapErrorWithStatusUpdate wraps the error and update the status. If the update failed then logs the error.
func (r *AuthorinoReconciler) wrapErrorWithStatusUpdate(logger logr.Logger, authorino *api.Authorino, updateStatus statusUpdater, err error) error {
	if err == nil {
		return nil
	}
	if err := updateStatus(logger, authorino, err.Error()); err != nil {
		logger.Error(err, "status update failed")
	}
	return err
}

func (r *AuthorinoReconciler) setStatusFailed(reason string) statusUpdater {
	return func(logger logr.Logger, authorino *api.Authorino, message string) error {
		return updateStatusConditions(
			logger,
			authorino,
			r.Client,
			statusNotReady(reason, message),
		)
	}
}

func updateStatusConditions(logger logr.Logger, authorino *api.Authorino, client client.Client, newConditions ...api.Condition) error {
	var updated bool
	authorino.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(authorino.Status.Conditions, newConditions...)
	if !updated {
		logger.Info("Authorino status conditions not changed")
		return nil
	}
	return client.Status().Update(context.TODO(), authorino)
}

func statusReady() api.Condition {
	return api.Condition{
		Type:   api.ConditionReady,
		Status: k8score.ConditionTrue,
		Reason: api.AuthorinoProvisionedReason,
	}
}

func statusNotReady(reason, message string) api.Condition {
	return api.Condition{
		Type:    api.ConditionReady,
		Status:  k8score.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}

func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}
