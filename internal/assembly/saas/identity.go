package saas

import (
	"context"
	"fmt"
	"os"

	bridgeconfig "github.com/domainry/domainry-identity-bridge/config"
	bridgemodule "github.com/domainry/domainry-identity-bridge/module"
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
	if len(config.PersonalWorkspace.InitialRoleKeys) != 1 {
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
	previous, err := reader.CurrentSourceSnapshot(ctx, identity.PermissionSourceSnapshotRequest{
		Application: reference, SourceOwner: deliveryPermissionOwner,
	})
	if err != nil {
		return ExternalIdentity{}, err
	}
	definitions := deliveryPermissionDefinitions()
	reconcile, err := identity.NewPermissionReconcileRequest(reference, deliveryPermissionOwner, previous.SnapshotHash, definitions)
	if err != nil {
		return ExternalIdentity{}, err
	}
	if _, err = binding.Permissions().Reconcile(ctx, reconcile); err != nil {
		return ExternalIdentity{}, fmt.Errorf("publish Delivery permissions: %w", err)
	}
	publisher, ok := binding.(identity.ProjectRoleCatalogPublisher)
	if !ok {
		return ExternalIdentity{}, fmt.Errorf("external Identity role catalog publisher is required")
	}
	grants := make([]identity.ProjectRolePermission, 0, len(definitions))
	for _, definition := range definitions {
		grants = append(grants, identity.ProjectRolePermission{PermissionKey: definition.PermissionKey, DataScope: identity.DataScopeAll})
	}
	if _, err = publisher.PublishProjectRoles(ctx, identity.ProjectRoleCatalog{Application: reference, Roles: []identity.ProjectRoleDefinition{{
		Key: config.PersonalWorkspace.InitialRoleKeys[0], Name: "Delivery owner", Audience: "user", AssignmentMode: "manual",
		ProvisionToWorkspaces: true, Permissions: grants,
	}}}); err != nil {
		return ExternalIdentity{}, fmt.Errorf("publish Delivery external role: %w", err)
	}
	failed = false
	return ExternalIdentity{Binding: binding, Config: config}, nil
}

func deliveryPermissionDefinitions() []identity.PermissionDefinition {
	return []identity.PermissionDefinition{
		{PermissionKey: application.PermissionProductRead, ResourceKey: "delivery_product", OperationKey: "read", Label: "Read products", Category: "delivery", SourceKind: "delivery"},
		{PermissionKey: application.PermissionProductWrite, ResourceKey: "delivery_product", OperationKey: "write", Label: "Write products", Category: "delivery", SourceKind: "delivery"},
		{PermissionKey: application.PermissionDeliveryRunRead, ResourceKey: "delivery_run", OperationKey: "read", Label: "Read delivery runs", Category: "delivery", SourceKind: "delivery"},
		{PermissionKey: application.PermissionDeliveryRunWrite, ResourceKey: "delivery_run", OperationKey: "write", Label: "Write delivery runs", Category: "delivery", SourceKind: "delivery"},
		{PermissionKey: application.PermissionDeploymentRecord, ResourceKey: "delivery_deployment", OperationKey: "record", Label: "Record deployments", Category: "delivery", SourceKind: "delivery"},
	}
}
