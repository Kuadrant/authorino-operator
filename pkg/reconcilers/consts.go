package reconcilers

const (
	DeleteTagAnnotation = "authorino.kuadrant.io/delete"

	RelatedImageAuthorino = "RELATED_IMAGE_AUTHORINO"

	// kubernetes objects
	AuthorinoContainerName                 string = "authorino"
	AuthorinoTlsCertVolumeName             string = "tls-cert"
	AuthorinoOidcTlsCertVolumeName         string = "oidc-cert"
	AuthorinoManagerClusterRoleName        string = "authorino-manager-role"
	AuthorinoK8sAuthClusterRoleName        string = "authorino-manager-k8s-auth-role"
	AuthorinoLeaderElectionRoleName        string = "authorino-leader-election-role"
	AuthorinoManagerClusterRoleBindingName string = "authorino"
	AuthorinoK8sAuthClusterRoleBindingName string = "authorino-k8s-auth"
	authorinoLeaderElectionRoleBindingName string = "authorino-leader-election"

	// env vars / command-line flags
	EnvWatchNamespace          string = "WATCH_NAMESPACE"
	EnvAuthConfigLabelSelector string = "AUTH_CONFIG_LABEL_SELECTOR"
	EnvSecretLabelSelector     string = "SECRET_LABEL_SELECTOR"
	EnvEvaluatorCacheSize      string = "EVALUATOR_CACHE_SIZE"
	EnvDeepMetricsEnabled      string = "DEEP_METRICS_ENABLED"
	EnvLogLevel                string = "LOG_LEVEL"
	EnvLogMode                 string = "LOG_MODE"
	EnvExtAuthGRPCPort         string = "EXT_AUTH_GRPC_PORT"
	EnvExtAuthHTTPPort         string = "EXT_AUTH_HTTP_PORT"
	EnvTlsCert                 string = "TLS_CERT"
	EnvTlsCertKey              string = "TLS_CERT_KEY"
	EnvTimeout                 string = "TIMEOUT"
	EnvOIDCHTTPPort            string = "OIDC_HTTP_PORT"
	EnvOidcTlsCertPath         string = "OIDC_TLS_CERT"
	EnvOidcTlsCertKeyPath      string = "OIDC_TLS_CERT_KEY"
	EnvMaxHttpRequestBodySize  string = "MAX_HTTP_REQUEST_BODY_SIZE"

	FlagWatchNamespace                 string = "watch-namespace"
	FlagWatchedAuthConfigLabelSelector string = "auth-config-label-selector"
	FlagWatchedSecretLabelSelector     string = "secret-label-selector"
	FlagSupersedingHostSubsets         string = "allow-superseding-host-subsets"
	FlagLogLevel                       string = "log-level"
	FlagLogMode                        string = "log-mode"
	FlagTimeout                        string = "timeout"
	FlagExtAuthGRPCPort                string = "ext-auth-grpc-port"
	FlagExtAuthHTTPPort                string = "ext-auth-http-port"
	FlagTlsCertPath                    string = "tls-cert"
	FlagTlsCertKeyPath                 string = "tls-cert-key"
	FlagOidcHTTPPort                   string = "oidc-http-port"
	FlagOidcTLSCertPath                string = "oidc-tls-cert"
	FlagOidcTLSCertKeyPath             string = "oidc-tls-cert-key"
	FlagEvaluatorCacheSize             string = "evaluator-cache-size"
	FlagTracingServiceEndpoint         string = "tracing-service-endpoint"
	FlagTracingServiceTag              string = "tracing-service-tag"
	FlagTracingServiceInsecure         string = "tracing-service-insecure"
	FlagDeepMetricsEnabled             string = "deep-metrics-enabled"
	FlagMetricsAddr                    string = "metrics-addr"
	FlagHealthProbeAddr                string = "health-probe-addr"
	FlagEnableLeaderElection           string = "enable-leader-election"
	FlagMaxHttpRequestBodySize         string = "max-http-request-body-size"

	// defaults
	DefaultTlsCertPath         string = "/etc/ssl/certs/tls.crt"
	DefaultTlsCertKeyPath      string = "/etc/ssl/private/tls.key"
	DefaultOidcTlsCertPath     string = "/etc/ssl/certs/oidc.crt"
	DefaultOidcTlsCertKeyPath  string = "/etc/ssl/private/oidc.key"
	DefaultAuthGRPCServicePort int32  = 50051
	DefaultAuthHTTPServicePort int32  = 5001
	DefaultOIDCServicePort     int32  = 8083
	DefaultMetricsServicePort  int32  = 8080
	DefaultHealthProbePort     int32  = 8081

	// status reasons
	statusProvisioning                            = "Provisioning"
	statusProvisioned                             = "Provisioned"
	statusUpdated                                 = "Updated"
	statusUnableToCreateServices                  = "UnableToCreateServices"
	statusUnableToCreateDeployment                = "UnableToCreateDeployment"
	statusUnableToCreateLeaderElectionRole        = "UnableToCreateLeaderElectionRole"
	statusUnableToCreatePermission                = "UnableToCreatePermission"
	StatusUnableToCreateServiceAccount            = "UnableToCreateServiceAccount"
	statusUnableToCreateBindingForClusterRole     = "UnableToBindingForClusterRole"
	statusUnableToCreateLeaderElectionRoleBinding = "UnableToCreateLeaderElectionRoleBinding"
	statusClusterRoleNotFound                     = "ClusterRoleNotFound"
	statusUnableToGetClusterRole                  = "UnableToGetClusterRole"
	statusUnableToGetServices                     = "UnableToGetServices"
	statusUnableToGetBindingForClusterRole        = "UnableToGetBindingForClusterRole"
	StatusUnableToGetServiceAccount               = "UnableToGetServiceAccount"
	statusUnableToGetLeaderElectionRole           = "UnableToGetLeaderElectionRole"
	statusUnableToGetLeaderElectionRoleBinding    = "UnableToGetLeaderElectionRoleBinding"
	statusUnableToGetDeployment                   = "UnableToGetDeployment"
	statusUnableToGetTlsSecret                    = "UnableToGetTlsSecret"
	statusTlsSecretNotFound                       = "TlsSecretNotFound"
	StatusTlsSecretNotProvided                    = "TlsSecretNotProvided"
	statusUnableToUpdateDeployment                = "UnableToUpdateDeployment"
	statusDeploymentNotReady                      = "DeploymentNotReady"
	StatusUnableToBuildDeploymentObject           = "UnableToBuildDeploymentObject"
)

// ldflags
var DefaultAuthorinoImage string
