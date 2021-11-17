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
	"time"

	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	api "github.com/kuadrant/authorino-operator/api/v1beta1"
)

// AuthorinoReconciler reconciles a Authorino object
type AuthorinoReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

const (
	tlsCertName     string = "tls-cert"
	oidcTlsCertName string = "oidc-cert"
)

//+kubebuilder:rbac:groups=operator.authorino.kuadrant.io,resources=authorinos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.authorino.kuadrant.io,resources=authorinos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.authorino.kuadrant.io,resources=authorinos/finalizers,verbs=update

// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="*",resources=services,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=clusterroles,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=rolebindings,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=clusterrolebindings,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=serviceaccounts,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=roles,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="*",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="*",resources=configmaps/status,verbs=get;update;delete;patch
// +kubebuilder:rbac:groups="*",resources=events,verbs=create;patch;
// +kubebuilder:rbac:groups="*",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="authorino.3scale.net",resources=authconfigs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="authorino.3scale.net",resources=authconfigs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="authorino.3scale.net",resources=authconfigs/status,verbs=get;patch;update
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=get;list;create;update;

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
	log := r.Log.WithValues("authorino", req.NamespacedName)

	// get authorino instance
	authorinoInstance, err := r.getAuthorinoInstance(req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if authorinoInstance == nil {
		log.Info("Authorino CR not found. returning the reconciler")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Found an instance of authorino", "authorinoInstanceName", authorinoInstance.Name)

	err = r.createOrUpdateAuthorinoServices(authorinoInstance)
	if err != nil {
		authorinoInstance.Status.Ready = false
		authorinoInstance.Status.LastError = err.Error()
		if statusErr := r.handleStatusUpdate(authorinoInstance); statusErr != nil {
			log.Error(statusErr, statusErr.Error())
		}
		log.Error(err, "Failed to create authorino services")
		return ctrl.Result{}, err
	}

	err = r.createAuthorinoPermission(authorinoInstance, req.NamespacedName.Namespace)
	if err != nil {
		authorinoInstance.Status.Ready = false
		authorinoInstance.Status.LastError = err.Error()
		if statusErr := r.handleStatusUpdate(authorinoInstance); statusErr != nil {
			log.Error(statusErr, statusErr.Error())
		}
		log.Error(err, "Failed to create authorino permission")
		return ctrl.Result{}, err
	}

	existingDeployment, err := r.getAuthorinoDeployment(authorinoInstance)
	if err != nil {
		authorinoInstance.Status.Ready = false
		authorinoInstance.Status.LastError = err.Error()
		if statusErr := r.handleStatusUpdate(authorinoInstance); statusErr != nil {
			log.Error(statusErr, statusErr.Error())
		}
		log.Error(err, "Failed to get Deployment for Authorino")
		return ctrl.Result{}, err
	} else if existingDeployment == nil {
		newDeployment := r.buildAuthorinoDeployment(authorinoInstance)
		err = r.Create(ctx, newDeployment)
		if err != nil {
			authorinoInstance.Status.Ready = false
			authorinoInstance.Status.LastError = err.Error()
			if statusErr := r.handleStatusUpdate(authorinoInstance); statusErr != nil {
				log.Error(statusErr, statusErr.Error())
			}
			log.Error(err, "Failed to create Authorino deployment resource", newDeployment.Name, newDeployment.Namespace)
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else {
		desiredDeployment := r.buildAuthorinoDeployment(authorinoInstance)
		if r.authorinoDeploymentChanges(existingDeployment, desiredDeployment) {
			err = r.Update(ctx, desiredDeployment)
			if err != nil {
				authorinoInstance.Status.Ready = false
				authorinoInstance.Status.LastError = err.Error()
				if statusErr := r.handleStatusUpdate(authorinoInstance); statusErr != nil {
					log.Error(statusErr, statusErr.Error())
				}
				log.Error(err, "Failed to update Authorino deployment resource", desiredDeployment.Name, desiredDeployment.Namespace)
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	authorinoInstance.Status.Ready = true
	authorinoInstance.Status.LastError = ""
	if statusErr := r.handleStatusUpdate(authorinoInstance); statusErr != nil {
		log.Error(statusErr, statusErr.Error())
	}

	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuthorinoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.Authorino{}).
		Complete(r)
}

func (r *AuthorinoReconciler) getAuthorinoInstance(namespacedName types.NamespacedName) (*api.Authorino, error) {
	authorinoInstance := &api.Authorino{}
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

func (r *AuthorinoReconciler) getAuthorinoDeployment(authorino *api.Authorino) (*k8sapps.Deployment, error) {
	authorinoDeployment := &k8sapps.Deployment{}
	err := r.Get(context.TODO(),
		types.NamespacedName{
			Name:      authorino.GetName(),
			Namespace: authorino.GetNamespace(),
		}, authorinoDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("Authorino deployment not found.")
			return nil, nil
		}
		return nil, err
	}
	return authorinoDeployment, nil
}

func (r *AuthorinoReconciler) buildAuthorinoDeployment(authorino *api.Authorino) *k8sapps.Deployment {
	prefix := authorino.GetName()

	objectMeta := v1.ObjectMeta{
		Name:      authorino.GetName(),
		Namespace: authorino.GetNamespace(),
	}

	labels := labelsForAuthorino(objectMeta.GetName())

	dep := &k8sapps.Deployment{
		ObjectMeta: objectMeta,
		Spec: k8sapps.DeploymentSpec{
			Replicas: authorino.Spec.Replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: labels,
			},
			Template: k8score.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: labels,
				},
				Spec: k8score.PodSpec{
					ServiceAccountName: prefix + "-authorino",
				},
			},
		},
	}

	if authorino.Spec.Image == "" {
		authorino.Spec.Image = fmt.Sprintf("quay.io/3scale/authorino:%s", api.AuthorinoVersion)
	}

	authorinoContainer := k8score.Container{
		Image:           authorino.Spec.Image,
		ImagePullPolicy: k8score.PullPolicy(authorino.Spec.ImagePullPolicy),
		Name:            api.AuthorinoContainerName,
		Env:             r.buildAuthorinoEnv(authorino),
	}

	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		secretName := authorino.Spec.Listener.Tls.CertSecret.Name
		authorinoContainer.VolumeMounts = append(authorinoContainer.VolumeMounts,
			buildTlsVolumeMount(tlsCertName, api.DefaultTlsCertPath, api.DefaultTlsCertKeyPath)...,
		)
		dep.Spec.Template.Spec.Volumes = append(dep.Spec.Template.Spec.Volumes,
			buildTlsVolume(tlsCertName, secretName),
		)
	}

	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		secretName := authorino.Spec.OIDCServer.Tls.CertSecret.Name
		authorinoContainer.VolumeMounts = append(authorinoContainer.VolumeMounts,
			buildTlsVolumeMount(oidcTlsCertName, api.DefaultOidcTlsCertPath, api.DefaultOidcTlsCertKeyPath)...,
		)
		dep.Spec.Template.Spec.Volumes = append(dep.Spec.Template.Spec.Volumes,
			buildTlsVolume(oidcTlsCertName, secretName),
		)
	}
	dep.Spec.Template.Spec.Containers = append(dep.Spec.Template.Spec.Containers, authorinoContainer)

	ctrl.SetControllerReference(authorino, dep, r.Scheme)

	return dep
}

func buildTlsVolume(certName, secretName string) k8score.Volume {
	return k8score.Volume{
		Name: certName,
		VolumeSource: k8score.VolumeSource{
			Secret: &k8score.SecretVolumeSource{
				SecretName: secretName,
			},
		},
	}
}

func buildTlsVolumeMount(certName, certPath, certKeyPath string) []k8score.VolumeMount {
	return []k8score.VolumeMount{
		{
			Name:      certName,
			MountPath: certPath,
			SubPath:   "tls.crt",
			ReadOnly:  true,
		},
		{
			Name:      certName,
			MountPath: certKeyPath,
			SubPath:   "tls.key",
			ReadOnly:  true,
		},
	}
}

func (r *AuthorinoReconciler) buildAuthorinoEnv(authorino *api.Authorino) []k8score.EnvVar {
	envVar := []k8score.EnvVar{}

	if !authorino.Spec.ClusterWide {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.WatchNamespace,
			Value: authorino.GetNamespace(),
		})
	}

	if authorino.Spec.AuthConfigLabelSelectors != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.AuthConfigLabelSelector,
			Value: fmt.Sprint(authorino.Spec.AuthConfigLabelSelectors),
		})
	}

	if authorino.Spec.SecretLabelSelectors != "" {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.SecretLabelSelector,
			Value: fmt.Sprint(authorino.Spec.SecretLabelSelectors),
		})
	}

	// external auth service via GRPC
	if authorino.Spec.Listener.Port != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.ExtAuthGRPCPort,
			Value: fmt.Sprintf("%v", *authorino.Spec.Listener.Port),
		})
	}

	if enabled := authorino.Spec.Listener.Tls.Enabled; enabled == nil || *enabled {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvVarTlsCert,
			Value: api.DefaultTlsCertPath,
		})

		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvVarTlsCertKey,
			Value: api.DefaultTlsCertKeyPath,
		})
	}

	// OIDC service
	if authorino.Spec.OIDCServer.Port != nil {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.OIDCHTTPPort,
			Value: fmt.Sprintf("%v", *authorino.Spec.OIDCServer.Port),
		})
	}
	if enabled := authorino.Spec.OIDCServer.Tls.Enabled; enabled == nil || *enabled {
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvVarOidcTlsCertPath,
			Value: api.DefaultOidcTlsCertPath,
		})
		envVar = append(envVar, k8score.EnvVar{
			Name:  api.EnvVarOidcTlsCertKeyPath,
			Value: api.DefaultOidcTlsCertKeyPath,
		})
	}

	return envVar
}

func (r *AuthorinoReconciler) authorinoDeploymentChanges(existingDeployment, desiredDeployment *k8sapps.Deployment) bool {
	changed := false

	if existingDeployment.Spec.Replicas != desiredDeployment.Spec.Replicas {
		changed = true
	}

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
	for _, desiredEnvvar := range desiredEnvvars {
		for _, existingEnvvar := range existingEnvvars {
			if existingEnvvar.Name == desiredEnvvar.Name && existingEnvvar.Value != desiredEnvvar.Value {
				changed = true
				break
			}
		}
	}

	// checking volume
	existingVolumes := existingDeployment.Spec.Template.Spec.Volumes
	desiredVolumes := desiredDeployment.Spec.Template.Spec.Volumes
	for _, desiredVolume := range desiredVolumes {
		if desiredVolume.Name == tlsCertName || desiredVolume.Name == oidcTlsCertName {
			for _, existingVolume := range existingVolumes {
				if existingVolume.Name == tlsCertName || desiredVolume.Name == oidcTlsCertName && existingVolume.VolumeSource.Secret.SecretName != desiredVolume.VolumeSource.Secret.SecretName {
					changed = true
					break
				}
			}
		}
	}

	return changed
}

func (r *AuthorinoReconciler) createAuthorinoPermission(authorino *api.Authorino, operatorNamespace string) error {
	prefix := authorino.GetName()
	authorinoInstanceNamespace := authorino.GetNamespace()
	authorinoClusterRoleName := "authorino-manager-role"

	authorinoClusterRole := &k8srbac.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name: authorinoClusterRoleName,
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
	serviceAccount := &k8score.ServiceAccount{
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
		clusterRoleBinding := &k8srbac.ClusterRoleBinding{
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
			clusterRoleBinding.RoleRef = k8srbac.RoleRef{
				Name: authorinoClusterRole.GetName(),
				Kind: "ClusterRole",
			}
			clusterRoleBinding.Subjects = []k8srbac.Subject{
				{
					Kind:      k8srbac.ServiceAccountKind,
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
		roleBinding := &k8srbac.RoleBinding{
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
			roleBinding.RoleRef = k8srbac.RoleRef{
				Name: authorinoClusterRole.GetName(),
				Kind: "ClusterRole",
			}
			roleBinding.Subjects = []k8srbac.Subject{
				{
					Kind:      k8srbac.ServiceAccountKind,
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

func (r *AuthorinoReconciler) leaderElectionPermission(authorino *api.Authorino, saName string) error {
	leaderElectionRole := &k8srbac.Role{
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
	roleBinding := &k8srbac.RoleBinding{
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
		roleBinding.RoleRef = k8srbac.RoleRef{
			Name: leaderElectionRole.GetName(),
			Kind: "Role",
		}
		roleBinding.Subjects = []k8srbac.Subject{
			{
				Kind:      k8srbac.ServiceAccountKind,
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

func getLeaderElectionRules() []k8srbac.PolicyRule {
	return []k8srbac.PolicyRule{
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

func (r *AuthorinoReconciler) createOrUpdateAuthorinoServices(authorino *api.Authorino) error {

	services := make(map[string][]k8score.ServicePort)
	services["authorino-authorization"] = []k8score.ServicePort{
		{
			Name:     "grpc",
			Port:     50051,
			Protocol: k8score.ProtocolTCP,
		},
	}
	services["authorino-oidc"] = []k8score.ServicePort{
		{
			Name:     "http",
			Port:     8083,
			Protocol: k8score.ProtocolTCP,
		},
	}
	services["controller-metrics"] = []k8score.ServicePort{
		{
			Name:       "https",
			Port:       8443,
			TargetPort: intstr.FromString("https"),
		},
	}

	for name, service := range services {
		obj := &k8score.Service{
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

func (r *AuthorinoReconciler) handleStatusUpdate(authorino *api.Authorino) error {
	// get authorino instance
	existingAuthorinoInstance, err := r.getAuthorinoInstance(types.NamespacedName{Name: authorino.GetName(), Namespace: authorino.GetNamespace()})
	if err != nil {
		return err
	}
	existingAuthorinoInstance.Status.Ready = authorino.Status.Ready

	err = r.Status().Update(context.TODO(), existingAuthorinoInstance)
	if err != nil {
		return fmt.Errorf("Failed to update authorino's %s status , err: %w", authorino.GetName(), err)
	}
	return nil
}

func labelsForAuthorino(name string) map[string]string {
	return map[string]string{
		"control-plane":      "controller-manager",
		"authorino-resource": name,
	}
}
