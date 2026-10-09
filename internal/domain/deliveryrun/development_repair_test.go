package deliveryrun_test

import (
	"encoding/json"
	"strings"
	"testing"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

func TestFinalGatesCannotCompleteOutstandingImplementationRepairs(t *testing.T) {
	for _, gate := range []struct{ command, phase string }{
		{"delivery_unit.contract.verify", "contract_verification"},
		{"delivery_unit.journey.complete", "journey_testing"},
	} {
		t.Run(gate.phase, func(t *testing.T) {
			run := completedRun(t)
			unit := &run.DeliveryUnits[0]
			run.Stage = delivery.StageDevelopment
			run.ActiveDeliveryUnitID = unit.ID
			unit.Phase = delivery.DeliveryUnitPhase(gate.phase)
			unit.DevelopmentRepairItems = []delivery.DevelopmentRepairWorkItem{{
				DevelopmentPhaseWorkItem: delivery.DevelopmentPhaseWorkItem{
					ID: "backend-repair", Phase: delivery.DeliveryUnitBackendImplementation,
					Status: delivery.DevelopmentTodoNotStarted, Evidence: []delivery.DevelopmentTodoEvidence{},
				},
				SourceKind: "verification_gap", SourceID: "gap", Title: "Repair backend", Detail: "Required repair remains open",
			}}
			before, err := json.Marshal(run)
			if err != nil {
				t.Fatal(err)
			}
			err = applyUnitAtRevision(&run, delivery.Actor{ID: "verification-runner", Kind: delivery.ActorSystem}, gate.command, gate.phase, unit.IntegratedGitRevision, "", nil)
			if err == nil || !strings.Contains(err.Error(), "delivery_unit_repairs_incomplete") {
				t.Fatalf("final gate accepted an outstanding implementation repair: %v", err)
			}
			after, err := json.Marshal(run)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("rejected final gate changed completion records")
			}
		})
	}
}

func TestFinalRepairGatesRecordTheirOwnEvidenceBeforeClosing(t *testing.T) {
	run := completedRun(t)
	unit := &run.DeliveryUnits[0]
	run.Stage = delivery.StageDevelopment
	run.ActiveDeliveryUnitID = unit.ID
	unit.Phase = delivery.DeliveryUnitContractVerification
	revision := unit.IntegratedGitRevision
	for _, phase := range []delivery.DeliveryUnitPhase{delivery.DeliveryUnitContractVerification, delivery.DeliveryUnitJourneyTesting} {
		unit.DevelopmentRepairItems = append(unit.DevelopmentRepairItems, delivery.DevelopmentRepairWorkItem{
			DevelopmentPhaseWorkItem: delivery.DevelopmentPhaseWorkItem{
				ID: string(phase), Phase: phase, Status: delivery.DevelopmentTodoNotStarted, Evidence: []delivery.DevelopmentTodoEvidence{},
			},
			SourceKind: "verification_gap", SourceID: "gap", Title: "Reverify repaired source", Detail: "Run final gate on integrated source",
		})
	}
	if err := applyUnitAtRevision(&run, delivery.Actor{ID: "contract-runner", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification", revision, "", nil); err != nil {
		t.Fatal(err)
	}
	unit = &run.DeliveryUnits[0]
	if unit.DevelopmentRepairItems[0].Status != delivery.DevelopmentTodoCompleted || len(unit.DevelopmentRepairItems[0].Evidence) != 1 || unit.DevelopmentRepairItems[1].Status != delivery.DevelopmentTodoInProgress || len(unit.DevelopmentRepairItems[1].Evidence) != 0 {
		t.Fatalf("contract gate completed or lost another repair's evidence: %#v", unit.DevelopmentRepairItems)
	}
	if err := applyUnitAtRevision(&run, delivery.Actor{ID: "journey-runner", Kind: delivery.ActorSystem}, "delivery_unit.journey.complete", "journey_testing", revision, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, repair := range run.DeliveryUnits[0].DevelopmentRepairItems {
		if repair.Status != delivery.DevelopmentTodoCompleted || len(repair.Evidence) != 1 || repair.Evidence[0].GitRevision != revision {
			t.Fatalf("closed repair lacks its own integrated revision evidence: %#v", repair)
		}
	}
}

func TestRepairRoutingRetainsOriginalVersionsAndVerifiedResults(t *testing.T) {
	for _, owner := range []string{"interaction", "model", "frontend", "backend"} {
		t.Run(owner, func(t *testing.T) {
			run := completedRun(t)
			unit := &run.DeliveryUnits[0]
			run.Stage = delivery.StageDevelopment
			run.ActiveDeliveryUnitID = unit.ID
			unit.Phase = delivery.DeliveryUnitJourneyTesting
			originalResults := func() []byte {
				value, err := json.Marshal([]any{unit.ModelGitRevision, unit.BackendGitRevision, unit.IntegratedGitRevision, unit.ModelEvidence, unit.ContractEvidence, unit.JourneyEvidence, unit.DevelopmentTodos})
				if err != nil {
					t.Fatal(err)
				}
				return value
			}
			before := originalResults()
			revision := unit.IntegratedGitRevision
			reportGap(t, &run, owner, "journey_testing", revision)
			unit = &run.DeliveryUnits[0]
			if string(originalResults()) != string(before) {
				t.Fatal("repair routing rewrote original versions, receipts or completed Todos")
			}
			if unit.ContractStatus != delivery.DeliveryGatePending || unit.JourneyStatus != delivery.DeliveryGatePending || unit.InvalidatedIntegratedGitRevision != revision {
				t.Fatalf("retained receipt was incorrectly treated as current: %#v", unit)
			}
		})
	}
}

func TestRetainedVersionsCannotBypassPendingGatePrerequisites(t *testing.T) {
	for _, phase := range []delivery.DeliveryUnitPhase{delivery.DeliveryUnitContractVerification, delivery.DeliveryUnitJourneyTesting} {
		t.Run(string(phase), func(t *testing.T) {
			run := completedRun(t)
			unit := &run.DeliveryUnits[0]
			run.Stage = delivery.StageDevelopment
			run.ActiveDeliveryUnitID = unit.ID
			unit.Phase = phase
			command := "delivery_unit.contract.verify"
			if phase == delivery.DeliveryUnitContractVerification {
				unit.ModelStatus = delivery.DeliveryGatePending
			} else {
				command = "delivery_unit.journey.complete"
				unit.ContractStatus = delivery.DeliveryGatePending
			}
			before, err := json.Marshal(run)
			if err != nil {
				t.Fatal(err)
			}
			err = applyUnitAtRevision(&run, delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, command, string(phase), unit.IntegratedGitRevision, "", nil)
			if err == nil || !strings.Contains(err.Error(), "delivery_unit_prerequisites_incomplete") {
				t.Fatalf("retained version bypassed pending prerequisite: %v", err)
			}
			after, err := json.Marshal(run)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("rejected gate changed original results")
			}
		})
	}
}

func TestSameQualityCaseCanCreateDistinctRoleRepairsWithoutLosingEvidence(t *testing.T) {
	run := completedRun(t)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
	fail := func(owner string) {
		mustApply(t, &run, agent("qa"), "quality.record", map[string]any{
			"test_case_id": run.TestCases[0].ID, "git_revision": revision, "result": "fail",
			"failure_owner": owner, "note": "Observed a reproducible role-specific defect",
			"evidence_refs": []string{"workspace:" + revision + "#evidence:evidence/qa.md"},
		})
	}
	fail("backend")
	advanceUnitAtRevision(t, &run, agent("backend"), "delivery_unit.backend.complete", "backend_implementation", revision)
	advanceUnitAtRevision(t, &run, delivery.Actor{ID: "contract", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification", revision)
	advanceUnitAtRevision(t, &run, delivery.Actor{ID: "journey", Kind: delivery.ActorSystem}, "delivery_unit.journey.complete", "journey_testing", revision)
	original, err := json.Marshal(run.DeliveryUnits[0].DevelopmentRepairItems[0])
	if err != nil {
		t.Fatal(err)
	}
	fail("frontend")
	repairs := run.DeliveryUnits[0].DevelopmentRepairItems
	if len(repairs) != 4 || repairs[0].ID == repairs[3].ID {
		t.Fatalf("role repairs collide: %#v", repairs)
	}
	retained, err := json.Marshal(repairs[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(retained) != string(original) || repairs[3].Status != delivery.DevelopmentTodoInProgress {
		t.Fatal("new frontend repair changed the completed backend repair")
	}
}
