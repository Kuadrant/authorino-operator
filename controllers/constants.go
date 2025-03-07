package controllers

const (
	// kubernetes objects
	authorinoContainerName                 string = "authorino"
	authorinoTlsCertVolumeName             string = "tls-cert"
	authorinoOidcTlsCertVolumeName         string = "oidc-cert"
	authorinoManagerClusterRoleName        string = "authorino-manager-role"
	authorinoK8sAuthClusterRoleName        string = "authorino-manager-k8s-auth-role"
	authorinoLeaderElectionRoleName        string = "authorino-leader-election-role"
	authorinoManagerClusterRoleBindingName string = "authorino"
	authorinoK8sAuthClusterRoleBindingName string = "authorino-k8s-auth"
	authorinoLeaderElectionRoleBindingName string = "authorino-leader-election"

	// env vars / command-line flags
	envWatchNamespace          string = "WATCH_NAMESPACE"
	envAuthConfigLabelSelector string = "AUTH_CONFIG_LABEL_SELECTOR"
	envSecretLabelSelector     string = "SECRET_LABEL_SELECTOR"
	envEvaluatorCacheSize      string = "EVALUATOR_CACHE_SIZE"
	envDeepMetricsEnabled      string = "DEEP_METRICS_ENABLED"
	envLogLevel                string = "LOG_LEVEL"
	envLogMode                 string = "LOG_MODE"
	envExtAuthGRPCPort         string = "EXT_AUTH_GRPC_PORT"
	envExtAuthHTTPPort         string = "EXT_AUTH_HTTP_PORT"
	envTlsCert                 string = "TLS_CERT"
	envTlsCertKey              string = "TLS_CERT_KEY"
	envTimeout                 string = "TIMEOUT"
	envOIDCHTTPPort            string = "OIDC_HTTP_PORT"
	envOidcTlsCertPath         string = "OIDC_TLS_CERT"
	envOidcTlsCertKeyPath      string = "OIDC_TLS_CERT_KEY"
	envMaxHttpRequestBodySize  string = "MAX_HTTP_REQUEST_BODY_SIZE"

	flagWatchNamespace                 string = "watch-namespace"
	flagWatchedAuthConfigLabelSelector string = "auth-config-label-selector"
	flagWatchedSecretLabelSelector     string = "secret-label-selector"
	flagSupersedingHostSubsets         string = "allow-superseding-host-subsets"
	flagLogLevel                       string = "log-level"
	flagLogMode                        string = "log-mode"
	flagTimeout                        string = "timeout"
	flagExtAuthGRPCPort                string = "ext-auth-grpc-port"
	flagExtAuthHTTPPort                string = "ext-auth-http-port"
	flagTlsCertPath                    string = "tls-cert"
	flagTlsCertKeyPath                 string = "tls-cert-key"
	flagOidcHTTPPort                   string = "oidc-http-port"
	flagOidcTLSCertPath                string = "oidc-tls-cert"
	flagOidcTLSCertKeyPath             string = "oidc-tls-cert-key"
	flagEvaluatorCacheSize             string = "evaluator-cache-size"
	flagTracingServiceEndpoint         string = "tracing-service-endpoint"
	flagTracingServiceTag              string = "tracing-service-tag"
	flagTracingServiceInsecure         string = "tracing-service-insecure"
	flagDeepMetricsEnabled             string = "deep-metrics-enabled"
	flagMetricsAddr                    string = "metrics-addr"
	flagHealthProbeAddr                string = "health-probe-addr"
	flagEnableLeaderElection           string = "enable-leader-election"
	flagMaxHttpRequestBodySize         string = "max-http-request-body-size"

	// defaults
	defaultTlsCertPath         string = "/etc/ssl/certs/tls.crt"
	defaultTlsCertKeyPath      string = "/etc/ssl/private/tls.key"
	defaultOidcTlsCertPath     string = "/etc/ssl/certs/oidc.crt"
	defaultOidcTlsCertKeyPath  string = "/etc/ssl/private/oidc.key"
	defaultAuthGRPCServicePort int32  = 50051
	defaultAuthHTTPServicePort int32  = 5001
	defaultOIDCServicePort     int32  = 8083
	defaultMetricsServicePort  int32  = 8080
	defaultHealthProbePort     int32  = 8081

	// status reasons
	statusProvisioning                            = "Provisioning"
	statusProvisioned                             = "Provisioned"
	statusUpdated                                 = "Updated"
	statusUnableToCreateServices                  = "UnableToCreateServices"
	statusUnableToCreateDeployment                = "UnableToCreateDeployment"
	statusUnableToCreateLeaderElectionRole        = "UnableToCreateLeaderElectionRole"
	statusUnableToCreatePermission                = "UnableToCreatePermission"
	statusUnableToCreateServiceAccount            = "UnableToCreateServiceAccount"
	statusUnableToCreateBindingForClusterRole     = "UnableToBindingForClusterRole"
	statusUnableToCreateLeaderElectionRoleBinding = "UnableToCreateLeaderElectionRoleBinding"
	statusClusterRoleNotFound                     = "ClusterRoleNotFound"
	statusUnableToGetClusterRole                  = "UnableToGetClusterRole"
	statusUnableToGetServices                     = "UnableToGetServices"
	statusUnableToGetBindingForClusterRole        = "UnableToGetBindingForClusterRole"
	statusUnableToGetServiceAccount               = "UnableToGetServiceAccount"
	statusUnableToGetLeaderElectionRole           = "UnableToGetLeaderElectionRole"
	statusUnableToGetLeaderElectionRoleBinding    = "UnableToGetLeaderElectionRoleBinding"
	statusUnableToGetDeployment                   = "UnableToGetDeployment"
	statusUnableToGetTlsSecret                    = "UnableToGetTlsSecret"
	statusTlsSecretNotFound                       = "TlsSecretNotFound"
	statusTlsSecretNotProvided                    = "TlsSecretNotProvided"
	statusUnableToUpdateDeployment                = "UnableToUpdateDeployment"
	statusDeploymentNotReady                      = "DeploymentNotReady"
	statusUnableToBuildDeploymentObject           = "UnableToBuildDeploymentObject"
)

// ldflags
var DefaultAuthorinoImage string
