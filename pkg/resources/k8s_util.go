package resources

import (
	"fmt"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getObjectMeta(namespace, name string) v1.ObjectMeta {
	return v1.ObjectMeta{Name: name, Namespace: namespace}
}

func labelsForAuthorino(name string) map[string]string {
	return map[string]string{
		"control-plane":      "controller-manager",
		"authorino-resource": name,
	}
}

func authorinoServiceAccountName(crName string) string {
	return fmt.Sprintf("%s-authorino", crName)
}

func authorinoRoleBindingName(crName, roleBindingNameSuffix string) string {
	return fmt.Sprintf("%s-%s", crName, roleBindingNameSuffix)
}
