package lifecycle

import (
	"encoding/json"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

// RecordDeploymentRemoval clears only the exact deployment whose removal was
// recorded by the trusted adapter. The caller persists Product and Run together.
func RecordDeploymentRemoval(product *Product, run *DeliveryRun, command Command, now time.Time) (bool, error) {
	var payload struct {
		ReleaseID string                        `json:"release_id"`
		Outcome   deliveryrun.DeploymentOutcome `json:"outcome"`
	}
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		return false, err
	}
	if payload.Outcome != deliveryrun.DeploymentUnpublished {
		return false, nil
	}
	if command.Actor.Kind != ActorSystem || run.Product.ID != product.ID {
		return false, Invalid("deployment_removal_actor_invalid")
	}
	for _, release := range run.Releases {
		if release.ID != payload.ReleaseID {
			continue
		}
		if release.Status != deliveryrun.ReleaseUnpublished || len(release.DeploymentAttempts) == 0 {
			return false, Invalid("deployment_removal_receipt_missing")
		}
		attempt := release.DeploymentAttempts[len(release.DeploymentAttempts)-1]
		if attempt.Outcome != deliveryrun.DeploymentUnpublished || attempt.ResolvedAt == nil {
			return false, Invalid("deployment_removal_receipt_missing")
		}
		current := product.CurrentDeployment
		if current != nil && current.ReleaseID == release.ID && current.Version == release.Version && current.EnvironmentRef == attempt.EnvironmentRef && current.ReceiptRef == attempt.ReceiptRef {
			product.CurrentDeployment = nil
			product.UpdatedAt = now
		}
		return true, nil
	}
	return false, NotFound("Release", payload.ReleaseID)
}
