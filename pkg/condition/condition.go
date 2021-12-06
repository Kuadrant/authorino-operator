package condition

import (
	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddOrUpdateStatusConditions includes the new conditions to the array of conditions, if a condition is already present in
// array, then the condition will be updated
// the current array of conditions is returned with a flag indicating if the conditions were updated or not
func AddOrUpdateStatusConditions(conditions []api.Condition, newConditions ...api.Condition) ([]api.Condition, bool) {
	var atLeastOneUpdated bool
	var updated bool
	for _, cond := range newConditions {
		conditions, updated = addOrUpdateStatusCondition(conditions, cond)
		atLeastOneUpdated = atLeastOneUpdated || updated
	}

	return conditions, atLeastOneUpdated
}

func addOrUpdateStatusCondition(conditions []api.Condition, newCondition api.Condition) ([]api.Condition, bool) {
	now := v1.Now()
	newCondition.LastTransitionTime = now

	if conditions == nil {
		return []api.Condition{newCondition}, true
	}
	for i, cond := range conditions {
		if cond.Type == newCondition.Type {
			// Condition already present. Update it if needed.
			if cond.Status == newCondition.Status &&
				cond.Reason == newCondition.Reason &&
				cond.Message == newCondition.Message {
				// Nothing changed. No need to update.
				return conditions, false
			}

			// Update LastTransitionTime only if the status changed otherwise keep the old time
			if newCondition.Status == cond.Status {
				newCondition.LastTransitionTime = cond.LastTransitionTime
			}
			// Don't modify the currentConditions slice. Generate a new slice instead.
			res := make([]api.Condition, len(conditions))
			copy(res, conditions)
			res[i] = newCondition
			return res, true
		}
	}
	return append(conditions, newCondition), true
}
