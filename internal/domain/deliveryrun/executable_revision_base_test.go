package deliveryrun_test

import (
	"strings"
	"testing"
	"time"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

func TestInstalledSourceRepairAllocatesANewRevisionWithoutChangingFeatureBaseline(t *testing.T) {
	run := completedRun(t)
	now := time.Now().UTC()
	originalBase := run.Feature.BaselineProductRevision
	originalCode := strings.Repeat("b", 40)
	run.Releases = []delivery.Release{{
		ID: "REL-001", Status: delivery.ReleaseFailed, ProductRevision: originalBase + 1,
		CodeRevision: originalCode,
		DeploymentAttempts: []delivery.DeploymentAttempt{{
			Outcome: delivery.DeploymentSuccess, ResolvedAt: &now,
			ReceiptRef: "receipt://original-install", LaunchURL: "https://product.test",
		}},
	}}
	repaired := run.ExecutableRevision
	run.Releases = append(run.Releases, delivery.Release{
		ID: "REL-002", Status: delivery.ReleaseApproved, ProductRevision: originalBase + 1,
		CodeRevision: repaired.CodeRevision, ProductRevisionRef: repaired.EvidenceRef,
		ModelSHA256: repaired.ModelSHA256, EnvironmentRef: "production",
	})
	mustApply(t, &run, delivery.Actor{ID: "release-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": "REL-002", "outcome": "failure", "environment_ref": "production",
		"receipt_ref": "receipt://revision-installation-rejected", "failure_kind": "platform",
	})
	mustApply(t, &run, agent("rd-agent"), "product_revision.record", map[string]any{
		"content": repaired.Content, "code_revision": repaired.CodeRevision,
		"evidence_ref": repaired.EvidenceRef, "model_sha256": repaired.ModelSHA256,
	})
	if run.ExecutableRevision.BaseRevision != originalBase+1 || run.ExecutableRevision.TargetRevision != originalBase+2 {
		t.Fatalf("repair reused installed revision: %#v", run.ExecutableRevision)
	}
	if run.Feature.BaselineProductRevision != originalBase || run.Product.ProductRevision != originalBase || run.Releases[0].CodeRevision != originalCode {
		t.Fatal("repair rewrote the frozen Feature or installed Release")
	}
	passIndependentQuality(t, &run)
	passBusinessAcceptance(t, &run)
	if run.Stage != delivery.StageRelease {
		t.Fatalf("verified repair did not reach release: %s", run.Stage)
	}
}

func TestRedeploymentOfInstalledCodeKeepsItsExistingRevision(t *testing.T) {
	run := completedRun(t)
	passIndependentQuality(t, &run)
	passBusinessAcceptance(t, &run)
	now := time.Now().UTC()
	run.Releases = []delivery.Release{{
		ID: "REL-001", Status: delivery.ReleaseLive,
		ProductRevision: run.ExecutableRevision.TargetRevision,
		CodeRevision:    run.ExecutableRevision.CodeRevision,
		DeploymentAttempts: []delivery.DeploymentAttempt{{
			Outcome: delivery.DeploymentSuccess, ResolvedAt: &now,
			ReceiptRef: "receipt://installed", LaunchURL: "https://product.test",
		}},
	}}
	for _, gate := range delivery.ProjectionFor(run).Workflow.ReleaseGates {
		if gate.Code == "product_revision_bound" && !gate.OK {
			t.Fatal("same-code redeployment invalidated its installed revision")
		}
	}
}
