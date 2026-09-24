package deliveryrun_test

import (
	"encoding/json"
	"testing"
	"time"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/testfixture"
)

func TestDeliveryRunFreezesOneFeatureRevisionAndGeneratesTests(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	if run.Feature.ID != "F-001" || run.Feature.Source.FeatureRevision != 1 || run.Product.ProductRevision != 1 {
		t.Fatalf("run did not freeze exact product and feature revisions: %#v", run)
	}
	if len(run.Feature.Source.ConversationIDs) != 1 || len(run.Feature.Source.AgentRuns) != 1 || run.Feature.Source.AgentRuns[0].RunID == "" {
		t.Fatalf("run lost exact Agent source: %#v", run.Feature.Source)
	}
	if len(run.TestCases) != len(run.Feature.Specification.Acceptance) || len(run.AcceptanceCases) != len(run.Feature.Specification.Acceptance) || len(run.DeliveryUnits) == 0 {
		t.Fatalf("run derived the wrong execution objects: units=%d tests=%d uat=%d", len(run.DeliveryUnits), len(run.TestCases), len(run.AcceptanceCases))
	}
}

func TestOnlyDeploymentAdapterCanRecordDeploymentResult(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	err := apply(&run, human("m-release"), "release.deploy_result", map[string]any{})
	assertCode(t, err, "system_execution_required")
}

func hasProjectedAction(projection delivery.Projection, command, targetID string) bool {
	for _, action := range projection.Workflow.AvailableActions {
		if action.Command == command && action.TargetID == targetID {
			return true
		}
	}
	return false
}

func mustApply(t *testing.T, run *delivery.DeliveryRun, actor delivery.Actor, commandType string, payload any) {
	t.Helper()
	if err := apply(run, actor, commandType, payload); err != nil {
		t.Fatalf("%s failed: %v", commandType, err)
	}
}

func apply(run *delivery.DeliveryRun, actor delivery.Actor, commandType string, payload any) error {
	raw, _ := json.Marshal(payload)
	return delivery.Apply(run, delivery.Command{Actor: actor, Type: commandType, Payload: raw}, time.Now().UTC())
}

func human(id string) delivery.Actor { return delivery.Actor{ID: id, Kind: delivery.ActorHuman} }
func agent(id string) delivery.Actor { return delivery.Actor{ID: id, Kind: delivery.ActorAgent} }

func assertCode(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", expected)
	}
	domainError, ok := err.(*delivery.Error)
	if !ok || domainError.Code != expected {
		t.Fatalf("expected %s, got %#v", expected, err)
	}
}
