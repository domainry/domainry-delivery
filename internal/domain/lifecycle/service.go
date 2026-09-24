package lifecycle

import (
	"strings"
	"time"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

// StartDelivery atomically prepares a single-Feature DeliveryRun and updates the
// Product aggregate. Persistence must commit both returned states together.
func StartDelivery(product *Product, deliveryRunID string, command Command, now time.Time) (DeliveryRun, error) {
	if command.Type != commanddomain.FeatureDeliveryStart {
		return DeliveryRun{}, Invalid("command_unknown", "Delivery start requires feature.delivery.start.")
	}
	if err := validateCommandActor(command, CommandTargetProduct); err != nil {
		return DeliveryRun{}, err
	}
	if command.Actor.Kind != ActorAgent {
		return DeliveryRun{}, Invalid("agent_execution_required", "A local Agent Runtime must start delivery.")
	}
	if product.Engineering.Status != EngineeringReady {
		return DeliveryRun{}, Invalid("product_engineering_incomplete", "Product engineering initialization must be ready before a confirmed Feature can start delivery.")
	}
	var payload struct {
		FeatureID       string   `json:"feature_id"`
		FeatureRevision uint64   `json:"feature_revision"`
		Name            string   `json:"name"`
		Code            string   `json:"code"`
		Goal            string   `json:"goal"`
		TargetDate      string   `json:"target_date"`
		Members         []Member `json:"members"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return DeliveryRun{}, err
	}
	spec := DeliveryRunSpec{
		ID: strings.TrimSpace(deliveryRunID), Name: payload.Name, Code: payload.Code,
		Goal: payload.Goal, TargetDate: payload.TargetDate, Members: payload.Members,
	}
	next := nextFeatureForDelivery(product)
	if next == nil || next.ID != strings.TrimSpace(payload.FeatureID) {
		return DeliveryRun{}, Invalid("feature_delivery_order_invalid", "Only the first complete Feature in the Product development queue can start delivery.")
	}
	run, err := NewDeliveryRun(*product, payload.FeatureID, payload.FeatureRevision, spec, command.Actor, now)
	if err != nil {
		return DeliveryRun{}, err
	}
	feature := findFeature(product, strings.TrimSpace(payload.FeatureID))
	feature.Status = FeatureDelivering
	feature.DeliveryRunID = run.ID
	feature.UpdatedAt = now
	product.UpdatedAt = now
	return run, nil
}

// InstallDeliveryRun applies a successful live release to the Product. The
// caller must persist this Product update in the same transaction as the final
// DeliveryRun command.
func InstallDeliveryRun(product *Product, run *DeliveryRun, actor Actor, now time.Time) error {
	if actor.Kind != ActorSystem && actor.Kind != ActorHuman {
		return Invalid("installation_actor_invalid", "Only a trusted deployment adapter or reconciliation owner can install a Feature.")
	}
	if run.Stage != StageLive {
		return Invalid("delivery_run_not_live", "A Feature can be installed only from a live DeliveryRun.")
	}
	feature := findFeature(product, run.Feature.ID)
	if feature == nil {
		return NotFound("Feature", run.Feature.ID)
	}
	if feature.Status != FeatureDelivering || feature.DeliveryRunID != run.ID || feature.ConfirmedRevision != run.Feature.Source.FeatureRevision {
		return Invalid("feature_not_delivering", "The live DeliveryRun does not match the delivering FeatureRevision.")
	}
	if run.Product.ProductRevision != product.CurrentReleaseRevision {
		return Invalid("feature_install_baseline_invalid", "The live DeliveryRun no longer matches the Product release baseline.")
	}
	if run.ExecutableRevision == nil {
		return Invalid("product_revision_missing", "The live DeliveryRun has no executable ProductRevision to install.")
	}
	executable := run.ExecutableRevision
	if executable.BaseRevision != product.CurrentDefinitionRevision || executable.TargetRevision != product.CurrentDefinitionRevision+1 {
		return Invalid("product_revision_baseline_invalid", "The executable ProductRevision no longer extends the current Product definition.")
	}
	var liveRelease *Release
	for index := range run.Releases {
		if run.Releases[index].Status == ReleaseLive {
			liveRelease = &run.Releases[index]
		}
	}
	if liveRelease == nil || liveRelease.DeploymentAttempt == nil || strings.TrimSpace(liveRelease.DeploymentAttempt.ReceiptRef) == "" {
		return Invalid("release_receipt_missing", "A live release with a deployment receipt is required to install the Feature.")
	}
	if strings.TrimSpace(liveRelease.DeploymentAttempt.LaunchURL) == "" || liveRelease.DeploymentAttempt.ResolvedAt == nil {
		return Invalid("release_launch_url_missing", "A live SaaS release requires a resolved public launch URL.")
	}
	if liveRelease.ProductRevision != executable.TargetRevision || liveRelease.ProductRevisionRef != executable.EvidenceRef || liveRelease.CodeRevision != executable.CodeRevision || liveRelease.ModelSHA256 != executable.ModelSHA256 {
		return Invalid("product_revision_release_mismatch", "The live Release is not bound to the executable ProductRevision produced by this DeliveryRun.")
	}
	product.Revisions = append(product.Revisions, ProductRevision{
		Number: executable.TargetRevision, Story: clone(executable.Content.Story),
		Definition: clone(executable.Content.Definition), Decisions: clone(executable.Content.Decisions),
		SourceFeatureID: feature.ID, FeatureRevision: run.Feature.Source.FeatureRevision,
		DeliveryRunID: run.ID, ReleaseID: liveRelease.ID, CodeRevision: liveRelease.CodeRevision,
		DefinitionRef: executable.EvidenceRef, ModelSHA256: executable.ModelSHA256,
		CreatedBy: executable.RecordedBy, CreatedAt: now,
	})
	product.CurrentDefinitionRevision = executable.TargetRevision
	product.CurrentReleaseRevision = executable.TargetRevision
	feature.Status = FeatureInstalled
	feature.InstalledReleaseID = liveRelease.ID
	feature.UpdatedAt = now
	product.Status = ProductActive
	product.CurrentDeployment = &ProductDeployment{
		ReleaseID:      liveRelease.ID,
		Version:        liveRelease.Version,
		EnvironmentRef: liveRelease.DeploymentAttempt.EnvironmentRef,
		LaunchURL:      liveRelease.DeploymentAttempt.LaunchURL,
		ReceiptRef:     liveRelease.DeploymentAttempt.ReceiptRef,
		DeployedAt:     *liveRelease.DeploymentAttempt.ResolvedAt,
	}
	product.UpdatedAt = now
	return nil
}
