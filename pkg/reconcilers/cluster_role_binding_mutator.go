package reconcilers

import (
	"fmt"
	"reflect"

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
	update := false

	if !reflect.DeepEqual(existing.ObjectMeta.Labels, desired.ObjectMeta.Labels) {
		existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
		update = true
	}

	return update
}

func ClusterRoleBindingSubjectMutator(desired, existing *k8srbac.ClusterRoleBinding) bool {
	update := false

	if !reflect.DeepEqual(existing.Subjects, desired.Subjects) {
		existing.Subjects = desired.Subjects
		update = true
	}

	return update
}
