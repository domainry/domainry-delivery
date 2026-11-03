package saas

import (
	"context"
	"fmt"
	"os"

	bridgemodule "github.com/domainry/domainry-identity-bridge/module"
	bridgeconfig "github.com/domainry/domainry-identity-bridge/module/config"
	identity "github.com/domainry/domainry-identity-sdk"

	"github.com/domainry/domainry-delivery/internal/application"
)

const deliveryPermissionOwner = "delivery:application"

type ExternalIdentity struct {
	Binding identity.Binding
	Config  bridgeconfig.Config
}

func (applicationRuntime *Application) OpenExternalIdentity(ctx context.Context, configFile string) (ExternalIdentity, error) {
	return applicationRuntime.openExternalIdentity(ctx, configFile, bridgemodule.Options{})
}

func (applicationRuntime *Application) openExternalIdentity(ctx context.Context, configFile string, options bridgemodule.Options) (ExternalIdentity, error) {
	if applicationRuntime == nil || applicationRuntime.store == nil {
		return ExternalIdentity{}, fmt.Errorf("Delivery application database is not open")
	}
	file, err := os.Open(configFile)
	if err != nil {
		return ExternalIdentity{}, fmt.Errorf("open external identity configuration: %w", err)
	}
	config, err := bridgeconfig.Load(file)
	closeErr := file.Close()
	if err != nil {
		return ExternalIdentity{}, err
	}
	if closeErr != nil {
		return ExternalIdentity{}, closeErr
	}
	if len(config.Workspace.InitialRoleKeys) != 1 {
		return ExternalIdentity{}, fmt.Errorf("Delivery external identity requires exactly one initial personal role")
	}
	handle, err := applicationRuntime.store.ExternalIdentityDatabaseHandle(ctx)
	if err != nil {
		return ExternalIdentity{}, err
	}
	reference := identity.ApplicationRef{
		WorkspaceID: identity.WorkspaceID(config.InstallationID), ApplicationKey: identity.ApplicationKey(config.ApplicationKey),
	}
	binding, err := bridgemodule.NewFactory(configFile, options).OpenExternalWithDatabase(ctx, reference, handle)
	if err != nil {
		return ExternalIdentity{}, err
	}
	failed := true
	defer func() {
		if failed {
			_ = binding.Close(context.Background())
		}
	}()
	if _, err = binding.Applications().Register(ctx, identity.ApplicationRegistration{Application: reference}); err != nil {
		return ExternalIdentity{}, fmt.Errorf("register Delivery identity application: %w", err)
	}
	reader, ok := binding.Permissions().(identity.PermissionSnapshotReader)
	if !ok {
		return ExternalIdentity{}, fmt.Errorf("external Identity permission snapshot reader is required")
	}
	snapshotRequest := identity.PermissionSourceSnapshotRequest{SourceOwner: deliveryPermissionOwner}
	previous, err := reader.CurrentSourceSnapshot(ctx, snapshotRequest)
	if err != nil {
		return ExternalIdentity{}, err
	}
	if err = previous.ValidateFor(snapshotRequest); err != nil {
		return ExternalIdentity{}, fmt.Errorf("validate Delivery permission snapshot: %w", err)
	}
	definitions := deliveryPermissionDefinitions()
	reconcile, err := identity.NewPermissionReconcileRequest(deliveryPermissionOwner, previous.SnapshotHash, definitions)
	if err != nil {
		return ExternalIdentity{}, err
	}
	receipt, err := binding.Permissions().Reconcile(ctx, reconcile)
	if err != nil {
		return ExternalIdentity{}, fmt.Errorf("publish Delivery permissions: %w", err)
	}
	if err = receipt.ValidateFor(reconcile); err != nil {
		return ExternalIdentity{}, fmt.Errorf("validate Delivery permission receipt: %w", err)
	}
	publisher, ok := binding.(identity.ProjectRoleCatalogPublisher)
	if !ok {
		return ExternalIdentity{}, fmt.Errorf("external Identity role catalog publisher is required")
	}
	grants := make([]identity.ProjectRolePermission, 0, len(definitions))
	for _, definition := range definitions {
		grants = append(grants, identity.ProjectRolePermission{PermissionKey: definition.PermissionKey, DataScope: identity.DataScopeAll})
	}
	if _, err = publisher.PublishProjectRoles(ctx, identity.ProjectRoleCatalog{SourceOwner: deliveryPermissionOwner, Roles: []identity.ProjectRoleDefinition{{
		Key: config.Workspace.InitialRoleKeys[0], Name: "Delivery owner", Audience: "user", AssignmentMode: "manual",
		Permissions: grants,
	}}}); err != nil {
		return ExternalIdentity{}, fmt.Errorf("publish Delivery external role: %w", err)
	}
	failed = false
	return ExternalIdentity{Binding: binding, Config: config}, nil
}

func deliveryPermissionDefinitions() []identity.PermissionDefinition {
	return []identity.PermissionDefinition{
		deliveryPermissionDefinition(application.PermissionProductRead, "delivery_product", "read", "Products", "Read", "Read products"),
		deliveryPermissionDefinition(application.PermissionProductWrite, "delivery_product", "write", "Products", "Write", "Write products"),
		deliveryPermissionDefinition(application.PermissionDeliveryRunRead, "delivery_run", "read", "Delivery runs", "Read", "Read delivery runs"),
		deliveryPermissionDefinition(application.PermissionDeliveryRunWrite, "delivery_run", "write", "Delivery runs", "Write", "Write delivery runs"),
		deliveryPermissionDefinition(application.PermissionDeploymentRecord, "delivery_deployment", "record", "Deployments", "Record", "Record deployments"),
	}
}

func deliveryPermissionDefinition(permissionKey, resourceKey, operationKey, resourceLabel, operationLabel, label string) identity.PermissionDefinition {
	return identity.PermissionDefinition{
		PermissionKey: permissionKey, ModuleKey: "delivery", GroupKey: "delivery", ResourceKey: resourceKey, OperationKey: operationKey,
		Label: label, Category: "Product delivery", SourceKind: "application_action", GrantMode: "direct", ScopeKind: "none", DefaultLocale: "en",
		I18n: identity.PermissionLocalizedText{"en": {
			ModuleLabel: "Delivery", GroupLabel: "Product delivery", ResourceLabel: resourceLabel, OperationLabel: operationLabel, Label: label,
		}},
	}
}
