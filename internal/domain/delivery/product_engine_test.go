package delivery_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain/delivery"
)

func TestProductRevisionKeepsStoryAndDefinitionTogether(t *testing.T) {
	product := newProduct(t)
	if product.CurrentDefinitionRevision != 1 || product.CurrentReleaseRevision != 1 || len(product.Revisions) != 1 {
		t.Fatalf("unexpected initial product revisions: %#v", product)
	}
	if product.Revisions[0].Story.Title != "Booking product" || product.Revisions[0].Definition.SchemaVersion != 2 {
		t.Fatalf("story and definition were not saved together: %#v", product.Revisions[0])
	}
	definition := product.Revisions[0].Definition
	if definition.Access.Roles == nil || definition.Access.PermissionAdmins == nil || definition.Access.AssignmentRules == nil || definition.Integrations == nil || definition.Automations == nil || definition.Configuration == nil || definition.QualityConstraints == nil {
		t.Fatalf("product definition collections must have a stable array representation: %#v", definition)
	}
}

func TestNewProductQueuesEngineeringInitializationBeforeFeatureDelivery(t *testing.T) {
	product := newQueuedProduct(t)
	if product.Industry != "Fitness and wellness" || product.Engineering.Status != delivery.EngineeringFrontendQueued {
		t.Fatalf("new Product did not retain its industry and queued engineering initialization: %#v", product)
	}
	projection := delivery.ProductProjectionFor(product)
	if len(projection.AvailableActions) != 4 || projection.AvailableActions[2].Command != "product.engineering.frontend.start" || projection.AvailableActions[2].ActorKind != delivery.ActorAgent || projection.AvailableActions[3].Command != "product.delete" || projection.AvailableActions[3].ActorKind != delivery.ActorHuman {
		t.Fatalf("queued Product did not expose the RD engineering claim: %#v", projection.AvailableActions)
	}

	mustApplyProduct(t, &product, agent("rd-agent"), "product.engineering.frontend.start", map[string]any{})
	if product.Engineering.Status != delivery.EngineeringFrontendInitializing || product.Engineering.StartedAt == nil {
		t.Fatalf("RD did not claim Product engineering initialization: %#v", product.Engineering)
	}
	projection = delivery.ProductProjectionFor(product)
	if projection.AvailableActions[2].Command != "product.engineering.frontend.complete" {
		t.Fatalf("initializing Product did not expose engineering completion: %#v", projection.AvailableActions)
	}

	mustApplyProduct(t, &product, agent("rd-agent"), "product.engineering.frontend.complete", frontendEvidence())
	projection = delivery.ProductProjectionFor(product)
	if product.Engineering.Status != delivery.EngineeringFrontendReady || projection.AvailableActions[2].Command != "product.engineering.backend.start" {
		t.Fatalf("reviewable frontend did not queue backend initialization: %#v %#v", product.Engineering, projection.AvailableActions)
	}
	mustApplyProduct(t, &product, agent("rd-agent"), "product.engineering.backend.start", map[string]any{})
	projection = delivery.ProductProjectionFor(product)
	if product.Engineering.Status != delivery.EngineeringBackendInitializing || projection.AvailableActions[2].Command != "product.engineering.backend.complete" {
		t.Fatalf("backend initialization was not assigned to RD: %#v %#v", product.Engineering, projection.AvailableActions)
	}
	mustApplyProduct(t, &product, agent("rd-agent"), "product.engineering.backend.complete", backendEvidence())
	if product.Engineering.Status != delivery.EngineeringReady || product.Engineering.APIContractRef == "" || product.Engineering.AuthorizationEntry == "" {
		t.Fatalf("Product engineering did not retain the backend foundation evidence: %#v", product.Engineering)
	}
}

func TestFeatureConversationBecomesConfirmedRequirement(t *testing.T) {
	product := newProduct(t)
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", featureDraftPayload("feature-cancel", "F-001"))

	feature := product.Features[0]
	if feature.Status != delivery.FeatureDraft || feature.Draft == nil || feature.Draft.Sources[0].ConversationID != "conversation-cancel" || feature.Draft.Sources[0].RunID != "run-cancel-1" || len(feature.Revisions) != 0 {
		t.Fatalf("feature discovery was not retained separately from revisions: %#v", feature)
	}
	err := applyProduct(&product, agent("pm-agent"), "feature.confirm", map[string]any{"feature_id": feature.ID, "draft_version": 1})
	assertCode(t, err, "human_confirmation_required")

	mustApplyProduct(t, &product, human("product-owner"), "feature.confirm", map[string]any{"feature_id": feature.ID, "draft_version": 1})
	if product.CurrentDefinitionRevision != 1 || len(product.Revisions) != 1 {
		t.Fatalf("PM confirmation must not create a ProductRevision: %#v", product.Revisions)
	}
	confirmed := product.Features[0]
	if confirmed.Status != delivery.FeatureConfirmed || confirmed.ConfirmedRevision != 1 || confirmed.Draft != nil || len(confirmed.Revisions) != 1 || len(confirmed.Revisions[0].Sources) != 1 {
		t.Fatalf("feature confirmation link is incomplete: %#v", confirmed)
	}
	if confirmed.DeliverySequence != 1 || confirmed.QueuedAt == nil {
		t.Fatalf("confirmed Feature was not placed in the development queue: %#v", confirmed)
	}
}

func TestConfirmedFeaturesAreOfferedForDeliveryInQueueOrder(t *testing.T) {
	product := newProduct(t)
	for _, item := range []struct {
		id   string
		code string
	}{
		{id: "feature-first", code: "F-001"},
		{id: "feature-second", code: "F-002"},
	} {
		payload := featureDraftPayload(item.id, item.code)
		if len(product.Features) > 0 {
			authorization := payload["specification"].(map[string]any)["authorization"].(map[string]any)
			authorization["mode"] = "feature_delta"
			authorization["authentication"] = ""
			authorization["permission_model"] = ""
			authorization["roles"] = []any{}
		}
		mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", payload)
		mustApplyProduct(t, &product, human("product-owner"), "feature.confirm", map[string]any{"feature_id": item.id, "draft_version": 1})
	}

	projection := delivery.ProductProjectionFor(product)
	var queued []string
	for _, action := range projection.AvailableActions {
		if action.Command == "feature.delivery.start" {
			queued = append(queued, action.TargetID)
		}
	}
	if len(queued) != 1 || queued[0] != "feature-first" {
		t.Fatalf("Delivery exposed requirements outside queue order: %#v", projection.AvailableActions)
	}

	payload, _ := json.Marshal(map[string]any{
		"feature_id": "feature-second", "feature_revision": 1, "name": "Second delivery", "code": "RUN-002",
		"goal": "Ship second", "target_date": "",
		"members": []map[string]any{{"id": "owner", "name": "Owner", "roles": []string{delivery.RoleProductOwner, delivery.RoleBusinessAcceptor, delivery.RoleReleaseApprover}}},
	})
	_, err := delivery.StartDelivery(&product, "run-second", delivery.Command{Actor: agent("rd-agent"), Type: "feature.delivery.start", Payload: payload}, time.Now().UTC())
	assertCode(t, err, "feature_delivery_order_invalid")
}

func TestHumanCanOpenFeatureDiscoveryBeforeProvidingTheFirstMessage(t *testing.T) {
	product := newProduct(t)
	err := applyProduct(&product, agent("pm-agent"), "feature.discovery.open", map[string]any{"feature_id": "feature-inquiry"})
	assertCode(t, err, "actor_unauthorized")

	mustApplyProduct(t, &product, human("product-owner"), "feature.discovery.open", map[string]any{"feature_id": "feature-inquiry"})
	feature := product.Features[0]
	if feature.ID != "feature-inquiry" || feature.Code != "F-001" || feature.Status != delivery.FeatureDraft || feature.Draft == nil {
		t.Fatalf("Feature discovery workspace was not opened: %#v", feature)
	}
	if feature.Draft.Version != 1 || feature.Draft.BaselineProductRevision != product.CurrentDefinitionRevision || feature.Draft.Readiness.Status != "discovering" {
		t.Fatalf("Feature discovery workspace has an invalid initial state: %#v", feature.Draft)
	}
	if feature.Draft.Discovery.Evidence == nil || feature.Draft.Specification.Scenarios == nil || feature.Attachments == nil || feature.Revisions == nil {
		t.Fatalf("Feature discovery collections must have a stable array representation: %#v", feature)
	}

	payload := featureDraftPayload("feature-inquiry", "F-001")
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", payload)
	if product.Features[0].Draft == nil || product.Features[0].Draft.Version != 2 {
		t.Fatalf("PM discovery did not continue in the opened workspace: %#v", product.Features[0])
	}
}

func TestFeatureDiscoveryKeepsTurnsMutableAndRanksTheNextQuestion(t *testing.T) {
	product := newProduct(t)
	first := featureDraftPayload("feature-cancel", "F-001")
	discovery := first["discovery"].(map[string]any)
	discovery["open_questions"] = []map[string]any{
		{"id": "question-label", "question": "What should the button say?", "kind": "clarify_experience", "blocks": []string{}, "impact": "low", "uncertainty": "medium", "dependency": "low", "answer_cost": "low", "priority_score": 18, "priority_reason": "Low business leverage", "status": "open"},
		{"id": "question-owner", "question": "Who may cancel another member's booking?", "kind": "clarify_access", "blocks": []string{"authorization", "exception-flow"}, "impact": "high", "uncertainty": "high", "dependency": "high", "answer_cost": "low", "priority_score": 94, "priority_reason": "Changes authorization and the exception flow", "status": "open"},
	}
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", first)

	second := featureDraftPayload("feature-cancel", "F-001")
	source := second["source"].(map[string]any)
	source["run_id"] = "run-cancel-2"
	source["source_ids"] = []string{"source-follow-up"}
	secondDiscovery := second["discovery"].(map[string]any)
	secondDiscovery["open_questions"] = discovery["open_questions"]
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", second)

	feature := product.Features[0]
	if feature.Draft == nil || feature.Draft.Version != 2 || len(feature.Draft.Sources) != 2 || len(feature.Revisions) != 0 {
		t.Fatalf("conversation turns were incorrectly frozen as revisions: %#v", feature)
	}
	if feature.Draft.NextQuestion != "Who may cancel another member's booking?" {
		t.Fatalf("Delivery did not select the structurally important question: %q", feature.Draft.NextQuestion)
	}
}

func TestFeatureDiscoveryRetainsActorsAndRulesAsFirstClassEvidence(t *testing.T) {
	product := newProduct(t)
	payload := featureDraftPayload("feature-cancel", "F-001")
	discovery := payload["discovery"].(map[string]any)
	discovery["evidence"] = append(discovery["evidence"].([]map[string]any),
		map[string]any{"id": "evidence-actor", "kind": "actor", "statement": "A receptionist cancels on behalf of a member", "status": "confirmed", "source_ids": []string{"source-actor"}, "related_ids": []string{"cancel-booking"}},
		map[string]any{"id": "evidence-rule", "kind": "rule", "statement": "A late cancellation consumes one class credit", "status": "confirmed", "source_ids": []string{"source-rule"}, "related_ids": []string{"cancel-booking"}},
	)

	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", payload)

	evidence := product.Features[0].Draft.Discovery.Evidence
	if evidence[len(evidence)-2].Kind != "actor" || evidence[len(evidence)-1].Kind != "rule" {
		t.Fatalf("actor and rule evidence were not retained: %#v", evidence)
	}
}

func TestIncompleteFeatureConversationIsSavedButCannotBeConfirmed(t *testing.T) {
	product := newProduct(t)
	payload := featureDraftPayload("feature-cancel", "F-001")
	specification := payload["specification"].(map[string]any)
	specification["acceptance"] = []any{}
	setOpenQuestion(payload, "What observable result proves cancellation works?")
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", payload)

	feature := product.Features[0]
	draft := feature.Draft
	if feature.Status != delivery.FeatureDraft || draft == nil || draft.Readiness.Status != "shaping" || draft.NextQuestion == "" || !contains(draft.Readiness.BlockingSections, "acceptance") {
		t.Fatalf("incomplete PM progress was not retained: %#v", feature)
	}
	for _, action := range delivery.ProductProjectionFor(product).AvailableActions {
		if action.Command == "feature.confirm" && action.TargetID == feature.ID {
			t.Fatalf("incomplete Feature exposed confirmation: %#v", action)
		}
	}
	err := applyProduct(&product, human("product-owner"), "feature.confirm", map[string]any{"feature_id": feature.ID, "draft_version": 1})
	assertCode(t, err, "feature_specification_incomplete")
}

func TestFeatureRequiresAConcreteBusinessScenario(t *testing.T) {
	product := newProduct(t)
	payload := featureDraftPayload("feature-cancel", "F-001")
	specification := payload["specification"].(map[string]any)
	specification["scenarios"] = []any{}
	setOpenQuestion(payload, "What happens from the member's cancellation request to the released place?")
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", payload)

	draft := product.Features[0].Draft
	if draft == nil || draft.Readiness.Status != "shaping" || !contains(draft.Readiness.BlockingSections, "scenarios") {
		t.Fatalf("missing business scenario did not block confirmation: %#v", draft)
	}
}

func TestOpenBusinessConflictBlocksConfirmation(t *testing.T) {
	product := newProduct(t)
	payload := featureDraftPayload("feature-cancel", "F-001")
	discovery := payload["discovery"].(map[string]any)
	discovery["evidence"] = []map[string]any{
		{"id": "evidence-owner", "kind": "fact", "statement": "Members can cancel only their own booking", "status": "stated", "source_ids": []string{"source-member"}, "related_ids": []string{"cancel-booking"}},
		{"id": "evidence-manager", "kind": "correction", "statement": "Location managers can cancel any booking", "status": "stated", "source_ids": []string{"source-manager"}, "related_ids": []string{"cancel-booking"}},
	}
	discovery["conflicts"] = []map[string]any{{"id": "conflict-owner", "summary": "Cancellation authority differs by actor", "evidence_ids": []string{"evidence-owner", "evidence-manager"}, "impact": "medium", "status": "open", "resolution": ""}}
	setOpenQuestion(payload, "Which actor may cancel which booking, and under what exception?")
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", payload)

	draft := product.Features[0].Draft
	if draft == nil || !contains(draft.Readiness.BlockingIssues, "open_conflict") {
		t.Fatalf("an unresolved business contradiction did not block confirmation: %#v", draft)
	}
}

func TestFeatureConflictMustReferenceRetainedEvidence(t *testing.T) {
	product := newProduct(t)
	payload := featureDraftPayload("feature-cancel", "F-001")
	discovery := payload["discovery"].(map[string]any)
	discovery["conflicts"] = []map[string]any{{"id": "conflict-owner", "summary": "Cancellation authority differs by actor", "evidence_ids": []string{"evidence-cancel", "evidence-missing"}, "impact": "high", "status": "open", "resolution": ""}}
	setOpenQuestion(payload, "Who may cancel another member's booking?")

	err := applyProduct(&product, agent("pm-agent"), "feature.discovery.replace", payload)
	assertCode(t, err, "feature_conflict_evidence_invalid")
}

func TestConfirmedFeatureCannotReturnToDraft(t *testing.T) {
	product := newProduct(t)
	payload := featureDraftPayload("feature-cancel", "F-001")
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", payload)
	mustApplyProduct(t, &product, human("product-owner"), "feature.confirm", map[string]any{"feature_id": "feature-cancel", "draft_version": 1})

	err := applyProduct(&product, agent("pm-agent"), "feature.discovery.replace", payload)
	assertCode(t, err, "feature_locked")
	feature := product.Features[0]
	if feature.Status != delivery.FeatureConfirmed || feature.CurrentRevision != 1 || feature.ConfirmedRevision != 1 || len(feature.Revisions) != 1 {
		t.Fatalf("rejected replacement changed the confirmed Feature: %#v", feature)
	}
}

func TestFeatureDraftRejectsIncompleteDecision(t *testing.T) {
	product := newProduct(t)
	payload := featureDraftPayload("feature-cancel", "F-001")
	payload["decisions"] = []map[string]any{{"id": "decision-window", "title": "Cancellation deadline", "question": "When does cancellation close?", "impact": "high", "options": []any{}, "resolution": "", "rationale": "The deadline changes eligibility", "tradeoffs": []any{}, "status": "open", "source_ids": []string{"source-interview"}}}
	err := applyProduct(&product, agent("pm-agent"), "feature.discovery.replace", payload)
	assertCode(t, err, "feature_decision_options_missing")
	if len(product.Features) != 0 {
		t.Fatal("rejected decision changed the product")
	}
}

func TestFeatureInstallRequiresExactRunReleaseAndSystemActor(t *testing.T) {
	product := newProduct(t)
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", featureDraftPayload("feature-cancel", "F-001"))
	mustApplyProduct(t, &product, human("product-owner"), "feature.confirm", map[string]any{"feature_id": "feature-cancel", "draft_version": 1})
	payload, _ := json.Marshal(map[string]any{
		"feature_id": "feature-cancel", "feature_revision": 1, "name": "Cancel booking delivery", "code": "RUN-001",
		"goal": "Ship verified booking cancellation", "target_date": "2026-10-01",
		"members": []map[string]any{{"id": "owner", "name": "Owner", "roles": []string{delivery.RoleProductOwner}}, {"id": "acceptor", "name": "Acceptor", "roles": []string{delivery.RoleBusinessAcceptor}}, {"id": "release", "name": "Release", "roles": []string{delivery.RoleReleaseApprover}}},
	})
	run, err := delivery.StartDelivery(&product, "run-feature-cancel-1", delivery.Command{Actor: agent("rd-agent"), Type: "feature.delivery.start", Payload: payload}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	deployedAt := time.Now().UTC()
	gitRevision := "0123456789abcdef0123456789abcdef01234567"
	unit := &run.DeliveryUnits[0]
	unit.Phase = delivery.DeliveryUnitComplete
	unit.ActiveRole = ""
	unit.ContractGitRevision = gitRevision
	unit.ImplementationGitRevision = gitRevision
	unit.InteractionStatus = delivery.DeliveryGatePassed
	unit.CompilerStatus = delivery.DeliveryGatePassed
	unit.FrontendStatus = delivery.DeliveryGatePassed
	unit.BackendStatus = delivery.DeliveryGatePassed
	unit.JourneyStatus = delivery.DeliveryGatePassed
	run.ActiveDeliveryUnitID = ""
	for _, testCase := range run.TestCases {
		run.QualityRuns = append(run.QualityRuns, delivery.QualityRun{
			ID: "quality-" + testCase.ID, GitRevision: gitRevision, TestCaseID: testCase.ID,
			Result: delivery.ResultPass, Note: "Verified", EvidenceRefs: []string{"git:" + gitRevision + "#evidence:evidence/quality.md"},
		})
	}
	for _, acceptanceCase := range run.AcceptanceCases {
		run.AcceptanceConfirmations = append(run.AcceptanceConfirmations, delivery.AcceptanceConfirmation{
			ID: "acceptance-" + acceptanceCase.ID, GitRevision: gitRevision, AcceptanceCaseID: acceptanceCase.ID,
			Result: delivery.ResultPass, Note: "Accepted", EvidenceRefs: []string{"git:" + gitRevision + "#evidence:evidence/acceptance.md"},
		})
	}
	run.ReleaseChecks = []delivery.ReleaseCheck{{ID: "RC-01", Required: true, Status: delivery.ReleaseCheckPassed}}
	run.Stage = delivery.StageLive
	run.Releases = []delivery.Release{{
		ID: "release-1", Version: "1.1.0", Status: delivery.ReleaseLive, CodeRevision: gitRevision,
		DeploymentAttempt: &delivery.DeploymentAttempt{ID: "deploy-1", Outcome: delivery.DeploymentSuccess, EnvironmentRef: "env://production", LaunchURL: "https://greenfit.example.test", ReceiptRef: "receipt://release-1", ResolvedAt: &deployedAt},
	}}
	err = delivery.InstallDeliveryRun(&product, &run, agent("op-agent"), time.Now().UTC())
	assertCode(t, err, "installation_actor_invalid")
	if err := delivery.InstallDeliveryRun(&product, &run, delivery.Actor{ID: "release-adapter", Kind: delivery.ActorSystem}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if product.Features[0].Status != delivery.FeatureInstalled || product.CurrentDefinitionRevision != 1 || product.CurrentReleaseRevision != 1 || product.Status != delivery.ProductActive {
		t.Fatalf("feature was not installed into the product: %#v", product)
	}
	if len(product.Revisions) != 1 {
		t.Fatalf("V3 installation invented an executable ProductRevision: %#v", product.Revisions)
	}
	expectedDeployment := delivery.ProductDeployment{ReleaseID: "release-1", Version: "1.1.0", EnvironmentRef: "env://production", LaunchURL: "https://greenfit.example.test", ReceiptRef: "receipt://release-1", DeployedAt: deployedAt}
	if product.CurrentDeployment == nil || *product.CurrentDeployment != expectedDeployment {
		t.Fatalf("installed product lost its deployment destination: %#v", product.CurrentDeployment)
	}
}

func TestFeatureDraftKeepsResolvedBusinessDecision(t *testing.T) {
	product := newProduct(t)
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", featureDraftPayload("feature-cancel", "F-001"))
	decisions := product.Features[0].Draft.Decisions
	if len(decisions) != 1 || decisions[0].Resolution != "Cancellation closes two hours before class" {
		t.Fatalf("resolved Feature decision was not retained: %#v", decisions)
	}
}

func TestFeatureRevisionNormalizesCoreCollections(t *testing.T) {
	product := newProduct(t)
	payload := featureDraftPayload("feature-cancel", "F-001")
	payload["decisions"] = nil
	source := payload["source"].(map[string]any)
	delete(source, "decision_ids")
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", payload)

	draft := product.Features[0].Draft
	if draft == nil || draft.Decisions == nil || draft.Sources[0].DecisionIDs == nil || draft.Discovery.Options == nil {
		t.Fatalf("Feature discovery collections must have a stable array representation: %#v", draft)
	}
	if draft.Specification.Scenarios == nil || draft.Specification.Impacts == nil || draft.Specification.Acceptance == nil {
		t.Fatalf("Feature contract collections must serialize as arrays: %#v", draft.Specification)
	}
}

func newProduct(t *testing.T) delivery.Product {
	t.Helper()
	product := newQueuedProduct(t)
	mustApplyProduct(t, &product, agent("rd-agent"), "product.engineering.frontend.start", map[string]any{})
	mustApplyProduct(t, &product, agent("rd-agent"), "product.engineering.frontend.complete", frontendEvidence())
	mustApplyProduct(t, &product, agent("rd-agent"), "product.engineering.backend.start", map[string]any{})
	mustApplyProduct(t, &product, agent("rd-agent"), "product.engineering.backend.complete", backendEvidence())
	return product
}

func frontendEvidence() map[string]any {
	return map[string]any{
		"code_revision": "git:frontend-foundation", "artifact_ref": "deck-artifact://sha256/frontend-foundation",
		"design_contract_ref": "deck-evidence://sha256/design", "login_entry": "frontend/src/pages/Login.tsx", "shell_entry": "frontend/src/AppShell.tsx", "preview_entry": "frontend/dist/index.html",
	}
}

func backendEvidence() map[string]any {
	return map[string]any{
		"code_revision": "git:engineering-foundation", "artifact_ref": "deck-artifact://sha256/backend-foundation",
		"api_contract_ref": "deck-evidence://sha256/openapi", "test_evidence_ref": "deck-evidence://sha256/backend-tests",
		"service_entry": "backend/src/main.rs", "authentication_entry": "backend/src/authentication.rs",
		"authorization_entry": "backend/src/authorization.rs", "health_entry": "backend/src/health.rs",
	}
}

func newQueuedProduct(t *testing.T) delivery.Product {
	t.Helper()
	payload := map[string]any{
		"name": "Booking product", "code": "BOOKING", "goal": "Make the booking process operational", "industry": "Fitness and wellness",
		"story":      map[string]any{"title": "Booking product", "summary": "Manage bookings consistently", "narrative": "A member requests a booking, and the location confirms capacity and completes the service."},
		"definition": map[string]any{"schema_version": 2, "actors": []any{}, "scenarios": []any{}, "objects": []any{}, "rules": []any{}, "exceptions": []any{}, "actions": []any{}, "pages": []any{}, "access": map[string]any{}, "integrations": []any{}, "automations": []any{}, "configuration": []any{}, "quality_constraints": []any{}},
		"decisions":  []any{},
	}
	raw, _ := json.Marshal(payload)
	product, err := delivery.NewProduct("workspace-1", "product-booking", delivery.Command{
		Actor: human("product-owner"), Type: "product.create", Payload: raw,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return product
}

func featureDraftPayload(featureID, code string) map[string]any {
	return map[string]any{
		"feature_id": featureID, "code": code, "title": "Booking cancellation", "summary": "Members can cancel a booking",
		"priority": "high", "baseline_product_revision": 1,
		"discovery": map[string]any{
			"focus":       map[string]any{"topic": "Booking cancellation", "scenario_id": "discovery-cancel", "dialogue_move": "summarize_and_confirm", "rationale": "The real cancellation flow and deadline are confirmed"},
			"evidence":    []map[string]any{{"id": "evidence-cancel", "kind": "fact", "statement": "Members need to cancel bookings they cannot attend", "status": "confirmed", "source_ids": []string{"source-interview"}, "related_ids": []string{"cancel-booking"}}},
			"scenarios":   []map[string]any{{"id": "discovery-cancel", "title": "Member cancels an eligible booking", "actor_ids": []string{"member"}, "trigger": "The member can no longer attend", "current_flow": []string{"Contact the location"}, "pain_points": []string{"Capacity remains held"}, "target_outcome": "Release capacity safely", "failure_modes": []string{"Late cancellation"}, "status": "confirmed"}},
			"assumptions": []any{}, "conflicts": []any{}, "options": []any{}, "open_questions": []any{},
		},
		"specification": map[string]any{
			"intent": map[string]any{
				"problem": "Members need to release bookings they cannot attend", "desired_outcome": "Release capacity safely",
				"success_measures": []string{"Capacity is released for every accepted cancellation"},
			},
			"scope": map[string]any{"in": []string{"Cancel active bookings"}, "out": []string{"Refund processing"}},
			"scenarios": []map[string]any{{
				"id": "cancel-booking", "title": "Cancel an eligible booking", "actor_ids": []string{"member"},
				"trigger": "A member can no longer attend", "preconditions": []string{"The booking is active"},
				"main_flow": []string{"Open booking", "Confirm cancellation"}, "alternate_flows": []string{"Reject late cancellation"},
				"outcome": "The booking is cancelled and capacity is released",
			}},
			"impacts": []map[string]any{{
				"id": "impact-cancel", "operation": "add", "target_kind": "action", "target_id": "booking.cancel",
				"summary": "Add cancellation to Booking", "details": []string{"Transition active bookings to cancelled", "Release reserved capacity"},
			}},
			"authorization": map[string]any{
				"mode": "establish_baseline", "authentication": "required", "permission_model": "fixed",
				"roles":                []map[string]any{{"id": "member", "name": "Member", "actor_ids": []string{"member"}, "responsibilities": []string{"Manage personal bookings"}, "capabilities": []string{"Cancel eligible bookings"}, "data_scopes": []string{"Own bookings"}, "source_ids": []string{"source-interview"}}},
				"grants":               []map[string]any{{"id": "grant-cancel", "role_ids": []string{"member"}, "capability_id": "booking.cancel", "capability": "Cancel an eligible booking", "scenario_ids": []string{"cancel-booking"}, "action_ids": []string{"booking.cancel"}, "data_scopes": []string{"Own bookings"}, "conditions": []string{"Before the cancellation cutoff"}}},
				"permission_admin_ids": []string{}, "assignment_rules": []string{},
			},
			"acceptance": []map[string]any{{
				"id": "accept-cancel", "title": "Eligible cancellation releases capacity",
				"given": []string{"An active booking exists before the deadline"}, "when": "The owning member cancels the booking",
				"then": []string{"The booking becomes cancelled", "The reserved capacity is released"},
			}},
		},
		"decisions": []map[string]any{{
			"id": "decision-window", "title": "Cancellation deadline", "question": "When does cancellation close?", "impact": "high",
			"options":               []map[string]any{{"id": "two-hours", "label": "Two hours", "description": "Close normal cancellation two hours before class"}, {"id": "four-hours", "label": "Four hours", "description": "Close normal cancellation four hours before class"}},
			"recommended_option_id": "two-hours", "selected_option_id": "two-hours", "resolution": "Cancellation closes two hours before class",
			"rationale": "The shared cutoff protects capacity while allowing member changes", "tradeoffs": []string{"A shorter cutoff gives the location less recovery time"},
			"status": "resolved", "source_ids": []string{"source-interview"},
		}},
		"source": map[string]any{"conversation_id": "conversation-cancel", "run_id": "run-cancel-1", "source_ids": []string{"source-interview"}, "decision_ids": []string{"decision-window"}},
	}
}

func TestEveryFeatureRequiresExplicitAuthorization(t *testing.T) {
	product := newProduct(t)
	first := featureDraftPayload("feature-cancel", "F-001")
	firstSpecification := first["specification"].(map[string]any)
	delete(firstSpecification, "authorization")
	setOpenQuestion(first, "Which roles may use the Product and what may each role do?")
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", first)
	if draft := product.Features[0].Draft; draft == nil || !contains(draft.Readiness.BlockingSections, "authorization") {
		t.Fatalf("the first Feature did not require an authorization baseline: %#v", draft)
	}

	complete := featureDraftPayload("feature-cancel", "F-001")
	source := complete["source"].(map[string]any)
	source["run_id"] = "run-cancel-2"
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", complete)
	mustApplyProduct(t, &product, human("product-owner"), "feature.confirm", map[string]any{"feature_id": "feature-cancel", "draft_version": 2})

	second := featureDraftPayload("feature-reschedule", "F-002")
	secondSpecification := second["specification"].(map[string]any)
	authorization := secondSpecification["authorization"].(map[string]any)
	authorization["mode"] = "feature_delta"
	authorization["authentication"] = ""
	authorization["permission_model"] = ""
	authorization["roles"] = []any{}
	authorization["grants"] = []any{}
	setOpenQuestion(second, "Which existing roles may use the rescheduling capability?")
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", second)
	if draft := product.Features[1].Draft; draft == nil || !contains(draft.Readiness.BlockingSections, "authorization") {
		t.Fatalf("a later Feature did not require capability grants: %#v", draft)
	}
}

func TestFeatureAuthorizationUnknownScenarioIdentifiesGrantAndScenario(t *testing.T) {
	product := newProduct(t)
	payload := featureDraftPayload("feature-cancel", "F-001")
	specification := payload["specification"].(map[string]any)
	authorization := specification["authorization"].(map[string]any)
	grants := authorization["grants"].([]map[string]any)
	grants[0]["scenario_ids"] = []string{"missing-scenario"}

	err := applyProduct(&product, agent("pm-agent"), "feature.discovery.replace", payload)
	domainError, ok := err.(*delivery.Error)
	if !ok {
		t.Fatalf("expected Delivery error, got %#v", err)
	}
	if domainError.Code != "feature_authorization_scenario_unknown" {
		t.Fatalf("unexpected error code: %#v", domainError)
	}
	want := map[string]any{"grant_id": "grant-cancel", "scenario_id": "missing-scenario"}
	if !reflect.DeepEqual(domainError.Details, want) {
		t.Fatalf("unexpected error details: %#v", domainError.Details)
	}
}

func TestProductDeleteRequiresHumanAndRemovesAvailableActions(t *testing.T) {
	product := newProduct(t)
	err := applyProduct(&product, agent("rd-agent"), "product.delete", map[string]any{})
	domainError, ok := err.(*delivery.Error)
	if !ok || domainError.Code != "human_deletion_required" {
		t.Fatalf("expected human deletion requirement, got %#v", err)
	}

	mustApplyProduct(t, &product, human("product-owner"), "product.delete", map[string]any{})
	if product.Status != delivery.ProductArchived {
		t.Fatalf("expected archived Product, got %q", product.Status)
	}
	if actions := delivery.ProductProjectionFor(product).AvailableActions; len(actions) != 0 {
		t.Fatalf("archived Product still exposes actions: %#v", actions)
	}
}

func setOpenQuestion(payload map[string]any, question string) {
	discovery := payload["discovery"].(map[string]any)
	discovery["open_questions"] = []map[string]any{{
		"id": "question-next", "question": question, "kind": "clarify_business_rule",
		"blocks": []string{"feature-confirmation"},
		"impact": "high", "uncertainty": "high", "dependency": "high", "answer_cost": "low",
		"priority_score": 90, "priority_reason": "Blocks Feature confirmation", "status": "open",
	}}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mustApplyProduct(t *testing.T, product *delivery.Product, actor delivery.Actor, commandType string, payload any) {
	t.Helper()
	if err := applyProduct(product, actor, commandType, payload); err != nil {
		t.Fatalf("%s failed: %v", commandType, err)
	}
}

func applyProduct(product *delivery.Product, actor delivery.Actor, commandType string, payload any) error {
	raw, _ := json.Marshal(payload)
	return delivery.ApplyProduct(product, delivery.Command{Actor: actor, Type: commandType, Payload: raw}, time.Now().UTC())
}
