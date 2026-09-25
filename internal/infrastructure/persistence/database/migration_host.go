package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	ormmigration "github.com/domainry/domainry-orm/migration"
	"github.com/domainry/domainry-orm/query"
	"github.com/domainry/domainry-orm/schema"
	"github.com/domainry/domainry-orm/sqlhost"
)

const migrationLedgerTable = "_schema_migrations"

// directMigrationRegistrar is the migration owner for the standalone SaaS
// database. Embedded Module assembly delegates this responsibility to its
// host. The ledger key includes the source owner because Delivery and shared
// Operations both start their immutable migration sequence at version 1.
type directMigrationRegistrar struct {
	database sqlhost.Database
	renderer query.Renderer
	driver   string
}

func (registrar directMigrationRegistrar) Driver() string { return registrar.driver }

func (directMigrationRegistrar) Schema() string { return "" }

func (registrar directMigrationRegistrar) ApplyOwnedMigrations(ctx context.Context, owner string, migrations []ormmigration.Migration) error {
	owner = strings.TrimSpace(owner)
	if registrar.database == nil || registrar.renderer == nil || owner == "" {
		return fmt.Errorf("standalone Delivery migration host is incomplete")
	}
	if err := registrar.ensureLedger(ctx); err != nil {
		return err
	}
	for _, migration := range migrations {
		if err := registrar.apply(ctx, owner, migration); err != nil {
			return err
		}
	}
	return nil
}

func (registrar directMigrationRegistrar) ensureLedger(ctx context.Context) error {
	statement, arguments, err := schema.NewTable(registrar.renderer, migrationLedgerTable).IfNotExists().Columns(
		schema.Column("owner", schema.TextKey(191)).NotNull(),
		schema.Column("version", schema.BigInt()).NotNull(),
		schema.Column("name", schema.TextKey(191)).NotNull(),
		schema.Column("checksum", schema.TextKey(64)).NotNull(),
		schema.Column("dirty", schema.Boolean()).NotNull(),
		schema.Column("applied_at", schema.BigInt()).NotNull(),
	).PrimaryKey("owner", "version").Build()
	if err != nil {
		return fmt.Errorf("build standalone Delivery migration ledger: %w", err)
	}
	if _, err := registrar.database.ExecContext(ctx, statement, arguments...); err != nil {
		return fmt.Errorf("prepare standalone Delivery migration ledger: %w", err)
	}
	return nil
}

func (registrar directMigrationRegistrar) apply(ctx context.Context, owner string, migration ormmigration.Migration) error {
	if migration.Version == 0 || strings.TrimSpace(migration.Name) == "" {
		return fmt.Errorf("invalid %s migration", owner)
	}
	checksum := ormmigration.Checksum(migration)
	statement, arguments, err := migrationLedgerQuery(registrar.renderer, owner, migration.Version)
	if err != nil {
		return err
	}
	applied, dirty, found, err := readMigrationLedger(ctx, registrar.database, statement, arguments)
	if err != nil {
		return fmt.Errorf("inspect %s migration %d: %w", owner, migration.Version, err)
	}
	if found {
		return validateAppliedMigration(migration, checksum, applied, dirty)
	}

	insert, insertArguments, err := query.NewInsertBuilder(registrar.renderer, migrationLedgerTable).
		Columns("owner", "version", "name", "checksum", "dirty", "applied_at").
		Values(owner, migration.Version, strings.TrimSpace(migration.Name), checksum, true, int64(0)).Build()
	if err != nil {
		return err
	}
	if _, err := registrar.database.ExecContext(ctx, insert, insertArguments...); err != nil {
		if !isMigrationConflict(err) {
			return fmt.Errorf("record dirty %s migration %d: %w", owner, migration.Version, err)
		}
		return registrar.waitForPeer(ctx, migration, checksum, statement, arguments)
	}

	transaction, err := registrar.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s migration %d: %w", owner, migration.Version, err)
	}
	defer func() { _ = transaction.Rollback() }()
	for _, ddl := range migration.Statements {
		if _, err := transaction.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("apply %s migration %d: %w", owner, migration.Version, err)
		}
	}
	complete, completeArguments, err := query.NewUpdateBuilder(registrar.renderer, migrationLedgerTable).
		Set("dirty", false).
		Set("applied_at", time.Now().UTC().UnixMilli()).
		Where(query.And(
			query.Equal("owner", owner), query.Equal("version", migration.Version),
			query.Equal("checksum", checksum), query.Equal("dirty", true),
		)).Build()
	if err != nil {
		return err
	}
	result, err := transaction.ExecContext(ctx, complete, completeArguments...)
	if err != nil {
		return fmt.Errorf("complete %s migration %d ledger: %w", owner, migration.Version, err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("complete %s migration %d ledger: affected=%d err=%v", owner, migration.Version, affected, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit %s migration %d: %w", owner, migration.Version, err)
	}
	return nil
}

func migrationLedgerQuery(renderer query.Renderer, owner string, version uint) (string, []any, error) {
	return query.NewSelectBuilder(renderer, migrationLedgerTable).
		Columns("checksum", "dirty").
		Where(query.And(query.Equal("owner", owner), query.Equal("version", version))).Build()
}

func readMigrationLedger(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, statement string, arguments []any) (string, bool, bool, error) {
	var checksum string
	var dirty bool
	err := queryer.QueryRowContext(ctx, statement, arguments...).Scan(&checksum, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, false, nil
	}
	return checksum, dirty, err == nil, err
}

func validateAppliedMigration(migration ormmigration.Migration, expected, applied string, dirty bool) error {
	if dirty {
		return &ormmigration.Error{Code: ormmigration.CodeDirty, Version: migration.Version, Name: migration.Name}
	}
	if applied != expected {
		return &ormmigration.Error{Code: ormmigration.CodeChecksumDrift, Version: migration.Version, Name: migration.Name}
	}
	return nil
}

func (registrar directMigrationRegistrar) waitForPeer(ctx context.Context, migration ormmigration.Migration, checksum, statement string, arguments []any) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		applied, dirty, found, err := readMigrationLedger(ctx, registrar.database, statement, arguments)
		if err != nil {
			return err
		}
		if found && applied != checksum {
			return &ormmigration.Error{Code: ormmigration.CodeChecksumDrift, Version: migration.Version, Name: migration.Name}
		}
		if found && !dirty {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return &ormmigration.Error{Code: ormmigration.CodeWaitTimeout, Version: migration.Version, Name: migration.Name}
		case <-ticker.C:
		}
	}
}

func isMigrationConflict(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "unique") || strings.Contains(value, "duplicate") || strings.Contains(value, "constraint")
}
