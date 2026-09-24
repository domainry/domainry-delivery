package database

import (
	"context"
	"fmt"

	sharedoperation "github.com/domainry/domainry-foundation/operation"
	ormdialect "github.com/domainry/domainry-orm/dialect"
	"github.com/domainry/domainry-orm/sqlhost"
)

type Store struct {
	db         sqlhost.Database
	dialect    string
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
	renderer, err := ormdialect.ParseRenderer(dialect, "", "")
	if err != nil {
		return nil, err
	}
	registrar := directMigrationRegistrar{database: db, renderer: renderer}
	if err := registrar.ApplyOwnedMigrations(ctx, "delivery", Migrations(dialect)); err != nil {
		return nil, err
	}
	operations, err := sharedoperation.Open(ctx, db, sharedoperation.AdaptDialect(renderer), registrar)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, dialect: dialect, operations: operations}
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

func NewBorrowed(ctx context.Context, db sqlhost.Database, dialect string, renderer sharedoperation.Renderer, migrations sharedoperation.MigrationRegistrar) (*Store, error) {
	if ctx == nil || db == nil || renderer == nil || migrations == nil || (dialect != "sqlite" && dialect != "mysql") {
		return nil, fmt.Errorf("Delivery borrowed database and supported dialect are required")
	}
	if err := migrations.ApplyOwnedMigrations(ctx, "delivery", Migrations(dialect)); err != nil {
		return nil, err
	}
	operations, err := sharedoperation.Open(ctx, db, sharedoperation.AdaptDialect(renderer), migrations)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, dialect: dialect, operations: operations}, nil
}
