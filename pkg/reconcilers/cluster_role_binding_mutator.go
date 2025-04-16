package reconcilers

import (
	"fmt"

	authorinoResources "github.com/kuadrant/authorino-operator/pkg/resources"
	k8srbac "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterRoleBindingMutateFn is a function which mutates the existing ClusterRoleBinding into it's desired state.
type ClusterRoleBindingMutateFn func(desired, existing *k8srbac.ClusterRoleBinding) bool

func ClusterRoleBindingMutator(opts ...ClusterRoleBindingMutateFn) MutateFn {
	return func(desiredObj, existingObj client.Object) (bool, error) {
		existing, ok := existingObj.(*k8srbac.ClusterRoleBinding)
		if !ok {
			return false, fmt.Errorf("%T is not a *k8srbac.ClusterRoleBinding", existingObj)
		}
		desired, ok := desiredObj.(*k8srbac.ClusterRoleBinding)
		if !ok {
			return false, fmt.Errorf("%T is not a *k8srbac.ClusterRoleBinding", desiredObj)
		}

		update := false

		// Loop through each option
		for _, opt := range opts {
			tmpUpdate := opt(desired, existing)
			update = update || tmpUpdate
		}

		return update, nil
	}
}

func ClusterRoleBindingLabelsMutator(desired, existing *k8srbac.ClusterRoleBinding) bool {
	return authorinoResources.MergeMapStringString(&existing.ObjectMeta.Labels, desired.ObjectMeta.Labels)
}

// ClusterRoleBindingSubjectMutator merges subject entries from the desired binding into the existing binding.
//
// The subject entries included in "existing" binding that are not included in the "desired" binding are preserved.
//
// It returns true if the existing binding was modified (i.e., at least one subject was added),
// and false otherwise.
func ClusterRoleBindingSubjectMutator(desired, existing *k8srbac.ClusterRoleBinding) bool {
	return authorinoResources.MergeBindingSubject(desired.Subjects, &existing.Subjects)
}
