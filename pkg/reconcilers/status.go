package reconcilers

import (
	"context"

	"github.com/go-logr/logr"
	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	"github.com/kuadrant/authorino-operator/pkg/condition"
	k8score "k8s.io/api/core/v1"
)

type statusUpdater func(logger logr.Logger, authorino *api.Authorino, message string) error

// WrapErrorWithStatusUpdate wraps the error and update the status. If the update failed then logs the error.
func (r *AuthorinoReconciler) WrapErrorWithStatusUpdate(logger logr.Logger, authorino *api.Authorino, updateStatus statusUpdater, err error) error {
	if err == nil {
		return nil
	}
	if err := updateStatus(logger, authorino, err.Error()); err != nil {
		logger.Error(err, "status update failed")
	}
	return err
}

func (r *AuthorinoReconciler) SetStatusFailed(reason string) statusUpdater {
	return func(logger logr.Logger, authorino *api.Authorino, message string) error {
		return r.updateStatusConditions(
			logger,
			authorino,
			statusNotReady(reason, message),
		)
	}
}

func (r *AuthorinoReconciler) updateStatusConditions(logger logr.Logger, authorino *api.Authorino, newConditions ...api.Condition) error {
	var updated bool
	authorino.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(authorino.Status.Conditions, newConditions...)
	if !updated {
		logger.Info("Authorino status conditions not changed")
		return nil
	}
	return r.Client.Status().Update(context.TODO(), authorino)
}

func statusReady() api.Condition {
	return api.Condition{
		Type:   api.ConditionReady,
		Status: k8score.ConditionTrue,
		Reason: statusProvisioned,
	}
}

func statusNotReady(reason, message string) api.Condition {
	return api.Condition{
		Type:    api.ConditionReady,
		Status:  k8score.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}
