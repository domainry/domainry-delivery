package deliveryrun_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	productdomain "github.com/domainry/domainry-delivery/internal/domain/product"
	"github.com/domainry/domainry-delivery/internal/testfixture"
)

func TestDeliveryUnitUsesOneAdaptiveEngineerBoundary(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	unit := run.DeliveryUnits[0]
	if unit.Phase != delivery.DeliveryUnitImplementation || unit.ActiveRole != "engineer" || len(unit.DevelopmentTodos) != 2 {
		t.Fatalf("new DeliveryUnit did not enter the single Engineer loop: %#v", unit)
	}
	assertPhaseAction(t, run, "implementation", "engineer", "delivery_unit.implementation.complete")

	revision := strings.Repeat("a", 40)
	advanceUnitAtRevision(t, &run, agent("engineer"), "delivery_unit.implementation.complete", "implementation", revision)
	assertPhaseAction(t, run, "verification", "system", "delivery_unit.verify")
	if !hasProjectedAction(delivery.ProjectionFor(run), "delivery_unit.gap.report", run.Feature.ID) {
		t.Fatal("integrated verification cannot return diagnostics to the Engineer")
	}

	reportGap(t, &run, "engineer", "verification", revision)
	unit = run.DeliveryUnits[0]
	if unit.Phase != delivery.DeliveryUnitImplementation || unit.ActiveRole != "engineer" || unit.ImplementationStatus != delivery.DeliveryGateNeedsChange || unit.VerificationStatus != delivery.DeliveryGateNeedsChange || len(unit.DevelopmentRepairItems) != 1 {
		t.Fatalf("verification gap did not return to the same Engineer loop: %#v", unit)
	}

	repairedRevision := strings.Repeat("b", 40)
	advanceUnitAtRevision(t, &run, agent("engineer"), "delivery_unit.implementation.complete", "implementation", repairedRevision)
	advanceUnitAtRevision(t, &run, delivery.Actor{ID: "integrated-verifier", Kind: delivery.ActorSystem}, "delivery_unit.verify", "verification", repairedRevision)
	unit = run.DeliveryUnits[0]
	if unit.Phase != delivery.DeliveryUnitComplete || unit.VerificationStatus != delivery.DeliveryGatePassed || unit.IntegratedGitRevision != repairedRevision || run.Stage != delivery.StageTesting || run.ActiveDeliveryUnitID != "" {
		t.Fatalf("integrated verification did not complete the Journey: run=%s unit=%#v", run.Stage, unit)
	}
}

func TestVerificationMustBindTheIntegratedRevision(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	advanceUnitAtRevision(t, &run, agent("engineer"), "delivery_unit.implementation.complete", "implementation", strings.Repeat("a", 40))
	err := applyUnitAtRevision(&run, delivery.Actor{ID: "integrated-verifier", Kind: delivery.ActorSystem}, "delivery_unit.verify", "verification", strings.Repeat("b", 40), "", nil)
	assertCode(t, err, "delivery_unit_integrated_revision_conflict")
}

func TestIntegratedVerificationRequiresTheWholeRuntimeContract(t *testing.T) {
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	revision := strings.Repeat("a", 40)
	advanceUnitAtRevision(t, &run, agent("engineer"), "delivery_unit.implementation.complete", "implementation", revision)
	evidence := backendGuideEvidence("delivery_unit.verify")
	evidence.RuntimeJourneyPassed = false
	bindBackendGuideEvidence(&run, evidence, revision, "integrated-verifier")
	payload, err := json.Marshal(map[string]any{
		"delivery_unit_id": run.ActiveDeliveryUnitID, "phase": "verification", "git_revision": revision,
		"summary": "The Runtime journey failed.", "evidence_refs": evidence.EvidenceRefs, "backend_guide": evidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = delivery.Apply(&run, delivery.Command{Actor: delivery.Actor{ID: "integrated-verifier", Kind: delivery.ActorSystem}, Type: "delivery_unit.verify", Payload: payload}, time.Now().UTC())
	assertCode(t, err, "integrated_verification_evidence_invalid")
}

func TestExistingSourceStartsAtIntegratedVerification(t *testing.T) {
	now := time.Now().UTC()
	product := testfixture.DemoProduct(now)
	sourceRevision := "workspace-" + strings.Repeat("a", 64)
	product.Engineering = productdomain.ProductEngineering{
		Mode: productdomain.EngineeringAdoptExistingSource, Status: productdomain.EngineeringReady,
		SourceRevision: sourceRevision, SourceRef: "source://workspace-snapshot/" + sourceRevision,
	}
	run, err := delivery.NewDeliveryRun(product, "F-001", 1, delivery.DeliveryRunSpec{
		ID: "existing-source-run", Name: "Existing source verification", Code: "VERIFY-1", Goal: "Verify the adopted source.",
		Members: []delivery.Member{
			{ID: "product", Name: "Product", Roles: []string{delivery.RoleProductOwner}},
			{ID: "development", Name: "Development", Roles: []string{delivery.RoleDevelopmentLead}},
			{ID: "quality", Name: "Quality", Roles: []string{delivery.RoleQualityLead}},
			{ID: "acceptance", Name: "Acceptance", Roles: []string{delivery.RoleBusinessAcceptor}},
			{ID: "release", Name: "Release", Roles: []string{delivery.RoleReleaseApprover}},
		},
	}, agent("verification-agent"), now)
	if err != nil {
		t.Fatal(err)
	}
	unit := run.DeliveryUnits[0]
	if unit.Phase != delivery.DeliveryUnitVerification || unit.ImplementationStatus != delivery.DeliveryGatePassed || unit.IntegratedGitRevision != sourceRevision || len(unit.DevelopmentTodos) != 1 {
		t.Fatalf("existing source did not skip implementation: %#v", unit)
	}
	err = applyUnitAtRevision(&run, delivery.Actor{ID: "integrated-verifier", Kind: delivery.ActorSystem}, "delivery_unit.verify", "verification", "workspace-"+strings.Repeat("b", 64), "", nil)
	assertCode(t, err, "delivery_source_revision_changed")
}

func completedRun(t *testing.T) delivery.DeliveryRun {
	t.Helper()
	run := testfixture.DemoDeliveryRun(time.Now().UTC())
	unit := &run.DeliveryUnits[0]
	unit.Phase = delivery.DeliveryUnitComplete
	unit.ActiveRole = ""
	unit.IntegratedGitRevision = strings.Repeat("a", 40)
	unit.ImplementationStatus = delivery.DeliveryGatePassed
	unit.VerificationStatus = delivery.DeliveryGatePassed
	unit.VerificationEvidence = backendGuideEvidence("delivery_unit.verify")
	bindBackendGuideEvidence(&run, unit.VerificationEvidence, unit.IntegratedGitRevision, "integrated-verifier")
	for index := range unit.DevelopmentTodos {
		unit.DevelopmentTodos[index].Status = delivery.DevelopmentTodoCompleted
		for workItemIndex := range unit.DevelopmentTodos[index].PhaseWorkItems {
			unit.DevelopmentTodos[index].PhaseWorkItems[workItemIndex].Status = delivery.DevelopmentTodoCompleted
		}
	}
	run.ActiveDeliveryUnitID = ""
	run.Stage = delivery.StageTesting
	product := testfixture.DemoProduct(time.Now().UTC())
	revision := product.Revisions[0]
	mustApply(t, &run, agent("engineer"), "product_revision.record", map[string]any{
		"content":       map[string]any{"story": revision.Story, "definition": revision.Definition, "decisions": revision.Decisions},
		"evidence_ref":  "workspace:" + unit.IntegratedGitRevision + "#evidence:evidence/product-revision.json",
		"code_revision": unit.IntegratedGitRevision, "model_sha256": strings.Repeat("d", 64),
	})
	return run
}

func passIndependentQuality(t *testing.T, run *delivery.DeliveryRun) {
	t.Helper()
	revision := run.DeliveryUnits[len(run.DeliveryUnits)-1].IntegratedGitRevision
	for {
		projection := delivery.ProjectionFor(*run)
		var testCaseID string
		for _, action := range projection.Workflow.AvailableActions {
			if action.Command == "quality.record" {
				testCaseID = action.TargetID
				break
			}
		}
		if testCaseID == "" {
			return
		}
		mustApply(t, run, agent("qa-agent"), "quality.record", map[string]any{
			"test_case_id": testCaseID, "git_revision": revision, "result": "pass",
			"note":          "The real end-to-end journey matched the expected outcome.",
			"evidence_refs": []string{"workspace:" + revision + "#evidence:evidence/quality/" + testCaseID + ".md"},
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
		"code": "verification_failed", "severity": "error", "path": "/", "message": "The integrated source does not satisfy the Journey.",
		"owner": owner, "category": "contract",
	}}
	if err := applyUnitAtRevision(run, delivery.Actor{ID: "integrated-verifier", Kind: delivery.ActorSystem}, "delivery_unit.gap.report", phase, revision, owner, diagnostics); err != nil {
		t.Fatalf("report %s gap: %v", owner, err)
	}
}

func applyUnitAtRevision(run *delivery.DeliveryRun, actor delivery.Actor, command, phase, revision, failureOwner string, diagnostics any) error {
	if diagnostics == nil {
		diagnostics = []any{}
	}
	evidence := backendGuideEvidence(command)
	if evidence != nil {
		bindBackendGuideEvidence(run, evidence, revision, actor.ID)
	}
	payloadValue := map[string]any{
		"delivery_unit_id": run.ActiveDeliveryUnitID, "phase": phase, "git_revision": revision,
		"summary": "The delivery boundary completed.", "evidence_refs": []string{"workspace:" + revision + "#evidence:" + phase + ".json"},
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
	return delivery.Apply(run, delivery.Command{Actor: actor, Type: command, Payload: payload}, time.Now().UTC())
}

func backendGuideEvidence(command string) *delivery.BackendGuideEvidence {
	if command != "delivery_unit.verify" {
		return nil
	}
	return &delivery.BackendGuideEvidence{
		ModelPath: "backend/model.json", ModelSHA256: strings.Repeat("d", 64),
		SingleBackendModel: true, StrictModelValidation: true, CrossReferencesResolved: true,
		GoBehaviorRegistry: true, NoExecutableBehaviorJSON: true, ProjectHTTP: true,
		HandlerUnitOfWork: true, HandlerIdempotency: true, DefinitionsRegistered: true,
		RuntimeBootstrap: true, NoProjectSchemaSQL: true, BackendGoModOnly: true,
		NoRootGoMod: true, NoGoWork: true, NoCompilerBuilder: true, NoGeneratedRuntimeContract: true,
		AuthWorkspacePermission: true, DataAuditPersistence: true, EmptyDatabaseInitPassed: true,
		SameModelRestartPassed: true, IncrementalModelRestartPassed: true,
		MockJourneyPassed: true, RuntimeJourneyPassed: true,
	}
}

func bindBackendGuideEvidence(run *delivery.DeliveryRun, evidence *delivery.BackendGuideEvidence, revision, executedBy string) {
	evidence.WorkspaceID = run.WorkspaceID
	evidence.ProductID = run.Product.ID
	evidence.FeatureRevision = run.Feature.Source.FeatureRevision
	evidence.RepositoryIdentity = "github.com/domainry/product-fixture"
	evidence.GitRevision = revision
	evidence.CheckSuite = "domainry-integrated-verification"
	evidence.CheckVersion = "1"
	evidence.ExecutedBy = executedBy
	evidence.OccurredAt = time.Now().UTC()
	evidence.EvidenceRefs = []string{"workspace:" + revision + "#evidence:integrated-verification.json"}
}

func assertPhaseAction(t *testing.T, run delivery.DeliveryRun, phase, role, command string) {
	t.Helper()
	unit := run.DeliveryUnits[0]
	if string(unit.Phase) != phase || unit.ActiveRole != role {
		t.Fatalf("unexpected DeliveryUnit assignment: %#v", unit)
	}
	if !hasProjectedAction(delivery.ProjectionFor(run), command, unit.ID) {
		t.Fatalf("phase %s did not expose %s", phase, command)
	}
}
