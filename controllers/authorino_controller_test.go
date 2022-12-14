package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/util/uuid"

	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	authorinoResources "github.com/kuadrant/authorino-operator/pkg/resources"
)

const (
	testAuthorinoNamespace = "default"
	testAuthorinoReplicas  = 1

	testTimeout  = time.Minute * 1
	testInterval = time.Second * 5
)

var _ = Describe("Authorino controller", func() {
	Context("Creating an new instance of authorino", func() {
		var authorinoInstance *api.Authorino

		BeforeEach(func() {
			_ = k8sClient.Create(context.TODO(), newExtServerConfigMap())

			authorinoInstance = newFullAuthorinoInstance()
			Expect(k8sClient.Create(context.TODO(), authorinoInstance)).Should(Succeed())

			nsdName := namespacedName(authorinoInstance.GetNamespace(), authorinoInstance.GetName())

			Eventually(func() bool {
				var authorino api.Authorino
				err := k8sClient.Get(context.TODO(), nsdName, &authorino)
				return err == nil && authorinoInstance.Status.Ready()
			}, testTimeout, testInterval).Should(BeFalse())
		})

		It("Should create authorino required services", func() {
			desiredServices := []*k8score.Service{
				authorinoResources.NewOIDCService(authorinoInstance.Name, authorinoInstance.Namespace, defaultOIDCServicePort, authorinoInstance.Labels),
				authorinoResources.NewMetricsService(authorinoInstance.Name, authorinoInstance.Namespace, defaultMetricsServicePort, authorinoInstance.Labels),
				authorinoResources.NewAuthService(authorinoInstance.Name, authorinoInstance.Namespace, defaultAuthGRPCServicePort, defaultAuthHTTPServicePort, authorinoInstance.Labels),
			}

			for _, service := range desiredServices {
				nsdName := namespacedName(service.GetNamespace(), service.GetName())

				Eventually(func() bool {
					err := k8sClient.Get(context.TODO(), nsdName, &k8score.Service{})
					return err == nil
				}, testTimeout, testInterval).Should(BeTrue())
			}
		})

		It("Should create authorino permission", func() {
			// service account
			sa := authorinoResources.GetAuthorinoServiceAccount(testAuthorinoNamespace, authorinoInstance.Name, authorinoInstance.Labels)
			nsdName := namespacedName(sa.GetNamespace(), sa.GetName())
			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), nsdName, sa)
				return err == nil
			}, testTimeout, testInterval).Should(BeTrue())

			// Authorino ClusterRoleBinding
			var binding client.Object
			var bindingNsdName types.NamespacedName
			if authorinoInstance.Spec.ClusterWide {
				binding = &k8srbac.ClusterRoleBinding{}
				bindingNsdName = types.NamespacedName{Name: "authorino"}
			} else {
				binding = &k8srbac.RoleBinding{}
				bindingNsdName = namespacedName(testAuthorinoNamespace, authorinoInstance.Name+"-authorino")
			}

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), bindingNsdName, binding)
				return err == nil
			}, testTimeout, testInterval).Should(BeTrue())

			// Authorino Auth ClusterRoleBinding
			k8sAuthBinding := &k8srbac.ClusterRoleBinding{}
			k8sAuthBindingNsdName := types.NamespacedName{Name: authorinoK8sAuthClusterRoleBindingName}

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), k8sAuthBindingNsdName, k8sAuthBinding)
				return err == nil
			}, testTimeout, testInterval).Should(BeTrue())

			// Authorino leaderElection ClusterRoleBinding
			leaderElectionRole := &k8srbac.Role{}
			leaderElectionNsdName := namespacedName(testAuthorinoNamespace, authorinoLeaderElectionRoleName)
			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), leaderElectionNsdName, leaderElectionRole)
				return err == nil
			}, testTimeout, testInterval).Should(BeTrue())
		})

		It("Should create authorino deployment", func() {
			deployment := &k8sapps.Deployment{}

			nsdName := namespacedName(testAuthorinoNamespace, authorinoInstance.Name)

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), nsdName, deployment)
				return err == nil
			}, testTimeout, testInterval).Should(BeTrue())

			replicas := int32(testAuthorinoReplicas)
			image := defaultAuthorinoImage
			existContainer := false

			Expect(deployment.Spec.Replicas).Should(Equal(&replicas))
			Expect(deployment.Labels).Should(Equal(map[string]string{"thisLabel": "willPropagate"}))
			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == authorinoContainerName {
					Expect(container.Image).Should(Equal(image))
					Expect(container.ImagePullPolicy).Should(Equal(k8score.PullAlways))
					checkAuthorinoArgs(authorinoInstance, container.Args)
					Expect(len(container.Env)).Should(Equal(0))
					existContainer = true
				}
			}
			Expect(existContainer).To(BeTrue())
		})
	})

	Context("Updating a instance of authorino object", func() {
		var authorinoInstance *api.Authorino

		BeforeEach(func() {
			authorinoInstance = newFullAuthorinoInstance()
			Expect(k8sClient.Create(context.TODO(), authorinoInstance)).Should(Succeed())
		})

		It("Should change the number of replicas", func() {
			existingAuthorinoInstance := &api.Authorino{}

			nsdName := namespacedName(testAuthorinoNamespace, authorinoInstance.Name)

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), nsdName, existingAuthorinoInstance)
				return err == nil
			}, testTimeout, testInterval).Should(BeTrue())

			replicas := int32(testAuthorinoReplicas + 1)
			existingAuthorinoInstance.Spec.Replicas = &replicas
			existingAuthorinoInstance.Spec.LogLevel = "debug"
			Expect(k8sClient.Update(context.TODO(), existingAuthorinoInstance)).Should(Succeed())

			desiredDevelopment := &k8sapps.Deployment{}

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					nsdName,
					desiredDevelopment)
				return err == nil
			}, testTimeout, testInterval).Should(BeTrue())

			Expect(desiredDevelopment.Spec.Replicas).Should(Equal(&replicas))
			for _, container := range desiredDevelopment.Spec.Template.Spec.Containers {
				if container.Name == authorinoContainerName {
					checkAuthorinoArgs(existingAuthorinoInstance, container.Args)
					Expect(len(container.Env)).Should(Equal(0))
				}
			}
		})
	})

	Context("Deploy an old version of Authorino", func() {
		var authorinoInstance *api.Authorino

		BeforeEach(func() {
			authorinoInstance = newFullAuthorinoInstance()
			authorinoInstance.Spec.Image = "quay.io/kuadrant/authorino:v0.8.0"
			Expect(k8sClient.Create(context.TODO(), authorinoInstance)).Should(Succeed())
		})

		It("Should have injected env vars", func() {
			deployment := &k8sapps.Deployment{}
			nsdName := namespacedName(testAuthorinoNamespace, authorinoInstance.Name)

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(), nsdName, deployment)
				return err == nil
			}, testTimeout, testInterval).Should(BeTrue())

			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == authorinoContainerName {
					checkAuthorinoEnvVar(authorinoInstance, container.Env)
					Expect(len(container.Args) <= 2).Should(BeTrue())
				}
			}
		})
	})
})

var _ = Describe("Detect Authorino old version", func() {
	// old authorino versions
	Expect(detectEnvVarAuthorinoVersion("v0.9.0")).Should(BeTrue())
	Expect(detectEnvVarAuthorinoVersion("v0.10.0")).Should(BeTrue())
	Expect(detectEnvVarAuthorinoVersion("v0.10.11")).Should(BeTrue())

	// new authorino versions
	Expect(detectEnvVarAuthorinoVersion("v0.11.0")).Should(BeFalse())

	// undetectable authorino versions
	Expect(detectEnvVarAuthorinoVersion("latest")).Should(BeFalse())
	Expect(detectEnvVarAuthorinoVersion("3ba0baa64b9b86a0a197e28fcb269a07cbae8e04")).Should(BeFalse())
	Expect(detectEnvVarAuthorinoVersion("git-ref-name")).Should(BeFalse())
	Expect(detectEnvVarAuthorinoVersion("very.weird.version")).Should(BeFalse())
})

func newExtServerConfigMap() *k8score.ConfigMap {
	return &k8score.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: "external-metadata-server-tls",
		},
		Data: map[string]string{
			"tls.crt": "-----BEGIN CERTIFICATE-----\nMIIGwjCCBKqgAwIBAgIUc13V+5zSFtQhEdAzXhtVXXh3D3MwDQYJKoZIhvcNAQEL\nBQAwgaIxCzAJBgNVBAYTAkVTMRIwEAYDVQQIDAlCYXJjZWxvbmExEjAQBgNVBAcM\nCUJhcmNlbG9uYTEWMBQGA1UECgwNUmVkIEhhdCwgSW5jLjEXMBUGA1UECwwOUmVk\nIEhhdCAzc2NhbGUxOjA4BgNVBAMMMUtleWNsb2FrIFNlcnZlciBvbiAzc2NhbGUg\nT3BlblNoaWZ0IGRldmVsIGNsdXN0ZXIwHhcNMjExMjE2MTkyMDA3WhcNMzExMjE0\nMTkyMDA3WjCBojELMAkGA1UEBhMCRVMxEjAQBgNVBAgMCUJhcmNlbG9uYTESMBAG\nA1UEBwwJQmFyY2Vsb25hMRYwFAYDVQQKDA1SZWQgSGF0LCBJbmMuMRcwFQYDVQQL\nDA5SZWQgSGF0IDNzY2FsZTE6MDgGA1UEAwwxS2V5Y2xvYWsgU2VydmVyIG9uIDNz\nY2FsZSBPcGVuU2hpZnQgZGV2ZWwgY2x1c3RlcjCCAiIwDQYJKoZIhvcNAQEBBQAD\nggIPADCCAgoCggIBAL1aPyDtqDBNziWLA2AhYPlOq4VBtnSNZJYwxWb1PMzZDw2M\nQxcaN+2/TGrFELv9RLFmJTYd9yMXk6ASJnx513bEqcMp4le2lREF+hUNFVNjQcF7\n3peoJNe06NcZIbLmCwJ8lR7SQD+lhjqr7rqsr9/+q9ZxCAMuCIkhF4BcBQV9Q2uH\n7juhJ0fEUOofqXfdGlyhTLecqQzfw/ZWEDc+uJWFWMB5OdBYJAphwIpyu6dFh245\nInuIHkO17MmFEWJX1HjkTNgIS+JHfJNmlwUBEG9d5/Lwy/NmLMnif6zdHfyjhEHv\nb0GI9n9zu1n6tcOpXSRL9bhYWYY9jxnVxZ2ubsKT0BZe8KHJDGdU1sOX6TWSA8zL\nDN2mIxQvPjGPq36pX32fesg+jUb2Y1ZEbXlrCm25K3L/TNe5G8EolowCd9EwyuYk\nwf1JlU2wO1zd1Y3V7/b3kHyQ4xlr9hjwnc4xcbZV3FGVyasxvtykvsgT3XtHroE9\nrqXcT+Rh6hMSIUFSWqIyON1h6ft8VPZjVhu51JdYk7h2VWFPsEzGi7SSU+f7Zdzj\nZ/9hyDINbUlHbluCBJxiTJb7Ig4t+XPj5etL0yvBh3/MLSHO9CCF8auGCmbTPR2/\njNESuJAs18uRA15EqqHGa0hC4NHuQqxRGsVgIxLKGi9kdPFvWI8pcCYw199RAgMB\nAAGjge0wgeowHQYDVR0OBBYEFCHy/ieeCgOXZvGrM/Qhvp5+Jt7IMB8GA1UdIwQY\nMBaAFCHy/ieeCgOXZvGrM/Qhvp5+Jt7IMA8GA1UdEwEB/wQFMAMBAf8wgZYGA1Ud\nEQSBjjCBi4I7a2V5Y2xvYWstYXBpY3VyaW8tcmVnaXN0cnkuYXBwcy5kZXYtZW5n\nLW9jcDQtOC5kZXYuM3NjYS5uZXSCHmtleWNsb2FrLWFwaWN1cmlvLXJlZ2lzdHJ5\nLnN2Y4Isa2V5Y2xvYWstYXBpY3VyaW8tcmVnaXN0cnkuc3ZjLmNsdXN0ZXIubG9j\nYWwwDQYJKoZIhvcNAQELBQADggIBAG5Dim4JDcYWeLrLyFs6byyV641FIaIRUlcd\ndj7L61LfjCMC7kjhl7ynLjiMxCtRBB04h56xGtncDG8kFFOAT26caNSkWzNnDFXI\n026gMSaamioqXoEKlRjbp2Lf+cLzqpaMN0vXJxdHoBrg74h7uptWkyWMqHVmaFy8\nlLi6T2ET9q/vXDPzKHHjwaN4KynRKgYfShY/UE3G/WmvstrrHF8zWQz5JN0TPhuv\n31LuSJkq1yRA9HNrLpBK685WYZ9vyPs+KUcG84sjTf1aaO8beAppYJc94knO28PA\nObT6YGQW1RxjH1XiCHFGXF5KL9HXMFfOpLK/FlFt5gUxUlqCKncK1ilyiRtNaNKZ\npJsmBnqPVV/ZbgR/Y1l1ucUT9OoEsPOPC/nBzQj4nue7seACGD9HJlapQml75Ix6\n5Ypmq+KyDU8GX+ejbeTnFY84xNqZPQhE7/lbTHKPj6zLD98IQt4FvOmKzdfZUhIG\nP8iWHYvV5NQ4XQUxu0s0kWJhSuTDZmrg9HtlXD2x1zi8ilAKCoJ7nu/avLvHemO5\nBgNixHMHTILZrd2xZ9xjyNPGi92EDK+WG6BHD3JAgLvbcBqB4eAi9EONj7qmw3Ry\n6FlViwpQjDQf3Aj2JZvGgqtCrj5TlvMXiwTdE3p29JTSiY9JE8jJVuqv93Af/HZJ\njqr1zGh3\n-----END CERTIFICATE-----",
		},
	}
}

func newFullAuthorinoInstance() *api.Authorino {
	name := "a" + string(uuid.NewUUID())
	image := defaultAuthorinoImage
	replicas := int32(testAuthorinoReplicas)
	tslEnable := true
	tlsDisabled := false
	portGRPC := int32(30051)
	portHTTP := int32(3000)
	cacheSize := 10
	secretName := "bkabk"
	label := "authorino"
	return &api.Authorino{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: testAuthorinoNamespace,
			Labels:    map[string]string{"thisLabel": "willPropagate"},
		},
		Spec: api.AuthorinoSpec{
			Image:           image,
			Replicas:        &replicas,
			ImagePullPolicy: string(k8score.PullAlways),
			Volumes: api.VolumesSpec{
				Items: []api.VolumeSpec{
					{
						Name:       "external-metadata-server-tls-cert",
						MountPath:  "/etc/ssl/certs",
						ConfigMaps: []string{"external-metadata-server-tls"},
						Items: []k8score.KeyToPath{
							{
								Key:  "tls.crt",
								Path: "tls.crt",
							},
						},
					},
				},
			},
			LogLevel:                 "info",
			LogMode:                  "production",
			ClusterWide:              false,
			AuthConfigLabelSelectors: label,
			SecretLabelSelectors:     label,
			EvaluatorCacheSize:       &cacheSize,
			Listener: api.Listener{
				Tls: api.Tls{
					Enabled: &tlsDisabled,
				},
				Ports: api.Ports{
					GRPC: &portGRPC,
					HTTP: &portHTTP,
				},
			},
			OIDCServer: api.OIDCServer{
				Port: &portHTTP,
				Tls: api.Tls{
					Enabled: &tslEnable,
					CertSecret: &k8score.LocalObjectReference{
						Name: secretName,
					},
				},
			},
		},
	}
}

func checkAuthorinoArgs(authorinoInstance *api.Authorino, args []string) {
	tslEnable := true

	for _, arg := range args {
		flagAndValue := strings.Split(strings.TrimPrefix(arg, "--"), "=")
		flag := flagAndValue[0]
		var value string
		if len(flagAndValue) > 1 {
			value = flagAndValue[1]
		}

		switch flag {
		case flagWatchNamespace:
			Expect(authorinoInstance.Spec.ClusterWide).To(BeFalse())
			Expect(value).Should(Equal(testAuthorinoNamespace))
		case flagWatchedAuthConfigLabelSelector:
			Expect(value).Should(Equal(authorinoInstance.Spec.AuthConfigLabelSelectors))
		case flagWatchedSecretLabelSelector:
			Expect(value).Should(Equal(authorinoInstance.Spec.SecretLabelSelectors))
		case flagLogLevel:
			Expect(value).Should(Equal(authorinoInstance.Spec.LogLevel))
		case flagLogMode:
			Expect(value).Should(Equal(authorinoInstance.Spec.LogMode))
		case flagTimeout:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Timeout)))
		case flagExtAuthGRPCPort:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.GRPC)))
		case flagExtAuthHTTPPort:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.HTTP)))
		case flagTlsCertPath, flagTlsCertKeyPath:
			Expect(authorinoInstance.Spec.Listener.Tls.Enabled).Should(SatisfyAny(BeNil(), Equal(&tslEnable)))
			Expect(value).Should(SatisfyAny(Equal(defaultTlsCertPath), Equal(defaultTlsCertKeyPath)))
		case flagOidcHTTPPort:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.OIDCServer.Port)))
		case flagOidcTLSCertPath, flagOidcTLSCertKeyPath:
			Expect(authorinoInstance.Spec.OIDCServer.Tls.Enabled).To(SatisfyAny(Equal(&tslEnable), BeNil()))
			Expect(value).Should(SatisfyAny(Equal(defaultOidcTlsCertPath), Equal(defaultOidcTlsCertKeyPath)))
		case flagEvaluatorCacheSize:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.EvaluatorCacheSize)))
		case flagDeepMetricsEnabled:
			Expect(authorinoInstance.Spec.Metrics.DeepMetricsEnabled).ShouldNot(BeNil())
			Expect(*authorinoInstance.Spec.Metrics.DeepMetricsEnabled).Should(BeTrue())
		case flagMetricsAddr:
			metricsAddr := fmt.Sprintf(":%d", defaultMetricsServicePort)
			if port := authorinoInstance.Spec.Metrics.Port; port != nil {
				metricsAddr = fmt.Sprintf(":%d", *port)
			}
			Expect(value).Should(Equal(metricsAddr))
		case flagHealthProbeAddr:
			healthProbeAddr := fmt.Sprintf(":%d", defaultHealthProbePort)
			if port := authorinoInstance.Spec.Healthz.Port; port != nil {
				healthProbeAddr = fmt.Sprintf(":%d", *port)
			}
			Expect(value).Should(Equal(healthProbeAddr))
		case flagEnableLeaderElection:
			replicas := authorinoInstance.Spec.Replicas
			if replicas == nil {
				value := int32(0)
				replicas = &value
			}
			Expect(*replicas > 1).Should(BeTrue())
		case flagMaxHttpRequestBodySize:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.MaxHttpRequestBodySize)))
		}
	}
}

func checkAuthorinoEnvVar(authorinoInstance *api.Authorino, envs []k8score.EnvVar) {
	tslEnable := true

	for _, env := range envs {
		switch env.Name {
		case envWatchNamespace:
			Expect(authorinoInstance.Spec.ClusterWide).To(BeFalse())
			Expect(env.Value).Should(Equal(testAuthorinoNamespace))
		case envAuthConfigLabelSelector:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.AuthConfigLabelSelectors))
		case envSecretLabelSelector:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.SecretLabelSelectors))
		case envEvaluatorCacheSize:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.EvaluatorCacheSize)))
		case envDeepMetricsEnabled:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Metrics.DeepMetricsEnabled)))
		case envLogLevel:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.LogLevel))
		case envLogMode:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.LogMode))
		case envExtAuthGRPCPort:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.GRPC)))
		case envExtAuthHTTPPort:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.HTTP)))
		case envTlsCert, envTlsCertKey:
			Expect(authorinoInstance.Spec.Listener.Tls.Enabled).Should(SatisfyAny(BeNil(), Equal(&tslEnable)))
			Expect(env.Value).Should(SatisfyAny(Equal(defaultTlsCertPath), Equal(defaultTlsCertKeyPath)))
		case envTimeout:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Timeout)))
		case envMaxHttpRequestBodySize:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.MaxHttpRequestBodySize)))
		case envOIDCHTTPPort:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.OIDCServer.Port)))
		case envOidcTlsCertPath, envOidcTlsCertKeyPath:
			Expect(authorinoInstance.Spec.OIDCServer.Tls.Enabled).To(SatisfyAny(Equal(&tslEnable), BeNil()))
			Expect(env.Value).Should(SatisfyAny(Equal(defaultOidcTlsCertPath), Equal(defaultOidcTlsCertKeyPath)))
		}
	}
}

func newAuthorinoClusterRolebinding(roleBindingName string, clusterScoped bool, clusterRoleName string, serviceAccount k8score.ServiceAccount, authorino *api.Authorino) client.Object {
	var binding client.Object
	if clusterScoped {
		binding = authorinoResources.GetAuthorinoClusterRoleBinding(roleBindingName, clusterRoleName, serviceAccount)
	} else {
		binding = authorinoResources.GetAuthorinoRoleBinding(
			authorino.Namespace,
			authorino.Name,
			roleBindingName,
			"ClusterRole",
			clusterRoleName,
			serviceAccount,
			authorino.Labels,
		)
		binding.SetNamespace(authorino.Namespace)
	}

	return binding
}
