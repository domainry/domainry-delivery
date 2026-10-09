package deliveryrun_test

import (
	"encoding/json"
	"testing"
	"time"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

func TestUnpublishRetainsAcceptedRevisionAndAllowsSameVersionRepublish(t *testing.T) {
	run := completedRun(t)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
	passIndependentQuality(t, &run)
	passBusinessAcceptance(t, &run)
	mustApply(t, &run, agent("op-agent"), "release_checks.replace", map[string]any{
		"environment_ref": "production", "checks": []map[string]any{{"id": "RC-01", "title": "Production configuration verified", "required": true}},
	})
	mustApply(t, &run, agent("op-agent"), "release_check.record", map[string]any{
		"check_id": "RC-01", "status": "passed", "note": "Configuration verified", "evidence_refs": []string{"workspace:" + revision + "#evidence:evidence/release/RC-01.md"},
	})
	mustApply(t, &run, human("m-product"), "release.prepare", map[string]any{"version": "1.0.0", "environment_ref": "production"})
	releaseID := run.Releases[0].ID
	mustApply(t, &run, human("m-release"), "release.approve", map[string]any{"release_id": releaseID})
	adapter := delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}
	const receipt = "verdent://deployments/deploy-1/versions/version-1"
	mustApply(t, &run, adapter, "release.deploy_result", map[string]any{"release_id": releaseID, "outcome": "success", "environment_ref": "production", "launch_url": "https://product.example", "receipt_ref": receipt})
	wrong, _ := json.Marshal(map[string]any{"release_id": releaseID, "outcome": "unpublished", "environment_ref": "production", "receipt_ref": "receipt://different"})
	if err := delivery.Apply(&run, delivery.Command{Actor: adapter, Type: "release.deploy_result", Payload: wrong}, time.Now().UTC()); err == nil {
		t.Fatal("unrelated cloud receipt could remove the deployment")
	}
	mustApply(t, &run, adapter, "release.deploy_result", map[string]any{"release_id": releaseID, "outcome": "unpublished", "environment_ref": "production", "receipt_ref": receipt})
	if run.Releases[0].Status != delivery.ReleaseUnpublished || run.Stage != delivery.StageRelease || run.AcceptanceReview.Status != delivery.AcceptanceReviewAccepted || run.ActiveDeliveryUnitID != "" || run.Releases[0].CodeRevision != revision {
		t.Fatalf("unpublish reset accepted implementation: %#v", run)
	}
	if hasProjectedAction(delivery.ProjectionFor(run), "release.deploy_result", releaseID) {
		t.Fatal("unpublish automatically republished the product")
	}
	mustApply(t, &run, adapter, "release.deploy_result", map[string]any{"release_id": releaseID, "outcome": "success", "environment_ref": "production", "launch_url": "https://product.example", "receipt_ref": "verdent://deployments/deploy-1/versions/version-2"})
	if run.Stage != delivery.StageLive || len(run.Releases) != 1 || run.Releases[0].Version != "1.0.0" || len(run.Releases[0].DeploymentAttempts) != 3 {
		t.Fatalf("same-version republish failed: %#v", run.Releases)
	}
}
