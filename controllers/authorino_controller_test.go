package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/env"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	"github.com/kuadrant/authorino-operator/pkg/reconcilers"
	authorinoResources "github.com/kuadrant/authorino-operator/pkg/resources"
)

const (
	testAuthorinoNamespace = "default"
	testAuthorinoReplicas  = 1

	testTimeout  = time.Minute * 1
	testInterval = time.Second * 5
)

var _ = Describe("Authorino controller", func() {

	SetDefaultEventuallyTimeout(testTimeout)
	SetDefaultEventuallyPollingInterval(testInterval)

	Context("Creating an new instance of authorino", func() {
		var authorinoInstance *api.Authorino

		BeforeEach(func(ctx context.Context) {

			createOrUpdateCfgMap := func(ctx context.Context) error {
				_, err := controllerruntime.CreateOrUpdate(ctx, k8sClient, newExtServerConfigMap(), func() error {
					return nil // noop as we pass entire object
				})

				return err
			}

			Expect(createOrUpdateCfgMap(ctx)).To(Succeed())

			authorinoInstance = newFullAuthorinoInstance()
			Expect(k8sClient.Create(ctx, authorinoInstance)).To(Succeed())
		})

		It("Should create authorino required services", func(ctx context.Context) {
			desiredServices := []*k8score.Service{
				authorinoResources.NewOIDCService(authorinoInstance.Name, authorinoInstance.Namespace, reconcilers.DefaultOIDCServicePort, authorinoInstance.Labels),
				authorinoResources.NewMetricsService(authorinoInstance.Name, authorinoInstance.Namespace, reconcilers.DefaultMetricsServicePort, authorinoInstance.Labels),
				authorinoResources.NewAuthService(authorinoInstance.Name, authorinoInstance.Namespace, reconcilers.DefaultAuthGRPCServicePort, reconcilers.DefaultAuthHTTPServicePort, authorinoInstance.Labels),
			}

			for _, service := range desiredServices {
				nsdName := namespacedName(service.GetNamespace(), service.GetName())
				clusterService := &k8score.Service{}
				Eventually(func(ctx context.Context) error {
					return k8sClient.Get(ctx, nsdName, clusterService)
				}).WithContext(ctx).Should(Succeed())

				Expect(clusterService.Labels).ShouldNot(HaveKeyWithValue("control-plane", "controller-manager"))
				Expect(clusterService.Labels).ShouldNot(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))
				Expect(clusterService.Labels).Should(HaveKeyWithValue("thisLabel", "willPropagate"))

				Expect(clusterService.Spec.Selector).Should(HaveKeyWithValue("control-plane", "controller-manager"))
				Expect(clusterService.Spec.Selector).Should(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))
			}
		})

		It("Should create authorino permission", func(ctx context.Context) {
			// service account
			sa := authorinoResources.GetAuthorinoServiceAccount(testAuthorinoNamespace, authorinoInstance.Name, authorinoInstance.Labels)
			nsdName := namespacedName(sa.GetNamespace(), sa.GetName())
			Eventually(func(ctx context.Context) bool {
				err := k8sClient.Get(ctx, nsdName, sa)
				return err == nil
			}).WithContext(ctx).Should(BeTrue())

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

			Eventually(func() error {
				return k8sClient.Get(ctx, bindingNsdName, binding)
			}).WithContext(ctx).Should(Succeed())

			// Authorino Auth ClusterRoleBinding
			k8sAuthBinding := &k8srbac.ClusterRoleBinding{}
			k8sAuthBindingNsdName := types.NamespacedName{Name: reconcilers.AuthorinoK8sAuthClusterRoleBindingName}

			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx, k8sAuthBindingNsdName, k8sAuthBinding)
			}).WithContext(ctx).Should(Succeed())

			// Authorino leaderElection ClusterRoleBinding
			leaderElectionRole := &k8srbac.Role{}
			leaderElectionNsdName := namespacedName(testAuthorinoNamespace, reconcilers.AuthorinoLeaderElectionRoleName)
			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx, leaderElectionNsdName, leaderElectionRole)
			}).WithContext(ctx).Should(Succeed())
		})

		It("Should create authorino deployment", func(ctx context.Context) {
			deployment := &k8sapps.Deployment{}

			nsdName := namespacedName(testAuthorinoNamespace, authorinoInstance.Name)

			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx, nsdName, deployment)
			}).WithContext(ctx).Should(Succeed())

			replicas := int32(testAuthorinoReplicas)
			image := authorinoInstance.Spec.Image
			existContainer := false

			Expect(deployment.Spec.Replicas).To(Equal(&replicas))

			Expect(deployment.Labels).Should(Equal(map[string]string{"thisLabel": "willPropagate"}))
			Expect(deployment.Spec.Selector.MatchLabels).ShouldNot(HaveKeyWithValue("thisLabel", "willPropagate"))
			Expect(deployment.Spec.Template.Labels).ShouldNot(HaveKeyWithValue("thisLabel", "willPropagate"))

			Expect(deployment.Labels).ShouldNot(HaveKeyWithValue("control-plane", "controller-manager"))
			Expect(deployment.Spec.Selector.MatchLabels).Should(HaveKeyWithValue("control-plane", "controller-manager"))
			Expect(deployment.Spec.Template.Labels).Should(HaveKeyWithValue("control-plane", "controller-manager"))

			Expect(deployment.Labels).ShouldNot(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))
			Expect(deployment.Spec.Selector.MatchLabels).Should(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))
			Expect(deployment.Spec.Template.Labels).Should(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))
			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == reconcilers.AuthorinoContainerName {
					if image == "" {
						image = env.GetString(reconcilers.RelatedImageAuthorino, reconcilers.DefaultAuthorinoImage)
					}
					Expect(container.Image).Should(Equal(image))
					Expect(container.ImagePullPolicy).Should(Equal(k8score.PullAlways))
					checkAuthorinoArgs(authorinoInstance, container.Args)
					Expect(container.Env).To(BeEmpty())
					existContainer = true
				}
			}
			Expect(existContainer).To(BeTrue())
		})
	})

	Context("Updating a instance of authorino object", func() {
		var authorinoInstance *api.Authorino

		BeforeEach(func(ctx context.Context) {
			authorinoInstance = newFullAuthorinoInstance()
			Expect(k8sClient.Create(ctx, authorinoInstance)).Should(Succeed())
		})

		It("Should change the number of replicas", func() {
			existingAuthorinoInstance := &api.Authorino{}

			nsdName := namespacedName(testAuthorinoNamespace, authorinoInstance.Name)

			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx, nsdName, existingAuthorinoInstance)
			}).WithContext(ctx).Should(Succeed())

			replicas := int32(testAuthorinoReplicas + 1)
			existingAuthorinoInstance.Spec.Replicas = &replicas
			existingAuthorinoInstance.Spec.LogLevel = "debug"
			Expect(k8sClient.Update(context.TODO(), existingAuthorinoInstance)).Should(Succeed())

			desiredDeployment := &k8sapps.Deployment{}

			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx,
					nsdName,
					desiredDeployment)
			}).WithContext(ctx).Should(Succeed())

			Expect(desiredDeployment.Spec.Replicas).Should(Equal(&replicas))
			for _, container := range desiredDeployment.Spec.Template.Spec.Containers {
				if container.Name == reconcilers.AuthorinoContainerName {
					checkAuthorinoArgs(existingAuthorinoInstance, container.Args)
					Expect(container.Env).To(BeEmpty())
				}
			}
		})
	})

	Context("Updating the labels on the deployment", func() {
		var authorinoInstance *api.Authorino

		BeforeEach(func() {
			_ = k8sClient.Create(context.TODO(), newExtServerConfigMap())

			authorinoInstance = newFullAuthorinoInstance()
			Expect(k8sClient.Create(context.TODO(), authorinoInstance)).Should(Succeed())
		})

		It("Should not have the label removed", func() {

			desiredDeployment := &k8sapps.Deployment{}
			nsdName := namespacedName(testAuthorinoNamespace, authorinoInstance.Name)

			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx,
					nsdName,
					desiredDeployment)
			}).WithContext(ctx).Should(Succeed())
			Expect(desiredDeployment.Spec.Template.Labels).ShouldNot(HaveKeyWithValue("user-added-label", "value"))
			Expect(desiredDeployment.Labels).ShouldNot(HaveKeyWithValue("user-added-label", "value"))
			Expect(desiredDeployment.Spec.Selector.MatchLabels).ShouldNot(HaveKeyWithValue("user-added-label", "value"))
			Expect(desiredDeployment.Labels).ShouldNot(HaveKeyWithValue("control-plane", "controller-manager"))
			Expect(desiredDeployment.Spec.Selector.MatchLabels).Should(HaveKeyWithValue("control-plane", "controller-manager"))
			Expect(desiredDeployment.Spec.Template.Labels).Should(HaveKeyWithValue("control-plane", "controller-manager"))

			Expect(desiredDeployment.Labels).ShouldNot(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))
			Expect(desiredDeployment.Spec.Selector.MatchLabels).Should(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))
			Expect(desiredDeployment.Spec.Template.Labels).Should(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))

			desiredDeployment.Spec.Template.Labels["user-added-label"] = "value"
			Expect(k8sClient.Update(context.TODO(), desiredDeployment)).Should(Succeed())

			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx,
					nsdName,
					authorinoInstance)
			}).WithContext(ctx).Should(Succeed())
			replicas := int32(testAuthorinoReplicas + 1)
			authorinoInstance.Spec.Replicas = &replicas
			Expect(k8sClient.Update(context.TODO(), authorinoInstance)).Should(Succeed())

			updatedDeployment := &k8sapps.Deployment{}

			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx,
					nsdName,
					updatedDeployment)
			}).WithContext(ctx).Should(Succeed())
			Expect(updatedDeployment.Spec.Template.Labels).Should(HaveKeyWithValue("user-added-label", "value"))
			Expect(updatedDeployment.Labels).ShouldNot(HaveKeyWithValue("user-added-label", "value"))
			Expect(updatedDeployment.Spec.Selector.MatchLabels).ShouldNot(HaveKeyWithValue("user-added-label", "value"))

			Expect(updatedDeployment.Labels).ShouldNot(HaveKeyWithValue("control-plane", "controller-manager"))
			Expect(updatedDeployment.Spec.Selector.MatchLabels).Should(HaveKeyWithValue("control-plane", "controller-manager"))
			Expect(updatedDeployment.Spec.Template.Labels).Should(HaveKeyWithValue("control-plane", "controller-manager"))

			Expect(updatedDeployment.Labels).ShouldNot(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))
			Expect(updatedDeployment.Spec.Selector.MatchLabels).Should(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))
			Expect(updatedDeployment.Spec.Template.Labels).Should(HaveKeyWithValue("authorino-resource", authorinoInstance.Name))

		})
	})

	Context("Deploy an old version of Authorino", func() {
		var authorinoInstance *api.Authorino

		BeforeEach(func(ctx context.Context) {
			authorinoInstance = newFullAuthorinoInstance()
			authorinoInstance.Spec.Image = "quay.io/kuadrant/authorino:v0.8.0"
			Expect(k8sClient.Create(ctx, authorinoInstance)).To(Succeed())
		})

		It("Should have injected env vars", func(ctx context.Context) {
			deployment := &k8sapps.Deployment{}
			nsdName := namespacedName(testAuthorinoNamespace, authorinoInstance.Name)

			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx, nsdName, deployment)
			}).WithContext(ctx).Should(Succeed())

			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == reconcilers.AuthorinoContainerName {
					checkAuthorinoEnvVar(authorinoInstance, container.Env)
					Expect(len(container.Args) <= 2).To(BeTrue())
				}
			}
		})
	})

	Context("Creating cluster wide authorino object", func() {
		var authorinoInstance *api.Authorino

		BeforeEach(func(ctx context.Context) {
			authorinoInstance = newFullAuthorinoInstance()
			authorinoInstance.Spec.ClusterWide = true
			Expect(k8sClient.Create(ctx, authorinoInstance)).Should(Succeed())
		})

		It("Should create authorino permission", func(ctx context.Context) {
			// service account
			sa := authorinoResources.GetAuthorinoServiceAccount(testAuthorinoNamespace, authorinoInstance.Name, authorinoInstance.Labels)
			nsdName := namespacedName(sa.GetNamespace(), sa.GetName())
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(k8sClient.Get(ctx, nsdName, sa)).ToNot(HaveOccurred())
			}).WithContext(ctx).Should(Succeed())

			// Authorino ClusterRoleBinding
			binding := &k8srbac.ClusterRoleBinding{}
			bindingNsdName := types.NamespacedName{Name: "authorino"}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(k8sClient.Get(ctx, bindingNsdName, binding)).ToNot(HaveOccurred())
			}).WithContext(ctx).Should(Succeed())
			Expect(binding.Subjects).To(ContainElement(
				authorinoResources.GetSubjectForRoleBinding(sa),
			))

			// Authorino Auth ClusterRoleBinding
			k8sAuthBinding := &k8srbac.ClusterRoleBinding{}
			k8sAuthBindingNsdName := types.NamespacedName{Name: reconcilers.AuthorinoK8sAuthClusterRoleBindingName}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(k8sClient.Get(ctx, k8sAuthBindingNsdName, k8sAuthBinding)).ToNot(HaveOccurred())
			}).WithContext(ctx).Should(Succeed())

			// Authorino leaderElection ClusterRoleBinding
			leaderElectionRole := &k8srbac.Role{}
			leaderElectionNsdName := namespacedName(testAuthorinoNamespace, reconcilers.AuthorinoLeaderElectionRoleName)
			Eventually(func(ctx context.Context) error {
				return k8sClient.Get(ctx, leaderElectionNsdName, leaderElectionRole)
			}).WithContext(ctx).Should(Succeed())

			//  delete authorino CR
			Expect(k8sClient.Delete(ctx, authorinoInstance)).ToNot(HaveOccurred())

			// manager cluster role removed
			Eventually(func(g Gomega, ctx context.Context) {
				err := k8sClient.Get(ctx, bindingNsdName, binding)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())

			// Create authorino CR back
			authorinoInstance = newFullAuthorinoInstance()
			authorinoInstance.Spec.ClusterWide = true
			Expect(k8sClient.Create(ctx, authorinoInstance)).Should(Succeed())

			// manager cluster role binding should get service account added
			Eventually(func(g Gomega, ctx context.Context) {
				sa := authorinoResources.GetAuthorinoServiceAccount(testAuthorinoNamespace, authorinoInstance.Name, authorinoInstance.Labels)
				g.Expect(k8sClient.Get(ctx, bindingNsdName, binding)).ToNot(HaveOccurred())
				g.Expect(binding.Subjects).To(ContainElement(
					authorinoResources.GetSubjectForRoleBinding(sa),
				))
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

func newExtServerConfigMap() *k8score.ConfigMap {
	return &k8score.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "external-metadata-server-tls",
			Namespace: testAuthorinoNamespace,
		},
		Data: map[string]string{
			"tls.crt": "-----BEGIN CERTIFICATE-----\nMIIGwjCCBKqgAwIBAgIUc13V+5zSFtQhEdAzXhtVXXh3D3MwDQYJKoZIhvcNAQEL\nBQAwgaIxCzAJBgNVBAYTAkVTMRIwEAYDVQQIDAlCYXJjZWxvbmExEjAQBgNVBAcM\nCUJhcmNlbG9uYTEWMBQGA1UECgwNUmVkIEhhdCwgSW5jLjEXMBUGA1UECwwOUmVk\nIEhhdCAzc2NhbGUxOjA4BgNVBAMMMUtleWNsb2FrIFNlcnZlciBvbiAzc2NhbGUg\nT3BlblNoaWZ0IGRldmVsIGNsdXN0ZXIwHhcNMjExMjE2MTkyMDA3WhcNMzExMjE0\nMTkyMDA3WjCBojELMAkGA1UEBhMCRVMxEjAQBgNVBAgMCUJhcmNlbG9uYTESMBAG\nA1UEBwwJQmFyY2Vsb25hMRYwFAYDVQQKDA1SZWQgSGF0LCBJbmMuMRcwFQYDVQQL\nDA5SZWQgSGF0IDNzY2FsZTE6MDgGA1UEAwwxS2V5Y2xvYWsgU2VydmVyIG9uIDNz\nY2FsZSBPcGVuU2hpZnQgZGV2ZWwgY2x1c3RlcjCCAiIwDQYJKoZIhvcNAQEBBQAD\nggIPADCCAgoCggIBAL1aPyDtqDBNziWLA2AhYPlOq4VBtnSNZJYwxWb1PMzZDw2M\nQxcaN+2/TGrFELv9RLFmJTYd9yMXk6ASJnx513bEqcMp4le2lREF+hUNFVNjQcF7\n3peoJNe06NcZIbLmCwJ8lR7SQD+lhjqr7rqsr9/+q9ZxCAMuCIkhF4BcBQV9Q2uH\n7juhJ0fEUOofqXfdGlyhTLecqQzfw/ZWEDc+uJWFWMB5OdBYJAphwIpyu6dFh245\nInuIHkO17MmFEWJX1HjkTNgIS+JHfJNmlwUBEG9d5/Lwy/NmLMnif6zdHfyjhEHv\nb0GI9n9zu1n6tcOpXSRL9bhYWYY9jxnVxZ2ubsKT0BZe8KHJDGdU1sOX6TWSA8zL\nDN2mIxQvPjGPq36pX32fesg+jUb2Y1ZEbXlrCm25K3L/TNe5G8EolowCd9EwyuYk\nwf1JlU2wO1zd1Y3V7/b3kHyQ4xlr9hjwnc4xcbZV3FGVyasxvtykvsgT3XtHroE9\nrqXcT+Rh6hMSIUFSWqIyON1h6ft8VPZjVhu51JdYk7h2VWFPsEzGi7SSU+f7Zdzj\nZ/9hyDINbUlHbluCBJxiTJb7Ig4t+XPj5etL0yvBh3/MLSHO9CCF8auGCmbTPR2/\njNESuJAs18uRA15EqqHGa0hC4NHuQqxRGsVgIxLKGi9kdPFvWI8pcCYw199RAgMB\nAAGjge0wgeowHQYDVR0OBBYEFCHy/ieeCgOXZvGrM/Qhvp5+Jt7IMB8GA1UdIwQY\nMBaAFCHy/ieeCgOXZvGrM/Qhvp5+Jt7IMA8GA1UdEwEB/wQFMAMBAf8wgZYGA1Ud\nEQSBjjCBi4I7a2V5Y2xvYWstYXBpY3VyaW8tcmVnaXN0cnkuYXBwcy5kZXYtZW5n\nLW9jcDQtOC5kZXYuM3NjYS5uZXSCHmtleWNsb2FrLWFwaWN1cmlvLXJlZ2lzdHJ5\nLnN2Y4Isa2V5Y2xvYWstYXBpY3VyaW8tcmVnaXN0cnkuc3ZjLmNsdXN0ZXIubG9j\nYWwwDQYJKoZIhvcNAQELBQADggIBAG5Dim4JDcYWeLrLyFs6byyV641FIaIRUlcd\ndj7L61LfjCMC7kjhl7ynLjiMxCtRBB04h56xGtncDG8kFFOAT26caNSkWzNnDFXI\n026gMSaamioqXoEKlRjbp2Lf+cLzqpaMN0vXJxdHoBrg74h7uptWkyWMqHVmaFy8\nlLi6T2ET9q/vXDPzKHHjwaN4KynRKgYfShY/UE3G/WmvstrrHF8zWQz5JN0TPhuv\n31LuSJkq1yRA9HNrLpBK685WYZ9vyPs+KUcG84sjTf1aaO8beAppYJc94knO28PA\nObT6YGQW1RxjH1XiCHFGXF5KL9HXMFfOpLK/FlFt5gUxUlqCKncK1ilyiRtNaNKZ\npJsmBnqPVV/ZbgR/Y1l1ucUT9OoEsPOPC/nBzQj4nue7seACGD9HJlapQml75Ix6\n5Ypmq+KyDU8GX+ejbeTnFY84xNqZPQhE7/lbTHKPj6zLD98IQt4FvOmKzdfZUhIG\nP8iWHYvV5NQ4XQUxu0s0kWJhSuTDZmrg9HtlXD2x1zi8ilAKCoJ7nu/avLvHemO5\nBgNixHMHTILZrd2xZ9xjyNPGi92EDK+WG6BHD3JAgLvbcBqB4eAi9EONj7qmw3Ry\n6FlViwpQjDQf3Aj2JZvGgqtCrj5TlvMXiwTdE3p29JTSiY9JE8jJVuqv93Af/HZJ\njqr1zGh3\n-----END CERTIFICATE-----",
		},
	}
}

func newFullAuthorinoInstance() *api.Authorino {
	name := "a" + string(uuid.NewUUID())
	image := reconcilers.DefaultAuthorinoImage
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
			ImagePullPolicy: k8score.PullAlways,
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
			Tracing: api.Tracing{
				Endpoint: "http://tracing/authorino",
				Tags: map[string]string{
					"env":     "test",
					"version": "1.0.0",
				},
				Insecure: true,
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
			value = strings.Join(flagAndValue[1:], "=")
		}

		switch flag {
		case reconcilers.FlagWatchNamespace:
			Expect(authorinoInstance.Spec.ClusterWide).To(BeFalse())
			Expect(value).Should(Equal(testAuthorinoNamespace))
		case reconcilers.FlagWatchedAuthConfigLabelSelector:
			Expect(value).Should(Equal(authorinoInstance.Spec.AuthConfigLabelSelectors))
		case reconcilers.FlagWatchedSecretLabelSelector:
			Expect(value).Should(Equal(authorinoInstance.Spec.SecretLabelSelectors))
		case reconcilers.FlagSupersedingHostSubsets:
			Expect(authorinoInstance.Spec.SupersedingHostSubsets).To(BeTrue())
		case reconcilers.FlagLogLevel:
			Expect(value).Should(Equal(authorinoInstance.Spec.LogLevel))
		case reconcilers.FlagLogMode:
			Expect(value).Should(Equal(authorinoInstance.Spec.LogMode))
		case reconcilers.FlagTimeout:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Timeout)))
		case reconcilers.FlagExtAuthGRPCPort:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.GRPC)))
		case reconcilers.FlagExtAuthHTTPPort:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.HTTP)))
		case reconcilers.FlagTlsCertPath, reconcilers.FlagTlsCertKeyPath:
			Expect(authorinoInstance.Spec.Listener.Tls.Enabled).Should(SatisfyAny(BeNil(), Equal(&tslEnable)))
			Expect(value).Should(SatisfyAny(Equal(reconcilers.DefaultTlsCertPath), Equal(reconcilers.DefaultTlsCertKeyPath)))
		case reconcilers.FlagOidcHTTPPort:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.OIDCServer.Port)))
		case reconcilers.FlagOidcTLSCertPath, reconcilers.FlagOidcTLSCertKeyPath:
			Expect(authorinoInstance.Spec.OIDCServer.Tls.Enabled).To(SatisfyAny(Equal(&tslEnable), BeNil()))
			Expect(value).Should(SatisfyAny(Equal(reconcilers.DefaultOidcTlsCertPath), Equal(reconcilers.DefaultOidcTlsCertKeyPath)))
		case reconcilers.FlagEvaluatorCacheSize:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.EvaluatorCacheSize)))
		case reconcilers.FlagTracingServiceEndpoint:
			Expect(value).Should(Equal(authorinoInstance.Spec.Tracing.Endpoint))
		case reconcilers.FlagTracingServiceTag:
			kv := strings.Split(value, "=")
			Expect(len(kv)).Should(Equal(2))
			Expect(kv[1]).Should(Equal(authorinoInstance.Spec.Tracing.Tags[kv[0]]))
		case reconcilers.FlagTracingServiceInsecure:
			Expect(authorinoInstance.Spec.Tracing.Insecure).To(BeTrue())
		case reconcilers.FlagDeepMetricsEnabled:
			Expect(authorinoInstance.Spec.Metrics.DeepMetricsEnabled).ShouldNot(BeNil())
			Expect(*authorinoInstance.Spec.Metrics.DeepMetricsEnabled).To(BeTrue())
		case reconcilers.FlagMetricsAddr:
			metricsAddr := fmt.Sprintf(":%d", reconcilers.DefaultMetricsServicePort)
			if port := authorinoInstance.Spec.Metrics.Port; port != nil {
				metricsAddr = fmt.Sprintf(":%d", *port)
			}
			Expect(value).Should(Equal(metricsAddr))
		case reconcilers.FlagHealthProbeAddr:
			healthProbeAddr := fmt.Sprintf(":%d", reconcilers.DefaultHealthProbePort)
			if port := authorinoInstance.Spec.Healthz.Port; port != nil {
				healthProbeAddr = fmt.Sprintf(":%d", *port)
			}
			Expect(value).Should(Equal(healthProbeAddr))
		case reconcilers.FlagEnableLeaderElection:
			replicas := authorinoInstance.Spec.Replicas
			if replicas == nil {
				value := int32(0)
				replicas = &value
			}
			Expect(*replicas > 1).To(BeTrue())
		case reconcilers.FlagMaxHttpRequestBodySize:
			Expect(value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.MaxHttpRequestBodySize)))
		}
	}
}

func checkAuthorinoEnvVar(authorinoInstance *api.Authorino, envs []k8score.EnvVar) {
	tslEnable := true

	for _, env := range envs {
		switch env.Name {
		case reconcilers.EnvWatchNamespace:
			Expect(authorinoInstance.Spec.ClusterWide).To(BeFalse())
			Expect(env.Value).Should(Equal(testAuthorinoNamespace))
		case reconcilers.EnvAuthConfigLabelSelector:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.AuthConfigLabelSelectors))
		case reconcilers.EnvSecretLabelSelector:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.SecretLabelSelectors))
		case reconcilers.EnvEvaluatorCacheSize:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.EvaluatorCacheSize)))
		case reconcilers.EnvDeepMetricsEnabled:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Metrics.DeepMetricsEnabled)))
		case reconcilers.EnvLogLevel:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.LogLevel))
		case reconcilers.EnvLogMode:
			Expect(env.Value).Should(Equal(authorinoInstance.Spec.LogMode))
		case reconcilers.EnvExtAuthGRPCPort:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.GRPC)))
		case reconcilers.EnvExtAuthHTTPPort:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Ports.HTTP)))
		case reconcilers.EnvTlsCert, reconcilers.EnvTlsCertKey:
			Expect(authorinoInstance.Spec.Listener.Tls.Enabled).Should(SatisfyAny(BeNil(), Equal(&tslEnable)))
			Expect(env.Value).Should(SatisfyAny(Equal(reconcilers.DefaultTlsCertPath), Equal(reconcilers.DefaultTlsCertKeyPath)))
		case reconcilers.EnvTimeout:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.Timeout)))
		case reconcilers.EnvMaxHttpRequestBodySize:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.Listener.MaxHttpRequestBodySize)))
		case reconcilers.EnvOIDCHTTPPort:
			Expect(env.Value).Should(Equal(fmt.Sprintf("%v", *authorinoInstance.Spec.OIDCServer.Port)))
		case reconcilers.EnvOidcTlsCertPath, reconcilers.EnvOidcTlsCertKeyPath:
			Expect(authorinoInstance.Spec.OIDCServer.Tls.Enabled).To(SatisfyAny(Equal(&tslEnable), BeNil()))
			Expect(env.Value).Should(SatisfyAny(Equal(reconcilers.DefaultOidcTlsCertPath), Equal(reconcilers.DefaultOidcTlsCertKeyPath)))
		}
	}
}
