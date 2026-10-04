package deliveryrun_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

func TestRepeatedModelRepairsRetainEachVerifiedImpactReceipt(t *testing.T) {
	run := completedRun(t)
	unit := &run.DeliveryUnits[0]
	run.Stage = delivery.StageDevelopment
	run.ActiveDeliveryUnitID = unit.ID
	var expected []delivery.ModelRepairImpact
	for _, revision := range []string{strings.Repeat("b", 40), strings.Repeat("c", 40)} {
		unit.Phase = delivery.DeliveryUnitJourneyTesting
		reportGap(t, &run, "model", "journey_testing", unit.IntegratedGitRevision)
		baselineRevision, baselineHash := unit.ModelGitRevision, unit.ModelEvidence.ModelSHA256
		advanceUnitAtRevision(t, &run, agent("model"), "delivery_unit.model.complete", "domain_modeling", revision)
		advanceUnitAtRevision(t, &run, delivery.Actor{ID: "model-runner", Kind: delivery.ActorSystem}, "delivery_unit.model.verify", "model_verification", revision)
		expected = append(expected, delivery.ModelRepairImpact{BaselineGitRevision: baselineRevision, BaselineModelSHA256: baselineHash,
			CurrentGitRevision: revision, CurrentModelSHA256: unit.ModelEvidence.ModelSHA256, SourceImpact: "unchanged"})
	}
	if !reflect.DeepEqual(unit.ModelRepairImpactHistory, expected) {
		t.Fatalf("impact history changed: %#v", unit.ModelRepairImpactHistory)
	}
	if !reflect.DeepEqual(unit.ModelRepairImpact, &expected[1]) {
		t.Fatal("latest impact does not identify the latest verified source")
	}
}

func TestAgentContextKeepsLatestModelImpactWithoutHistoricalComparisons(t *testing.T) {
	run := completedRun(t)
	receipt := delivery.ModelRepairImpact{BaselineGitRevision: strings.Repeat("a", 40), CurrentGitRevision: strings.Repeat("b", 40),
		BaselineModelSHA256: strings.Repeat("c", 64), CurrentModelSHA256: strings.Repeat("d", 64), SourceImpact: "permissions_only"}
	run.DeliveryUnits[0].ModelRepairImpact = &receipt
	for index := 0; index < 100; index++ {
		run.DeliveryUnits[0].ModelRepairImpactHistory = append(run.DeliveryUnits[0].ModelRepairImpactHistory, receipt)
	}
	before, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	context := delivery.AgentContextFor(run)
	if context.DeliveryRun.DeliveryUnits[0].ModelRepairImpactHistory != nil || !reflect.DeepEqual(context.DeliveryRun.DeliveryUnits[0].ModelRepairImpact, &receipt) {
		t.Fatal("Agent context did not isolate the latest impact from historical records")
	}
	serialized, err := json.Marshal(context)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "model_repair_impact_history") {
		t.Fatal("Historical model comparisons entered Agent context")
	}
	after, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("Building Agent context mutated the authoritative history")
	}
}

func TestInteractionRepairSchedulesAffectedRolesAndRetainsUnchangedModel(t *testing.T) {
	for _, roles := range [][]string{{}, {"frontend"}, {"frontend", "backend"}} {
		t.Run("interaction_"+strings.Join(roles, "_"), func(t *testing.T) {
			run := completedRun(t)
			unit := &run.DeliveryUnits[0]
			run.Stage = delivery.StageDevelopment
			run.ActiveDeliveryUnitID = unit.ID
			unit.Phase = delivery.DeliveryUnitJourneyTesting
			revision := unit.IntegratedGitRevision
			before, err := json.Marshal([]any{unit.ModelGitRevision, unit.ModelEvidence, unit.DevelopmentTodos})
			if err != nil {
				t.Fatal(err)
			}
			diagnostics := []delivery.GateDiagnostic{{Code: "account_format", Severity: "error", Path: "docs/interaction-contract.md", Message: "Define account format", Owner: "interaction", Category: "account"}}
			for _, role := range roles {
				diagnostics = append(diagnostics, delivery.GateDiagnostic{Code: "account_implementation", Severity: "error", Path: role + "/account", Message: "Align account behavior", Owner: role, Category: "account"})
			}
			if err := applyUnitAtRevision(&run, delivery.Actor{ID: "journey", Kind: delivery.ActorSystem}, "delivery_unit.gap.report", "journey_testing", revision, "interaction", diagnostics); err != nil {
				t.Fatal(err)
			}
			advanceUnitAtRevision(t, &run, agent("interaction"), "delivery_unit.interaction.complete", "interaction_modeling", revision)
			for _, role := range roles {
				phase := role + "_implementation"
				if string(run.DeliveryUnits[0].Phase) != phase {
					t.Fatalf("expected affected role %s, got %s", phase, run.DeliveryUnits[0].Phase)
				}
				advanceUnitAtRevision(t, &run, agent(role), "delivery_unit."+role+".complete", phase, revision)
			}
			if run.DeliveryUnits[0].Phase != delivery.DeliveryUnitContractVerification {
				t.Fatalf("interaction repair unnecessarily replayed a phase: %s", run.DeliveryUnits[0].Phase)
			}
			advanceUnitAtRevision(t, &run, delivery.Actor{ID: "contract", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification", revision)
			advanceUnitAtRevision(t, &run, delivery.Actor{ID: "journey", Kind: delivery.ActorSystem}, "delivery_unit.journey.complete", "journey_testing", revision)
			unit = &run.DeliveryUnits[0]
			after, err := json.Marshal([]any{unit.ModelGitRevision, unit.ModelEvidence, unit.DevelopmentTodos})
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("repair rewrote the unchanged model or original development records")
			}
			for _, repair := range unit.DevelopmentRepairItems {
				if repair.Status != delivery.DevelopmentTodoCompleted || len(repair.Evidence) != 1 {
					t.Fatalf("repair did not save its own result: %#v", repair)
				}
			}
			if run.Stage != delivery.StageTesting {
				t.Fatalf("repair did not return to QA: %s", run.Stage)
			}
		})
	}
}

func TestSystemVerifiedModelHashDeterminesImplementationRepairs(t *testing.T) {
	for _, scenario := range []struct {
		name, impact string
		changed      bool
	}{
		{"unchanged", "unchanged", false},
		{"runtime_format_change", "unchanged", true},
		{"permissions_only", "permissions_only", true},
		{"implementation_contract", "implementation_contract", true},
	} {
		impact, changed, name := scenario.impact, scenario.changed, scenario.name
		t.Run(name, func(t *testing.T) {
			run := completedRun(t)
			unit := &run.DeliveryUnits[0]
			run.Stage = delivery.StageDevelopment
			run.ActiveDeliveryUnitID = unit.ID
			unit.Phase = delivery.DeliveryUnitJourneyTesting
			originalRevision := unit.IntegratedGitRevision
			newRevision := strings.Repeat("b", 40)
			reportGap(t, &run, "model", "journey_testing", originalRevision)
			unit = &run.DeliveryUnits[0]
			if unit.FrontendStatus != delivery.DeliveryGatePassed || unit.BackendStatus != delivery.DeliveryGatePassed {
				t.Fatal("failure owner invalidated unrelated implementations before verification")
			}
			advanceUnitAtRevision(t, &run, agent("model"), "delivery_unit.model.complete", "domain_modeling", newRevision)
			guide := *unit.ModelEvidence
			if changed {
				guide.ModelSHA256 = strings.Repeat("e", 64)
			}
			guide.GitRevision = newRevision
			guide.EvidenceRefs = []string{"git:" + newRevision + "#evidence:model.json"}
			mustApply(t, &run, delivery.Actor{ID: "model-runner", Kind: delivery.ActorSystem}, "delivery_unit.model.verify", map[string]any{
				"delivery_unit_id": unit.ID, "phase": "model_verification", "git_revision": newRevision,
				"summary": "Verified actual model contents", "evidence_refs": guide.EvidenceRefs, "backend_guide": guide,
				"model_repair_impact": delivery.ModelRepairImpact{BaselineGitRevision: unit.ModelGitRevision, BaselineModelSHA256: unit.ModelEvidence.ModelSHA256, CurrentGitRevision: newRevision, CurrentModelSHA256: guide.ModelSHA256, SourceImpact: impact},
			})
			receipt := run.DeliveryUnits[0].ModelRepairImpact
			if receipt == nil || receipt.SourceImpact != impact || receipt.CurrentGitRevision != newRevision || receipt.CurrentModelSHA256 != guide.ModelSHA256 {
				t.Fatal("verified impact was not persisted with its version")
			}
			if impact == "implementation_contract" {
				if run.DeliveryUnits[0].Phase != delivery.DeliveryUnitFrontendImplementation || run.DeliveryUnits[0].BackendStatus != delivery.DeliveryGateNeedsChange {
					t.Fatal("changed model skipped implementation alignment")
				}
				advanceUnitAtRevision(t, &run, agent("frontend"), "delivery_unit.frontend.complete", "frontend_implementation", newRevision)
				advanceUnitAtRevision(t, &run, agent("backend"), "delivery_unit.backend.complete", "backend_implementation", newRevision)
			}
			if run.DeliveryUnits[0].Phase != delivery.DeliveryUnitContractVerification {
				t.Fatalf("unchanged inputs replayed implementation: %s", run.DeliveryUnits[0].Phase)
			}
			for _, final := range []struct {
				command, phase string
				evidence       *delivery.BackendGuideEvidence
			}{
				{"delivery_unit.contract.verify", "contract_verification", run.DeliveryUnits[0].ContractEvidence},
				{"delivery_unit.journey.complete", "journey_testing", run.DeliveryUnits[0].JourneyEvidence},
			} {
				evidence := *final.evidence
				evidence.ModelSHA256 = guide.ModelSHA256
				evidence.GitRevision = newRevision
				evidence.EvidenceRefs = []string{"git:" + newRevision + "#evidence:" + final.phase}
				mustApply(t, &run, delivery.Actor{ID: "system-runner", Kind: delivery.ActorSystem}, final.command, map[string]any{
					"delivery_unit_id": unit.ID, "phase": final.phase, "git_revision": newRevision,
					"summary": "Reverified integrated source", "evidence_refs": evidence.EvidenceRefs, "backend_guide": evidence,
				})
			}
			for _, repair := range run.DeliveryUnits[0].DevelopmentRepairItems {
				if repair.Status != delivery.DevelopmentTodoCompleted || len(repair.Evidence) != 1 || repair.Evidence[0].GitRevision != newRevision {
					t.Fatalf("repair lacks its own current result: %#v", repair)
				}
			}
		})
	}
}
