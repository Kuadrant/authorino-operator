package reconcilers

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	"k8s.io/utils/env"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	authorinoResources "github.com/kuadrant/authorino-operator/pkg/resources"
)

// MutateFn is a function which mutates the existing object into it's desired state.
type MutateFn func(desired, existing client.Object) (bool, error)

// DeploymentMutateFn is a function which mutates the existing Deployment into it's desired state.
type DeploymentMutateFn func(desired, existing *k8sapps.Deployment) bool

func DeploymentMutator(opts ...DeploymentMutateFn) MutateFn {
	return func(desiredObj, existingObj client.Object) (bool, error) {
		existing, ok := existingObj.(*k8sapps.Deployment)
		if !ok {
			return false, fmt.Errorf("%T is not a *appsv1.Deployment", existingObj)
		}
		desired, ok := desiredObj.(*k8sapps.Deployment)
		if !ok {
			return false, fmt.Errorf("%T is not a *appsv1.Deployment", desiredObj)
		}

		update := false

		// Loop through each option
		for _, opt := range opts {
			tmpUpdate := opt(desired, existing)
			update = update || tmpUpdate
		}

		return update, nil
	}
}

func DeploymentReplicasMutator(desired, existing *k8sapps.Deployment) bool {
	update := false

	var existingReplicas int32 = 1
	if existing.Spec.Replicas != nil {
		existingReplicas = *existing.Spec.Replicas
	}

	var desiredReplicas int32 = 1
	if desired.Spec.Replicas != nil {
		desiredReplicas = *desired.Spec.Replicas
	}

	if desiredReplicas != existingReplicas {
		existing.Spec.Replicas = &desiredReplicas
		update = true
	}

	return update
}

func DeploymentContainerListMutator(desired, existing *k8sapps.Deployment) bool {
	update := false

	if len(existing.Spec.Template.Spec.Containers) != len(desired.Spec.Template.Spec.Containers) {
		existing.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
		update = true
	}

	return update
}

func DeploymentImageMutator(desired, existing *k8sapps.Deployment) bool {
	update := false

	if existing.Spec.Template.Spec.Containers[0].Image != desired.Spec.Template.Spec.Containers[0].Image {
		existing.Spec.Template.Spec.Containers[0].Image = desired.Spec.Template.Spec.Containers[0].Image
		update = true
	}

	return update
}

func DeploymentVolumesMutator(desired, existing *k8sapps.Deployment) bool {
	update := false

	if !reflect.DeepEqual(existing.Spec.Template.Spec.Volumes, desired.Spec.Template.Spec.Volumes) {
		existing.Spec.Template.Spec.Volumes = desired.Spec.Template.Spec.Volumes
		update = true
	}

	return update
}

func DeploymentVolumeMountsMutator(desired, existing *k8sapps.Deployment) bool {
	update := false

	existingContainer := &existing.Spec.Template.Spec.Containers[0]
	desiredContainer := &desired.Spec.Template.Spec.Containers[0]

	if !reflect.DeepEqual(existingContainer.VolumeMounts, desiredContainer.VolumeMounts) {
		existingContainer.VolumeMounts = desiredContainer.VolumeMounts
		update = true
	}

	return update
}

func DeploymentServiceAccountMutator(desired, existing *k8sapps.Deployment) bool {
	update := false

	if existing.Spec.Template.Spec.ServiceAccountName != desired.Spec.Template.Spec.ServiceAccountName {
		existing.Spec.Template.Spec.ServiceAccountName = desired.Spec.Template.Spec.ServiceAccountName
		update = true
	}

	return update
}

func DeploymentLabelsMutator(desired, existing *k8sapps.Deployment) bool {
	update := false

	if !reflect.DeepEqual(existing.ObjectMeta.Labels, desired.ObjectMeta.Labels) {
		existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
		update = true
	}

	return update
}

func DeploymentSpecTemplateLabelsMutator(desired, existing *k8sapps.Deployment) bool {
	return authorinoResources.MergeMapStringString(&existing.Spec.Template.Labels, desired.Spec.Template.Labels)
}

func IsObjectTaggedToDelete(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}

	annotation, ok := annotations[DeleteTagAnnotation]
	return ok && annotation == "true"
}

func AuthorinoDeployment(authorino *api.Authorino) *k8sapps.Deployment {
	var containers []k8score.Container
	var saName = authorino.Name + "-authorino"

	image := authorino.Spec.Image

	if image == "" {
		image = env.GetString(RelatedImageAuthorino, DefaultAuthorinoImage)
	}

	if image == "" {
		// `DefaultAuthorinoImage can be empty string. But image cannot be or deployment will fail
		panic("DefaultAuthorinoImage is empty")
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

	// mount tls cert volume for the ext_authz listener if enable
	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		secretName := authorino.Spec.Listener.Tls.CertSecret.Name
		volumeMounts = append(volumeMounts, authorinoResources.GetTlsVolumeMount(AuthorinoTlsCertVolumeName, DefaultTlsCertPath, DefaultTlsCertKeyPath)...)
		volumes = append(volumes, authorinoResources.GetTlsVolume(AuthorinoTlsCertVolumeName, secretName))
	}

	// mount tls cert volume for the oidc listener if enabled
	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		secretName := authorino.Spec.OIDCServer.Tls.CertSecret.Name
		volumeMounts = append(volumeMounts, authorinoResources.GetTlsVolumeMount(AuthorinoOidcTlsCertVolumeName, DefaultTlsCertPath, DefaultOidcTlsCertKeyPath)...)
		volumes = append(volumes, authorinoResources.GetTlsVolume(AuthorinoOidcTlsCertVolumeName, secretName))
	}

	args := buildAuthorinoArgs(authorino)
	var envs []k8score.EnvVar

	// Deprecated: configure authorino using env vars (only for old Authorino versions)
	authorinoVersion := authorinoVersionFromImageTag(image)
	if detectEnvVarAuthorinoVersion(authorinoVersion) {
		envs = buildAuthorinoEnv(authorino)

		var compatibleArgs []string
		for _, arg := range args {
			parts := strings.Split(strings.TrimPrefix(arg, "--"), "=")
			switch parts[0] {
			case FlagMetricsAddr, FlagEnableLeaderElection:
				compatibleArgs = append(compatibleArgs, arg)
			}
		}
		args = compatibleArgs
	}

	// generates the Container where authorino will be running
	// adds to the list of containers available in the deployment
	authorinoContainer := authorinoResources.GetContainer(image, authorino.Spec.ImagePullPolicy, AuthorinoContainerName, args, envs, volumeMounts)
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

	return deployment
}

func buildAuthorinoArgs(authorino *api.Authorino) []string {
	var args []string

	// watch-namespace
	if !authorino.Spec.ClusterWide {
		args = append(args, fmt.Sprintf("--%s=%s", FlagWatchNamespace, authorino.GetNamespace()))
	}

	// auth-config-label-selector
	if selectors := authorino.Spec.AuthConfigLabelSelectors; selectors != "" {
		args = append(args, fmt.Sprintf("--%s=%s", FlagWatchedAuthConfigLabelSelector, selectors))
	}

	// secret-label-selector
	if selectors := authorino.Spec.SecretLabelSelectors; selectors != "" {
		args = append(args, fmt.Sprintf("--%s=%s", FlagWatchedSecretLabelSelector, selectors))
	}

	// allow-superseding-host-subsets
	if authorino.Spec.SupersedingHostSubsets {
		args = append(args, fmt.Sprintf("--%s", FlagSupersedingHostSubsets))
	}

	// log-level
	if logLevel := authorino.Spec.LogLevel; logLevel != "" {
		args = append(args, fmt.Sprintf("--%s=%s", FlagLogLevel, logLevel))
	}

	// log-mode
	if logMode := authorino.Spec.LogMode; logMode != "" {
		args = append(args, fmt.Sprintf("--%s=%s", FlagLogMode, logMode))
	}

	// timeout
	if timeout := authorino.Spec.Listener.Timeout; timeout != nil {
		args = append(args, fmt.Sprintf("--%s=%d", FlagTimeout, *timeout))
	}

	// ext-auth-grpc-port
	port := authorino.Spec.Listener.Ports.GRPC
	if port == nil {
		port = authorino.Spec.Listener.Port // deprecated
	}
	if port != nil {
		args = append(args, fmt.Sprintf("--%s=%d", FlagExtAuthGRPCPort, *port))
	}

	// ext-auth-http-port
	if port := authorino.Spec.Listener.Ports.HTTP; port != nil {
		args = append(args, fmt.Sprintf("--%s=%d", FlagExtAuthHTTPPort, *port))
	}

	// tls-cert and tls-cert-key
	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		args = append(args, fmt.Sprintf("--%s=%s", FlagTlsCertPath, DefaultTlsCertPath))
		args = append(args, fmt.Sprintf("--%s=%s", FlagTlsCertKeyPath, DefaultTlsCertKeyPath))
	}

	// oidc-http-port
	if port := authorino.Spec.OIDCServer.Port; port != nil {
		args = append(args, fmt.Sprintf("--%s=%d", FlagOidcHTTPPort, *port))
	}

	// oidc-tls-cert and oidc-tls-cert-key
	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		args = append(args, fmt.Sprintf("--%s=%s", FlagOidcTLSCertPath, DefaultOidcTlsCertPath))
		args = append(args, fmt.Sprintf("--%s=%s", FlagOidcTLSCertKeyPath, DefaultOidcTlsCertKeyPath))
	}

	// evaluator-cache-size
	if evaluatorCacheSize := authorino.Spec.EvaluatorCacheSize; evaluatorCacheSize != nil {
		args = append(args, fmt.Sprintf("--%s=%d", FlagEvaluatorCacheSize, *evaluatorCacheSize))
	}

	// tracing-service-endpoint, tracing-service-tag, and tracing-service-insecure
	if tracingServiceEndpoint := authorino.Spec.Tracing.Endpoint; tracingServiceEndpoint != "" {
		args = append(args, fmt.Sprintf("--%s=%s", FlagTracingServiceEndpoint, tracingServiceEndpoint))
		for key, value := range authorino.Spec.Tracing.Tags {
			args = append(args, fmt.Sprintf(`--%s=%s=%s`, FlagTracingServiceTag, key, value))
		}
		if authorino.Spec.Tracing.Insecure {
			args = append(args, fmt.Sprintf(`--%s`, FlagTracingServiceInsecure))
		}
	}

	// deep-metrics-enabled
	if enabled := authorino.Spec.Metrics.DeepMetricsEnabled; enabled != nil && *enabled {
		args = append(args, fmt.Sprintf("--%s", FlagDeepMetricsEnabled))
	}

	// metrics-addr
	if port := authorino.Spec.Metrics.Port; port != nil {
		args = append(args, fmt.Sprintf("--%s=:%d", FlagMetricsAddr, *port))
	}

	// health-probe-addr
	if port := authorino.Spec.Healthz.Port; port != nil {
		args = append(args, fmt.Sprintf("--%s=:%d", FlagHealthProbeAddr, *port))
	}

	// enable-leader-election
	if replicas := authorino.Spec.Replicas; replicas != nil && *replicas > 1 {
		args = append(args, fmt.Sprintf("--%s", FlagEnableLeaderElection))
	}

	// max-http-request-body-size
	if maxRequestBodySize := authorino.Spec.Listener.MaxHttpRequestBodySize; maxRequestBodySize != nil {
		args = append(args, fmt.Sprintf("--%s=%d", FlagMaxHttpRequestBodySize, *maxRequestBodySize))
	}

	return args
}

// Deprecated: Configures Authorino by defining environment variables (instead of command-line args)
// Kept for backward compatibility with older versions of Authorino (<= v0.10.x)
func buildAuthorinoEnv(authorino *api.Authorino) []k8score.EnvVar {
	envVar := []k8score.EnvVar{}

	if !authorino.Spec.ClusterWide {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvWatchNamespace,
			Value: authorino.GetNamespace(),
		})
	}

	if v := authorino.Spec.AuthConfigLabelSelectors; v != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvAuthConfigLabelSelector,
			Value: v,
		})
	}

	if v := authorino.Spec.SecretLabelSelectors; v != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvSecretLabelSelector,
			Value: v,
		})
	}

	if v := authorino.Spec.EvaluatorCacheSize; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvEvaluatorCacheSize,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	if v := authorino.Spec.Metrics.DeepMetricsEnabled; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvDeepMetricsEnabled,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	if v := authorino.Spec.LogLevel; v != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvLogLevel,
			Value: v,
		})
	}

	if v := authorino.Spec.LogMode; v != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvLogMode,
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
			Name:  EnvExtAuthGRPCPort,
			Value: fmt.Sprintf("%v", *p),
		})
	}

	// external auth service via HTTP
	if p = authorino.Spec.Listener.Ports.HTTP; p != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvExtAuthHTTPPort,
			Value: fmt.Sprintf("%v", *p),
		})
	}

	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvTlsCert,
			Value: DefaultTlsCertPath,
		})

		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvTlsCertKey,
			Value: DefaultTlsCertKeyPath,
		})
	}

	if v := authorino.Spec.Listener.Timeout; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvTimeout,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	if v := authorino.Spec.Listener.MaxHttpRequestBodySize; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvMaxHttpRequestBodySize,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	// oidc service
	if v := authorino.Spec.OIDCServer.Port; v != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvOIDCHTTPPort,
			Value: fmt.Sprintf("%v", *v),
		})
	}

	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvOidcTlsCertPath,
			Value: DefaultOidcTlsCertPath,
		})
		envVar = append(envVar, k8score.EnvVar{
			Name:  EnvOidcTlsCertKeyPath,
			Value: DefaultOidcTlsCertKeyPath,
		})
	}

	return envVar
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

func DeploymentAvailable(deployment *k8sapps.Deployment) bool {
	for _, condition := range deployment.Status.Conditions {
		switch condition.Type {
		case "Available":
			return condition.Status == "True"
		}
	}
	return false
}
