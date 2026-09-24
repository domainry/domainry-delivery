package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	productdomain "github.com/domainry/domainry-delivery/internal/domain/product"
)

type startDeliveryResult struct {
	Product productdomain.Product   `json:"product"`
	Run     deliveryrun.DeliveryRun `json:"delivery_run"`
}

func (store *Store) StartDelivery(
	ctx context.Context,
	workspaceID string,
	productID string,
	deliveryRunID string,
	mutation application.Mutation,
	expectedRevision uint64,
	create func(*productdomain.Product) (deliveryrun.DeliveryRun, error),
) (productdomain.Product, deliveryrun.DeliveryRun, error) {
	result, err := executeCommand(ctx, store, commandTarget{
		workspaceID: workspaceID, resourceType: "product", resourceID: productID, operationKind: "start_delivery",
	}, mutation, func(transaction *sql.Tx) (startDeliveryResult, error) {
		product, err := loadProductState(ctx, transaction, workspaceID, productID)
		if err != nil {
			return startDeliveryResult{}, err
		}
		actualRevision := product.Revision
		if expectedRevision != actualRevision {
			return startDeliveryResult{}, domain.Conflict(actualRevision)
		}
		var existingRunID string
		err = transaction.QueryRowContext(ctx, `
SELECT run_id FROM delivery_runs WHERE workspace_id = ? AND run_id = ?
`, workspaceID, deliveryRunID).Scan(&existingRunID)
		if err == nil {
			return startDeliveryResult{}, domain.Invalid("delivery_run_exists", "The delivery run already exists.")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return startDeliveryResult{}, storageError(err)
		}

		run, err := create(&product)
		if err != nil {
			return startDeliveryResult{}, err
		}
		if run.ID != deliveryRunID || run.WorkspaceID != workspaceID || run.Product.ID != productID {
			return startDeliveryResult{}, domain.Invalid("delivery_run_identity_invalid", "The created delivery run does not match the requested resource identity.")
		}
		product.Revision = actualRevision + 1
		if err := store.insertRunState(ctx, transaction, run); err != nil {
			return startDeliveryResult{}, err
		}
		if err := store.updateProductState(ctx, transaction, product, actualRevision); err != nil {
			return startDeliveryResult{}, err
		}
		return startDeliveryResult{Product: product, Run: run}, nil
	})
	return result.Product, result.Run, err
}
