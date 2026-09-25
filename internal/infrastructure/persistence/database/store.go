package database

import (
	"context"
	"database/sql"
	"fmt"

	sharedoperation "github.com/domainry/domainry-foundation/operation"
	identity "github.com/domainry/domainry-identity-sdk"
	ormdialect "github.com/domainry/domainry-orm/dialect"
	ormdriver "github.com/domainry/domainry-orm/driver"
	"github.com/domainry/domainry-orm/sqlhost"

	"github.com/domainry/domainry-delivery/internal/infrastructure/persistence/identityhost"
)

type Store struct {
	db         sqlhost.Database
	dialect    string
	renderer   sqlRenderer
	profile    ormdriver.Profile
	registrar  directMigrationRegistrar
	operations sharedoperation.Store
	closer     interface{ Close() error }
}

func New(ctx context.Context, db sqlhost.Database, dialect string, ownsDatabase bool) (*Store, error) {
	if ctx == nil || db == nil {
		return nil, fmt.Errorf("a context and host database are required")
	}
	if dialect != "sqlite" && dialect != "mysql" {
		return nil, fmt.Errorf("unsupported Delivery database dialect %q", dialect)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	baseRenderer, err := ormdialect.ParseRenderer(dialect, "", "")
	if err != nil {
		return nil, err
	}
	renderer, err := deliveryRenderer(dialect, baseRenderer)
	if err != nil {
		return nil, err
	}
	profile, err := deliveryProfile(dialect)
	if err != nil {
		return nil, err
	}
	migrationValues, err := migrationsForRenderer(dialect, renderer)
	if err != nil {
		return nil, err
	}
	registrar := directMigrationRegistrar{database: db, renderer: renderer, driver: dialect}
	if err := registrar.ApplyOwnedMigrations(ctx, "delivery", migrationValues); err != nil {
		return nil, err
	}
	operations, err := sharedoperation.Open(ctx, db, sharedoperation.AdaptDialect(renderer), registrar)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, dialect: dialect, renderer: renderer, profile: profile, registrar: registrar, operations: operations}
	if ownsDatabase {
		store.closer, _ = db.(interface{ Close() error })
	}
	return store, nil
}

func (store *Store) Close() error {
	if store.closer == nil {
		return nil
	}
	return store.closer.Close()
}

func (store *Store) ExternalIdentityDatabaseHandle(ctx context.Context) (identity.DatabaseHandle, error) {
	if store == nil || store.db == nil {
		return identity.DatabaseHandle{}, fmt.Errorf("Delivery database is not open")
	}
	database, ok := store.db.(*sql.DB)
	if !ok || database == nil {
		return identity.DatabaseHandle{}, fmt.Errorf("external identity requires the standalone Delivery database pool")
	}
	return identityhost.NewDatabaseHandle(ctx, database, store.dialect, store.renderer, store.registrar)
}

func NewBorrowed(ctx context.Context, db sqlhost.Database, dialect string, hostRenderer sharedoperation.Dialect, migrations sharedoperation.MigrationRegistrar) (*Store, error) {
	if ctx == nil || db == nil || hostRenderer == nil || migrations == nil || (dialect != "sqlite" && dialect != "mysql") {
		return nil, fmt.Errorf("Delivery borrowed database and supported dialect are required")
	}
	renderer, err := deliveryRenderer(dialect, hostRenderer)
	if err != nil {
		return nil, err
	}
	profile, err := deliveryProfile(dialect)
	if err != nil {
		return nil, err
	}
	migrationValues, err := migrationsForRenderer(dialect, renderer)
	if err != nil {
		return nil, err
	}
	if err := migrations.ApplyOwnedMigrations(ctx, "delivery", migrationValues); err != nil {
		return nil, err
	}
	operations, err := sharedoperation.Open(ctx, db, hostRenderer, migrations)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, dialect: dialect, renderer: renderer, profile: profile, operations: operations}, nil
}
