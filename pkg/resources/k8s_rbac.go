package resources

import (
	k8score "k8s.io/api/core/v1"
	k8srbac "k8s.io/api/rbac/v1"
	k8smeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetAuthorinoServiceAccount(namespace, crName string, labels map[string]string) *k8score.ServiceAccount {
	return &k8score.ServiceAccount{
		TypeMeta:   k8smeta.TypeMeta{Kind: "ServiceAccount"},
		ObjectMeta: getObjectMeta(namespace, authorinoServiceAccountName(crName), labels),
	}
}

func GetAuthorinoClusterRoleBinding(roleBindingName, clusterRoleName string, serviceAccount k8score.ServiceAccount) *k8srbac.ClusterRoleBinding {
	roleRef, roleSubject := getRoleRefAndSubject(clusterRoleName, "ClusterRole", serviceAccount)
	return &k8srbac.ClusterRoleBinding{
		ObjectMeta: k8smeta.ObjectMeta{Name: roleBindingName},
		RoleRef:    roleRef,
		Subjects:   []k8srbac.Subject{roleSubject},
	}
}

func GetAuthorinoRoleBinding(namespace, crName, roleBindingNameSuffix, roleKind, roleName string, serviceAccount k8score.ServiceAccount, labels map[string]string) *k8srbac.RoleBinding {
	roleRef, roleSubject := getRoleRefAndSubject(roleName, roleKind, serviceAccount)
	return &k8srbac.RoleBinding{
		ObjectMeta: getObjectMeta(namespace, authorinoRoleBindingName(crName, roleBindingNameSuffix), labels),
		RoleRef:    roleRef,
		Subjects:   []k8srbac.Subject{roleSubject},
	}
}

func getRoleRefAndSubject(roleName, roleKind string, serviceAccount k8score.ServiceAccount) (k8srbac.RoleRef, k8srbac.Subject) {
	var roleRef = k8srbac.RoleRef{
		Name: roleName,
		Kind: roleKind,
	}

	var roleSubject = k8srbac.Subject{
		Kind:      "ServiceAccount",
		Name:      serviceAccount.Name,
		Namespace: serviceAccount.Namespace,
	}

	return roleRef, roleSubject
}

// Makes sure a given serviceaccount is among the subjects of a rolebinding or clusterrolebinding
func AppendSubjectToRoleBinding(roleBinding client.Object, serviceAccount k8score.ServiceAccount) client.Object {
	subject := GetSubjectForRoleBinding(serviceAccount)
	if rb, ok := roleBinding.(*k8srbac.RoleBinding); ok {
		if subjectIncluded(rb.Subjects, subject) {
			return rb
		}
		rb.Subjects = append(rb.Subjects, subject)
		return rb
	} else {
		return appendSubjectToClusterRoleBinding(roleBinding, subject)
	}
}

func appendSubjectToClusterRoleBinding(roleBinding client.Object, subject k8srbac.Subject) client.Object {
	if rb, ok := roleBinding.(*k8srbac.ClusterRoleBinding); ok {
		if subjectIncluded(rb.Subjects, subject) {
			return rb
		}
		rb.Subjects = append(rb.Subjects, subject)
		return rb
	} else {
		return nil
	}
}

func GetSubjectForRoleBinding(serviceAccount k8score.ServiceAccount) k8srbac.Subject {
	return k8srbac.Subject{
		Kind:      "ServiceAccount",
		Name:      serviceAccount.Name,
		Namespace: serviceAccount.Namespace,
	}
}

func subjectIncluded(subjects []k8srbac.Subject, subject k8srbac.Subject) bool {
	for _, s := range subjects {
		if s.Kind == subject.Kind && s.Name == subject.Name && s.Namespace == subject.Namespace {
			return true
		}
	}
	return false
}

func GetLeaderElectionRules() []k8srbac.PolicyRule {
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
