package database

import (
	"context"
	"database/sql"
	"reflect"
	"time"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain"
	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	productdomain "github.com/domainry/domainry-delivery/internal/domain/product"
	"github.com/domainry/domainry-delivery/internal/utcjson"
	ormquery "github.com/domainry/domainry-orm/query"
	"github.com/domainry/domainry-orm/sqlhost"
)

func (store *Store) ListDeliveryRunSummaries(ctx context.Context, workspaceID, productID string) ([]deliveryrun.Summary, error) {
	builder := ormquery.NewWorkspaceSelectBuilder(store.renderer, TableRuns, workspaceID).Columns(
		"run_id", "workspace_id", "product_snapshot_json", "feature_snapshot_json", "code", "goal", "stage", "revision", "updated_at",
	)
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
	runs := make([]deliveryrun.DeliveryRun, 0)
	for rows.Next() {
		var run deliveryrun.DeliveryRun
		var productJSON, featureJSON []byte
		var updatedAt int64
		if err := rows.Scan(&run.ID, &run.WorkspaceID, &productJSON, &featureJSON, &run.Code, &run.Goal, &run.Stage, &run.Revision, &updatedAt); err != nil {
			_ = rows.Close()
			return nil, storageError(err)
		}
		if err := utcjson.Unmarshal(productJSON, &run.Product); err != nil {
			_ = rows.Close()
			return nil, storageError(err)
		}
		if err := utcjson.Unmarshal(featureJSON, &run.Feature); err != nil {
			_ = rows.Close()
			return nil, storageError(err)
		}
		run.UpdatedAt = time.UnixMilli(updatedAt).UTC()
		runs = append(runs, run)
	}
	if err := rows.Close(); err != nil {
		return nil, storageError(err)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError(err)
	}
	if len(runs) == 0 {
		return []deliveryrun.Summary{}, nil
	}
	runIDs := make([]string, len(runs))
	for index := range runs {
		runIDs[index] = runs[index].ID
	}
	units, err := loadSummaryComponents[deliveryrun.DeliveryUnit](ctx, store.db, store.renderer, TableDeliveryUnits, workspaceID, runIDs)
	if err != nil {
		return nil, err
	}
	checks, err := loadSummaryComponents[deliveryrun.ReleaseCheck](ctx, store.db, store.renderer, TableReleaseChecks, workspaceID, runIDs)
	if err != nil {
		return nil, err
	}
	releases, err := loadSummaryComponents[deliveryrun.Release](ctx, store.db, store.renderer, TableReleases, workspaceID, runIDs)
	if err != nil {
		return nil, err
	}
	summaries := make([]deliveryrun.Summary, 0, len(runs))
	for index := range runs {
		runs[index].DeliveryUnits = units[runs[index].ID]
		runs[index].ReleaseChecks = checks[runs[index].ID]
		runs[index].Releases = releases[runs[index].ID]
		summaries = append(summaries, deliveryrun.SummaryFor(runs[index]))
	}
	return summaries, nil
}

func loadSummaryComponents[T any](ctx context.Context, database sqlhost.DBTX, renderer sqlRenderer, table, workspaceID string, runIDs []string) (map[string][]T, error) {
	values := make([]any, len(runIDs))
	for index, runID := range runIDs {
		values[index] = runID
	}
	statement, arguments, err := ormquery.NewWorkspaceSelectBuilder(renderer, table, workspaceID).
		Columns("run_id", "value_json").
		Where(ormquery.In("run_id", values...)).
		OrderBy(ormquery.Ascending("run_id"), ormquery.Ascending("ordinal")).
		Build()
	if err != nil {
		return nil, storageError(err)
	}
	rows, err := database.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, storageError(err)
	}
	components := make(map[string][]T, len(runIDs))
	for rows.Next() {
		var runID string
		var raw []byte
		if err := rows.Scan(&runID, &raw); err != nil {
			_ = rows.Close()
			return nil, storageError(err)
		}
		var value T
		if err := utcjson.Unmarshal(raw, &value); err != nil {
			_ = rows.Close()
			return nil, storageError(err)
		}
		components[runID] = append(components[runID], value)
	}
	if err := rows.Close(); err != nil {
		return nil, storageError(err)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError(err)
	}
	return components, nil
}

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
		if (run.Stage == deliveryrun.StageLive || mutation.CommandType == commanddomain.ReleaseDeployResult) && install != nil {
			product, err := loadProductState(ctx, transaction, store.renderer, workspaceID, run.Product.ID)
			if err != nil {
				return deliveryrun.DeliveryRun{}, err
			}
			productRevision := product.Revision
			previousProduct := domain.Clone(product)
			if err := install(&product, &run); err != nil {
				return deliveryrun.DeliveryRun{}, err
			}
			if !reflect.DeepEqual(previousProduct, product) {
				product.Revision = productRevision + 1
				if err := store.updateProductState(ctx, transaction, product, productRevision); err != nil {
					return deliveryrun.DeliveryRun{}, err
				}
			}
		}
		run.Revision = actualRevision + 1
		if err := store.updateRunState(ctx, transaction, run, actualRevision); err != nil {
			return deliveryrun.DeliveryRun{}, err
		}
		return run, nil
	})
}
