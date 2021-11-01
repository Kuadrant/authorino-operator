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
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	authorinooperatorv1beta1 "github.com/kuadrant/authorino-operator/api/v1beta1"
)

// AuthorinoReconciler reconciles a Authorino object
type AuthorinoReconciler struct {
	client.Client
	OperatorNamespace string
	Log               logr.Logger
	Scheme            *runtime.Scheme
}

//+kubebuilder:rbac:groups=authorino-operator.kuadrant.3scale.net,resources=authorinoes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=authorino-operator.kuadrant.3scale.net,resources=authorinoes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=authorino-operator.kuadrant.3scale.net,resources=authorinoes/finalizers,verbs=update

// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Authorino object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *AuthorinoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("authorinoReconciler", req.NamespacedName)

	//get operator namespace
	r.OperatorNamespace = authorinooperatorv1beta1.AuthorinoOperatorNamespace
	ns, found := os.LookupEnv("WATCH_NAMESPACE")
	if found {
		r.OperatorNamespace = ns
	}

	// get authorino instance
	authorinoInstance, err := r.authorinoInstance(req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if authorinoInstance == nil {
		log.Info("Authorino CR not found. returning the reconciler")
		return ctrl.Result{}, nil
	}

	log.Info("Found an instance of authorino", "authorinoInstanceName", authorinoInstance.Name)

	err = r.authorinoServices(authorinoInstance)
	if err != nil {
		log.Error(err, "Failed to create authorino services")
		return ctrl.Result{}, err
	}

	err = r.authorinoPermission(authorinoInstance, req.NamespacedName.Namespace)
	if err != nil {
		log.Error(err, "Failed to create authorino permission")
		return ctrl.Result{}, err
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(context.TODO(),
		types.NamespacedName{
			Name:      authorinoInstance.GetName(),
			Namespace: authorinoInstance.GetNamespace(),
		}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		newDeployment := r.authorinoDeployment(authorinoInstance)
		err = r.Create(ctx, newDeployment)
		if err != nil {
			log.Error(err, "Failed to create Authorino deployment resource", newDeployment.Name, newDeployment.Namespace)
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment for Authorino")
		return ctrl.Result{}, err
	}

	// checks for upgrades
	desiredDeployment := r.authorinoDeployment(authorinoInstance)
	if r.authorinoDeploymentChanges(existingDeployment, desiredDeployment) {
		err = r.Update(ctx, desiredDeployment)
		if err != nil {
			log.Error(err, "Failed to update Authorino deployment resource", desiredDeployment.Name, desiredDeployment.Namespace)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	//TODO: handle deletiton

	//TODO: update status

	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuthorinoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authorinooperatorv1beta1.Authorino{}).
		Complete(r)
}

func (r *AuthorinoReconciler) authorinoInstance(namespacedName types.NamespacedName) (*authorinooperatorv1beta1.Authorino, error) {
	authorinoInstance := &authorinooperatorv1beta1.Authorino{}
	err := r.Get(context.TODO(), namespacedName, authorinoInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("Authorino CR not found.")
			return nil, nil
		}
		return nil, err
	}
	return authorinoInstance, nil
}

func (r *AuthorinoReconciler) authorinoDeployment(authorino *authorinooperatorv1beta1.Authorino) *appsv1.Deployment {
	prefix := authorino.GetName()

	objectMeta := v1.ObjectMeta{
		Name:      authorino.GetName(),
		Namespace: authorino.GetNamespace(),
	}

	labels := labelsForAuthorino(objectMeta.GetName())

	dep := &appsv1.Deployment{
		ObjectMeta: objectMeta,
		Spec: appsv1.DeploymentSpec{
			Replicas: authorino.Spec.Replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: prefix + "-authorino",
					Containers: []corev1.Container{
						{
							Image:           authorino.Spec.Image,
							ImagePullPolicy: corev1.PullPolicy(authorino.Spec.ImagePullPolicy),
							Name:            authorinooperatorv1beta1.AuthorinoContainerName,
							Env:             r.buildAuthorinoEnv(authorino),
						},
					},
				},
			},
		},
	}

	ctrl.SetControllerReference(authorino, dep, r.Scheme)

	return dep
}

func (r *AuthorinoReconciler) buildAuthorinoEnv(authorino *authorinooperatorv1beta1.Authorino) []corev1.EnvVar {
	envVar := []corev1.EnvVar{}

	if !authorino.Spec.ClusterWide {
		envVar = append(envVar, corev1.EnvVar{
			Name:  "WATCH_NAMESPACE",
			Value: authorino.GetNamespace(),
		})
	}

	if authorino.Spec.AuthConfigLabelSelectors != "" {
		envVar = append(envVar, corev1.EnvVar{
			Name:  authorinooperatorv1beta1.AuthConfigLabelSelector,
			Value: fmt.Sprint(authorino.Spec.AuthConfigLabelSelectors),
		})
	}

	if authorino.Spec.SecretLabelSelectors != "" {
		envVar = append(envVar, corev1.EnvVar{
			Name:  authorinooperatorv1beta1.SecretLabelSelector,
			Value: fmt.Sprint(authorino.Spec.SecretLabelSelectors),
		})
	}

	// external auth service via GRPC
	if authorino.Spec.Listener.Port != nil {
		envVar = append(envVar, corev1.EnvVar{
			Name:  authorinooperatorv1beta1.ExtAuthGRPCPort,
			Value: fmt.Sprint(authorino.Spec.Listener.Port),
		})
	}
	if authorino.Spec.Listener.CertPath != "" {
		envVar = append(envVar, corev1.EnvVar{
			Name:  authorinooperatorv1beta1.TSLCertPath,
			Value: authorino.Spec.Listener.CertPath,
		})
	}
	if authorino.Spec.Listener.CertKeyPath != "" {
		envVar = append(envVar, corev1.EnvVar{
			Name:  authorinooperatorv1beta1.TSLCertKeyPath,
			Value: authorino.Spec.Listener.CertKeyPath,
		})
	}

	// OIDC service
	if authorino.Spec.OIDCServer.Port != nil {
		envVar = append(envVar, corev1.EnvVar{
			Name:  authorinooperatorv1beta1.OIDCHTTPPort,
			Value: fmt.Sprint(authorino.Spec.OIDCServer.Port),
		})
	}
	if authorino.Spec.OIDCServer.CertKeyPath != "" {
		envVar = append(envVar, corev1.EnvVar{
			Name:  authorinooperatorv1beta1.OIDCTSLCertPath,
			Value: authorino.Spec.OIDCServer.CertPath,
		})
	}
	if authorino.Spec.OIDCServer.CertKeyPath != "" {
		envVar = append(envVar, corev1.EnvVar{
			Name:  authorinooperatorv1beta1.OIDCTLSCertKeyPath,
			Value: authorino.Spec.OIDCServer.CertKeyPath,
		})
	}

	return envVar
}

func (r *AuthorinoReconciler) authorinoDeploymentChanges(existingDeployment, desiredDeployment *appsv1.Deployment) bool {
	changed := false

	if len(desiredDeployment.Spec.Template.Spec.Containers) != 1 {
		// error
	}

	existingContainer := existingDeployment.Spec.Template.Spec.Containers[0]
	desiredContainer := desiredDeployment.Spec.Template.Spec.Containers[0]

	if existingContainer.Image != desiredContainer.Image {
		changed = true
	}

	if existingContainer.ImagePullPolicy != desiredContainer.ImagePullPolicy {
		changed = true
	}

	// checking envvars
	existingEnvvars := existingContainer.Env
	desiredEnvvars := desiredContainer.Env
	for envIndex, existingEnvvar := range existingEnvvars {
		desiredEnvvar := desiredEnvvars[envIndex]
		if existingEnvvar.Name == desiredEnvvar.Name && existingEnvvar.Value != desiredEnvvar.Value {
			changed = true
		}
	}
	return changed
}

func (r *AuthorinoReconciler) authorinoPermission(authorino *authorinooperatorv1beta1.Authorino, operatorNamespace string) error {
	prefix := authorino.GetName()
	authorinoInstanceNamespace := authorino.GetNamespace()
	authorinoClusterRoleName := "authorino-operator-authorino-manager-role"

	authorinoClusterRole := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:      authorinoClusterRoleName,
			Namespace: r.OperatorNamespace,
		},
	}
	err := r.Get(context.TODO(), client.ObjectKeyFromObject(authorinoClusterRole), authorinoClusterRole)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("Authorino cluster role not found, err: %d", err)
		}
		return fmt.Errorf("Failed to get authorino cluster role, err: %d", err)
	}

	roleName := prefix + "-authorino"
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      roleName,
			Namespace: authorinoInstanceNamespace,
		},
	}
	ctrl.SetControllerReference(authorino, serviceAccount, r.Scheme)
	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.Client, serviceAccount, func() error {
		return nil
	})
	if err != nil {
		return fmt.Errorf(
			"Failed to create/update authorino instance %s service account, err: %d",
			authorino.GetName(),
			err,
		)
	}

	// creates leader election role
	err = r.leaderElectionPermission(authorino, serviceAccount.GetName())
	if err != nil {
		return err
	}

	if authorino.Spec.ClusterWide {
		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: v1.ObjectMeta{
				Name:      roleName,
				Namespace: authorinoInstanceNamespace,
			},
		}

		err = r.Get(context.TODO(), client.ObjectKeyFromObject(clusterRoleBinding), clusterRoleBinding)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf(
				"Failed to get ClusterRoleBinding %s for authorino instance %s, err: %d",
				clusterRoleBinding.GetName(),
				authorino.GetName(),
				err,
			)
		}
		if errors.IsNotFound(err) {
			clusterRoleBinding.RoleRef = rbacv1.RoleRef{
				Name: authorinoClusterRole.GetName(),
				Kind: "ClusterRole",
			}
			clusterRoleBinding.Subjects = []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      serviceAccount.GetName(),
					Namespace: authorinoInstanceNamespace,
				},
			}
			ctrl.SetControllerReference(authorino, clusterRoleBinding, r.Scheme)
			err = r.Create(context.TODO(), clusterRoleBinding)
			if err != nil {
				return fmt.Errorf(
					"Failed to create ClusterRoleBinding %s for authorino instance %s, err: %d",
					clusterRoleBinding.GetName(),
					authorino.GetName(),
					err,
				)
			}
		}
	} else {
		roleBinding := &rbacv1.RoleBinding{
			ObjectMeta: v1.ObjectMeta{
				Name:      roleName,
				Namespace: authorinoInstanceNamespace,
			},
		}

		ctrl.SetControllerReference(authorino, roleBinding, r.Scheme)
		err = r.Get(context.TODO(), client.ObjectKeyFromObject(roleBinding), roleBinding)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf(
				"Failed to get roleBinding %s for authorino instance %s, err: %d",
				roleBinding.GetName(),
				authorino.GetName(),
				err,
			)
		}

		if errors.IsNotFound(err) {
			roleBinding.RoleRef = rbacv1.RoleRef{
				Name: authorinoClusterRole.GetName(),
				Kind: "ClusterRole",
			}
			roleBinding.Subjects = []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      serviceAccount.GetName(),
					Namespace: authorinoInstanceNamespace,
				},
			}
			ctrl.SetControllerReference(authorino, roleBinding, r.Scheme)
			err = r.Create(context.TODO(), roleBinding)
			if err != nil {
				return fmt.Errorf(
					"Failed to create roleBinding %s for authorino instance %s, err: %d",
					roleBinding.GetName(),
					authorino.GetName(),
					err,
				)
			}
		}
	}

	return nil
}

func (r *AuthorinoReconciler) leaderElectionPermission(authorino *authorinooperatorv1beta1.Authorino, saName string) error {
	leaderElectionRole := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "authorino-leader-election-role",
			Namespace: authorino.GetNamespace(),
			Labels:    labelsForAuthorino(authorino.GetName()),
		},
	}

	err := r.Get(context.TODO(), client.ObjectKeyFromObject(leaderElectionRole), leaderElectionRole)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf(
			"Failed to get leader election role err: %d",
			err,
		)
	}

	if errors.IsNotFound(err) {
		ctrl.SetControllerReference(authorino, leaderElectionRole, r.Scheme)
		leaderElectionRole.Rules = getLeaderElectionRules()
		err = r.Create(context.TODO(), leaderElectionRole)
		if err != nil {
			return fmt.Errorf(
				"Failed to create leader election role, err: %d",
				err,
			)
		}
	}

	prefix := authorino.GetName()
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      prefix + "-authorino-leader-election",
			Namespace: authorino.GetNamespace(),
		},
	}
	err = r.Get(context.TODO(), client.ObjectKeyFromObject(roleBinding), roleBinding)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf(
			"Failed to get roleBinding %s for authorino instance %s, err: %d",
			roleBinding.GetName(),
			authorino.GetName(),
			err,
		)
	}

	if errors.IsNotFound(err) {
		roleBinding.RoleRef = rbacv1.RoleRef{
			Name: roleBinding.GetName(),
			Kind: "Role",
		}
		roleBinding.Subjects = []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      saName,
				Namespace: authorino.GetNamespace(),
			},
		}
		ctrl.SetControllerReference(authorino, roleBinding, r.Scheme)
		err = r.Create(context.TODO(), roleBinding)
		if err != nil {
			return fmt.Errorf(
				"Failed to create leader election role binding, err: %d",
				err,
			)
		}
	}
	return nil
}

func getLeaderElectionRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		{
			APIGroups: []string{"*"},
			Resources: []string{"configmaps/status"},
			Verbs:     []string{"get", "update", "patch"},
		},
		{
			APIGroups: []string{"*"},
			Resources: []string{"events"},
			Verbs:     []string{"create", "patch"},
		},
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"get", "list", "create", "update"},
		},
	}
}

func (r *AuthorinoReconciler) authorinoServices(authorino *authorinooperatorv1beta1.Authorino) error {

	services := make(map[string][]corev1.ServicePort)
	services["authorino-authorization"] = []corev1.ServicePort{
		{
			Name:     "grpc",
			Port:     50051,
			Protocol: corev1.ProtocolTCP,
		},
	}
	services["authorino-oidc"] = []corev1.ServicePort{
		{
			Name:     "http",
			Port:     8083,
			Protocol: corev1.ProtocolTCP,
		},
	}
	services["authorino-controller-manager-metrics-service"] = []corev1.ServicePort{
		{
			Name:       "https",
			Port:       8443,
			TargetPort: intstr.FromString("https"),
		},
	}

	for name, service := range services {
		obj := &corev1.Service{
			ObjectMeta: v1.ObjectMeta{
				Name:      authorino.GetName() + "-" + name,
				Namespace: authorino.GetNamespace(),
			},
		}

		ctrl.SetControllerReference(authorino, obj, r.Scheme)

		_, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, obj, func() error {
			obj.Spec.Ports = service
			obj.Spec.Selector = labelsForAuthorino(authorino.GetName())
			return nil
		})
		if err != nil {
			return fmt.Errorf("Failed creating %s service, err: %w", name, err)
		}
	}

	return nil
}

func labelsForAuthorino(name string) map[string]string {
	return map[string]string{
		"control-plane":     "controller-manager",
		"authorino_cr_name": name,
	}
}
