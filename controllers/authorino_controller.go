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
	"sort"
	"strconv"
	"strings"
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

	if err := r.installationPreflightCheck(authorinoInstance); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

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
		return ctrl.Result{}, r.wrapErrorWithStatusUpdate(logger, authorinoInstance, r.setStatusFailed(statusUnableToGetDeployment),
			fmt.Errorf("failed to get %s Deployment resource, err: %v", authorinoInstance.Name, err),
		)
	} else if existingDeployment == nil {
		// Creates a new deployment resource to deploy the new authorino instance
		newDeployment := r.buildAuthorinoDeployment(authorinoInstance)
		if err := r.Client.Create(context.TODO(), newDeployment); err != nil {
			return ctrl.Result{}, r.wrapErrorWithStatusUpdate(
				logger, authorinoInstance, r.setStatusFailed(statusUnableToCreateDeployment),
				fmt.Errorf("failed to create %s Deployment resource, err: %v", newDeployment.Name, err),
			)
		}
		// Updates the status conditions to provisioning
		if err := updateStatusConditions(logger, authorinoInstance, r.Client, statusNotReady(statusProvisioning, "")); err != nil {
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
					logger, authorinoInstance, r.setStatusFailed(statusUnableToUpdateDeployment),
					fmt.Errorf("failed to update %s Deployment resource, err: %v", desiredDeployment.Name, err),
				)
			}

			err = updateStatusConditions(logger, authorinoInstance, r.Client, statusNotReady(statusUpdated, "Authorino Deployment resource updated"))
			return ctrl.Result{RequeueAfter: time.Second}, err
		}

		if !deploymentAvailable(existingDeployment) {
			// Deployment not ready – return and requeue
			err = updateStatusConditions(logger, authorinoInstance, r.Client, statusNotReady(statusDeploymentNotReady, "Authorino Deployment resource not ready"))
			return ctrl.Result{RequeueAfter: time.Second}, err
		}
	}

	// Updates the status conditions to provisioned
	return ctrl.Result{}, updateStatusConditions(logger, authorinoInstance, r.Client, statusReady())
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
		authorino.Spec.Image = DefaultAuthorinoImage
	}

	var volumes []k8score.Volume
	var volumeMounts []k8score.VolumeMount

	for _, volume := range authorino.Spec.Volumes.Items {
		var sources []k8score.VolumeProjection

		if volume.ConfigMaps != nil {
			for _, name := range volume.ConfigMaps {
				sources = append(sources, k8score.VolumeProjection{
					ConfigMap: &k8score.ConfigMapProjection{
						LocalObjectReference: k8score.LocalObjectReference{
							Name: name,
						},
						Items: volume.Items,
					},
				})
			}
		}

		if volume.Secrets != nil {
			for _, name := range volume.Secrets {
				sources = append(sources, k8score.VolumeProjection{
					Secret: &k8score.SecretProjection{
						LocalObjectReference: k8score.LocalObjectReference{
							Name: name,
						},
						Items: volume.Items,
					},
				})
			}
		}

		volumes = append(volumes, k8score.Volume{
			Name: volume.Name,
			VolumeSource: k8score.VolumeSource{
				Projected: &k8score.ProjectedVolumeSource{
					Sources:     sources,
					DefaultMode: authorino.Spec.Volumes.DefaultMode,
				},
			},
		})

		volumeMounts = append(volumeMounts, k8score.VolumeMount{
			Name:      volume.Name,
			MountPath: volume.MountPath,
		})
	}

	// if an external auth server is enabled mounts a volume to the container
	// by using the secret with the cert
	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		secretName := authorino.Spec.Listener.Tls.CertSecret.Name
		volumeMounts = append(volumeMounts, authorinoResources.GetTlsVolumeMount(authorinoTlsCertVolumeName, defaultTlsCertPath, defaultTlsCertKeyPath)...)
		volumes = append(volumes, authorinoResources.GetTlsVolume(authorinoTlsCertVolumeName, secretName))
	}

	// if an external OIDC server is enable mounts a volume to the container
	// by using the secret with the certs
	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		secretName := authorino.Spec.OIDCServer.Tls.CertSecret.Name
		volumeMounts = append(volumeMounts, authorinoResources.GetTlsVolumeMount(authorinoOidcTlsCertVolumeName, defaultOidcTlsCertPath, defaultOidcTlsCertKeyPath)...)
		volumes = append(volumes, authorinoResources.GetTlsVolume(authorinoOidcTlsCertVolumeName, secretName))
	}

	image := authorino.Spec.Image

	args := r.buildAuthorinoArgs(authorino)
	var envs []k8score.EnvVar

	// [DEPRECATED] configure authorino using env vars (only for old Authorino versions)
	authorinoVersion := authorinoVersionFromImageTag(image)
	if detectEnvVarAuthorinoVersion(authorinoVersion) {
		envs = r.buildAuthorinoEnv(authorino)

		var compatibleArgs []string
		for _, arg := range args {
			parts := strings.Split(strings.TrimPrefix(arg, "--"), "=")
			switch parts[0] {
			case flagMetricsAddr, flagEnableLeaderElection:
				compatibleArgs = append(compatibleArgs, arg)
			}
		}
		args = compatibleArgs
	}

	// generates the Container where authorino will be running
	// adds to the list of containers available in the deployment
	authorinoContainer := authorinoResources.GetContainer(image, authorino.Spec.ImagePullPolicy, authorinoContainerName, args, envs, volumeMounts)
	containers = append(containers, authorinoContainer)

	replicas := authorino.Spec.Replicas
	if replicas == nil {
		value := int32(1)
		replicas = &value
	}

	// generate Deployment resource to deploy an authorino instance
	deployment := authorinoResources.GetDeployment(
		authorino.Name,
		authorino.Namespace,
		saName,
		replicas,
		containers,
		volumes,
		authorino.Labels,
	)

	_ = ctrl.SetControllerReference(authorino, deployment, r.Scheme)
	return deployment
}

func (r *AuthorinoReconciler) buildAuthorinoArgs(authorino *api.Authorino) []string {
	var args []string

	// watch-namespace
	if !authorino.Spec.ClusterWide {
		args = append(args, fmt.Sprintf("--%s=%s", flagWatchNamespace, authorino.GetNamespace()))
	}

	// auth-config-label-selector
	if selectors := authorino.Spec.AuthConfigLabelSelectors; selectors != "" {
		args = append(args, fmt.Sprintf("--%s=%s", flagWatchedAuthConfigLabelSelector, selectors))
	}

	// secret-label-selector
	if selectors := authorino.Spec.SecretLabelSelectors; selectors != "" {
		args = append(args, fmt.Sprintf("--%s=%s", flagWatchedSecretLabelSelector, selectors))
	}

	// allow-superseding-host-subsets
	if authorino.Spec.SupersedingHostSubsets {
		args = append(args, fmt.Sprintf("--%s", flagSupersedingHostSubsets))
	}

	// log-level
	if logLevel := authorino.Spec.LogLevel; logLevel != "" {
		args = append(args, fmt.Sprintf("--%s=%s", flagLogLevel, logLevel))
	}

	// log-mode
	if logMode := authorino.Spec.LogMode; logMode != "" {
		args = append(args, fmt.Sprintf("--%s=%s", flagLogMode, logMode))
	}

	// timeout
	if timeout := authorino.Spec.Listener.Timeout; timeout != nil {
		args = append(args, fmt.Sprintf("--%s=%d", flagTimeout, *timeout))
	}

	// ext-auth-grpc-port
	port := authorino.Spec.Listener.Ports.GRPC
	if port == nil {
		port = authorino.Spec.Listener.Port // deprecated
	}
	if port != nil {
		args = append(args, fmt.Sprintf("--%s=%d", flagExtAuthGRPCPort, *port))
	}

	// ext-auth-http-port
	if port := authorino.Spec.Listener.Ports.HTTP; port != nil {
		args = append(args, fmt.Sprintf("--%s=%d", flagExtAuthHTTPPort, *port))
	}

	// tls-cert and tls-cert-key
	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		args = append(args, fmt.Sprintf("--%s=%s", flagTlsCertPath, defaultTlsCertPath))
		args = append(args, fmt.Sprintf("--%s=%s", flagTlsCertKeyPath, defaultTlsCertKeyPath))
	}

	// oidc-http-port
	if port := authorino.Spec.OIDCServer.Port; port != nil {
		args = append(args, fmt.Sprintf("--%s=%d", flagOidcHTTPPort, *port))
	}

	// oidc-tls-cert and oidc-tls-cert-key
	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		args = append(args, fmt.Sprintf("--%s=%s", flagOidcTLSCertPath, defaultOidcTlsCertPath))
		args = append(args, fmt.Sprintf("--%s=%s", flagOidcTLSCertKeyPath, defaultOidcTlsCertKeyPath))
	}

	// evaluator-cache-size
	if evaluatorCacheSize := authorino.Spec.EvaluatorCacheSize; evaluatorCacheSize != nil {
		args = append(args, fmt.Sprintf("--%s=%d", flagEvaluatorCacheSize, *evaluatorCacheSize))
	}

	// tracing-service-endpoint and tracing-service-tag
	if tracingServiceEndpoint := authorino.Spec.Tracing.Endpoint; tracingServiceEndpoint != "" {
		args = append(args, fmt.Sprintf("--%s=%s", flagTracingServiceEndpoint, tracingServiceEndpoint))
		for key, value := range authorino.Spec.Tracing.Tags {
			args = append(args, fmt.Sprintf(`--%s="%s=%s"`, flagTracingServiceTag, key, value))
		}
	}

	// deep-metrics-enabled
	if enabled := authorino.Spec.Metrics.DeepMetricsEnabled; enabled != nil && *enabled {
		args = append(args, fmt.Sprintf("--%s", flagDeepMetricsEnabled))
	}

	// metrics-addr
	if port := authorino.Spec.Metrics.Port; port != nil {
		args = append(args, fmt.Sprintf("--%s=:%d", flagMetricsAddr, *port))
	}

	// health-probe-addr
	if port := authorino.Spec.Healthz.Port; port != nil {
		args = append(args, fmt.Sprintf("--%s=:%d", flagHealthProbeAddr, *port))
	}

	// enable-leader-election
	if replicas := authorino.Spec.Replicas; replicas != nil && *replicas > 1 {
		args = append(args, fmt.Sprintf("--%s", flagEnableLeaderElection))
	}

	// max-http-request-body-size
	if maxRequestBodySize := authorino.Spec.Listener.MaxHttpRequestBodySize; maxRequestBodySize != nil {
		args = append(args, fmt.Sprintf("--%s=%d", flagMaxHttpRequestBodySize, *maxRequestBodySize))
	}

	return args
}

// [DEPRECATED] Configures Authorino by defining environment variables (instead of command-line args)
// Kept for backward compatibility with older versions of Authorino (<= v0.10.x)
func (r *AuthorinoReconciler) buildAuthorinoEnv(authorino *api.Authorino) []k8score.EnvVar {
	envVar := []k8score.EnvVar{}

	if !authorino.Spec.ClusterWide {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envWatchNamespace,
			Value: authorino.GetNamespace(),
		})
	}

	if v := authorino.Spec.AuthConfigLabelSelectors; v != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envAuthConfigLabelSelector,
			Value: v,
		})
	}

	if v := authorino.Spec.SecretLabelSelectors; v != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envSecretLabelSelector,
			Value: v,
		})
	}

	if v := authorino.Spec.EvaluatorCacheSize; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envEvaluatorCacheSize,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	if v := authorino.Spec.Metrics.DeepMetricsEnabled; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envDeepMetricsEnabled,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	if v := authorino.Spec.LogLevel; v != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envLogLevel,
			Value: v,
		})
	}

	if v := authorino.Spec.LogMode; v != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envLogMode,
			Value: v,
		})
	}

	var p *int32

	// external auth service via GRPC
	p = authorino.Spec.Listener.Ports.GRPC
	if p == nil {
		p = authorino.Spec.Listener.Port // deprecated
	}
	if p != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envExtAuthGRPCPort,
			Value: fmt.Sprintf("%v", *p),
		})
	}

	// external auth service via HTTP
	if p = authorino.Spec.Listener.Ports.HTTP; p != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envExtAuthHTTPPort,
			Value: fmt.Sprintf("%v", *p),
		})
	}

	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envTlsCert,
			Value: defaultTlsCertPath,
		})

		envVar = append(envVar, k8score.EnvVar{
			Name:  envTlsCertKey,
			Value: defaultTlsCertKeyPath,
		})
	}

	if v := authorino.Spec.Listener.Timeout; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envTimeout,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	if v := authorino.Spec.Listener.MaxHttpRequestBodySize; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envMaxHttpRequestBodySize,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	// oidc service
	if v := authorino.Spec.OIDCServer.Port; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envOIDCHTTPPort,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		envVar = append(envVar, k8score.EnvVar{
			Name:  envOidcTlsCertPath,
			Value: defaultOidcTlsCertPath,
		})
		envVar = append(envVar, k8score.EnvVar{
			Name:  envOidcTlsCertKeyPath,
			Value: defaultOidcTlsCertKeyPath,
		})
	}

	return envVar
}

func (r *AuthorinoReconciler) authorinoDeploymentChanges(existingDeployment, desiredDeployment *k8sapps.Deployment) bool {
	if *existingDeployment.Spec.Replicas != *desiredDeployment.Spec.Replicas {
		return true
	}

	existingContainer := existingDeployment.Spec.Template.Spec.Containers[0]
	desiredContainer := desiredDeployment.Spec.Template.Spec.Containers[0]

	if existingContainer.Image != desiredContainer.Image {
		return true
	}

	if existingContainer.ImagePullPolicy != desiredContainer.ImagePullPolicy {
		return true
	}

	existingArgs := sort.StringSlice(existingContainer.Args)
	existingArgs.Sort()
	desiredArgs := sort.StringSlice(desiredContainer.Args)
	desiredArgs.Sort()
	if strings.Join(existingArgs, " ") != strings.Join(desiredArgs, " ") {
		return true
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

func (r *AuthorinoReconciler) createAuthorinoServices(authorino *api.Authorino) error {
	logger := r.Log

	authorinoInstanceName := authorino.Name
	authorinoInstanceNamespace := authorino.Namespace

	var desiredServices []*k8score.Service
	var grpcPort, httpPort int32

	// auth service
	if p := authorino.Spec.Listener.Ports.GRPC; p != nil {
		grpcPort = *p
	} else if p := authorino.Spec.Listener.Port; p != nil { // deprecated
		grpcPort = *p
	} else {
		grpcPort = defaultAuthGRPCServicePort
	}
	if p := authorino.Spec.Listener.Ports.HTTP; p != nil {
		httpPort = *p
	} else {
		httpPort = defaultAuthHTTPServicePort
	}
	desiredServices = append(desiredServices, authorinoResources.NewAuthService(
		authorinoInstanceName,
		authorinoInstanceNamespace,
		grpcPort,
		httpPort,
		authorino.Labels,
	))

	// oidc service
	if p := authorino.Spec.OIDCServer.Port; p != nil {
		httpPort = *p
	} else {
		httpPort = defaultOIDCServicePort
	}
	desiredServices = append(desiredServices, authorinoResources.NewOIDCService(
		authorinoInstanceName,
		authorinoInstanceNamespace,
		httpPort,
		authorino.Labels,
	))

	// metrics service
	if p := authorino.Spec.Metrics.Port; p != nil {
		httpPort = *p
	} else {
		httpPort = defaultMetricsServicePort
	}
	desiredServices = append(desiredServices, authorinoResources.NewMetricsService(
		authorinoInstanceName,
		authorinoInstanceNamespace,
		httpPort,
		authorino.Labels,
	))

	for _, desiredService := range desiredServices {
		// get existing service for the authorino instance
		existingService := &k8score.Service{}
		if err := r.Client.Get(context.TODO(), namespacedName(desiredService.Namespace, desiredService.Name), existingService); err != nil {
			if errors.IsNotFound(err) {
				// service doesn't exist then create
				_ = ctrl.SetControllerReference(authorino, desiredService, r.Scheme)
				if err := r.Client.Create(context.TODO(), desiredService); err != nil {
					return r.wrapErrorWithStatusUpdate(
						logger, authorino, r.setStatusFailed(statusUnableToGetLeaderElectionRoleBinding),
						fmt.Errorf("failed to create %s service, err: %v", desiredService.Name, err),
					)
				}
			}
			return r.wrapErrorWithStatusUpdate(
				logger, authorino, r.setStatusFailed(statusUnableToGetServices),
				fmt.Errorf("failed to get %s service, err: %v", desiredService.Name, err),
			)
		}

		if equal := authorinoResources.EqualServices(desiredService, existingService); !equal {
			existingService.Spec.Selector = desiredService.Spec.Selector
			existingService.Spec.Ports = desiredService.Spec.Ports

			if err := r.Client.Update(context.Background(), existingService); err != nil {
				return r.wrapErrorWithStatusUpdate(
					logger, authorino, r.setStatusFailed(statusUnableToGetServices),
					fmt.Errorf("failed to update %s service, err: %v", existingService.Name, err),
				)
			}
		}
	}
	return nil
}

func (r *AuthorinoReconciler) createAuthorinoPermission(authorino *api.Authorino, operatorNamespace string) error {
	// get ServiceAccount from authorino instance namespace
	if sa, err := r.createAuthorinoServiceAccount(authorino); err != nil {
		return err
	} else {
		// creates the manager ClusterRoleBinding/RoleBinding depending on type of installation
		if err := r.bindAuthorinoServiceAccountToClusterRole(authorinoManagerClusterRoleBindingName, authorino.Spec.ClusterWide, authorinoManagerClusterRoleName, *sa, authorino); err != nil {
			return err
		}

		// creates the K8s Auth ClusterRoleBinding (for Authorino's Kubernetes TokenReview and SubjectAccessReview features)
		// Disclaimer: this has nothing to do with kube-rbac-proxy, but to authn/authz features of Authorino that also require cluster scope role bindings
		if err := r.bindAuthorinoServiceAccountToClusterRole(authorinoK8sAuthClusterRoleBindingName, true, authorinoK8sAuthClusterRoleName, *sa, authorino); err != nil {
			return err
		}

		// creates leader election role (for the replicas of the Auhtorino instance to choose the one replica responsible for updating the status of the reconciled AuthConfig CRs)
		return r.bindAuthorinoServiceAccountToLeaderElectionRole(authorino, *sa)
	}
}

func (r *AuthorinoReconciler) createAuthorinoServiceAccount(authorino *api.Authorino) (*k8score.ServiceAccount, error) {
	var logger = r.Log
	sa := authorinoResources.GetAuthorinoServiceAccount(authorino.Namespace, authorino.Name, authorino.Labels)
	if err := r.Get(context.TODO(), namespacedName(sa.Namespace, sa.Name), sa); err != nil {
		if errors.IsNotFound(err) {
			// ServiceAccount doesn't exit - create one
			_ = ctrl.SetControllerReference(authorino, sa, r.Scheme)
			if err := r.Client.Create(context.TODO(), sa); err != nil {
				return nil, r.wrapErrorWithStatusUpdate(
					logger, authorino, r.setStatusFailed(statusUnableToCreateServiceAccount),
					fmt.Errorf("failed to create %s ServiceAccount, err: %v", sa.Name, err),
				)
			}
		}
		return nil, r.wrapErrorWithStatusUpdate(
			logger, authorino, r.setStatusFailed(statusUnableToGetServiceAccount),
			fmt.Errorf("failed to get %s ServiceAccount, err: %v", sa.Name, err),
		)
	}
	// ServiceAccount exists
	return sa, nil
}

func (r *AuthorinoReconciler) bindAuthorinoServiceAccountToClusterRole(roleBindingName string, clusterScoped bool, clusterRoleName string, serviceAccount k8score.ServiceAccount, authorino *api.Authorino) error {
	var ctx = context.TODO()
	var logger = r.Log

	// check if clusterrole exists
	clusterRole := &k8srbac.ClusterRole{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: clusterRoleName}, clusterRole); err != nil {
		if errors.IsNotFound(err) {
			return r.wrapErrorWithStatusUpdate(logger, authorino, r.setStatusFailed(statusClusterRoleNotFound), fmt.Errorf("failed to find authorino ClusterRole %s: %v", clusterRoleName, err))
		} else {
			return r.wrapErrorWithStatusUpdate(logger, authorino, r.setStatusFailed(statusUnableToGetClusterRole), fmt.Errorf("failed to get authorino ClusterRole %s: %v", clusterRoleName, err))
		}
	}

	var roleBinding client.Object

	if clusterScoped {
		roleBinding = authorinoResources.GetAuthorinoClusterRoleBinding(roleBindingName, clusterRoleName, serviceAccount)
	} else {
		roleBinding = authorinoResources.GetAuthorinoRoleBinding(authorino.Namespace, authorino.Name, roleBindingName, "ClusterRole", clusterRoleName, serviceAccount, authorino.Labels)
		roleBinding.SetNamespace(authorino.Namespace)
	}

	if err := r.Get(ctx, namespacedName(roleBinding.GetNamespace(), roleBinding.GetName()), roleBinding); err != nil {
		// failed to get (cluster)rolebinding -> check if not found or other error

		if errors.IsNotFound(err) {
			// (cluster)rolebinding does not exist -> create one
			_ = ctrl.SetControllerReference(authorino, roleBinding, r.Scheme) // useful for namespaced role bindings
			if err := r.Client.Create(ctx, roleBinding); err != nil {
				return r.wrapErrorWithStatusUpdate(
					logger, authorino, r.setStatusFailed(statusUnableToCreateBindingForClusterRole),
					fmt.Errorf("failed to create %s binding for authorino ClusterRole, err: %v", roleBinding.GetName(), err),
				)
			}
		} else {
			return r.wrapErrorWithStatusUpdate(
				logger, authorino, r.setStatusFailed(statusUnableToGetBindingForClusterRole),
				fmt.Errorf("failed to get %s binding for authorino ClusterRole, err: %v", roleBindingName, err),
			)
		}

		// other error -> return
		return r.wrapErrorWithStatusUpdate(
			logger, authorino, r.setStatusFailed(statusUnableToGetBindingForClusterRole),
			fmt.Errorf("failed to get %s binding for authorino ClusterRole, err: %v", roleBinding.GetName(), err),
		)
	} else {
		// (cluster)rolebinding exists -> ensure it includes the serviceaccount among the subjects

		rb := authorinoResources.AppendSubjectToRoleBinding(roleBinding, serviceAccount)
		if err := r.Client.Update(ctx, rb); err != nil {
			return r.wrapErrorWithStatusUpdate(
				logger, authorino, r.setStatusFailed(statusUnableToCreateBindingForClusterRole),
				fmt.Errorf("failed to update %s binding for authorino ClusterRole, err: %v", roleBinding.GetName(), err),
			)
		}

		return nil
	}
}

func (r *AuthorinoReconciler) bindAuthorinoServiceAccountToLeaderElectionRole(authorino *api.Authorino, serviceAccount k8score.ServiceAccount) error {
	var logger = r.Log

	leaderElectionRole := &k8srbac.Role{}
	leaderElectionNsdName := namespacedName(authorino.Namespace, authorinoLeaderElectionRoleName)
	if err := r.Get(context.TODO(), leaderElectionNsdName, leaderElectionRole); err != nil {
		if errors.IsNotFound(err) {
			// leader election Role doesn't exist then create
			leaderElectionRole.Name = authorinoLeaderElectionRoleName
			leaderElectionRole.Namespace = authorino.Namespace
			leaderElectionRole.Rules = authorinoResources.GetLeaderElectionRules()
			_ = ctrl.SetControllerReference(authorino, leaderElectionRole, r.Scheme)
			if err := r.Client.Create(context.TODO(), leaderElectionRole); err != nil {
				return r.wrapErrorWithStatusUpdate(
					logger, authorino, r.setStatusFailed(statusUnableToCreateLeaderElectionRole),
					fmt.Errorf("failed to create %s role, err: %v", leaderElectionRole, err),
				)
			}
		}
		return r.wrapErrorWithStatusUpdate(
			logger, authorino, r.setStatusFailed(statusUnableToGetLeaderElectionRole),
			fmt.Errorf("failed to get %s Role, err: %v", authorinoLeaderElectionRoleName, err),
		)
	}

	leRoleBinding := authorinoResources.GetAuthorinoRoleBinding(
		authorino.Namespace,
		authorino.Name,
		authorinoLeaderElectionRoleBindingName,
		"Role",
		authorinoLeaderElectionRoleName,
		serviceAccount,
		authorino.Labels,
	)
	if err := r.Get(context.TODO(), namespacedName(leRoleBinding.Namespace, leRoleBinding.Name), leRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			_ = ctrl.SetControllerReference(authorino, leRoleBinding, r.Scheme)
			// doesn't exist - create one
			if err := r.Client.Create(context.TODO(), leRoleBinding); err != nil {
				return r.wrapErrorWithStatusUpdate(
					logger, authorino, r.setStatusFailed(statusUnableToCreateLeaderElectionRoleBinding),
					fmt.Errorf("failed to create %s RoleBinding, err: %v", leRoleBinding.Name, err),
				)
			}
		}
		return r.wrapErrorWithStatusUpdate(
			logger, authorino, r.setStatusFailed(statusUnableToGetLeaderElectionRoleBinding),
			fmt.Errorf("failed to get %s RoleBinding, err: %v", leRoleBinding.Name, err),
		)
	}
	return nil
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
				return r.wrapErrorWithStatusUpdate(
					r.Log, authorino, r.setStatusFailed(statusTlsSecretNotProvided),
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
				return r.wrapErrorWithStatusUpdate(
					r.Log, authorino, r.setStatusFailed(statusTlsSecretNotProvided),
					errorMessage,
				)
			}
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
		Reason: statusProvisioned,
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

func deploymentAvailable(deployment *k8sapps.Deployment) bool {
	for _, condition := range deployment.Status.Conditions {
		switch condition.Type {
		case "Available":
			return condition.Status == "True"
		}
	}
	return false
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

func authorinoVersionFromImageTag(image string) string {
	parts := strings.Split(image, ":")
	return parts[len(parts)-1]
}
