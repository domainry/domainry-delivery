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
	applyFixturePhase(t, &run, agent("engineer"), "delivery_unit.implementation.complete", "implementation", fixture.GitRevision, nil)
	evidence := backendGuideEvidence("delivery_unit.verify")
	bindBackendGuideEvidence(&run, evidence, fixture.GitRevision, "integrated-verifier")
	applyFixturePhase(t, &run, delivery.Actor{ID: "integrated-verifier", Kind: delivery.ActorSystem}, "delivery_unit.verify", "verification", fixture.GitRevision, evidence)

	projection := delivery.ProjectionFor(run)
	if run.Stage != delivery.StageTesting || run.ActiveDeliveryUnitID != "" || len(run.DeliveryUnits) != 1 || run.DeliveryUnits[0].Phase != delivery.DeliveryUnitComplete {
		t.Fatalf("Plane fixture did not complete the authoritative DeliveryUnit: %#v", run)
	}
	if len(projection.Workflow.ReleaseGates) < 4 || !projection.Workflow.ReleaseGates[3].OK {
		t.Fatalf("Plane backend guide gate is not satisfied: %#v", projection.Workflow.ReleaseGates)
	}
	unit := run.DeliveryUnits[0]
	if unit.VerificationEvidence == nil || unit.VerificationEvidence.ExecutedBy != "integrated-verifier" || unit.VerificationEvidence.OccurredAt.IsZero() {
		t.Fatalf("trusted command context did not stamp evidence provenance: %#v", unit.VerificationEvidence)
	}
}

func applyFixturePhase(t *testing.T, run *delivery.DeliveryRun, actor delivery.Actor, command string, phase delivery.DeliveryUnitPhase, revision string, evidence *delivery.BackendGuideEvidence) {
	t.Helper()
	payloadValue := map[string]any{
		"delivery_unit_id": run.ActiveDeliveryUnitID,
		"phase":            phase,
		"git_revision":     revision,
		"summary":          "The Plane backend guide check passed.",
		"evidence_refs":    []string{"workspace:" + revision + "#evidence:" + string(phase) + ".json"},
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
