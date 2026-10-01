package deliveryrun_test

import (
	"testing"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

func TestCancelledDeploymentCanBeRetriedWithoutAutomaticScheduling(t *testing.T) {
	run := completedRun(t)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
	passIndependentQuality(t, &run)
	passBusinessAcceptance(t, &run)
	mustApply(t, &run, agent("op-agent"), "release_checks.replace", map[string]any{
		"environment_ref": "production",
		"checks":          []map[string]any{{"id": "RC-01", "title": "Production configuration verified", "required": true}},
	})
	mustApply(t, &run, agent("op-agent"), "release_check.record", map[string]any{
		"check_id": "RC-01", "status": "passed", "note": "Configuration verified",
		"evidence_refs": []string{"git:" + revision + "#evidence:evidence/release/RC-01.md"},
	})
	mustApply(t, &run, human("m-product"), "release.prepare", map[string]any{
		"version": "1.0.0", "environment_ref": "production",
	})
	releaseID := run.Releases[0].ID
	mustApply(t, &run, human("m-release"), "release.approve", map[string]any{"release_id": releaseID})
	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": releaseID, "outcome": "cancelled", "environment_ref": "production",
		"launch_url": "", "receipt_ref": "verdent://deployments/deploy-1/versions/version-1",
	})

	if run.Stage != delivery.StageRelease || run.Releases[0].Status != delivery.ReleaseCancelled {
		t.Fatalf("cancelled deployment did not remain resumable in release: %#v", run.Releases[0])
	}
	if hasProjectedAction(delivery.ProjectionFor(run), "release.deploy_result", releaseID) {
		t.Fatal("cancelled deployment was immediately scheduled again")
	}

	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": releaseID, "outcome": "success", "environment_ref": "production",
		"launch_url": "https://product.example", "receipt_ref": "verdent://deployments/deploy-2/versions/version-2",
	})
	if run.Stage != delivery.StageLive || run.Releases[0].Status != delivery.ReleaseLive || len(run.Releases[0].DeploymentAttempts) != 2 {
		t.Fatalf("retry after cancellation did not go live with both attempts retained: %#v", run.Releases[0])
	}
}

func TestFailedPlatformDeploymentCanRetryTheSameRelease(t *testing.T) {
	run := completedRun(t)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
	passIndependentQuality(t, &run)
	passBusinessAcceptance(t, &run)
	mustApply(t, &run, agent("op-agent"), "release_checks.replace", map[string]any{
		"environment_ref": "production",
		"checks":          []map[string]any{{"id": "RC-01", "title": "Production configuration verified", "required": true}},
	})
	mustApply(t, &run, agent("op-agent"), "release_check.record", map[string]any{
		"check_id": "RC-01", "status": "passed", "note": "Configuration verified",
		"evidence_refs": []string{"git:" + revision + "#evidence:evidence/release/RC-01.md"},
	})
	mustApply(t, &run, human("m-product"), "release.prepare", map[string]any{
		"version": "1.0.0", "environment_ref": "production",
	})
	releaseID := run.Releases[0].ID
	mustApply(t, &run, human("m-release"), "release.approve", map[string]any{"release_id": releaseID})
	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": releaseID, "outcome": "failure", "environment_ref": "production",
		"receipt_ref": "verdent://deployments/deploy-1/versions/version-1", "failure_kind": "platform",
	})
	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": releaseID, "outcome": "success", "environment_ref": "production",
		"launch_url": "https://product.example", "receipt_ref": "verdent://deployments/deploy-2/versions/version-2",
	})

	if len(run.Releases) != 1 || run.Releases[0].ID != releaseID || run.Releases[0].Version != "1.0.0" {
		t.Fatalf("retry created or changed the immutable release: %#v", run.Releases)
	}
	if run.Stage != delivery.StageLive || run.Releases[0].Status != delivery.ReleaseLive || len(run.Releases[0].DeploymentAttempts) != 2 {
		t.Fatalf("same release retry did not retain both attempts and go live: %#v", run.Releases[0])
	}
	if !hasProjectedAction(delivery.ProjectionFor(run), "release.deploy_result", releaseID) {
		t.Fatal("live release did not permit recording a subsequent deployment result")
	}
	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": releaseID, "outcome": "failure", "environment_ref": "production",
		"receipt_ref": "verdent://deployments/deploy-1/versions/version-3", "failure_kind": "platform",
	})
	if run.Stage != delivery.StageRelease || run.Releases[0].Status != delivery.ReleaseFailed || len(run.Releases[0].DeploymentAttempts) != 3 || run.AcceptanceReview.Status != delivery.AcceptanceReviewAccepted || run.ActiveDeliveryUnitID != "" {
		t.Fatalf("failure after a live deployment did not retain approval and permit retry: %#v", run)
	}
	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": releaseID, "outcome": "success", "environment_ref": "production",
		"launch_url": "https://product.example", "receipt_ref": "verdent://deployments/deploy-1/versions/version-4",
	})
	if run.Stage != delivery.StageLive || run.Releases[0].Status != delivery.ReleaseLive || len(run.Releases[0].DeploymentAttempts) != 4 || len(run.Releases) != 1 || run.Releases[0].CodeRevision != revision {
		t.Fatalf("live deployment retry did not retain the original release and every result: %#v", run.Releases)
	}
}
