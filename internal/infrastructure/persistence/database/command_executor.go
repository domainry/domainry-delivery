package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/domainry/domainry-delivery/internal/application"
)

// commandTarget is the persistence identity used by the shared Foundation
// operation ledger. It is deliberately aggregate-neutral: Product,
// DeliveryRun, and cross-aggregate lifecycle repositories all execute through
// the same transaction/receipt boundary.
type commandTarget struct {
	workspaceID   string
	resourceType  string
	resourceID    string
	operationKind string
}

// executeCommand owns the mechanics shared by every strong Delivery write:
// begin one transaction, claim or replay the immutable operation receipt,
// execute the aggregate-specific commit, persist the exact terminal result,
// and commit. Aggregate repositories supply only the work performed inside
// that boundary.
func executeCommand[T any](
	ctx context.Context,
	store *Store,
	target commandTarget,
	mutation application.Mutation,
	commit func(*sql.Tx) (T, error),
) (T, error) {
	var zero T
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return zero, storageError(err)
	}
	defer func() { _ = transaction.Rollback() }()

	receipt, replay, err := store.claimOperation(
		ctx,
		transaction,
		target.workspaceID,
		target.resourceType,
		target.resourceID,
		target.operationKind,
		mutation,
	)
	if err != nil {
		return zero, err
	}
	if replay {
		var saved T
		if err := json.Unmarshal(receipt.Result, &saved); err != nil {
			return zero, storageError(err)
		}
		if err := transaction.Commit(); err != nil {
			return zero, storageError(err)
		}
		return saved, nil
	}

	result, err := commit(transaction)
	if err != nil {
		return zero, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return zero, storageError(err)
	}
	if err := store.completeOperation(ctx, transaction, receipt, resultJSON, mutation.OccurredAt); err != nil {
		return zero, err
	}
	if err := transaction.Commit(); err != nil {
		return zero, storageError(err)
	}
	return result, nil
}
