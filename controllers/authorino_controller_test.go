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
)

const (
	AuthorinoNamespace = "default"
	AuthorinoReplicas  = 1
	AuthorinoImage     = "quay.io/3scale/authorino"

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
		})

		It("Should create authorino required services", func() {
			servicesName := []string{"authorino-authorization", "authorino-oidc", "controller-metrics"}

			for _, serviceName := range servicesName {
				service := k8score.Service{}

				nsdName := types.NamespacedName{
					Namespace: AuthorinoNamespace,
					Name:      serviceName + "-" + authorinoInstance.Name,
				}

				Eventually(func() bool {
					err := k8sClient.Get(context.TODO(),
						nsdName,
						&service)
					return err == nil
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("Should create authorino permission", func() {
			sa := &k8score.ServiceAccount{}
			nsdName := namespacedName(AuthorinoNamespace, authorinoInstance.Name+"-authorino")

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					nsdName,
					sa)
				return err == nil
			}, timeout, interval).Should(BeTrue())

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

			k8sAuthBinding := &k8srbac.ClusterRoleBinding{}
			k8sAuthBindingNsdName := types.NamespacedName{Name: "authorino-k8s-auth"}

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					k8sAuthBindingNsdName,
					k8sAuthBinding)
				return err == nil
			}, timeout, interval).Should(BeTrue())

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
			image := fmt.Sprintf("%s:%s", AuthorinoImage, api.AuthorinoVersion)
			existContainer := false

			Expect(deployment.Spec.Replicas).Should(Equal(&replicas))
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
	image := fmt.Sprintf("%s:%s", AuthorinoImage, api.AuthorinoVersion)
	replicas := int32(AuthorinoReplicas)
	tslEnable := true
	port := int32(1000)
	secretName := "bkabk"
	label := "authorino"
	return &api.Authorino{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: AuthorinoNamespace,
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
			Listener: api.Listener{
				Port: &port,
				Tls: api.Tls{
					CertSecret: &k8score.LocalObjectReference{
						Name: secretName,
					},
				},
			},
			OIDCServer: api.OIDCServer{
				Port: &port,
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
		if env.Name == api.WatchNamespace {
			Expect(authorinoInstance.Spec.ClusterWide).To(BeFalse())
			Expect(env.Value).Should(Equal(AuthorinoNamespace))
		}
		if env.Name == api.AuthConfigLabelSelector {
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.AuthConfigLabelSelectors))
		}
		if env.Name == api.EnvLogLevel {
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.LogLevel))
		}
		if env.Name == api.EnvLogMode {
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.LogMode))
		}
		if env.Name == api.SecretLabelSelector {
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.SecretLabelSelectors))
		}

		if env.Name == api.ExtAuthGRPCPort {
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Port)))
		}
		if env.Name == api.EnvVarTlsCert || env.Name == api.EnvVarTlsCertKey {
			Expect(authorinoInstance.Spec.Listener.Tls.Enabled).Should(SatisfyAny(
				BeNil(), Equal(&tslEnable),
			))
			Expect(env.Value).Should(SatisfyAny(
				Equal(api.DefaultTlsCertPath), Equal(api.DefaultTlsCertKeyPath)))
		}

		if env.Name == api.EnvVarOidcTlsCertPath || env.Name == api.EnvVarOidcTlsCertKeyPath {
			Expect(authorinoInstance.Spec.OIDCServer.Tls.Enabled).To(SatisfyAny(
				Equal(&tslEnable), BeNil(),
			))
			Expect(env.Value).Should(SatisfyAny(
				Equal(api.DefaultOidcTlsCertPath), Equal(api.DefaultOidcTlsCertKeyPath),
			))
		}
	}
}
