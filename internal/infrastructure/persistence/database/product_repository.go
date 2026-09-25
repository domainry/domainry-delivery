package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain"
	productdomain "github.com/domainry/domainry-delivery/internal/domain/product"
	ormquery "github.com/domainry/domainry-orm/query"
)

func (store *Store) GetProduct(ctx context.Context, workspaceID, productID string) (productdomain.Product, error) {
	return loadProductState(ctx, store.db, store.renderer, workspaceID, productID)
}

func (store *Store) ListProducts(ctx context.Context, workspaceID string) ([]productdomain.Product, error) {
	statement, arguments, err := ormquery.NewWorkspaceSelectBuilder(store.renderer, TableProducts, workspaceID).
		Columns("product_id").
		OrderBy(ormquery.Descending("updated_at"), ormquery.Ascending("product_id")).
		Build()
	if err != nil {
		return nil, storageError(err)
	}
	rows, err := store.db.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, storageError(err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, storageError(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, storageError(err)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError(err)
	}
	products := make([]productdomain.Product, 0, len(ids))
	for _, id := range ids {
		product, err := loadProductState(ctx, store.db, store.renderer, workspaceID, id)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, nil
}

func (store *Store) CreateProduct(
	ctx context.Context,
	workspaceID string,
	productID string,
	mutation application.Mutation,
	expectedRevision uint64,
	create func() (productdomain.Product, error),
) (productdomain.Product, error) {
	return executeCommand(ctx, store, commandTarget{
		workspaceID: workspaceID, resourceType: "product", resourceID: productID, operationKind: "product_command",
	}, mutation, func(transaction *sql.Tx) (productdomain.Product, error) {
		var actualRevision uint64
		statement, arguments, buildErr := ormquery.NewWorkspaceSelectBuilder(store.renderer, TableProducts, workspaceID).
			Columns("revision").
			Where(ormquery.Equal("product_id", productID)).
			Build()
		if buildErr != nil {
			return productdomain.Product{}, storageError(buildErr)
		}
		err := transaction.QueryRowContext(ctx, statement, arguments...).Scan(&actualRevision)
		if err == nil {
			return productdomain.Product{}, domain.Conflict(actualRevision)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return productdomain.Product{}, storageError(err)
		}
		if expectedRevision != 0 {
			return productdomain.Product{}, domain.Conflict(0)
		}
		product, err := create()
		if err != nil {
			return productdomain.Product{}, err
		}
		var existingProductID string
		statement, arguments, buildErr = ormquery.NewWorkspaceSelectBuilder(store.renderer, TableProducts, workspaceID).
			Columns("product_id").
			Where(ormquery.Equal("code", product.Code)).
			Build()
		if buildErr != nil {
			return productdomain.Product{}, storageError(buildErr)
		}
		err = transaction.QueryRowContext(ctx, statement, arguments...).Scan(&existingProductID)
		if err == nil {
			return productdomain.Product{}, domain.Invalid("product_code_duplicate")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return productdomain.Product{}, storageError(err)
		}
		if err := store.insertProductState(ctx, transaction, product); err != nil {
			return productdomain.Product{}, err
		}
		return product, nil
	})
}

func (store *Store) TransactProduct(
	ctx context.Context,
	workspaceID string,
	productID string,
	mutation application.Mutation,
	expectedRevision uint64,
	mutate func(*productdomain.Product) error,
) (productdomain.Product, error) {
	return executeCommand(ctx, store, commandTarget{
		workspaceID: workspaceID, resourceType: "product", resourceID: productID, operationKind: "product_command",
	}, mutation, func(transaction *sql.Tx) (productdomain.Product, error) {
		product, err := loadProductState(ctx, transaction, store.renderer, workspaceID, productID)
		if err != nil {
			return productdomain.Product{}, err
		}
		actualRevision := product.Revision
		if expectedRevision != actualRevision {
			return productdomain.Product{}, domain.Conflict(actualRevision)
		}
		if err := mutate(&product); err != nil {
			return productdomain.Product{}, err
		}
		product.Revision = actualRevision + 1
		if err := store.updateProductState(ctx, transaction, product, actualRevision); err != nil {
			return productdomain.Product{}, err
		}
		return product, nil
	})
}
