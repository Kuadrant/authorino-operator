package resources

import (
	k8sapps "k8s.io/api/apps/v1"
	k8score "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetDeployment(name, namespace, saName string, replicas *int32, containers []k8score.Container, vol []k8score.Volume, labelsFromAuthorinoCR map[string]string) *k8sapps.Deployment {
	overrideLabels := defaultAuthorinoLabels(name)
	for key, value := range labelsFromAuthorinoCR {
		overrideLabels[key] = value
	}
	objMeta := getObjectMeta(namespace, name, overrideLabels)

	return &k8sapps.Deployment{
		ObjectMeta: objMeta,
		Spec: k8sapps.DeploymentSpec{
			Replicas: replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: overrideLabels,
			},
			Template: k8score.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: defaultAuthorinoLabels(name),
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
