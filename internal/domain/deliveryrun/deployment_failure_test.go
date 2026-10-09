package deliveryrun_test

import (
	"strings"
	"testing"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

func TestFailedSourceDeploymentRoutesOnlyTheOwningPhaseBackToRDAgent(t *testing.T) {
	for _, initialStatus := range []string{"approved", "live"} {
		t.Run(initialStatus, func(t *testing.T) {
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
				"evidence_refs": []string{"workspace:" + revision + "#evidence:evidence/release/RC-01.md"},
			})
			mustApply(t, &run, human("m-product"), "release.prepare", map[string]any{
				"version": "1.0.0", "environment_ref": "production",
			})
			releaseID := run.Releases[0].ID
			mustApply(t, &run, human("m-release"), "release.approve", map[string]any{"release_id": releaseID})
			if initialStatus == "live" {
				mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
					"release_id": releaseID, "outcome": "success", "environment_ref": "production",
					"launch_url": "https://product.example", "receipt_ref": "verdent://deployments/deploy-1/versions/version-live",
				})
			}
			mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
				"release_id": releaseID, "outcome": "failure", "environment_ref": "production",
				"launch_url": "", "receipt_ref": "verdent://deployments/deploy-1/versions/version-1",
				"failure_kind": "source", "failure_owner": "backend", "delivery_unit_id": run.DeliveryUnits[0].ID,
				"diagnostics": []map[string]any{{
					"code": "cloud_deployment_build_failed", "severity": "error", "path": "backend",
					"message": "go build failed in the cloud image build", "owner": "backend", "category": "cloud_build",
					"remediation": "Repair the backend build without rerunning unrelated phases.",
				}},
			})

			unit := run.DeliveryUnits[0]
			if run.Stage != delivery.StageAcceptance || run.ActiveDeliveryUnitID != unit.ID || unit.Phase != delivery.DeliveryUnitBackendImplementation {
				t.Fatalf("deployment failure did not reactivate the backend repair phase: stage=%s active=%q phase=%s", run.Stage, run.ActiveDeliveryUnitID, unit.Phase)
			}
			if unit.FrontendStatus != delivery.DeliveryGatePassed || unit.BackendStatus != delivery.DeliveryGateNeedsChange || unit.ContractStatus != delivery.DeliveryGatePending || unit.JourneyStatus != delivery.DeliveryGatePending {
				t.Fatalf("deployment repair reset unrelated work or failed to invalidate downstream gates: %#v", unit)
			}
			if len(unit.DevelopmentRepairItems) != 3 || unit.DevelopmentRepairItems[0].SourceKind != "cloud_deployment" {
				t.Fatalf("deployment failure did not create the targeted repair and independent final gate tasks: %#v", unit.DevelopmentRepairItems)
			}
			if len(run.ReleaseChecks) != 0 {
				t.Fatalf("old release checks survived a source repair: %#v", run.ReleaseChecks)
			}
			if run.AcceptanceReview == nil || run.AcceptanceReview.Status != delivery.AcceptanceReviewOpen || run.AcceptanceReview.AcceptedBy != "" || run.AcceptanceReview.AcceptedAt != nil {
				t.Fatalf("accepted review was not reopened for repaired-revision retest: %#v", run.AcceptanceReview)
			}
			bug := run.AcceptanceReview.Bugs[len(run.AcceptanceReview.Bugs)-1]
			if bug.Status != delivery.AcceptanceBugFixing || bug.Owner != "backend" || bug.DeliveryUnitID != unit.ID {
				t.Fatalf("deployment failure did not enter the acceptance repair loop: %#v", bug)
			}
			attempt := run.Releases[0].DeploymentAttempts[len(run.Releases[0].DeploymentAttempts)-1]
			if run.Releases[0].Status != delivery.ReleaseFailed || attempt.FailureKind != "source" || len(attempt.Diagnostics) != 1 {
				t.Fatalf("failed deployment receipt did not retain its repair evidence: %#v", run.Releases[0])
			}
			projection := delivery.ProjectionFor(run)
			if !hasProjectedAction(projection, "delivery_unit.backend.complete", unit.ID) || hasProjectedAction(projection, "release.prepare", "") {
				t.Fatalf("workflow did not hand the targeted repair to RD: %#v", projection.Workflow.AvailableActions)
			}
			repairedRevision := strings.Repeat("b", 40)
			advanceUnitAtRevision(t, &run, agent("rd-agent"), "delivery_unit.backend.complete", "backend_implementation", repairedRevision)
			verifier := delivery.Actor{ID: "verification-runner", Kind: delivery.ActorSystem}
			advanceUnitAtRevision(t, &run, verifier, "delivery_unit.contract.verify", "contract_verification", repairedRevision)
			advanceUnitAtRevision(t, &run, verifier, "delivery_unit.journey.complete", "journey_testing", repairedRevision)
			if !hasProjectedAction(delivery.ProjectionFor(run), "product_revision.record", "") {
				t.Fatal("verified deployment repair did not offer executable revision recording")
			}
			mustApply(t, &run, agent("rd-agent"), "product_revision.record", map[string]any{
				"content":       run.ExecutableRevision.Content,
				"evidence_ref":  "workspace:" + repairedRevision + "#evidence:evidence/product-revision.json",
				"code_revision": repairedRevision, "model_sha256": run.ExecutableRevision.ModelSHA256,
			})
			if run.ExecutableRevision.CodeRevision != repairedRevision || len(run.Releases) != 1 || run.Releases[0].ID != releaseID || run.Releases[0].CodeRevision != revision {
				t.Fatal("repair failed to bind the new revision while preserving the failed release history")
			}
		})
	}
}

func TestPlatformDeploymentFailureDoesNotInventSourceRepair(t *testing.T) {
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
		"evidence_refs": []string{"workspace:" + revision + "#evidence:evidence/release/RC-01.md"},
	})
	mustApply(t, &run, human("m-product"), "release.prepare", map[string]any{
		"version": "1.0.0", "environment_ref": "production",
	})
	releaseID := run.Releases[0].ID
	mustApply(t, &run, human("m-release"), "release.approve", map[string]any{"release_id": releaseID})
	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": releaseID, "outcome": "failure", "environment_ref": "production",
		"launch_url": "", "receipt_ref": "verdent://deployments/deploy-1/versions/version-1",
		"failure_kind": "platform", "failure_owner": "", "delivery_unit_id": "", "diagnostics": []map[string]any{},
	})

	if run.ActiveDeliveryUnitID != "" || run.AcceptanceReview == nil || run.AcceptanceReview.Status != delivery.AcceptanceReviewAccepted {
		t.Fatalf("platform failure was incorrectly routed into product source repair: %#v", run)
	}
}
