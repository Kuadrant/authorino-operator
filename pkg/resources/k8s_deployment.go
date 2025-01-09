package resources

import (
	"github.com/go-logr/logr"
	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetDeployment(name, namespace, saName string, replicas *int32, containers []k8score.Container, vol []k8score.Volume, labelsFromAuthorinoCR map[string]string, logger logr.Logger) *k8sapps.Deployment {
	mutableLabels := defaultAuthorinoLabels(name)
	if labelsFromAuthorinoCR != nil {
		for key, value := range labelsFromAuthorinoCR {
			if key == "limitador-resource" || key == "app" {
				logger.V(1).Info("skipping authorino labels with keys \"control-plane\" and \"authorino-resource\" as these are reserved for use by the operator")
				continue
			}
			mutableLabels[key] = value
		}
	}
	immutableLabels := defaultAuthorinoLabels(name)
	objMeta := getObjectMeta(namespace, name, mutableLabels)

	return &k8sapps.Deployment{
		ObjectMeta: objMeta,
		Spec: k8sapps.DeploymentSpec{
			Replicas: replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: immutableLabels,
			},
			Template: k8score.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: mutableLabels,
				},
				Spec: k8score.PodSpec{
					ServiceAccountName: saName,
					Containers:         containers,
					Volumes:            vol,
				},
			},
		},
	}
}

func GetContainer(image string, imagePullPolicy k8score.PullPolicy, containerName string, args []string, envVars []k8score.EnvVar, volMounts []k8score.VolumeMount) k8score.Container {
	if imagePullPolicy == "" {
		imagePullPolicy = k8score.PullAlways
	}
	c := k8score.Container{
		Image:           image,
		ImagePullPolicy: imagePullPolicy,
		Name:            containerName,
		Env:             envVars,
		VolumeMounts:    volMounts,
	}
	if len(args) > 0 {
		c.Args = args
	}
	return c
}

func GetTlsVolumeMount(certName, certPath, certKeyPath string) []k8score.VolumeMount {
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

func GetTlsVolume(certName, secretName string) k8score.Volume {
	return k8score.Volume{
		Name: certName,
		VolumeSource: k8score.VolumeSource{
			Secret: &k8score.SecretVolumeSource{
				SecretName: secretName,
			},
		},
	}
}

func MapUpdateNeeded(existing map[string]string, desired map[string]string) bool {
	if existing == nil {
		existing = map[string]string{}
	}

	for k, v := range desired {
		if existingVal, exists := (existing)[k]; !exists || v != existingVal {
			return true
		}
	}
	return false
}
