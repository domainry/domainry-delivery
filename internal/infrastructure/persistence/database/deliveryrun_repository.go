package database

import (
	"context"
	"database/sql"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	productdomain "github.com/domainry/domainry-delivery/internal/domain/product"
	ormquery "github.com/domainry/domainry-orm/query"
)

func (store *Store) Get(ctx context.Context, workspaceID, deliveryRunID string) (deliveryrun.DeliveryRun, error) {
	return loadRunState(ctx, store.db, store.renderer, workspaceID, deliveryRunID)
}

func (store *Store) ListDeliveryRuns(ctx context.Context, workspaceID, productID string) ([]deliveryrun.DeliveryRun, error) {
	builder := ormquery.NewWorkspaceSelectBuilder(store.renderer, TableRuns, workspaceID).Columns("run_id")
	if productID != "" {
		builder.Where(ormquery.Equal("product_id", productID))
	}
	statement, arguments, err := builder.
		OrderBy(ormquery.Descending("updated_at"), ormquery.Ascending("run_id")).
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
	runs := make([]deliveryrun.DeliveryRun, 0, len(ids))
	for _, id := range ids {
		run, err := loadRunState(ctx, store.db, store.renderer, workspaceID, id)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (store *Store) Transact(
	ctx context.Context,
	workspaceID string,
	deliveryRunID string,
	mutation application.Mutation,
	expectedRevision uint64,
	mutate func(*deliveryrun.DeliveryRun) error,
	install func(*productdomain.Product, *deliveryrun.DeliveryRun) error,
) (deliveryrun.DeliveryRun, error) {
	return executeCommand(ctx, store, commandTarget{
		workspaceID: workspaceID, resourceType: "delivery_run", resourceID: deliveryRunID, operationKind: "delivery_run_command",
	}, mutation, func(transaction *sql.Tx) (deliveryrun.DeliveryRun, error) {
		run, err := loadRunState(ctx, transaction, store.renderer, workspaceID, deliveryRunID)
		if err != nil {
			return deliveryrun.DeliveryRun{}, err
		}
		actualRevision := run.Revision
		if expectedRevision != actualRevision {
			return deliveryrun.DeliveryRun{}, domain.Conflict(actualRevision)
		}
		if err := mutate(&run); err != nil {
			return deliveryrun.DeliveryRun{}, err
		}
		if run.Stage == deliveryrun.StageLive && install != nil {
			product, err := loadProductState(ctx, transaction, store.renderer, workspaceID, run.Product.ID)
			if err != nil {
				return deliveryrun.DeliveryRun{}, err
			}
			productRevision := product.Revision
			if err := install(&product, &run); err != nil {
				return deliveryrun.DeliveryRun{}, err
			}
			product.Revision = productRevision + 1
			if err := store.updateProductState(ctx, transaction, product, productRevision); err != nil {
				return deliveryrun.DeliveryRun{}, err
			}
		}
		run.Revision = actualRevision + 1
		if err := store.updateRunState(ctx, transaction, run, actualRevision); err != nil {
			return deliveryrun.DeliveryRun{}, err
		}
		return run, nil
	})
}
