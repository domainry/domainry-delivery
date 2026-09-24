package deliveryrun_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/testfixture"
)

type planeBackendGuideFixture struct {
	GitRevision string                                    `json:"git_revision"`
	Checks      map[string]*delivery.BackendGuideEvidence `json:"checks"`
}

func TestPlaneBackendGuideFixtureCompletesTheAuthoritativeDeliveryUnit(t *testing.T) {
	file, err := os.Open("testdata/plane_backend_guide_evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var fixture planeBackendGuideFixture
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Checks) != 3 {
		t.Fatalf("fixture must contain the model, contract, and journey checks: %#v", fixture.Checks)
	}

	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	applyFixturePhase(t, &run, agent("frontend-agent"), "delivery_unit.interaction.complete", "interaction_modeling", fixture.GitRevision, nil)
	applyFixturePhase(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "domain_modeling", fixture.GitRevision, nil)
	applyFixturePhase(t, &run, delivery.Actor{ID: "model-verifier", Kind: delivery.ActorSystem}, "delivery_unit.model.verify", "model_verification", fixture.GitRevision, fixture.Checks["delivery_unit.model.verify"])
	applyFixturePhase(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation", fixture.GitRevision, nil)
	applyFixturePhase(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_convergence", fixture.GitRevision, nil)
	applyFixturePhase(t, &run, delivery.Actor{ID: "contract-verifier", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification", fixture.GitRevision, fixture.Checks["delivery_unit.contract.verify"])
	applyFixturePhase(t, &run, delivery.Actor{ID: "journey-verifier", Kind: delivery.ActorSystem}, "delivery_unit.journey.complete", "journey_testing", fixture.GitRevision, fixture.Checks["delivery_unit.journey.complete"])

	projection := delivery.ProjectionFor(run)
	if run.Stage != delivery.StageTesting || run.ActiveDeliveryUnitID != "" || len(run.DeliveryUnits) != 1 || run.DeliveryUnits[0].Phase != delivery.DeliveryUnitComplete {
		t.Fatalf("Plane fixture did not complete the authoritative DeliveryUnit: %#v", run)
	}
	if len(projection.Workflow.ReleaseGates) < 4 || !projection.Workflow.ReleaseGates[3].OK {
		t.Fatalf("Plane backend guide gate is not satisfied: %#v", projection.Workflow.ReleaseGates)
	}
	unit := run.DeliveryUnits[0]
	for evidence, actorID := range map[*delivery.BackendGuideEvidence]string{
		unit.ModelEvidence: "model-verifier", unit.ContractEvidence: "contract-verifier", unit.JourneyEvidence: "journey-verifier",
	} {
		if evidence == nil || evidence.ExecutedBy != actorID || evidence.OccurredAt.IsZero() {
			t.Fatalf("trusted command context did not stamp evidence provenance: %#v", evidence)
		}
	}
}

func applyFixturePhase(t *testing.T, run *delivery.DeliveryRun, actor delivery.Actor, command string, phase delivery.DeliveryUnitPhase, revision string, evidence *delivery.BackendGuideEvidence) {
	t.Helper()
	payloadValue := map[string]any{
		"delivery_unit_id": run.ActiveDeliveryUnitID,
		"phase":            phase,
		"git_revision":     revision,
		"summary":          "The Plane backend guide check passed.",
	}
	if evidence != nil {
		payloadValue["backend_guide"] = evidence
	}
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		t.Fatal(err)
	}
	if err = delivery.Apply(run, delivery.Command{Actor: actor, Type: command, Payload: payload}, time.Now().UTC()); err != nil {
		t.Fatalf("apply %s from Plane fixture: %v", command, err)
	}
}
