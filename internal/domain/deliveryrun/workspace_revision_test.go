package deliveryrun_test

import (
	"strings"
	"testing"
	"time"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/testfixture"
)

func TestWorkspaceSnapshotDrivesDevelopmentAndQuality(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	revision := "workspace-" + strings.Repeat("a", 64)
	phases := []struct {
		command string
		phase   string
		actor   delivery.Actor
	}{
		{"delivery_unit.interaction.complete", "interaction_modeling", agent("rd")},
		{"delivery_unit.model.complete", "domain_modeling", agent("rd")},
		{"delivery_unit.model.verify", "model_verification", delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}},
		{"delivery_unit.frontend.complete", "frontend_implementation", agent("rd")},
		{"delivery_unit.backend.complete", "backend_implementation", agent("rd")},
		{"delivery_unit.contract.verify", "contract_verification", delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}},
		{"delivery_unit.journey.complete", "journey_testing", delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}},
	}
	for _, step := range phases {
		if err := applyUnitAtRevision(&run, step.actor, step.command, step.phase, revision, "", nil); err != nil {
			t.Fatalf("%s rejected workspace evidence: %v", step.phase, err)
		}
	}
	product := testfixture.DemoProduct(time.Now().UTC())
	content := product.Revisions[0]
	mustApply(t, &run, agent("rd"), "product_revision.record", map[string]any{
		"content":       map[string]any{"story": content.Story, "definition": content.Definition, "decisions": content.Decisions},
		"evidence_ref":  "workspace:" + revision + "#evidence:product-revision.json",
		"code_revision": revision, "model_sha256": strings.Repeat("d", 64),
	})
	passIndependentQuality(t, &run)
	if run.Stage != delivery.StageAcceptance || run.AcceptanceReview == nil || run.AcceptanceReview.CandidateGitRevision != revision {
		t.Fatalf("workspace snapshot did not reach business acceptance: %#v", run.AcceptanceReview)
	}
}
