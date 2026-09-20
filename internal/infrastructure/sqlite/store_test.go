package sqlite_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain/delivery"
	"github.com/domainry/domainry-delivery/internal/infrastructure/sqlite"
)

const verifiedGitRevision = "0123456789abcdef0123456789abcdef01234567"

func TestCommandReceiptIsIdempotentAndRevisionIsOptimistic(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := application.NewService(store)
	ctx := context.Background()
	if err := service.Ensure(ctx, application.DemoDeliveryRun(time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	command := delivery.Command{
		ClientID:         "client-once",
		ExpectedRevision: 1,
		Actor:            delivery.Actor{ID: "rd-agent", Kind: delivery.ActorAgent},
		Type:             "work.plan.replace",
		Payload:          json.RawMessage(`{"note":"Initial implementation plan","items":[]}`),
	}
	dispatchContext := application.WithTrustedPrincipal(ctx, application.DemoWorkspaceID, command.Actor, application.PermissionDeliveryRunWrite)
	first, err := service.Dispatch(dispatchContext, application.DemoWorkspaceID, application.DemoDeliveryRunID, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Dispatch(dispatchContext, application.DemoWorkspaceID, application.DemoDeliveryRunID, command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 2 || second.Revision != 2 || second.WorkPlanRevision != 1 {
		t.Fatalf("idempotent retry changed state: first=%d second=%d", first.Revision, second.Revision)
	}
	stale := command
	stale.ClientID = "client-stale"
	stale.Type = "build.create"
	stale.Payload = json.RawMessage([]byte("{\"code_revision\":\"git:x\",\"artifact_ref\":\"oci://x\"}"))
	_, err = service.Dispatch(dispatchContext, application.DemoWorkspaceID, application.DemoDeliveryRunID, stale)
	domainError, ok := err.(*delivery.Error)
	if !ok || domainError.Code != "revision_conflict" {
		t.Fatalf("expected revision conflict, got %#v", err)
	}
}

func TestProductCommandsAreIdempotentAndRevisionFenced(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := application.NewService(store)
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
	if deleted.Status != delivery.ProductArchived {
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
	if retained.Status != delivery.ProductArchived || retained.ID != "product-1" {
		t.Fatalf("logical deletion did not retain the Product: %#v", retained)
	}
}

func TestDeliveryStartAndSuccessfulInstallAreAtomic(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := application.NewService(store)
	ctx := context.Background()
	product := application.DemoProduct(time.Now().UTC())
	if err := service.EnsureProduct(ctx, product); err != nil {
		t.Fatal(err)
	}

	startPayload, _ := json.Marshal(map[string]any{
		"feature_id": "F-001", "feature_revision": 1,
		"name": "Booking Cancellation Delivery", "code": "RUN-ATOMIC-1",
		"goal": "Deliver a verified cancellation capability", "target_date": "2026-10-15",
		"members": []map[string]any{
			{"id": "m-product", "name": "Product Owner", "roles": []string{delivery.RoleProductOwner}, "initials": "PO"},
			{"id": "m-dev", "name": "Development Lead", "roles": []string{delivery.RoleDevelopmentLead}, "initials": "DL"},
			{"id": "m-qa", "name": "Quality Lead", "roles": []string{delivery.RoleQualityLead}, "initials": "QL"},
			{"id": "m-business", "name": "Business Acceptor", "roles": []string{delivery.RoleBusinessAcceptor}, "initials": "BA"},
			{"id": "m-release", "name": "Release Approver", "roles": []string{delivery.RoleReleaseApprover}, "initials": "RA"},
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
	if updatedProduct.Revision != 2 || updatedProduct.Features[0].Status != delivery.FeatureDelivering || run.Feature.Source.FeatureRevision != 1 {
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
		{"compile", delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.compile.complete", "compiling"},
		{"frontend", delivery.Actor{ID: "frontend-agent", Kind: delivery.ActorAgent}, "delivery_unit.frontend.complete", "frontend_convergence"},
		{"freeze", delivery.Actor{ID: "runner", Kind: delivery.ActorSystem}, "delivery_unit.contract.freeze", "contract_frozen"},
		{"backend", delivery.Actor{ID: "backend-agent", Kind: delivery.ActorAgent}, "delivery_unit.backend.complete", "backend_implementation"},
		{"journey", delivery.Actor{ID: "journey-runner", Kind: delivery.ActorSystem}, "delivery_unit.journey.complete", "journey_testing"},
	}
	for _, phase := range phaseCommands {
		run = dispatchRunCommand(t, ctx, service, run, phase.client, phase.actor, phase.command, map[string]any{
			"delivery_unit_id": run.Feature.ID, "phase": phase.phase,
			"git_revision": verifiedGitRevision, "summary": "The trusted V3 phase completed.", "diagnostics": []any{},
		})
	}
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
	if run.Stage != delivery.StageLive || run.Releases[0].CodeRevision != verifiedGitRevision || run.Releases[0].ArtifactRef != "" || run.Releases[0].DeploymentAttempt.ReceiptRef != "deployment://greenfit/1.1.0" {
		t.Fatalf("successful deployment lost release evidence: %#v", run.Releases[0])
	}
	readContext := application.WithTrustedPrincipal(ctx, product.WorkspaceID, delivery.Actor{ID: "reader", Kind: delivery.ActorHuman}, application.PermissionProductRead)
	installedProduct, err := service.GetProduct(readContext, product.WorkspaceID, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if installedProduct.Revision != 3 || installedProduct.CurrentDefinitionRevision != product.CurrentDefinitionRevision || installedProduct.CurrentReleaseRevision != product.CurrentReleaseRevision || installedProduct.Features[0].Status != delivery.FeatureInstalled || installedProduct.Features[0].InstalledReleaseID != "REL-001" || installedProduct.CurrentDeployment == nil || installedProduct.CurrentDeployment.LaunchURL != "https://greenfit.example.test" {
		t.Fatalf("live delivery was not atomically installed: %#v", installedProduct)
	}
	if len(installedProduct.Revisions) != len(product.Revisions) {
		t.Fatalf("V3 installation invented an executable ProductRevision: %#v", installedProduct.Revisions)
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

func dispatchRunCommand(t *testing.T, ctx context.Context, service *application.Service, run delivery.DeliveryRun, clientSuffix string, actor delivery.Actor, commandType string, payload any) delivery.DeliveryRun {
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
