package reconcilers

import (
	"context"
	"github.com/go-logr/logr"
	api "github.com/kuadrant/authorino-operator/api/v1beta1"
	"github.com/kuadrant/authorino-operator/pkg/condition"
	k8score "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			authorino,
			statusNotReady(reason, message),
		)
	}
}

func (r *AuthorinoReconciler) updateStatusConditions(authorino *api.Authorino, newConditions ...api.Condition) error {
	newStatus := api.AuthorinoStatus{}
	newStatus.Conditions, _ = condition.AddOrUpdateStatusConditions(authorino.Status.Conditions, newConditions...)

	patch := &api.Authorino{
		TypeMeta: metav1.TypeMeta{
			APIVersion: api.GroupVersion.String(),
			Kind:       "Authorino",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      authorino.Name,
			Namespace: authorino.Namespace,
		},
		Status: newStatus,
	}

	return r.Client.Status().Patch(context.TODO(), patch, client.Apply, client.ForceOwnership, client.FieldOwner("authorino-operator"))
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
