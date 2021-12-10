package resources

import (
	k8srbac "k8s.io/api/rbac/v1"
)

func GetAuthorinoClusterRoleBinding(clusterRoleName, saName, saNamespace string) *k8srbac.ClusterRoleBinding {
	roleRef, roleSubject := getRoleRefAndSubject(clusterRoleName, saName, saNamespace)
	return &k8srbac.ClusterRoleBinding{
		RoleRef:  roleRef,
		Subjects: []k8srbac.Subject{roleSubject},
	}
}

func GetAuthorinoRoleBinding(roleName, saName, saNamespace string) *k8srbac.RoleBinding {
	roleRef, roleSubject := getRoleRefAndSubject(roleName, saName, saNamespace)
	return &k8srbac.RoleBinding{
		RoleRef:  roleRef,
		Subjects: []k8srbac.Subject{roleSubject},
	}
}

func GetAuthorinoLeaderElectionRoleBinding(roleName, saName, saNamespace string) *k8srbac.RoleBinding {
	return GetAuthorinoRoleBinding(roleName, saName, saNamespace)
}

func getRoleRefAndSubject(clusterRoleName, saName, saNamespace string) (k8srbac.RoleRef, k8srbac.Subject) {
	var roleRef = k8srbac.RoleRef{
		Name: clusterRoleName,
		Kind: "ClusterRole",
	}
	var roleSubject = k8srbac.Subject{
		Kind:      k8srbac.ServiceAccountKind,
		Name:      saName,
		Namespace: saNamespace,
	}

	return roleRef, roleSubject
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
