package resources

import (
	"encoding/json"
	"sort"

	k8score "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func NewAuthService(authorinoName, serviceNamespace string, grpcPort, httpPort int32) *k8score.Service {
	var ports []k8score.ServicePort
	if grpcPort != 0 {
		ports = append(ports, newServicePort("grpc", grpcPort))
	}
	if httpPort != 0 {
		ports = append(ports, newServicePort("http", httpPort))
	}
	return newService("authorino-authorization", serviceNamespace, authorinoName, ports...)
}

func NewOIDCService(authorinoName, authorinoNamespace string, port int32) *k8score.Service {
	var ports []k8score.ServicePort
	if port != 0 {
		ports = append(ports, newServicePort("http", port))
	}
	return newService("authorino-oidc", authorinoNamespace, authorinoName, ports...)
}

func NewMetricsService(authorinoName, serviceNamespace string, port int32) *k8score.Service {
	var ports []k8score.ServicePort
	if port != 0 {
		ports = append(ports, newServicePort("http", port))
	}
	return newService("controller-metrics", serviceNamespace, authorinoName, ports...)
}

func EqualServices(s1, s2 *k8score.Service) bool {
	sortedSpec := func(s k8score.ServiceSpec) k8score.ServiceSpec {
		var ports []k8score.ServicePort
		ports = append(ports, s.Ports...)
		sort.Slice(ports, func(a, b int) bool {
			return ports[a].Name < ports[b].Name
		})
		return newServiceSpec(s.Selector, ports...)
	}

	if spec1, err := json.Marshal(sortedSpec(s1.Spec)); err == nil {
		spec2, err := json.Marshal(sortedSpec(s2.Spec))
		return err == nil && string(spec1) == string(spec2)
	}

	return false
}

func newService(serviceName, serviceNamespace, authorinoName string, servicePorts ...k8score.ServicePort) *k8score.Service {
	objMeta := getObjectMeta(serviceNamespace, authorinoName+"-"+serviceName)
	return &k8score.Service{
		ObjectMeta: objMeta,
		Spec:       newServiceSpec(labelsForAuthorino(authorinoName), servicePorts...),
	}
}

func newServiceSpec(selector map[string]string, ports ...k8score.ServicePort) k8score.ServiceSpec {
	return k8score.ServiceSpec{
		Ports:    ports,
		Selector: selector,
	}
}

func newServicePort(name string, number int32) k8score.ServicePort {
	port := k8score.ServicePort{
		Name:       name,
		Port:       number,
		Protocol:   k8score.ProtocolTCP,
		TargetPort: intstr.FromInt(int(number)),
	}
	return port
}
