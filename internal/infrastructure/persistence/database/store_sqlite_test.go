package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/domainry/domainry-delivery/internal/application"
	delivery "github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	productdomain "github.com/domainry/domainry-delivery/internal/domain/product"
	"github.com/domainry/domainry-delivery/internal/testfixture"
	_ "modernc.org/sqlite"
)

const verifiedGitRevision = "0123456789abcdef0123456789abcdef01234567"

func storeBackendGuideEvidence(run deliveryrun.DeliveryRun, actor delivery.Actor, command, revision string) map[string]any {
	evidence := map[string]any{
		"workspace_id": run.WorkspaceID, "product_id": run.Product.ID, "feature_revision": run.Feature.Source.FeatureRevision,
		"repository_identity": "github.com/domainry/product-fixture", "git_revision": revision, "git_status": "clean",
		"check_suite": "plane-backend-guide", "check_version": "1", "executed_by": actor.ID, "occurred_at": time.Now().UTC(),
		"evidence_refs": []string{"git:" + revision + "#evidence:backend-guide.json"},
		"model_path":    "backend/model.json", "model_sha256": strings.Repeat("d", 64),
	}
	switch command {
	case "delivery_unit.model.verify":
		evidence["single_backend_model"] = true
		evidence["strict_model_validation"] = true
		evidence["cross_references_resolved"] = true
		evidence["go_behavior_registry"] = true
		evidence["no_executable_behavior_json"] = true
	case "delivery_unit.contract.verify":
		evidence["project_http"] = true
		evidence["handler_unit_of_work"] = true
		evidence["handler_idempotency"] = true
		evidence["definitions_registered"] = true
		evidence["runtime_bootstrap"] = true
		evidence["no_project_schema_sql"] = true
		evidence["backend_go_mod_only"] = true
		evidence["no_root_go_mod"] = true
		evidence["no_go_work"] = true
		evidence["no_compiler_builder"] = true
		evidence["no_generated_runtime_contract"] = true
		evidence["auth_workspace_permission"] = true
		evidence["data_audit_persistence"] = true
		evidence["empty_database_init_passed"] = true
		evidence["same_model_restart_passed"] = true
		evidence["changed_model_rejected"] = true
	case "delivery_unit.journey.complete":
		evidence["mock_journey_passed"] = true
		evidence["runtime_journey_passed"] = true
	default:
		return nil
	}
	return evidence
}

func TestCommandReceiptIsIdempotentAndRevisionIsOptimistic(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	service := application.NewService(application.Ports{Products: store, Runs: store, Lifecycle: store})
	ctx := context.Background()
	seedRun(t, store, testfixture.DemoDeliveryRun(time.Now().UTC()))
	command := delivery.Command{
		ClientID:         "client-once",
		ExpectedRevision: 1,
		Actor:            delivery.Actor{ID: "rd-agent", Kind: delivery.ActorAgent},
		Type:             "delivery_unit.interaction.complete",
		Payload:          json.RawMessage(`{"delivery_unit_id":"F-001","phase":"interaction_modeling","git_revision":"0123456","summary":"interaction model completed"}`),
	}
	dispatchContext := application.WithTrustedPrincipal(ctx, testfixture.DemoWorkspaceID, command.Actor, application.PermissionDeliveryRunWrite)
	first, err := service.Dispatch(dispatchContext, testfixture.DemoWorkspaceID, testfixture.DemoDeliveryRunID, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Dispatch(dispatchContext, testfixture.DemoWorkspaceID, testfixture.DemoDeliveryRunID, command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 2 || second.Revision != 2 || second.DeliveryUnits[0].Phase != deliveryrun.DeliveryUnitDomainModeling {
		t.Fatalf("idempotent retry changed state: first=%d second=%d", first.Revision, second.Revision)
	}
	progress := command
	progress.ClientID = "client-progress"
	progress.ExpectedRevision = 2
	progress.Type = "delivery_unit.model.complete"
	progress.Payload = json.RawMessage(`{"delivery_unit_id":"F-001","phase":"domain_modeling","git_revision":"0123456","summary":"model completed"}`)
	current, err := service.Dispatch(dispatchContext, testfixture.DemoWorkspaceID, testfixture.DemoDeliveryRunID, progress)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 3 || current.DeliveryUnits[0].Phase != deliveryrun.DeliveryUnitModelVerification {
		t.Fatalf("progress command did not advance current state: %#v", current)
	}
	lateReplay, err := service.Dispatch(dispatchContext, testfixture.DemoWorkspaceID, testfixture.DemoDeliveryRunID, command)
	if err != nil {
		t.Fatal(err)
	}
	if lateReplay.Revision != first.Revision || lateReplay.DeliveryUnits[0].Phase != deliveryrun.DeliveryUnitDomainModeling {
		t.Fatalf("late replay was rebuilt from current state instead of the original receipt: %#v", lateReplay)
	}
	changedFingerprint := progress
	changedFingerprint.ClientID = command.ClientID
	_, err = service.Dispatch(dispatchContext, testfixture.DemoWorkspaceID, testfixture.DemoDeliveryRunID, changedFingerprint)
	domainError, ok := err.(*delivery.Error)
	if !ok || domainError.Code != "idempotency_key_reused" {
		t.Fatalf("expected changed fingerprint conflict, got %#v", err)
	}
	stale := command
	stale.ClientID = "client-stale"
	stale.Type = "delivery_unit.model.complete"
	stale.Payload = json.RawMessage(`{"delivery_unit_id":"F-001","phase":"domain_modeling","git_revision":"0123456","summary":"model completed"}`)
	_, err = service.Dispatch(dispatchContext, testfixture.DemoWorkspaceID, testfixture.DemoDeliveryRunID, stale)
	domainError, ok = err.(*delivery.Error)
	if !ok || domainError.Code != "revision_conflict" {
		t.Fatalf("expected revision conflict, got %#v", err)
	}
}

func TestProductCommandsAreIdempotentAndRevisionFenced(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	service := application.NewService(application.Ports{Products: store, Runs: store, Lifecycle: store})
	ctx := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		application.PermissionProductRead, application.PermissionProductWrite,
	)
	createPayload, _ := json.Marshal(map[string]any{
		"name": "Booking Product", "code": "BOOKING", "goal": "Make booking operations executable", "industry": "Fitness and wellness",
		"story":      map[string]any{"title": "Booking Product", "summary": "Unified booking", "narrative": "A member books a session, and the location confirms and fulfills it."},
		"definition": map[string]any{"schema_version": 2, "actors": []any{}, "scenarios": []any{}, "objects": []any{}, "rules": []any{}, "exceptions": []any{}, "actions": []any{}, "pages": []any{}, "access": map[string]any{}, "integrations": []any{}, "automations": []any{}, "configuration": []any{}, "quality_constraints": []any{}},
		"decisions":  []any{},
	})
	create := delivery.Command{
		ClientID: "create-once", ExpectedRevision: 0, Actor: delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		Type: "product.create", Payload: createPayload,
	}
	first, err := service.DispatchProduct(ctx, "workspace-1", "product-1", create)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.DispatchProduct(ctx, "workspace-1", "product-1", create)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || second.Revision != 1 {
		t.Fatalf("idempotent product create changed revision: first=%d second=%d", first.Revision, second.Revision)
	}
	products, err := service.ListProducts(ctx, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].ID != "product-1" {
		t.Fatalf("created product is not listed: %#v", products)
	}
	duplicateCode := create
	duplicateCode.ClientID = "create-duplicate-code"
	_, err = service.DispatchProduct(ctx, "workspace-1", "product-2", duplicateCode)
	domainError, ok := err.(*delivery.Error)
	if !ok || domainError.Code != "product_code_duplicate" {
		t.Fatalf("expected duplicate product code error, got %#v", err)
	}

	stale := create
	stale.ClientID = "stale-product-command"
	stale.Type = "feature.confirm"
	stale.Payload = json.RawMessage(`{"feature_id":"missing","feature_revision":1}`)
	_, err = service.DispatchProduct(ctx, "workspace-1", "product-1", stale)
	domainError, ok = err.(*delivery.Error)
	if !ok || domainError.Code != "revision_conflict" {
		t.Fatalf("expected product revision conflict, got %#v", err)
	}

	deleted, err := service.DispatchProduct(ctx, "workspace-1", "product-1", delivery.Command{
		ClientID: "delete-product", ExpectedRevision: 1, Type: "product.delete", Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Status != productdomain.ProductArchived {
		t.Fatalf("expected archived Product, got %q", deleted.Status)
	}
	products, err = service.ListProducts(ctx, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 0 {
		t.Fatalf("deleted Product is still visible: %#v", products)
	}
	_, err = service.GetProduct(ctx, "workspace-1", "product-1")
	domainError, ok = err.(*delivery.Error)
	if !ok || domainError.Code != "not_found" {
		t.Fatalf("expected deleted Product to be hidden, got %#v", err)
	}
	retained, err := store.GetProduct(ctx, "workspace-1", "product-1")
	if err != nil {
		t.Fatal(err)
	}
	if retained.Status != productdomain.ProductArchived || retained.ID != "product-1" {
		t.Fatalf("logical deletion did not retain the Product: %#v", retained)
	}
}

func TestFoundationCommandsRequireSystemDeploymentPermissionAndPersistEvidence(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	service := application.NewService(application.Ports{Products: store, Runs: store, Lifecycle: store})
	createPayload, _ := json.Marshal(map[string]any{
		"name": "Foundation Product", "code": "FOUNDATION", "goal": "Verify initialization", "industry": "Software",
		"story":      map[string]any{"title": "Foundation Product", "summary": "Verified initialization", "narrative": "A product repository is initialized and verified before Feature delivery."},
		"definition": map[string]any{"schema_version": 2, "actors": []any{}, "scenarios": []any{}, "objects": []any{}, "rules": []any{}, "exceptions": []any{}, "actions": []any{}, "pages": []any{}, "access": map[string]any{}, "integrations": []any{}, "automations": []any{}, "configuration": []any{}, "quality_constraints": []any{}},
		"decisions":  []any{},
	})
	humanContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		application.PermissionProductRead, application.PermissionProductWrite,
	)
	product, err := service.DispatchProduct(humanContext, "workspace-1", "product-foundation", delivery.Command{
		ClientID: "create-foundation", ExpectedRevision: 0, Type: "product.create", Payload: createPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	agentContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "rd-agent", Kind: delivery.ActorAgent},
		application.PermissionProductWrite,
	)
	product, err = service.DispatchProduct(agentContext, "workspace-1", product.ID, delivery.Command{
		ClientID: "frontend-start", ExpectedRevision: product.Revision, Type: "product.engineering.frontend.start", Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	frontendPayload, _ := json.Marshal(map[string]any{
		"code_revision": "git:frontend", "artifact_ref": "artifact://frontend", "design_contract_ref": "evidence://design",
		"login_entry": "frontend/login.tsx", "shell_entry": "frontend/shell.tsx", "preview_entry": "frontend/dist/index.html",
	})
	product, err = service.DispatchProduct(agentContext, "workspace-1", product.ID, delivery.Command{
		ClientID: "frontend-complete", ExpectedRevision: product.Revision, Type: "product.engineering.frontend.complete", Payload: frontendPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	startPayload, _ := json.Marshal(map[string]any{
		"application_delivery_sha256": strings.Repeat("a", 64), "idempotency_key": strings.Repeat("b", 64),
	})
	_, err = service.DispatchProduct(humanContext, "workspace-1", product.ID, delivery.Command{
		ClientID: "foundation-start-denied", ExpectedRevision: product.Revision, Type: "product.engineering.foundation.started", Payload: startPayload,
	})
	domainError, ok := err.(*delivery.Error)
	if !ok || domainError.Code != "permission_denied" {
		t.Fatalf("expected deployment permission denial, got %#v", err)
	}
	product, err = service.DispatchProduct(humanContext, "workspace-1", product.ID, delivery.Command{
		ClientID: "frontend-approve", ExpectedRevision: product.Revision, Type: "product.engineering.frontend.approve", Payload: json.RawMessage(`{"code_revision":"git:frontend"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	systemContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "foundation-installer", Kind: delivery.ActorAgent},
		application.PermissionProductRead, application.PermissionDeploymentRecord,
	)
	product, err = service.DispatchProduct(systemContext, "workspace-1", product.ID, delivery.Command{
		ClientID: "foundation-start", ExpectedRevision: product.Revision, Type: "product.engineering.foundation.started", Payload: startPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	completePayload, _ := json.Marshal(map[string]any{
		"application_delivery_sha256": strings.Repeat("a", 64), "foundation_release_sha256": strings.Repeat("c", 64),
		"foundation_package_sha256": strings.Repeat("d", 64), "model_sha256": strings.Repeat("e", 64),
		"idempotency_key": strings.Repeat("b", 64), "code_revision": strings.Repeat("1", 40), "git_status": "clean",
		"verification_sha256": strings.Repeat("2", 64), "identity_baseline_result": "passed",
	})
	product, err = service.DispatchProduct(systemContext, "workspace-1", product.ID, delivery.Command{
		ClientID: "foundation-complete", ExpectedRevision: product.Revision, Type: "product.engineering.foundation.completed", Payload: completePayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := store.GetProduct(context.Background(), "workspace-1", product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Engineering.Status != productdomain.EngineeringReady || persisted.Engineering.FoundationCompletedBy != "foundation-installer" || persisted.Engineering.FoundationPackageSHA256 != strings.Repeat("d", 64) {
		t.Fatalf("foundation evidence was not persisted: %#v", persisted.Engineering)
	}
}

func TestDeliveryStartAndSuccessfulInstallAreAtomic(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	service := application.NewService(application.Ports{Products: store, Runs: store, Lifecycle: store})
	ctx := context.Background()
	product := testfixture.DemoProduct(time.Now().UTC())
	seedProduct(t, store, product)

	startPayload, _ := json.Marshal(map[string]any{
		"feature_id": "F-001", "feature_revision": 1,
		"name": "Booking Cancellation Delivery", "code": "RUN-ATOMIC-1",
		"goal": "Deliver a verified cancellation capability", "target_date": "2026-10-15",
		"members": []map[string]any{
			{"id": "m-product", "name": "Product Owner", "roles": []string{deliveryrun.RoleProductOwner}, "initials": "PO"},
			{"id": "m-dev", "name": "Development Lead", "roles": []string{deliveryrun.RoleDevelopmentLead}, "initials": "DL"},
			{"id": "m-qa", "name": "Quality Lead", "roles": []string{deliveryrun.RoleQualityLead}, "initials": "QL"},
			{"id": "m-business", "name": "Business Acceptor", "roles": []string{deliveryrun.RoleBusinessAcceptor}, "initials": "BA"},
			{"id": "m-release", "name": "Release Approver", "roles": []string{deliveryrun.RoleReleaseApprover}, "initials": "RA"},
		},
	})
	startCommand := delivery.Command{
		ClientID: "start-delivery-once", ExpectedRevision: product.Revision,
		Actor: delivery.Actor{ID: "rd-agent", Kind: delivery.ActorAgent},
		Type:  "feature.delivery.start", Payload: startPayload,
	}
	startContext := application.WithTrustedPrincipal(ctx, product.WorkspaceID, startCommand.Actor, application.PermissionProductWrite)
	updatedProduct, run, err := service.StartDelivery(startContext, product.WorkspaceID, product.ID, "run-atomic-1", startCommand)
	if err != nil {
		t.Fatal(err)
	}
	if updatedProduct.Revision != 2 || updatedProduct.Features[0].Status != productdomain.FeatureDelivering || run.Feature.Source.FeatureRevision != 1 {
		t.Fatalf("delivery start did not atomically freeze and mark the feature: product=%#v run=%#v", updatedProduct, run)
	}
	retriedProduct, retriedRun, err := service.StartDelivery(startContext, product.WorkspaceID, product.ID, "run-atomic-1", startCommand)
	if err != nil {
		t.Fatal(err)
	}
	if retriedProduct.Revision != 2 || retriedRun.Revision != 1 {
		t.Fatalf("delivery start retry was not idempotent: product=%d run=%d", retriedProduct.Revision, retriedRun.Revision)
	}

	phaseCommands := []struct {
		client  string
		actor   delivery.Actor
		command string
		phase   string
	}{
		{"interaction", delivery.Actor{ID: "frontend-agent", Kind: delivery.ActorAgent}, "delivery_unit.interaction.complete", "interaction_modeling"},
		{"model", delivery.Actor{ID: "backend-agent", Kind: delivery.ActorAgent}, "delivery_unit.model.complete", "domain_modeling"},
		{"model-verify", delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.model.verify", "model_verification"},
		{"backend", delivery.Actor{ID: "backend-agent", Kind: delivery.ActorAgent}, "delivery_unit.backend.complete", "backend_implementation"},
		{"frontend", delivery.Actor{ID: "frontend-agent", Kind: delivery.ActorAgent}, "delivery_unit.frontend.complete", "frontend_convergence"},
		{"contract-verify", delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.contract.verify", "contract_verification"},
		{"journey", delivery.Actor{ID: "journey-runner", Kind: delivery.ActorSystem}, "delivery_unit.journey.complete", "journey_testing"},
	}
	for _, phase := range phaseCommands {
		payload := map[string]any{
			"delivery_unit_id": run.Feature.ID, "phase": phase.phase,
			"git_revision": verifiedGitRevision, "summary": "The trusted delivery phase completed.",
		}
		if evidence := storeBackendGuideEvidence(run, phase.actor, phase.command, verifiedGitRevision); evidence != nil {
			payload["backend_guide"] = evidence
		}
		run = dispatchRunCommand(t, ctx, service, run, phase.client, phase.actor, phase.command, payload)
	}
	baseRevision := product.Revisions[0]
	run = dispatchRunCommand(t, ctx, service, run, "product-revision", delivery.Actor{ID: "rd-agent", Kind: delivery.ActorAgent}, "product_revision.record", map[string]any{
		"content":       map[string]any{"story": baseRevision.Story, "definition": baseRevision.Definition, "decisions": baseRevision.Decisions},
		"evidence_ref":  "git:" + verifiedGitRevision + "#evidence:evidence/product-revision.json",
		"code_revision": verifiedGitRevision, "model_sha256": strings.Repeat("d", 64),
	})
	for _, testCase := range run.TestCases {
		run = dispatchRunCommand(t, ctx, service, run, "quality-"+testCase.ID, delivery.Actor{ID: "qa-agent", Kind: delivery.ActorAgent}, "quality.record", map[string]any{
			"test_case_id": testCase.ID, "git_revision": verifiedGitRevision,
			"result": "pass", "note": "Observed result matches the acceptance criterion.",
			"evidence_refs": []string{"git:" + verifiedGitRevision + "#evidence:evidence/quality/" + testCase.ID + ".md"},
		})
	}
	for _, acceptanceCase := range run.AcceptanceCases {
		run = dispatchRunCommand(t, ctx, service, run, "acceptance-"+acceptanceCase.ID, delivery.Actor{ID: "m-business", Kind: delivery.ActorHuman}, "acceptance.confirm", map[string]any{
			"acceptance_case_id": acceptanceCase.ID, "git_revision": verifiedGitRevision,
			"result": "pass", "note": "Business outcome is accepted.",
			"evidence_refs": []string{"git:" + verifiedGitRevision + "#evidence:evidence/acceptance/" + acceptanceCase.ID + ".md"},
		})
	}
	run = dispatchRunCommand(t, ctx, service, run, "release-checks", delivery.Actor{ID: "op-agent", Kind: delivery.ActorAgent}, "release_checks.replace", map[string]any{
		"environment_ref": "env://production/greenfit",
		"checks":          []map[string]any{{"id": "RC-01", "title": "Production configuration and rollback path verified", "required": true}},
	})
	run = dispatchRunCommand(t, ctx, service, run, "release-check-result", delivery.Actor{ID: "op-agent", Kind: delivery.ActorAgent}, "release_check.record", map[string]any{
		"check_id": "RC-01", "status": "passed", "note": "Configuration and rollback evidence verified.",
		"evidence_refs": []string{"git:" + verifiedGitRevision + "#evidence:evidence/release/RC-01.md"},
	})
	run = dispatchRunCommand(t, ctx, service, run, "release-prepare", delivery.Actor{ID: "m-product", Kind: delivery.ActorHuman}, "release.prepare", map[string]any{
		"version": "1.1.0", "environment_ref": "env://production/greenfit",
	})
	run = dispatchRunCommand(t, ctx, service, run, "release-approve", delivery.Actor{ID: "m-release", Kind: delivery.ActorHuman}, "release.approve", map[string]any{
		"release_id": "REL-001",
	})
	deploymentCommand := delivery.Command{
		ClientID: "run-command-deploy-success", ExpectedRevision: run.Revision,
		Actor: delivery.Actor{ID: "deployment-adapter", Kind: delivery.ActorSystem}, Type: "release.deploy_result",
		Payload: json.RawMessage(`{"release_id":"REL-001","outcome":"success","environment_ref":"env://production/greenfit","launch_url":"https://greenfit.example.test","receipt_ref":"deployment://greenfit/1.1.0"}`),
	}
	deploymentContext := application.WithTrustedPrincipal(ctx, run.WorkspaceID, deploymentCommand.Actor, application.PermissionDeploymentRecord)
	run, err = service.Dispatch(deploymentContext, run.WorkspaceID, run.ID, deploymentCommand)
	if err != nil {
		t.Fatal(err)
	}
	if run.Stage != deliveryrun.StageLive || run.Releases[0].CodeRevision != verifiedGitRevision || run.Releases[0].DeploymentAttempt.ReceiptRef != "deployment://greenfit/1.1.0" {
		t.Fatalf("successful deployment lost release evidence: %#v", run.Releases[0])
	}
	readContext := application.WithTrustedPrincipal(ctx, product.WorkspaceID, delivery.Actor{ID: "reader", Kind: delivery.ActorHuman}, application.PermissionProductRead)
	installedProduct, err := service.GetProduct(readContext, product.WorkspaceID, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if installedProduct.Revision != 3 || installedProduct.CurrentDefinitionRevision != product.CurrentDefinitionRevision+1 || installedProduct.CurrentReleaseRevision != product.CurrentReleaseRevision+1 || installedProduct.Features[0].Status != productdomain.FeatureInstalled || installedProduct.Features[0].InstalledReleaseID != "REL-001" || installedProduct.CurrentDeployment == nil || installedProduct.CurrentDeployment.LaunchURL != "https://greenfit.example.test" {
		t.Fatalf("live delivery was not atomically installed: %#v", installedProduct)
	}
	if len(installedProduct.Revisions) != len(product.Revisions)+1 {
		t.Fatalf("installation did not append exactly one executable ProductRevision: %#v", installedProduct.Revisions)
	}
	if _, err := service.Dispatch(deploymentContext, run.WorkspaceID, run.ID, deploymentCommand); err != nil {
		t.Fatal(err)
	}
	afterRetry, err := service.GetProduct(readContext, product.WorkspaceID, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRetry.Revision != installedProduct.Revision {
		t.Fatalf("deployment retry installed the feature twice: before=%d after=%d", installedProduct.Revision, afterRetry.Revision)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(t.Context(), db, "sqlite", true)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return store
}

func seedProduct(t *testing.T, store *Store, product productdomain.Product) {
	t.Helper()
	transaction, err := store.db.BeginTx(t.Context(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = transaction.Rollback() }()
	if err := store.insertProductState(t.Context(), transaction, product); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func seedRun(t *testing.T, store *Store, run deliveryrun.DeliveryRun) {
	t.Helper()
	transaction, err := store.db.BeginTx(t.Context(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = transaction.Rollback() }()
	if err := store.insertRunState(t.Context(), transaction, run); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func dispatchRunCommand(t *testing.T, ctx context.Context, service *application.Service, run deliveryrun.DeliveryRun, clientSuffix string, actor delivery.Actor, commandType string, payload any) deliveryrun.DeliveryRun {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	permission := application.PermissionDeliveryRunWrite
	if actor.Kind == delivery.ActorSystem {
		permission = application.PermissionDeploymentRecord
	}
	ctx = application.WithTrustedPrincipal(ctx, run.WorkspaceID, actor, permission)
	updated, err := service.Dispatch(ctx, run.WorkspaceID, run.ID, delivery.Command{
		ClientID: "run-command-" + clientSuffix, ExpectedRevision: run.Revision,
		Actor: actor, Type: commandType, Payload: raw,
	})
	if err != nil {
		t.Fatalf("%s failed: %v", commandType, err)
	}
	return updated
}
