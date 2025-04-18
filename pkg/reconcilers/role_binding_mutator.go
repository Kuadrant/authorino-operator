package reconcilers

import (
	"fmt"
	"reflect"

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
	update := false

	if !reflect.DeepEqual(existing.ObjectMeta.Labels, desired.ObjectMeta.Labels) {
		existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
		update = true
	}

	return update
}

func RoleBindingSubjectMutator(desired, existing *k8srbac.RoleBinding) bool {
	update := false

	if !reflect.DeepEqual(existing.Subjects, desired.Subjects) {
		existing.Subjects = desired.Subjects
		update = true
	}

	return update
}
