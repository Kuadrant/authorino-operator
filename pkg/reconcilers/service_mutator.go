package reconcilers

import (
	"fmt"
	"reflect"

	k8score "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceMutateFn is a function which mutates the existing Service into it's desired state.
type ServiceMutateFn func(desired, existing *k8score.Service) bool

func ServiceMutator(opts ...ServiceMutateFn) MutateFn {
	return func(desiredObj, existingObj client.Object) (bool, error) {
		existing, ok := existingObj.(*k8score.Service)
		if !ok {
			return false, fmt.Errorf("%T is not a *k8score.Service", existingObj)
		}
		desired, ok := desiredObj.(*k8score.Service)
		if !ok {
			return false, fmt.Errorf("%T is not a *k8score.Service", desiredObj)
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

func LabelsMutator(desired, existing *k8score.Service) bool {
	update := false

	if !reflect.DeepEqual(existing.ObjectMeta.Labels, desired.ObjectMeta.Labels) {
		existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
		update = true
	}

	return update
}

func PortMutator(desired, existing *k8score.Service) bool {
	update := false

	if !reflect.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) {
		existing.Spec.Ports = desired.Spec.Ports
		update = true
	}

	return update
}

func SelectorMutator(desired, existing *k8score.Service) bool {
	update := false

	if !reflect.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		existing.Spec.Selector = desired.Spec.Selector
		update = true
	}

	return update
}
