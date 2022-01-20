package resources

import (
	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetDeployment(name, namespace, saName string, replicas *int32, containers []k8score.Container, vol []k8score.Volume) *k8sapps.Deployment {
	objMeta := getObjectMeta(namespace, name)
	labels := labelsForAuthorino(name)

	numOfReplicas := replicas
	if numOfReplicas == nil {
		value := int32(1)
		numOfReplicas = &value
	}

	return &k8sapps.Deployment{
		ObjectMeta: objMeta,
		Spec: k8sapps.DeploymentSpec{
			Replicas: numOfReplicas,
			Selector: &v1.LabelSelector{
				MatchLabels: labels,
			},
			Template: k8score.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: labels,
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

func GetContainer(image, imagePullPolicy, containerName string, envVars []k8score.EnvVar, volMounts []k8score.VolumeMount, args []string, ports []k8score.ContainerPort) k8score.Container {
	if imagePullPolicy == "" {
		imagePullPolicy = "Always"
	}
	return k8score.Container{
		Image:           image,
		ImagePullPolicy: k8score.PullPolicy(imagePullPolicy),
		Name:            containerName,
		Env:             envVars,
		VolumeMounts:    volMounts,
		Args:            args,
		Ports:           ports,
	}
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
