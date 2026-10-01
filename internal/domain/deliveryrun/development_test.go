package deliveryrun_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/testfixture"
)

func TestDeliveryUnitDrivesTheDevelopmentLifecycle(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	if len(run.DeliveryUnits) != 1 || run.ActiveDeliveryUnitID != run.Feature.ID {
		t.Fatalf("new DeliveryRun has no authoritative DeliveryUnit: %#v", run.DeliveryUnits)
	}
	if run.Feature.Discovery.Focus.Topic == "" {
		t.Fatal("DeliveryRun discarded confirmed discovery facts")
	}
	todos := run.DeliveryUnits[0].DevelopmentTodos
	expectedPhases := []delivery.DeliveryUnitPhase{
		delivery.DeliveryUnitInteractionModeling,
		delivery.DeliveryUnitDomainModeling,
		delivery.DeliveryUnitModelVerification,
		delivery.DeliveryUnitFrontendImplementation,
		delivery.DeliveryUnitBackendImplementation,
		delivery.DeliveryUnitContractVerification,
		delivery.DeliveryUnitJourneyTesting,
	}
	if len(todos) != len(expectedPhases) {
		t.Fatalf("development does not expose exactly seven fixed phases: %#v", todos)
	}
	for index, phase := range expectedPhases {
		todo := todos[index]
		if todo.Sequence != index+1 || todo.Category != "development_phase" || todo.SourceID != string(phase) || len(todo.PhaseWorkItems) != 1 || todo.PhaseWorkItems[0].Phase != phase {
			t.Fatalf("fixed development phase %d is inconsistent: %#v", index, todo)
		}
		expectedStatus := delivery.DevelopmentTodoNotStarted
		if index == 0 {
			expectedStatus = delivery.DevelopmentTodoInProgress
		}
		if todo.Status != expectedStatus {
			t.Fatalf("fixed development phase %s has status %s", phase, todo.Status)
		}
	}
	assertPhaseAction(t, run, "interaction_modeling", "frontend", "delivery_unit.interaction.complete")

	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.interaction.complete", "interaction_modeling")
	if todos := run.DeliveryUnits[0].DevelopmentTodos; todos[0].Status != delivery.DevelopmentTodoCompleted || len(todos[0].PhaseWorkItems[0].Evidence) != 1 || todos[1].Status != delivery.DevelopmentTodoInProgress {
		t.Fatalf("completed phase did not retain evidence and activate the next fixed phase: %#v", todos)
	}
	assertPhaseAction(t, run, "domain_modeling", "backend", "delivery_unit.model.complete")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "domain_modeling")
	assertPhaseAction(t, run, "model_verification", "system", "delivery_unit.model.verify")
	if !hasProjectedAction(delivery.ProjectionFor(run), "delivery_unit.gap.report", run.Feature.ID) {
		t.Fatal("Model verification cannot route a structured failure")
	}
	reportGap(t, &run, "model", "model_verification", strings.Repeat("a", 40))
	unit := run.DeliveryUnits[0]
	if unit.Phase != delivery.DeliveryUnitDomainModeling || unit.ModelStatus != delivery.DeliveryGateNeedsChange {
		t.Fatalf("Model verification failure did not return to Domain Modeling: %#v", unit)
	}
	if len(unit.DevelopmentTodos) != 7 || len(unit.DevelopmentRepairItems) != 1 {
		t.Fatalf("Model verification gap changed the fixed phase plan: todos=%#v repairs=%#v", unit.DevelopmentTodos, unit.DevelopmentRepairItems)
	}
	repairTodo := unit.DevelopmentRepairItems[0]
	if repairTodo.Phase != delivery.DeliveryUnitDomainModeling || repairTodo.Status != delivery.DevelopmentTodoInProgress || repairTodo.SourceKind != "verification_gap" {
		t.Fatalf("Model verification gap did not activate the targeted repair Todo: %#v", repairTodo)
	}
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "domain_modeling")
	advanceUnit(t, &run, delivery.Actor{ID: "runtime-check", Kind: delivery.ActorSystem}, "delivery_unit.model.verify", "model_verification")
	assertPhaseAction(t, run, "frontend_implementation", "frontend", "delivery_unit.frontend.complete")
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_implementation")
	assertPhaseAction(t, run, "backend_implementation", "backend", "delivery_unit.backend.complete")
	if !hasProjectedAction(delivery.ProjectionFor(run), "delivery_unit.gap.report", run.Feature.ID) {
		t.Fatal("Backend implementation cannot route a deterministically verified model gap")
	}
	repairTodoCount := len(run.DeliveryUnits[0].DevelopmentRepairItems)
	reportGap(t, &run, "model", "backend_implementation", strings.Repeat("a", 40))
	unit = run.DeliveryUnits[0]
	if unit.Phase != delivery.DeliveryUnitDomainModeling || unit.ModelStatus != delivery.DeliveryGateNeedsChange || unit.ModelGitRevision != "" || unit.ModelEvidence != nil {
		t.Fatalf("Backend implementation model gap retained stale model evidence: %#v", unit)
	}
	if len(unit.DevelopmentRepairItems) != repairTodoCount {
		t.Fatalf("Repeated verification gap duplicated its repair Todo: %#v", unit.DevelopmentRepairItems)
	}
	reopenedRepair := unit.DevelopmentRepairItems[0]
	if reopenedRepair.Status != delivery.DevelopmentTodoInProgress || len(reopenedRepair.Evidence) != 1 {
		t.Fatalf("Repeated verification gap did not reopen the same repair Todo with its evidence preserved: %#v", reopenedRepair)
	}
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "domain_modeling")
	advanceUnit(t, &run, delivery.Actor{ID: "runtime-check", Kind: delivery.ActorSystem}, "delivery_unit.model.verify", "model_verification")
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_implementation")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation")
	assertPhaseAction(t, run, "contract_verification", "system", "delivery_unit.contract.verify")
	advanceUnit(t, &run, delivery.Actor{ID: "contract-check", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification")
	assertPhaseAction(t, run, "journey_testing", "system", "delivery_unit.journey.complete")

	reportGap(t, &run, "backend", "journey_testing", strings.Repeat("a", 40))
	unit = run.DeliveryUnits[0]
	if unit.Phase != delivery.DeliveryUnitBackendImplementation || unit.BackendGitRevision != "" || unit.IntegratedGitRevision != "" || unit.InvalidatedIntegratedGitRevision != strings.Repeat("a", 40) {
		t.Fatalf("Journey Backend failure retained stale integrated evidence: %#v", unit)
	}
	newRevision := strings.Repeat("b", 40)
	advanceUnitAtRevision(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation", newRevision)
	if run.DeliveryUnits[0].Phase != delivery.DeliveryUnitContractVerification {
		t.Fatalf("targeted Backend repair reran the already completed Frontend phase: %#v", run.DeliveryUnits[0])
	}
	err := applyUnitAtRevision(&run, delivery.Actor{ID: "contract-check", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification", strings.Repeat("a", 40), "", nil)
	assertCode(t, err, "delivery_unit_revision_not_advanced")
	advanceUnitAtRevision(t, &run, delivery.Actor{ID: "contract-check", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification", newRevision)
	advanceUnitAtRevision(t, &run, delivery.Actor{ID: "journey-runner", Kind: delivery.ActorSystem}, "delivery_unit.journey.complete", "journey_testing", newRevision)

	unit = run.DeliveryUnits[0]
	if run.ActiveDeliveryUnitID != "" || run.Stage != delivery.StageTesting || unit.Phase != "complete" {
		t.Fatalf("DeliveryUnit did not complete authoritatively: run=%#v unit=%#v", run.Stage, unit)
	}
	if unit.ModelGitRevision == "" || unit.BackendGitRevision == "" || unit.IntegratedGitRevision != newRevision || unit.JourneyStatus != "passed" {
		t.Fatalf("DeliveryUnit lost Git-bound gate results: %#v", unit)
	}
	projection := delivery.ProjectionFor(run)
	if len(projection.Workflow.AvailableActions) != 1 || projection.Workflow.AvailableActions[0].Command != "product_revision.record" || len(projection.Workflow.ReleaseGates) != 8 || !projection.Workflow.ReleaseGates[2].OK || !projection.Workflow.ReleaseGates[3].OK || projection.Workflow.ReleaseGates[4].OK {
		t.Fatalf("completed lifecycle projection is inconsistent: %#v", projection.Workflow)
	}
}

func TestEnvironmentRepairReverifiesTheSameSourceWithoutReplayingDevelopment(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	system := delivery.Actor{ID: "runtime-check", Kind: delivery.ActorSystem}
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.interaction.complete", "interaction_modeling")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "domain_modeling")
	advanceUnit(t, &run, system, "delivery_unit.model.verify", "model_verification")
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_implementation")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation")
	advanceUnit(t, &run, system, "delivery_unit.contract.verify", "contract_verification")
	revision := run.DeliveryUnits[0].IntegratedGitRevision
	reportGap(t, &run, "backend", "journey_testing", revision)
	advanceUnitAtRevision(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation", revision)
	advanceUnitAtRevision(t, &run, system, "delivery_unit.contract.verify", "contract_verification", revision)
	unit := run.DeliveryUnits[0]
	if unit.Phase != delivery.DeliveryUnitJourneyTesting || unit.IntegratedGitRevision != revision || unit.InvalidatedIntegratedGitRevision != "" || unit.JourneyStatus != delivery.DeliveryGatePending || unit.JourneyEvidence != nil || run.Stage != delivery.StageDevelopment {
		t.Fatalf("same-source revalidation bypassed Journey or replayed development: %#v", unit)
	}
	advanceUnitAtRevision(t, &run, system, "delivery_unit.journey.complete", "journey_testing", revision)
	if run.Stage != delivery.StageTesting || run.DeliveryUnits[0].Phase != delivery.DeliveryUnitComplete {
		t.Fatalf("reverified source did not complete after independent Journey: %#v", run.DeliveryUnits[0])
	}
}

func TestIndependentQualityIsBoundToJourneyRevisionAndCanReopenBackend(t *testing.T) {
	run := completedRun(t)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
	originalTodoCount := len(run.DeliveryUnits[0].DevelopmentTodos)
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
	if run.Stage != delivery.StageDevelopment || run.ActiveDeliveryUnitID != unit.ID || unit.Phase != delivery.DeliveryUnitBackendImplementation || unit.IntegratedGitRevision != "" {
		t.Fatalf("Backend QA failure did not reopen the Backend gate: run=%s unit=%#v", run.Stage, unit)
	}
	if len(unit.DevelopmentTodos) != originalTodoCount || len(unit.DevelopmentRepairItems) != 1 {
		t.Fatalf("Backend QA failure changed business Todos instead of creating one targeted repair: todos=%#v repairs=%#v", unit.DevelopmentTodos, unit.DevelopmentRepairItems)
	}
	if unit.DevelopmentTodos[3].Status != delivery.DevelopmentTodoCompleted || unit.DevelopmentTodos[4].Status != delivery.DevelopmentTodoInProgress {
		t.Fatalf("Backend QA failure did not preserve Frontend or target only Backend: %#v", unit.DevelopmentTodos)
	}
	repairTodo := unit.DevelopmentRepairItems[0]
	if repairTodo.SourceKind != "quality_case" || repairTodo.SourceID != run.TestCases[0].ID || repairTodo.Status != delivery.DevelopmentTodoInProgress {
		t.Fatalf("Backend QA failure did not activate its exact quality repair: %#v", repairTodo)
	}
}

func TestIndependentQualityPassesEveryCaseBeforeAcceptance(t *testing.T) {
	run := completedRun(t)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
	for _, testCase := range run.TestCases {
		mustApply(t, &run, agent("qa-agent"), "quality.record", map[string]any{
			"test_case_id": testCase.ID, "git_revision": revision,
			"result": "pass", "note": "Independent observation matched the acceptance scenario", "evidence_refs": []string{"git:" + revision + "#evidence:evidence/" + testCase.ID + ".md"},
		})
	}
	projection := delivery.ProjectionFor(run)
	if run.Stage != delivery.StageAcceptance || run.AcceptanceReview == nil || !hasProjectedAction(projection, "acceptance.environment.ready", run.AcceptanceReview.ID) || !projection.Workflow.ReleaseGates[5].OK || projection.Workflow.ReleaseGates[6].OK {
		t.Fatalf("Independent QA did not hand off to acceptance: stage=%s workflow=%#v", run.Stage, projection.Workflow)
	}
}

func TestAcceptanceBugTriageReopensFrontendWithoutLeavingAcceptance(t *testing.T) {
	run := completedRun(t)
	passIndependentQuality(t, &run)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
	readyAcceptanceEnvironment(t, &run)
	mustApply(t, &run, human("m-business"), "acceptance.bug.report", map[string]any{
		"git_revision": revision, "title": "Approval state disappears",
		"description": "Returning to the list loses the approved state.", "expected": "The approved state remains visible.",
		"actual": "The row returns to pending.", "severity": "major", "evidence_refs": []string{"attachment://attachment-visual-1"},
	})
	bug := run.AcceptanceReview.Bugs[0]
	originalTodoCount := len(run.DeliveryUnits[0].DevelopmentTodos)
	mustApply(t, &run, agent("qa-agent"), "acceptance.bug.triage", map[string]any{
		"bug_id": bug.ID, "outcome": "reproduced", "owner": "frontend", "delivery_unit_id": run.Feature.ID,
		"note": "Reproduced after returning from the detail page.", "evidence_refs": []string{"git:" + revision + "#evidence:evidence/acceptance/triage.md"},
	})
	unit := run.DeliveryUnits[0]
	if run.Stage != delivery.StageAcceptance || run.ActiveDeliveryUnitID != unit.ID || unit.Phase != delivery.DeliveryUnitFrontendImplementation || unit.IntegratedGitRevision != "" || run.AcceptanceReview.Bugs[0].Status != delivery.AcceptanceBugFixing {
		t.Fatalf("Acceptance bug did not enter the repair loop: run=%s unit=%#v review=%#v", run.Stage, unit, run.AcceptanceReview)
	}
	if len(unit.DevelopmentTodos) != originalTodoCount || len(unit.DevelopmentRepairItems) != 1 {
		t.Fatalf("Acceptance bug changed business Todos instead of creating one targeted repair: todos=%#v repairs=%#v", unit.DevelopmentTodos, unit.DevelopmentRepairItems)
	}
	if unit.DevelopmentTodos[3].Status != delivery.DevelopmentTodoInProgress || unit.DevelopmentTodos[4].Status != delivery.DevelopmentTodoCompleted {
		t.Fatalf("Acceptance bug did not reopen only Frontend while preserving Backend: %#v", unit.DevelopmentTodos)
	}
	repairTodo := unit.DevelopmentRepairItems[0]
	if repairTodo.SourceKind != "acceptance_bug" || repairTodo.SourceID != bug.ID || repairTodo.Status != delivery.DevelopmentTodoInProgress {
		t.Fatalf("Acceptance bug did not activate its exact repair: %#v", repairTodo)
	}
}

func TestAcceptanceBugRepairRetestAndFinalConfirmationStayInOneReview(t *testing.T) {
	run := completedRun(t)
	passIndependentQuality(t, &run)
	readyAcceptanceEnvironment(t, &run)
	originalRevision := run.AcceptanceReview.CandidateGitRevision
	mustApply(t, &run, human("m-business"), "acceptance.bug.report", map[string]any{
		"git_revision": originalRevision, "title": "Approval state disappears",
		"description": "Returning to the list loses the approved state.", "expected": "The approved state remains visible.",
		"actual": "The row returns to pending.", "severity": "major", "evidence_refs": []string{"attachment://attachment-acceptance-1"},
	})
	bugID := run.AcceptanceReview.Bugs[0].ID
	mustApply(t, &run, agent("qa-agent"), "acceptance.bug.triage", map[string]any{
		"bug_id": bugID, "outcome": "reproduced", "owner": "frontend", "delivery_unit_id": run.Feature.ID,
		"note": "Reproduced against the acceptance Runtime.", "evidence_refs": []string{"git:" + originalRevision + "#evidence:evidence/acceptance/triage.md"},
	})
	if run.Stage != delivery.StageAcceptance || run.AcceptanceReview.Bugs[0].Status != delivery.AcceptanceBugFixing {
		t.Fatalf("triage left the acceptance repair loop: stage=%s review=%#v", run.Stage, run.AcceptanceReview)
	}

	repairedRevision := strings.Repeat("b", 40)
	advanceUnitAtRevision(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_implementation", repairedRevision)
	advanceUnitAtRevision(t, &run, delivery.Actor{ID: "contract-check", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification", repairedRevision)
	advanceUnitAtRevision(t, &run, delivery.Actor{ID: "journey-runner", Kind: delivery.ActorSystem}, "delivery_unit.journey.complete", "journey_testing", repairedRevision)
	product := testfixture.DemoProduct(time.Now().UTC())
	revision := product.Revisions[0]
	mustApply(t, &run, agent("rd-agent"), "product_revision.record", map[string]any{
		"content":       map[string]any{"story": revision.Story, "definition": revision.Definition, "decisions": revision.Decisions},
		"evidence_ref":  "git:" + repairedRevision + "#evidence:evidence/product-revision.json",
		"code_revision": repairedRevision, "model_sha256": strings.Repeat("d", 64),
	})
	passIndependentQuality(t, &run)
	if !hasProjectedAction(delivery.ProjectionFor(run), "acceptance.bug.fix.ready", bugID) {
		t.Fatal("verified repair did not expose the trusted retest handoff")
	}
	mustApply(t, &run, delivery.Actor{ID: "acceptance-runner", Kind: delivery.ActorSystem}, "acceptance.bug.fix.ready", map[string]any{
		"bug_id": bugID, "git_revision": repairedRevision,
		"evidence_refs": []string{"git:" + repairedRevision + "#evidence:evidence/acceptance/fix-ready.md"},
	})
	readyAcceptanceEnvironment(t, &run)
	if run.AcceptanceReview.CandidateGitRevision != repairedRevision || run.AcceptanceReview.Bugs[0].Status != delivery.AcceptanceBugReadyForRetest {
		t.Fatalf("repair was not published for user retest: %#v", run.AcceptanceReview)
	}
	if hasProjectedAction(delivery.ProjectionFor(run), "acceptance.confirm", run.AcceptanceReview.ID) {
		t.Fatal("final acceptance was exposed before the user resolved the bug")
	}
	mustApply(t, &run, human("m-business"), "acceptance.bug.resolve", map[string]any{
		"bug_id": bugID, "note": "Retested in the refreshed Runtime; the approved state remains visible.", "evidence_refs": []string{},
	})
	if !hasProjectedAction(delivery.ProjectionFor(run), "acceptance.confirm", run.AcceptanceReview.ID) {
		t.Fatal("resolved bugs did not expose final user acceptance")
	}
	mustApply(t, &run, human("m-business"), "acceptance.confirm", map[string]any{
		"git_revision": repairedRevision, "note": "The repaired candidate satisfies the confirmed business outcome.",
	})
	if run.Stage != delivery.StageRelease || run.AcceptanceReview.Status != delivery.AcceptanceReviewAccepted {
		t.Fatalf("final user acceptance did not close the review: stage=%s review=%#v", run.Stage, run.AcceptanceReview)
	}
}

func TestReleaseBindsTheAcceptedGitRevisionAndGoesLive(t *testing.T) {
	run := completedRun(t)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
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
	if release.CodeRevision != revision || release.ProductRevision != run.ExecutableRevision.TargetRevision || release.ModelSHA256 != run.ExecutableRevision.ModelSHA256 {
		t.Fatalf("Release retained a removed build/artifact binding: %#v", release)
	}
	mustApply(t, &run, human("m-release"), "release.approve", map[string]any{"release_id": release.ID})
	mustApply(t, &run, delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, "release.deploy_result", map[string]any{
		"release_id": release.ID, "outcome": "success", "environment_ref": "production",
		"launch_url": "https://product.example", "receipt_ref": "provider://deployment/42",
	})
	if run.Stage != delivery.StageLive || run.Releases[0].Status != delivery.ReleaseLive {
		t.Fatalf("Trusted deployment receipt did not make the release live: %#v", run.Releases[0])
	}
}

func completedRun(t *testing.T) delivery.DeliveryRun {
	t.Helper()
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	unit := &run.DeliveryUnits[0]
	unit.Phase = delivery.DeliveryUnitComplete
	unit.ActiveRole = ""
	unit.ModelGitRevision = strings.Repeat("a", 40)
	unit.BackendGitRevision = strings.Repeat("a", 40)
	unit.IntegratedGitRevision = strings.Repeat("a", 40)
	unit.InteractionStatus = delivery.DeliveryGatePassed
	unit.ModelStatus = delivery.DeliveryGatePassed
	unit.FrontendStatus = delivery.DeliveryGatePassed
	unit.BackendStatus = delivery.DeliveryGatePassed
	unit.ContractStatus = delivery.DeliveryGatePassed
	unit.JourneyStatus = delivery.DeliveryGatePassed
	unit.ModelEvidence = backendGuideEvidence("delivery_unit.model.verify")
	unit.ContractEvidence = backendGuideEvidence("delivery_unit.contract.verify")
	unit.JourneyEvidence = backendGuideEvidence("delivery_unit.journey.complete")
	for index := range unit.DevelopmentTodos {
		unit.DevelopmentTodos[index].Status = delivery.DevelopmentTodoCompleted
		for workItemIndex := range unit.DevelopmentTodos[index].PhaseWorkItems {
			unit.DevelopmentTodos[index].PhaseWorkItems[workItemIndex].Status = delivery.DevelopmentTodoCompleted
		}
	}
	for _, evidence := range []*delivery.BackendGuideEvidence{unit.ModelEvidence, unit.ContractEvidence, unit.JourneyEvidence} {
		evidence.WorkspaceID = run.WorkspaceID
		evidence.ProductID = run.Product.ID
		evidence.FeatureRevision = run.Feature.Source.FeatureRevision
		evidence.RepositoryIdentity = "github.com/domainry/product-fixture"
		evidence.GitRevision = strings.Repeat("a", 40)
		evidence.GitStatus = "clean"
		evidence.CheckSuite = "plane-backend-guide"
		evidence.CheckVersion = "1"
		evidence.ExecutedBy = "verification-runner"
		evidence.OccurredAt = time.Now().UTC()
		evidence.EvidenceRefs = []string{"git:" + strings.Repeat("a", 40) + "#evidence:backend-guide.json"}
	}
	run.ActiveDeliveryUnitID = ""
	run.Stage = delivery.StageTesting
	product := testfixture.DemoProduct(time.Now().UTC())
	revision := product.Revisions[0]
	mustApply(t, &run, agent("rd-agent"), "product_revision.record", map[string]any{
		"content":       map[string]any{"story": revision.Story, "definition": revision.Definition, "decisions": revision.Decisions},
		"evidence_ref":  "git:" + unit.IntegratedGitRevision + "#evidence:evidence/product-revision.json",
		"code_revision": unit.IntegratedGitRevision, "model_sha256": strings.Repeat("d", 64),
	})
	return run
}

func passIndependentQuality(t *testing.T, run *delivery.DeliveryRun) {
	t.Helper()
	revision := run.DeliveryUnits[len(run.DeliveryUnits)-1].IntegratedGitRevision
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
	readyAcceptanceEnvironment(t, run)
	revision := run.DeliveryUnits[len(run.DeliveryUnits)-1].IntegratedGitRevision
	mustApply(t, run, human("m-business"), "acceptance.confirm", map[string]any{
		"git_revision": revision, "note": "The delivered workflow satisfies the confirmed business outcome.",
	})
}

func readyAcceptanceEnvironment(t *testing.T, run *delivery.DeliveryRun) {
	t.Helper()
	revision := run.DeliveryUnits[len(run.DeliveryUnits)-1].IntegratedGitRevision
	mustApply(t, run, delivery.Actor{ID: "acceptance-runtime", Kind: delivery.ActorSystem}, "acceptance.environment.ready", map[string]any{
		"git_revision": revision, "environment_ref": "acceptance://" + run.ID + "/" + revision,
		"runtime_url": "http://127.0.0.1:4173",
	})
}

func TestJourneyEvidenceMustMatchImplementationRevision(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	unit := &run.DeliveryUnits[0]
	unit.Phase = delivery.DeliveryUnitJourneyTesting
	unit.ActiveRole = "system"
	unit.ModelGitRevision = strings.Repeat("a", 40)
	unit.BackendGitRevision = strings.Repeat("b", 40)
	unit.IntegratedGitRevision = strings.Repeat("b", 40)
	unit.ModelEvidence = backendGuideEvidence("delivery_unit.model.verify")
	actor := delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}
	now := time.Now().UTC()
	evidence := backendGuideEvidence("delivery_unit.journey.complete")
	evidence.WorkspaceID = run.WorkspaceID
	evidence.ProductID = run.Product.ID
	evidence.FeatureRevision = run.Feature.Source.FeatureRevision
	evidence.RepositoryIdentity = "github.com/domainry/product-fixture"
	evidence.GitRevision = strings.Repeat("c", 40)
	evidence.GitStatus = "clean"
	evidence.CheckSuite = "plane-backend-guide"
	evidence.CheckVersion = "1"
	evidence.ExecutedBy = actor.ID
	evidence.OccurredAt = now
	evidence.EvidenceRefs = []string{"git:" + strings.Repeat("c", 40) + "#evidence:backend-guide.json"}
	payload, err := json.Marshal(map[string]any{
		"delivery_unit_id": unit.ID,
		"phase":            "journey_testing",
		"git_revision":     strings.Repeat("c", 40),
		"summary":          "Journey passed against an unrelated revision.",
		"evidence_refs":    evidence.EvidenceRefs,
		"backend_guide":    evidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = delivery.Apply(&run, delivery.Command{
		Actor:   actor,
		Type:    "delivery_unit.journey.complete",
		Payload: payload,
	}, now)
	assertCode(t, err, "delivery_unit_integrated_revision_conflict")
}

func TestContractEvidenceMustMatchTheVerifiedModelHash(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.interaction.complete", "interaction_modeling")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "domain_modeling")
	advanceUnit(t, &run, delivery.Actor{ID: "model-verifier", Kind: delivery.ActorSystem}, "delivery_unit.model.verify", "model_verification")
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_implementation")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation")
	evidence := backendGuideEvidence("delivery_unit.contract.verify")
	evidence.ModelSHA256 = strings.Repeat("e", 64)
	evidence.WorkspaceID = run.WorkspaceID
	evidence.ProductID = run.Product.ID
	evidence.FeatureRevision = run.Feature.Source.FeatureRevision
	evidence.RepositoryIdentity = "github.com/domainry/product-fixture"
	evidence.GitRevision = strings.Repeat("a", 40)
	evidence.GitStatus = "clean"
	evidence.CheckSuite = "plane-backend-guide"
	evidence.CheckVersion = "1"
	evidence.EvidenceRefs = []string{"git:" + strings.Repeat("a", 40) + "#evidence:contract.json"}
	payload, err := json.Marshal(map[string]any{
		"delivery_unit_id": run.ActiveDeliveryUnitID, "phase": "contract_verification",
		"git_revision": strings.Repeat("a", 40), "summary": "Contract passed against another model.",
		"evidence_refs": evidence.EvidenceRefs,
		"backend_guide": evidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = delivery.Apply(&run, delivery.Command{Actor: delivery.Actor{ID: "contract-verifier", Kind: delivery.ActorSystem}, Type: "delivery_unit.contract.verify", Payload: payload}, time.Now().UTC())
	assertCode(t, err, "backend_evidence_model_mismatch")
}

func TestDevelopmentPlanIsAlwaysTheSevenFixedPhases(t *testing.T) {
	now := time.Now().UTC()
	run, err := delivery.NewDeliveryRun(testfixture.DemoProduct(now), "F-001", 1, delivery.DeliveryRunSpec{
		ID: "three-capability-run", Name: "Three capability delivery", Code: "RUN-3", Goal: "Deliver three business capabilities.",
		Members: []delivery.Member{
			{ID: "product", Name: "Product", Roles: []string{delivery.RoleProductOwner}},
			{ID: "development", Name: "Development", Roles: []string{delivery.RoleDevelopmentLead}},
			{ID: "quality", Name: "Quality", Roles: []string{delivery.RoleQualityLead}},
			{ID: "acceptance", Name: "Acceptance", Roles: []string{delivery.RoleBusinessAcceptor}},
			{ID: "release", Name: "Release", Roles: []string{delivery.RoleReleaseApprover}},
		},
	}, agent("planning-agent"), now)
	if err != nil {
		t.Fatal(err)
	}
	unit := run.DeliveryUnits[0]
	if len(unit.DevelopmentTodos) != 7 {
		t.Fatalf("Delivery did not create exactly seven fixed development phases: %#v", unit.DevelopmentTodos)
	}
	if unit.DevelopmentTodos[3].SourceID != "frontend_implementation" || unit.DevelopmentTodos[4].SourceID != "backend_implementation" || len(unit.DevelopmentRepairItems) != 0 {
		t.Fatalf("Frontend is not implemented before Backend in the fixed phase plan: %#v", unit.DevelopmentTodos)
	}
}

func advanceUnit(t *testing.T, run *delivery.DeliveryRun, actor delivery.Actor, command, phase string) {
	t.Helper()
	advanceUnitAtRevision(t, run, actor, command, phase, strings.Repeat("a", 40))
}

func advanceUnitAtRevision(t *testing.T, run *delivery.DeliveryRun, actor delivery.Actor, command, phase, revision string) {
	t.Helper()
	if err := applyUnitAtRevision(run, actor, command, phase, revision, "", nil); err != nil {
		t.Fatalf("advance %s: %v", phase, err)
	}
}

func reportGap(t *testing.T, run *delivery.DeliveryRun, owner, phase, revision string) {
	t.Helper()
	diagnostics := []map[string]any{{
		"code": "verification_failed", "severity": "error", "path": "/",
		"message": "The verified source does not satisfy the current contract.",
		"owner":   owner, "category": "contract",
	}}
	if err := applyUnitAtRevision(run, delivery.Actor{ID: "verification-runner", Kind: delivery.ActorSystem}, "delivery_unit.gap.report", phase, revision, owner, diagnostics); err != nil {
		t.Fatalf("report %s gap: %v", owner, err)
	}
}

func applyUnitAtRevision(run *delivery.DeliveryRun, actor delivery.Actor, command, phase, revision, failureOwner string, diagnostics any) error {
	if diagnostics == nil {
		diagnostics = []any{}
	}
	now := time.Now().UTC()
	evidence := backendGuideEvidence(command)
	if evidence != nil {
		evidence.WorkspaceID = run.WorkspaceID
		evidence.ProductID = run.Product.ID
		evidence.FeatureRevision = run.Feature.Source.FeatureRevision
		evidence.RepositoryIdentity = "github.com/domainry/product-fixture"
		evidence.GitRevision = revision
		evidence.GitStatus = "clean"
		evidence.CheckSuite = "plane-backend-guide"
		evidence.CheckVersion = "1"
		evidence.ExecutedBy = actor.ID
		evidence.OccurredAt = now
		evidence.EvidenceRefs = []string{"git:" + revision + "#evidence:backend-guide.json"}
	}
	payloadValue := map[string]any{
		"delivery_unit_id": run.ActiveDeliveryUnitID,
		"phase":            phase,
		"git_revision":     revision,
		"summary":          "The deterministic phase gate passed.",
		"evidence_refs":    []string{"git:" + revision + "#evidence:" + phase + ".json"},
	}
	if command == "delivery_unit.gap.report" {
		payloadValue["diagnostics"] = diagnostics
		payloadValue["failure_owner"] = failureOwner
	} else if evidence != nil {
		payloadValue["backend_guide"] = evidence
	}
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		return err
	}
	return delivery.Apply(run, delivery.Command{Actor: actor, Type: command, Payload: payload}, now)
}

func backendGuideEvidence(command string) *delivery.BackendGuideEvidence {
	evidence := &delivery.BackendGuideEvidence{ModelPath: "backend/model.json", ModelSHA256: strings.Repeat("d", 64)}
	switch command {
	case "delivery_unit.model.verify":
		evidence.SingleBackendModel = true
		evidence.StrictModelValidation = true
		evidence.CrossReferencesResolved = true
		evidence.GoBehaviorRegistry = true
		evidence.NoExecutableBehaviorJSON = true
	case "delivery_unit.contract.verify":
		evidence.ProjectHTTP = true
		evidence.HandlerUnitOfWork = true
		evidence.HandlerIdempotency = true
		evidence.DefinitionsRegistered = true
		evidence.RuntimeBootstrap = true
		evidence.NoProjectSchemaSQL = true
		evidence.BackendGoModOnly = true
		evidence.NoRootGoMod = true
		evidence.NoGoWork = true
		evidence.NoCompilerBuilder = true
		evidence.NoGeneratedRuntimeContract = true
		evidence.AuthWorkspacePermission = true
		evidence.DataAuditPersistence = true
		evidence.EmptyDatabaseInitPassed = true
		evidence.SameModelRestartPassed = true
		evidence.ChangedModelRejected = true
	case "delivery_unit.journey.complete":
		evidence.MockJourneyPassed = true
		evidence.RuntimeJourneyPassed = true
	default:
		return nil
	}
	return evidence
}

func assertPhaseAction(t *testing.T, run delivery.DeliveryRun, phase, role, command string) {
	t.Helper()
	unit := run.DeliveryUnits[0]
	if string(unit.Phase) != phase || unit.ActiveRole != role {
		t.Fatalf("unexpected DeliveryUnit assignment: %#v", unit)
	}
	projection := delivery.ProjectionFor(run)
	if role == "system" {
		if !hasProjectedAction(projection, command, unit.ID) {
			t.Fatalf("system phase %s did not expose %s: %#v", phase, command, projection.Workflow.AvailableActions)
		}
	} else if !hasProjectedAction(projection, command, unit.ID) {
		t.Fatalf("agent phase %s did not expose its phase completion action: %#v", phase, projection.Workflow.AvailableActions)
	}
	if hasProjectedAction(projection, "work.plan.replace", "") {
		t.Fatal("DeliveryUnit exposed the removed WorkPlan development path")
	}
}
