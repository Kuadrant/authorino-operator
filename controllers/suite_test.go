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
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	"github.com/kuadrant/authorino-operator/pkg/reconcilers"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var k8sClient client.Client
var testEnv *envtest.Environment
var cancel context.CancelFunc
var ctx = context.Background()

const testCertSecretName = "bkabk"

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	ctx, cancel = context.WithCancel(context.TODO())

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = api.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	// creates authorino ClusterRole
	Expect(k8sClient.Create(context.TODO(), getAuthorinoClusterRole(reconcilers.AuthorinoManagerClusterRoleName))).Should(Succeed())

	Expect(k8sClient.Create(context.TODO(), getAuthorinoClusterRole(reconcilers.AuthorinoK8sAuthClusterRoleName))).Should(Succeed())

	Expect(k8sClient.Create(context.TODO(), newCertSecret())).Should(Succeed())

	authorinoReconciler := &reconcilers.AuthorinoReconciler{
		Client: k8sClient,
		Log:    ctrl.Log.WithName("authorino-operator").WithName("controller").WithName("Authorino"),
		Scheme: mgr.GetScheme(),
	}

	err = (&AuthorinoReconciler{
		authorinoReconciler,
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err = mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func getAuthorinoClusterRole(clusterRoleName string) *k8srbac.ClusterRole {
	return &k8srbac.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name: clusterRoleName,
		},
	}
}

func newCertSecret() *k8score.Secret {
	return &k8score.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      testCertSecretName,
			Namespace: testAuthorinoNamespace,
		},
	}
}
