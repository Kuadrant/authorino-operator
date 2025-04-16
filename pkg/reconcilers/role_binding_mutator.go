package reconcilers

import (
	"fmt"

	authorinoResources "github.com/kuadrant/authorino-operator/pkg/resources"
	k8srbac "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RoleBindingMutateFn is a function which mutates the existing RoleBinding into it's desired state.
type RoleBindingMutateFn func(desired, existing *k8srbac.RoleBinding) bool

func RoleBindingMutator(opts ...RoleBindingMutateFn) MutateFn {
	return func(desiredObj, existingObj client.Object) (bool, error) {
		existing, ok := existingObj.(*k8srbac.RoleBinding)
		if !ok {
			return false, fmt.Errorf("%T is not a *k8srbac.RoleBinding", existingObj)
		}
		desired, ok := desiredObj.(*k8srbac.RoleBinding)
		if !ok {
			return false, fmt.Errorf("%T is not a *k8srbac.RoleBinding", desiredObj)
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

func RoleBindingLabelsMutator(desired, existing *k8srbac.RoleBinding) bool {
	return authorinoResources.MergeMapStringString(&existing.ObjectMeta.Labels, desired.ObjectMeta.Labels)
}

// RoleBindingSubjectMutator merges subject entries from the desired binding into the existing binding.
//
// The subject entries included in "existing" binding that are not included in the "desired" binding are preserved.
//
// It returns true if the existing binding was modified (i.e., at least one subject was added),
// and false otherwise.
func RoleBindingSubjectMutator(desired, existing *k8srbac.RoleBinding) bool {
	return authorinoResources.MergeBindingSubject(desired.Subjects, &existing.Subjects)
}
