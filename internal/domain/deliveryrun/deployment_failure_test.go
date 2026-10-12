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
			if run.AcceptanceReview == nil || run.AcceptanceReview.Status != delivery.AcceptanceReviewAccepted || run.AcceptanceReview.AcceptedBy == "" || run.AcceptanceReview.AcceptedAt == nil || len(run.AcceptanceReview.Bugs) != 0 {
				t.Fatalf("deployment failure corrupted the completed business acceptance: %#v", run.AcceptanceReview)
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
			if run.Stage != delivery.StageTesting || len(run.ExecutableRevision.RequiredTestCaseIDs) == 0 {
				t.Fatalf("deployment repair did not enter targeted testing: stage=%s required=%#v", run.Stage, run.ExecutableRevision.RequiredTestCaseIDs)
			}
			passIndependentQuality(t, &run)
			if run.Stage != delivery.StageRelease || run.AcceptanceReview.Status != delivery.AcceptanceReviewAccepted || len(run.AcceptanceReview.Bugs) != 0 {
				t.Fatalf("targeted testing did not return the deployment repair to release: stage=%s acceptance=%#v", run.Stage, run.AcceptanceReview)
			}
			mustApply(t, &run, agent("op-agent"), "release_checks.replace", map[string]any{
				"environment_ref": "production",
				"checks":          []map[string]any{{"id": "RC-02", "title": "Repaired production source verified", "required": true}},
			})
			mustApply(t, &run, agent("op-agent"), "release_check.record", map[string]any{
				"check_id": "RC-02", "status": "passed", "note": "Repaired source is ready for the approved target",
				"evidence_refs": []string{"workspace:" + repairedRevision + "#evidence:evidence/release/RC-02.md"},
			})
			projection = delivery.ProjectionFor(run)
			if !hasProjectedAction(projection, "release.reissue", releaseID) || hasProjectedAction(projection, "release.prepare", "") {
				t.Fatalf("verified deployment repair did not request an automatic replacement Release: %#v", projection.Workflow.AvailableActions)
			}
			mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.reissue", map[string]any{"release_id": releaseID})
			if len(run.Releases) != 2 || run.Releases[1].Status != delivery.ReleaseApproved || run.Releases[1].CodeRevision != repairedRevision || run.Releases[1].EnvironmentRef != run.Releases[0].EnvironmentRef || run.Releases[1].Version != run.Releases[0].Version {
				t.Fatalf("replacement Release did not retain the approved target on the repaired revision: %#v", run.Releases)
			}
			replacementID := run.Releases[1].ID
			mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
				"release_id": replacementID, "outcome": "failure", "environment_ref": "production",
				"launch_url": "", "receipt_ref": "verdent://deployments/deploy-1/versions/version-2",
				"failure_kind": "source", "failure_owner": "backend", "delivery_unit_id": run.DeliveryUnits[0].ID,
				"diagnostics": []map[string]any{{
					"code": "cloud_deployment_build_failed", "severity": "error", "path": "backend",
					"message": "the repaired source still fails the cloud build", "owner": "backend", "category": "cloud_build",
					"remediation": "Continue the targeted backend repair without rerunning unrelated phases.",
				}},
			})
			if run.Stage != delivery.StageAcceptance || run.ActiveDeliveryUnitID != run.DeliveryUnits[0].ID || run.DeliveryUnits[0].Phase != delivery.DeliveryUnitBackendImplementation || run.Releases[1].Status != delivery.ReleaseFailed {
				t.Fatalf("replacement Release failure was not routed back to the owning repair phase: stage=%s active=%q unit=%#v release=%#v", run.Stage, run.ActiveDeliveryUnitID, run.DeliveryUnits[0], run.Releases[1])
			}
		})
	}
}

func TestStaleApprovedReleaseIsReissuedForTheCurrentVerifiedRevision(t *testing.T) {
	run := completedRun(t)
	currentRevision := run.DeliveryUnits[0].IntegratedGitRevision
	passIndependentQuality(t, &run)
	passBusinessAcceptance(t, &run)
	mustApply(t, &run, agent("op-agent"), "release_checks.replace", map[string]any{
		"environment_ref": "production",
		"checks":          []map[string]any{{"id": "RC-01", "title": "Production configuration verified", "required": true}},
	})
	mustApply(t, &run, agent("op-agent"), "release_check.record", map[string]any{
		"check_id": "RC-01", "status": "passed", "note": "Configuration verified",
		"evidence_refs": []string{"workspace:" + currentRevision + "#evidence:evidence/release/RC-01.md"},
	})
	mustApply(t, &run, human("m-product"), "release.prepare", map[string]any{
		"version": "1.0.0", "environment_ref": "production",
	})
	staleReleaseID := run.Releases[0].ID
	mustApply(t, &run, human("m-release"), "release.approve", map[string]any{"release_id": staleReleaseID})
	run.Releases[0].CodeRevision = strings.Repeat("f", 40)
	run.Releases[0].ProductRevision = 0
	run.Releases[0].ProductRevisionRef = "workspace:stale#evidence:product-revision.json"
	run.Releases[0].ModelSHA256 = strings.Repeat("e", 64)

	projection := delivery.ProjectionFor(run)
	if len(projection.Workflow.AvailableActions) != 1 || !hasProjectedAction(projection, "release.reissue", staleReleaseID) || hasProjectedAction(projection, "release.deploy_result", staleReleaseID) {
		t.Fatalf("stale approved Release was not replaced before deployment: %#v", projection.Workflow.AvailableActions)
	}
	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.reissue", map[string]any{"release_id": staleReleaseID})
	if len(run.Releases) != 2 || run.Releases[0].Status != delivery.ReleaseCancelled {
		t.Fatalf("stale approved Release was not superseded: %#v", run.Releases)
	}
	replacement := run.Releases[1]
	if replacement.Status != delivery.ReleaseApproved || replacement.CodeRevision != currentRevision || replacement.ProductRevision != run.ExecutableRevision.TargetRevision || replacement.ProductRevisionRef != run.ExecutableRevision.EvidenceRef || replacement.ModelSHA256 != run.ExecutableRevision.ModelSHA256 || replacement.EnvironmentRef != run.Releases[0].EnvironmentRef || replacement.Version != run.Releases[0].Version || replacement.ApprovedBy != run.Releases[0].ApprovedBy {
		t.Fatalf("replacement Release did not bind the current verified source and approved target: %#v", replacement)
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
