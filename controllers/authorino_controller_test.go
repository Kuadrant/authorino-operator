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

			var binding client.Object = &k8srbac.ClusterRoleBinding{}
			if !authorinoInstance.Spec.ClusterWide {
				binding = &k8srbac.RoleBinding{}
			}
			bindingNsdName := namespacedName(AuthorinoNamespace, authorinoInstance.Name+"-authorino")

			Eventually(func() bool {
				err := k8sClient.Get(context.TODO(),
					bindingNsdName,
					binding)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			leaderElectionRole := &k8srbac.Role{}
			leaderElectionNsdName := namespacedName(AuthorinoNamespace, leaderElectionRoleName)
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
			Image:                    image,
			Replicas:                 &replicas,
			ImagePullPolicy:          string(k8score.PullAlways),
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
