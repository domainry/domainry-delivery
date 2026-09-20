package delivery_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain/delivery"
)

func TestDeliveryUnitDrivesTheV3DevelopmentLifecycle(t *testing.T) {
	run := application.DemoDeliveryRun(time.Now().UTC())
	if len(run.DeliveryUnits) != 1 || run.ActiveDeliveryUnitID != run.Feature.ID {
		t.Fatalf("new DeliveryRun has no authoritative DeliveryUnit: %#v", run.DeliveryUnits)
	}
	if run.Feature.Discovery.Focus.Topic == "" {
		t.Fatal("DeliveryRun discarded confirmed discovery facts")
	}
	assertPhaseAction(t, run, "interaction_modeling", "frontend", "delivery_unit.interaction.complete")

	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.interaction.complete", "interaction_modeling")
	assertPhaseAction(t, run, "domain_modeling", "backend", "delivery_unit.model.complete")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "domain_modeling")
	assertPhaseAction(t, run, "compiling", "system", "delivery_unit.compile.complete")
	advanceUnit(t, &run, delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.compile.complete", "compiling")
	assertPhaseAction(t, run, "frontend_convergence", "frontend", "delivery_unit.frontend.complete")

	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.contract_gap.report", "frontend_convergence")
	assertPhaseAction(t, run, "contract_gap", "backend", "delivery_unit.model.complete")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "contract_gap")
	advanceUnit(t, &run, delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.compile.complete", "compiling")
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_convergence")
	assertPhaseAction(t, run, "contract_frozen", "system", "delivery_unit.contract.freeze")
	advanceUnit(t, &run, delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.contract.freeze", "contract_frozen")
	assertPhaseAction(t, run, "backend_implementation", "backend", "delivery_unit.backend.complete")
	if !hasProjectedAction(delivery.ProjectionFor(run), "delivery_unit.contract_gap.report", run.Feature.ID) {
		t.Fatal("Backend implementation cannot reopen an invalid frozen contract")
	}
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.contract_gap.report", "backend_implementation")
	unit := run.DeliveryUnits[0]
	if unit.ContractGitRevision != "" || unit.ImplementationGitRevision != "" || unit.Phase != "contract_gap" {
		t.Fatalf("Contract reopen retained stale frozen revisions: %#v", unit)
	}
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "contract_gap")
	advanceUnit(t, &run, delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.compile.complete", "compiling")
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_convergence")
	advanceUnit(t, &run, delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.contract.freeze", "contract_frozen")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation")
	advanceUnit(t, &run, delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.backend.reopen", "journey_testing")
	unit = run.DeliveryUnits[0]
	if unit.Phase != "backend_implementation" || unit.ImplementationGitRevision != "" || unit.BackendStatus != "pending" {
		t.Fatalf("Backend reopen retained stale implementation state: %#v", unit)
	}
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation")
	advanceUnit(t, &run, delivery.Actor{ID: "journey-runner", Kind: delivery.ActorSystem}, "delivery_unit.journey.complete", "journey_testing")

	unit = run.DeliveryUnits[0]
	if run.ActiveDeliveryUnitID != "" || run.Stage != delivery.StageTesting || unit.Phase != "complete" {
		t.Fatalf("DeliveryUnit did not complete authoritatively: run=%#v unit=%#v", run.Stage, unit)
	}
	if unit.ContractGitRevision == "" || unit.ImplementationGitRevision == "" || unit.JourneyStatus != "passed" {
		t.Fatalf("DeliveryUnit lost Git-bound gate results: %#v", unit)
	}
	projection := delivery.ProjectionFor(run)
	if len(projection.Workflow.AvailableActions) != len(run.TestCases) || len(projection.Workflow.ReleaseGates) != 6 || !projection.Workflow.ReleaseGates[2].OK || projection.Workflow.ReleaseGates[3].OK {
		t.Fatalf("completed V3 projection is inconsistent: %#v", projection.Workflow)
	}
}

func TestIndependentQualityIsBoundToJourneyRevisionAndCanReopenBackend(t *testing.T) {
	run := completedV3Run(t)
	revision := run.DeliveryUnits[0].ImplementationGitRevision
	err := apply(&run, agent("qa-agent"), "quality.record", map[string]any{
		"test_case_id": run.TestCases[0].ID, "git_revision": strings.Repeat("b", 40),
		"result": "pass", "note": "Observed the complete business flow", "evidence_refs": []string{"git:bad#evidence/qa.md"},
	})
	assertCode(t, err, "quality_revision_conflict")

	mustApply(t, &run, agent("qa-agent"), "quality.record", map[string]any{
		"test_case_id": run.TestCases[0].ID, "git_revision": revision,
		"result": "fail", "failure_owner": "backend", "note": "Rollback left a partial record", "evidence_refs": []string{"git:" + revision + "#evidence:evidence/qa.md"},
	})
	unit := run.DeliveryUnits[0]
	if run.Stage != delivery.StageDevelopment || run.ActiveDeliveryUnitID != unit.ID || unit.Phase != delivery.DeliveryUnitBackendImplementation || unit.ImplementationGitRevision != "" {
		t.Fatalf("Backend QA failure did not reopen the Backend gate: run=%s unit=%#v", run.Stage, unit)
	}
}

func TestIndependentQualityPassesEveryCaseBeforeAcceptance(t *testing.T) {
	run := completedV3Run(t)
	revision := run.DeliveryUnits[0].ImplementationGitRevision
	for _, testCase := range run.TestCases {
		mustApply(t, &run, agent("qa-agent"), "quality.record", map[string]any{
			"test_case_id": testCase.ID, "git_revision": revision,
			"result": "pass", "note": "Independent observation matched the acceptance scenario", "evidence_refs": []string{"git:" + revision + "#evidence:evidence/" + testCase.ID + ".md"},
		})
	}
	projection := delivery.ProjectionFor(run)
	if run.Stage != delivery.StageAcceptance || len(projection.Workflow.AvailableActions) != len(run.AcceptanceCases) || !projection.Workflow.ReleaseGates[3].OK || projection.Workflow.ReleaseGates[4].OK {
		t.Fatalf("Independent QA did not hand off to acceptance: stage=%s workflow=%#v", run.Stage, projection.Workflow)
	}
}

func TestBusinessAcceptanceFailureReopensFrontend(t *testing.T) {
	run := completedV3Run(t)
	passIndependentQuality(t, &run)
	revision := run.DeliveryUnits[0].ImplementationGitRevision
	mustApply(t, &run, human("m-business"), "acceptance.confirm", map[string]any{
		"acceptance_case_id": run.AcceptanceCases[0].ID,
		"git_revision":       revision,
		"result":             "fail",
		"failure_owner":      "frontend",
		"note":               "The approval state is not visible after returning to the list.",
		"evidence_refs":      []string{"git:" + revision + "#evidence:evidence/acceptance/UAT-01.md"},
	})
	unit := run.DeliveryUnits[0]
	if run.Stage != delivery.StageDevelopment || run.ActiveDeliveryUnitID != unit.ID || unit.Phase != delivery.DeliveryUnitFrontendConvergence || unit.ImplementationGitRevision != "" {
		t.Fatalf("Business acceptance failure did not reopen the Frontend gate: run=%s unit=%#v", run.Stage, unit)
	}
}

func TestV3ReleaseBindsTheAcceptedGitRevisionAndGoesLive(t *testing.T) {
	run := completedV3Run(t)
	revision := run.DeliveryUnits[0].ImplementationGitRevision
	passIndependentQuality(t, &run)
	passBusinessAcceptance(t, &run)
	if run.Stage != delivery.StageRelease || !hasProjectedAction(delivery.ProjectionFor(run), "release_checks.replace", "") {
		t.Fatalf("Accepted revision did not enter release preparation: %#v", delivery.ProjectionFor(run).Workflow)
	}
	mustApply(t, &run, agent("op-agent"), "release_checks.replace", map[string]any{
		"environment_ref": "production",
		"checks":          []map[string]any{{"id": "RC-01", "title": "Production configuration verified", "required": true}},
	})
	mustApply(t, &run, agent("op-agent"), "release_check.record", map[string]any{
		"check_id": "RC-01", "status": "passed", "note": "Configuration and rollback entry point verified", "evidence_refs": []string{"git:" + revision + "#evidence:evidence/release/RC-01.md"},
	})
	if !hasProjectedAction(delivery.ProjectionFor(run), "release.prepare", "") {
		t.Fatal("Passed release checks did not expose human release preparation")
	}
	mustApply(t, &run, human("m-product"), "release.prepare", map[string]any{
		"version": "1.0.0", "environment_ref": "production",
	})
	release := run.Releases[0]
	if release.CodeRevision != revision || release.BuildID != "" || release.ArtifactRef != "" {
		t.Fatalf("V3 Release retained an old Build/artifact binding: %#v", release)
	}
	mustApply(t, &run, human("m-release"), "release.approve", map[string]any{"release_id": release.ID})
	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": release.ID, "outcome": "success", "environment_ref": "production",
		"launch_url": "https://product.example", "receipt_ref": "provider://deployment/42",
	})
	if run.Stage != delivery.StageLive || run.Releases[0].Status != delivery.ReleaseLive {
		t.Fatalf("Trusted deployment receipt did not make the V3 release live: %#v", run.Releases[0])
	}
}

func completedV3Run(t *testing.T) delivery.DeliveryRun {
	t.Helper()
	run := application.DemoDeliveryRun(time.Now().UTC())
	unit := &run.DeliveryUnits[0]
	unit.Phase = delivery.DeliveryUnitComplete
	unit.ActiveRole = ""
	unit.ContractGitRevision = strings.Repeat("a", 40)
	unit.ImplementationGitRevision = strings.Repeat("a", 40)
	unit.InteractionStatus = delivery.DeliveryGatePassed
	unit.CompilerStatus = delivery.DeliveryGatePassed
	unit.FrontendStatus = delivery.DeliveryGatePassed
	unit.BackendStatus = delivery.DeliveryGatePassed
	unit.JourneyStatus = delivery.DeliveryGatePassed
	run.ActiveDeliveryUnitID = ""
	run.Stage = delivery.StageTesting
	return run
}

func passIndependentQuality(t *testing.T, run *delivery.DeliveryRun) {
	t.Helper()
	revision := run.DeliveryUnits[len(run.DeliveryUnits)-1].ImplementationGitRevision
	for _, testCase := range run.TestCases {
		mustApply(t, run, agent("qa-agent"), "quality.record", map[string]any{
			"test_case_id": testCase.ID, "git_revision": revision,
			"result": "pass", "note": "The real end-to-end journey matched the expected outcome.",
			"evidence_refs": []string{"git:" + revision + "#evidence:evidence/quality/" + testCase.ID + ".md"},
		})
	}
}

func passBusinessAcceptance(t *testing.T, run *delivery.DeliveryRun) {
	t.Helper()
	revision := run.DeliveryUnits[len(run.DeliveryUnits)-1].ImplementationGitRevision
	for _, acceptanceCase := range run.AcceptanceCases {
		mustApply(t, run, human("m-business"), "acceptance.confirm", map[string]any{
			"acceptance_case_id": acceptanceCase.ID, "result": "pass",
			"git_revision":  revision,
			"note":          "The delivered workflow satisfies the confirmed business outcome.",
			"evidence_refs": []string{"git:" + revision + "#evidence:evidence/acceptance/" + acceptanceCase.ID + ".md"},
		})
	}
}

func TestJourneyEvidenceMustMatchImplementationRevision(t *testing.T) {
	run := application.DemoDeliveryRun(time.Now().UTC())
	unit := &run.DeliveryUnits[0]
	unit.Phase = delivery.DeliveryUnitJourneyTesting
	unit.ActiveRole = "system"
	unit.ContractGitRevision = strings.Repeat("a", 40)
	unit.ImplementationGitRevision = strings.Repeat("b", 40)
	payload, err := json.Marshal(map[string]any{
		"delivery_unit_id": unit.ID,
		"phase":            "journey_testing",
		"git_revision":     strings.Repeat("c", 40),
		"summary":          "Journey passed against an unrelated revision.",
		"diagnostics":      []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = delivery.Apply(&run, delivery.Command{
		Actor:   delivery.Actor{ID: "runner", Kind: delivery.ActorSystem},
		Type:    "delivery_unit.journey.complete",
		Payload: payload,
	}, time.Now().UTC())
	assertCode(t, err, "delivery_unit_implementation_revision_conflict")
}

func advanceUnit(t *testing.T, run *delivery.DeliveryRun, actor delivery.Actor, command, phase string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"delivery_unit_id": run.ActiveDeliveryUnitID,
		"phase":            phase,
		"git_revision":     strings.Repeat("a", 40),
		"summary":          "The deterministic phase gate passed.",
		"diagnostics":      []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := delivery.Apply(run, delivery.Command{Actor: actor, Type: command, Payload: payload}, time.Now().UTC()); err != nil {
		t.Fatalf("advance %s: %v", phase, err)
	}
}

func assertPhaseAction(t *testing.T, run delivery.DeliveryRun, phase, role, command string) {
	t.Helper()
	unit := run.DeliveryUnits[0]
	if string(unit.Phase) != phase || unit.ActiveRole != role {
		t.Fatalf("unexpected DeliveryUnit assignment: %#v", unit)
	}
	projection := delivery.ProjectionFor(run)
	if len(projection.Workflow.AvailableActions) == 0 || projection.Workflow.AvailableActions[0].Command != command {
		t.Fatalf("phase %s did not expose %s: %#v", phase, command, projection.Workflow.AvailableActions)
	}
	if hasProjectedAction(projection, "work.plan.replace", "") {
		t.Fatal("V3 DeliveryUnit exposed the removed WorkPlan development path")
	}
}
