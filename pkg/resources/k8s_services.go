package resources

import (
	k8score "k8s.io/api/core/v1"
)

// GetAuthorinoServices returns all the service required by an instance of authorino
func GetAuthorinoServices(authorinoInstanceName, authorinoInstanceNamespace string) []*k8score.Service {
	return []*k8score.Service{
		newOIDCService(authorinoInstanceName, authorinoInstanceNamespace),
		newAuthService(authorinoInstanceName, authorinoInstanceNamespace),
		newMetricService(authorinoInstanceName, authorinoInstanceNamespace),
	}
}

func newOIDCService(authorinoName, authorinoNamespace string) *k8score.Service {
	serviceName := "authorino-oidc"
	return newService(serviceName, authorinoNamespace, authorinoName, k8score.ServicePort{
		Name:     "http",
		Port:     8083,
		Protocol: k8score.ProtocolTCP,
	})

}

func newAuthService(authorinoName, serviceNamespace string) *k8score.Service {
	serviceName := "authorino-authorization"
	return newService(serviceName, serviceNamespace, authorinoName, k8score.ServicePort{
		Name:     "grpc",
		Port:     50051,
		Protocol: k8score.ProtocolTCP,
	})
}

func newMetricService(authorinoName, serviceNamespace string) *k8score.Service {
	serviceName := "controller-metrics"
	return newService(serviceName, serviceNamespace, authorinoName, k8score.ServicePort{
		Name:     "http",
		Port:     8080,
		Protocol: k8score.ProtocolTCP,
	})
}

func newService(serviceName, serviceNamespace, authorinoName string, servicePort k8score.ServicePort) *k8score.Service {
	objMeta := getObjectMeta(serviceNamespace, authorinoName+"-"+serviceName)
	return &k8score.Service{
		ObjectMeta: objMeta,
		Spec: k8score.ServiceSpec{
			Ports:    []k8score.ServicePort{servicePort},
			Selector: labelsForAuthorino(authorinoName),
		},
	}
}
