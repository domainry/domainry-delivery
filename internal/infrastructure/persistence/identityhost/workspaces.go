package identityhost

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	identity "github.com/domainry/domainry-identity-sdk"
	identitymodulehost "github.com/domainry/domainry-identity-sdk/modulehost"
	ormquery "github.com/domainry/domainry-orm/query"
	"github.com/domainry/domainry-orm/schema"
)

const workspaceTable = "delivery_workspaces"

type Workspaces struct {
	db         *sql.DB
	renderer   ormquery.Renderer
	migrations identitymodulehost.MigrationRegistrar
}

func NewDatabaseHandle(ctx context.Context, database *sql.DB, driver string, renderer ormquery.Renderer, migrations identitymodulehost.MigrationRegistrar) (identity.DatabaseHandle, error) {
	if database == nil || renderer == nil || migrations == nil || driver == "" {
		return identity.DatabaseHandle{}, fmt.Errorf("external identity requires the standalone Delivery database host")
	}
	workspaces := &Workspaces{db: database, renderer: renderer, migrations: migrations}
	if err := workspaces.prepare(ctx); err != nil {
		return identity.DatabaseHandle{}, err
	}
	return identity.DatabaseHandle{
		Pool: database, Driver: driver, ModuleMigrations: migrations, ExternalWorkspaces: workspaces,
	}, nil
}

func (host *Workspaces) prepare(ctx context.Context) error {
	statement, arguments, err := schema.NewTable(host.renderer, workspaceTable).Columns(
		schema.Column("id", schema.TextKey(191)).NotNull(),
		schema.Column("owner_user_id", schema.TextKey(191)).NotNull(),
		schema.Column("name", schema.Text()).NotNull(),
		schema.Column("active", schema.Boolean()).NotNull(),
	).PrimaryKey("id").Build()
	if err != nil {
		return err
	}
	if len(arguments) != 0 {
		return fmt.Errorf("Delivery Workspace schema must not contain bound DDL values")
	}
	return host.migrations.ApplyOwnedMigrations(ctx, "delivery_host", []identitymodulehost.SchemaMigration{{
		Version: 1, Name: "personal_workspaces", Statements: []string{statement},
	}})
}

func (host *Workspaces) RunExternalWorkspaceTransaction(ctx context.Context, apply func(context.Context, identity.EmbeddedTransaction) error) error {
	if apply == nil {
		return fmt.Errorf("Workspace transaction callback is required")
	}
	transaction, err := host.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	if err := apply(ctx, identity.EmbeddedTransaction{Executor: transaction}); err != nil {
		return err
	}
	return transaction.Commit()
}

func (host *Workspaces) CreateExternalWorkspace(ctx context.Context, request identity.ExternalWorkspaceCreate, transaction identity.EmbeddedTransaction) error {
	if request.WorkspaceID == "" || request.UserID == "" || transaction.Executor == nil {
		return fmt.Errorf("verified Workspace owner and transaction are required")
	}
	statement, arguments, err := ormquery.NewInsertBuilder(host.renderer, workspaceTable).
		Columns("id", "owner_user_id", "name", "active").
		Values(request.WorkspaceID, request.UserID, request.Name, true).Build()
	if err != nil {
		return err
	}
	_, err = transaction.Executor.ExecContext(ctx, statement, arguments...)
	return err
}

func (*Workspaces) InitializeExternalWorkspaceApplication(_ context.Context, request identity.ExternalWorkspaceCreate, _ identity.EmbeddedTransaction) error {
	if len(request.ApplicationBootstrap) != 0 {
		return fmt.Errorf("Delivery does not support external Workspace bootstrap data")
	}
	return nil
}

func (host *Workspaces) ExternalWorkspaceActive(ctx context.Context, workspaceID string) (bool, error) {
	statement, arguments, err := ormquery.NewSelectBuilder(host.renderer, workspaceTable).
		Columns("active").Where(ormquery.Equal("id", workspaceID)).Build()
	if err != nil {
		return false, err
	}
	var active bool
	err = host.db.QueryRowContext(ctx, statement, arguments...).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return active, err
}

var _ identity.ExternalWorkspaceHost = (*Workspaces)(nil)
