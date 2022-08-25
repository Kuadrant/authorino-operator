package controllers

import (
	"context"
	"fmt"
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
	AuthorinoNamespace = "default"
	AuthorinoReplicas  = 1

	timeout  = time.Minute * 1
	interval = time.Second * 5
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
				err := k8sClient.Get(context.TODO(),
					nsdName,
					&authorino)
				return err == nil && authorinoInstance.Status.Ready()
			}, timeout, interval).Should(BeFalse())
		})

		It("Should create authorino required services", func() {
			desiredServices := []*k8score.Service{
				authorinoResources.NewOIDCService(authorinoInstance.Name, authorinoInstance.Namespace, api.DefaultOIDCServicePort, authorinoInstance.Labels),
				authorinoResources.NewMetricsService(authorinoInstance.Name, authorinoInstance.Namespace, api.DefaultMetricsServicePort, authorinoInstance.Labels),
				authorinoResources.NewAuthService(authorinoInstance.Name, authorinoInstance.Namespace, api.DefaultAuthGRPCServicePort, api.DefaultAuthHTTPServicePort, authorinoInstance.Labels),
			}

			for _, service := range desiredServices {
				nsdName := namespacedName(service.GetNamespace(), service.GetName())

				Eventually(func() bool {
					err := k8sClient.Get(context.TODO(),
						nsdName,
						&k8score.Service{})
					return err == nil
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("Should create authorino permission", func() {

			// service account
			sa := authorinoResources.GetAuthorinoServiceAccount(AuthorinoNamespace, authorinoInstance.Name, authorinoInstance.Labels)
			nsdName := namespacedName(sa.GetNamespace(), sa.GetName())
			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					nsdName,
					sa)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Authorino ClusterRoleBinding
			var binding client.Object
			var bindingNsdName types.NamespacedName
			if authorinoInstance.Spec.ClusterWide {
				binding = &k8srbac.ClusterRoleBinding{}
				bindingNsdName = types.NamespacedName{Name: "authorino"}
			} else {
				binding = &k8srbac.RoleBinding{}
				bindingNsdName = namespacedName(AuthorinoNamespace, authorinoInstance.Name+"-authorino")
			}

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					bindingNsdName,
					binding)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Authorino Auth ClusterRoleBinding
			k8sAuthBinding := &k8srbac.ClusterRoleBinding{}
			k8sAuthBindingNsdName := types.NamespacedName{Name: authorinoK8sAuthClusterRoleBindingName}

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					k8sAuthBindingNsdName,
					k8sAuthBinding)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Authorino leaderElection ClusterRoleBinding
			leaderElectionRole := &k8srbac.Role{}
			leaderElectionNsdName := namespacedName(AuthorinoNamespace, authorinoLeaderElectionRoleName)
			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					leaderElectionNsdName,
					leaderElectionRole)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		})

		It("Should create authorino deployment", func() {
			deployment := &k8sapps.Deployment{}

			nsdName := namespacedName(AuthorinoNamespace, authorinoInstance.Name)

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					nsdName,
					deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			replicas := int32(AuthorinoReplicas)
			image := api.DefaultAuthorinoImage
			existContainer := false

			Expect(deployment.Spec.Replicas).Should(Equal(&replicas))
			Expect(deployment.Labels).Should(Equal(map[string]string{"thisLabel": "willPropagate"}))
			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == api.AuthorinoContainerName {
					Expect(container.Image).Should(Equal(image))
					Expect(container.ImagePullPolicy).Should(Equal(k8score.PullAlways))
					checkAuthorinoEnvVar(authorinoInstance, container.Env)
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

			nsdName := namespacedName(AuthorinoNamespace, authorinoInstance.Name)

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					nsdName,
					existingAuthorinoInstance)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			replicas := int32(AuthorinoReplicas + 1)
			existingAuthorinoInstance.Spec.Replicas = &replicas
			existingAuthorinoInstance.Spec.LogLevel = "debug"
			Expect(k8sClient.Update(context.TODO(), existingAuthorinoInstance)).Should(Succeed())

			desiredDevelopment := &k8sapps.Deployment{}

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					nsdName,
					desiredDevelopment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(desiredDevelopment.Spec.Replicas).Should(Equal(&replicas))
			for _, container := range desiredDevelopment.Spec.Template.Spec.Containers {
				if container.Name == api.AuthorinoContainerName {
					checkAuthorinoEnvVar(existingAuthorinoInstance, container.Env)
				}
			}
		})
	})
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
	image := api.DefaultAuthorinoImage
	replicas := int32(AuthorinoReplicas)
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
			Namespace: AuthorinoNamespace,
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

func checkAuthorinoEnvVar(authorinoInstance *api.Authorino, envs []k8score.EnvVar) {
	tslEnable := true

	for _, env := range envs {
		switch env.Name {
		case api.EnvWatchNamespace:
			Expect(authorinoInstance.Spec.ClusterWide).To(BeFalse())
			Expect(env.Value).Should(Equal(AuthorinoNamespace))
		case api.EnvAuthConfigLabelSelector:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.AuthConfigLabelSelectors))
		case api.EnvSecretLabelSelector:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.SecretLabelSelectors))
		case api.EnvEvaluatorCacheSize:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.EvaluatorCacheSize)))
		case api.EnvDeepMetricsEnabled:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Metrics.DeepMetricsEnabled)))
		case api.EnvLogLevel:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.LogLevel))
		case api.EnvLogMode:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.LogMode))
		case api.EnvExtAuthGRPCPort:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.GRPC)))
		case api.EnvExtAuthHTTPPort:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.HTTP)))
		case api.EnvTlsCert, api.EnvTlsCertKey:
			Expect(authorinoInstance.Spec.Listener.Tls.Enabled).Should(SatisfyAny(
				BeNil(), Equal(&tslEnable),
			))
			Expect(env.Value).Should(SatisfyAny(
				Equal(api.DefaultTlsCertPath), Equal(api.DefaultTlsCertKeyPath)))
		case api.EnvTimeout:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Timeout)))
		case api.EnvMaxHttpRequestBodySize:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.MaxHttpRequestBodySize)))
		case api.EnvOIDCHTTPPort:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.OIDCServer.Port)))
		case api.EnvOidcTlsCertPath, api.EnvOidcTlsCertKeyPath:
			Expect(authorinoInstance.Spec.OIDCServer.Tls.Enabled).To(SatisfyAny(
				Equal(&tslEnable), BeNil(),
			))
			Expect(env.Value).Should(SatisfyAny(
				Equal(api.DefaultOidcTlsCertPath), Equal(api.DefaultOidcTlsCertKeyPath),
			))
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
