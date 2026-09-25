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
	if len(todos) != 8 || todos[0].Status != delivery.DevelopmentTodoInProgress {
		t.Fatalf("development Todo batch was not initialized: %#v", todos)
	}
	categories := map[string]bool{}
	for index, todo := range todos {
		categories[todo.Category] = true
		if todo.Sequence != index+1 || todo.Category == "" {
			t.Fatalf("development Todo order or dynamic category was lost: %#v", todos)
		}
		if index > 0 && (todo.Status != delivery.DevelopmentTodoNotStarted || todo.Title == "" || todo.SourceID == "") {
			t.Fatalf("new development todo has the wrong scope or status: %#v", todo)
		}
	}
	if len(categories) >= 7 {
		t.Fatalf("Todo categories were incorrectly coupled to the seven technical phases: %#v", categories)
	}
	skipPayload, err := json.Marshal(map[string]any{
		"delivery_unit_id": run.ActiveDeliveryUnitID, "todo_id": todos[1].ID,
		"git_revision": strings.Repeat("a", 40), "summary": "Skipped ahead.",
		"evidence_refs": []string{"git:" + strings.Repeat("a", 40) + "#evidence:skip.json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = delivery.Apply(&run, delivery.Command{Actor: agent("frontend-agent"), Type: "development_todo.complete", Payload: skipPayload}, time.Now().UTC())
	assertCode(t, err, "development_todo_conflict")
	assertPhaseAction(t, run, "interaction_modeling", "frontend", "delivery_unit.interaction.complete")

	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.interaction.complete", "interaction_modeling")
	if todos := run.DeliveryUnits[0].DevelopmentTodos; todos[0].Status != delivery.DevelopmentTodoCompleted || len(todos[0].Evidence) != 1 || todos[1].Status != delivery.DevelopmentTodoInProgress {
		t.Fatalf("completed todo did not retain evidence and activate the next item: %#v", todos)
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
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "domain_modeling")
	advanceUnit(t, &run, delivery.Actor{ID: "runtime-check", Kind: delivery.ActorSystem}, "delivery_unit.model.verify", "model_verification")
	assertPhaseAction(t, run, "backend_implementation", "backend", "delivery_unit.backend.complete")
	if !hasProjectedAction(delivery.ProjectionFor(run), "delivery_unit.gap.report", run.Feature.ID) {
		t.Fatal("Backend implementation cannot route a deterministically verified model gap")
	}
	reportGap(t, &run, "model", "backend_implementation", strings.Repeat("a", 40))
	unit = run.DeliveryUnits[0]
	if unit.Phase != delivery.DeliveryUnitDomainModeling || unit.ModelStatus != delivery.DeliveryGateNeedsChange || unit.ModelGitRevision != "" || unit.ModelEvidence != nil {
		t.Fatalf("Backend implementation model gap retained stale model evidence: %#v", unit)
	}
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.model.complete", "domain_modeling")
	advanceUnit(t, &run, delivery.Actor{ID: "runtime-check", Kind: delivery.ActorSystem}, "delivery_unit.model.verify", "model_verification")
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation")
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_convergence")
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
	advanceUnitAtRevision(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_convergence", newRevision)
	err = applyUnitAtRevision(&run, delivery.Actor{ID: "contract-check", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification", strings.Repeat("a", 40), "", nil)
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

func TestIndependentQualityIsBoundToJourneyRevisionAndCanReopenBackend(t *testing.T) {
	run := completedRun(t)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
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
	if run.Stage != delivery.StageAcceptance || len(projection.Workflow.AvailableActions) != len(run.AcceptanceCases) || !projection.Workflow.ReleaseGates[5].OK || projection.Workflow.ReleaseGates[6].OK {
		t.Fatalf("Independent QA did not hand off to acceptance: stage=%s workflow=%#v", run.Stage, projection.Workflow)
	}
}

func TestBusinessAcceptanceFailureReopensFrontend(t *testing.T) {
	run := completedRun(t)
	passIndependentQuality(t, &run)
	revision := run.DeliveryUnits[0].IntegratedGitRevision
	mustApply(t, &run, human("m-business"), "acceptance.confirm", map[string]any{
		"acceptance_case_id": run.AcceptanceCases[0].ID,
		"git_revision":       revision,
		"result":             "fail",
		"failure_owner":      "frontend",
		"note":               "The approval state is not visible after returning to the list.",
		"evidence_refs":      []string{"git:" + revision + "#evidence:evidence/acceptance/UAT-01.md"},
	})
	unit := run.DeliveryUnits[0]
	if run.Stage != delivery.StageDevelopment || run.ActiveDeliveryUnitID != unit.ID || unit.Phase != delivery.DeliveryUnitFrontendConvergence || unit.IntegratedGitRevision != "" {
		t.Fatalf("Business acceptance failure did not reopen the Frontend gate: run=%s unit=%#v", run.Stage, unit)
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
	revision := run.DeliveryUnits[len(run.DeliveryUnits)-1].IntegratedGitRevision
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
	advanceUnit(t, &run, agent("backend-agent"), "delivery_unit.backend.complete", "backend_implementation")
	advanceUnit(t, &run, agent("frontend-agent"), "delivery_unit.frontend.complete", "frontend_convergence")
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

func advanceUnit(t *testing.T, run *delivery.DeliveryRun, actor delivery.Actor, command, phase string) {
	t.Helper()
	advanceUnitAtRevision(t, run, actor, command, phase, strings.Repeat("a", 40))
}

func advanceUnitAtRevision(t *testing.T, run *delivery.DeliveryRun, actor delivery.Actor, command, phase, revision string) {
	t.Helper()
	if actor.Kind == delivery.ActorAgent {
		for {
			var active *delivery.DevelopmentTodo
			for index := range run.DeliveryUnits[0].DevelopmentTodos {
				todo := &run.DeliveryUnits[0].DevelopmentTodos[index]
				if todo.Phase == delivery.DeliveryUnitPhase(phase) && todo.Status == delivery.DevelopmentTodoInProgress {
					active = todo
					break
				}
			}
			if active == nil {
				break
			}
			mustApply(t, run, actor, "development_todo.complete", map[string]any{
				"delivery_unit_id": run.ActiveDeliveryUnitID,
				"todo_id":          active.ID,
				"git_revision":     revision,
				"summary":          "The business todo was implemented and verified.",
				"evidence_refs":    []string{"git:" + revision + "#evidence:" + active.SourceID + ".json"},
			})
		}
	}
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
	} else {
		todo := unit.DevelopmentTodos[0]
		for _, candidate := range unit.DevelopmentTodos {
			if candidate.Phase == unit.Phase && candidate.Status == delivery.DevelopmentTodoInProgress {
				todo = candidate
				break
			}
		}
		if !hasProjectedAction(projection, "development_todo.complete", todo.ID) {
			t.Fatalf("agent phase %s did not expose its current business todo: %#v", phase, projection.Workflow.AvailableActions)
		}
		if hasProjectedAction(projection, command, unit.ID) {
			t.Fatalf("agent phase %s exposed its gate before every business todo completed: %#v", phase, projection.Workflow.AvailableActions)
		}
	}
	if hasProjectedAction(projection, "work.plan.replace", "") {
		t.Fatal("DeliveryUnit exposed the removed WorkPlan development path")
	}
}
