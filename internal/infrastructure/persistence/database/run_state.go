package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	ormquery "github.com/domainry/domainry-orm/query"
	"github.com/domainry/domainry-orm/sqlhost"
)

func loadRunState(ctx context.Context, database sqlhost.DBTX, renderer sqlRenderer, workspaceID, runID string) (deliveryrun.DeliveryRun, error) {
	statement, arguments, err := ormquery.NewWorkspaceSelectBuilder(renderer, TableRuns, workspaceID).
		Columns(
			"run_id", "workspace_id", "name", "code", "goal", "target_date", "stage", "revision", "product_snapshot_json",
			"feature_snapshot_json", "members_json", "active_delivery_unit_id", "executable_revision_json", "created_at", "updated_at",
		).
		Where(ormquery.Equal("run_id", runID)).
		Build()
	if err != nil {
		return deliveryrun.DeliveryRun{}, storageError(err)
	}
	var run deliveryrun.DeliveryRun
	var productJSON, featureJSON, membersJSON, executableJSON []byte
	var createdAt, updatedAt string
	err = database.QueryRowContext(ctx, statement, arguments...).Scan(
		&run.ID, &run.WorkspaceID, &run.Name, &run.Code, &run.Goal, &run.TargetDate, &run.Stage, &run.Revision,
		&productJSON, &featureJSON, &membersJSON, &run.ActiveDeliveryUnitID, &executableJSON, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return deliveryrun.DeliveryRun{}, domain.NotFound("delivery run", runID)
	}
	if err != nil {
		return deliveryrun.DeliveryRun{}, storageError(err)
	}
	if err := json.Unmarshal(productJSON, &run.Product); err != nil {
		return deliveryrun.DeliveryRun{}, storageError(err)
	}
	if err := json.Unmarshal(featureJSON, &run.Feature); err != nil {
		return deliveryrun.DeliveryRun{}, storageError(err)
	}
	if err := json.Unmarshal(membersJSON, &run.Members); err != nil {
		return deliveryrun.DeliveryRun{}, storageError(err)
	}
	if len(executableJSON) > 0 {
		run.ExecutableRevision = &deliveryrun.ExecutableProductRevision{}
		if err := json.Unmarshal(executableJSON, run.ExecutableRevision); err != nil {
			return deliveryrun.DeliveryRun{}, storageError(err)
		}
	}
	if run.CreatedAt, err = parseStoredTime(createdAt); err != nil {
		return deliveryrun.DeliveryRun{}, storageError(err)
	}
	if run.UpdatedAt, err = parseStoredTime(updatedAt); err != nil {
		return deliveryrun.DeliveryRun{}, storageError(err)
	}
	if run.DeliveryUnits, err = loadRunComponents[deliveryrun.DeliveryUnit](ctx, database, renderer, TableDeliveryUnits, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.TestCases, err = loadRunComponents[deliveryrun.TestCase](ctx, database, renderer, TableTestCases, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.QualityRuns, err = loadRunComponents[deliveryrun.QualityRun](ctx, database, renderer, TableQualityRuns, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.AcceptanceCases, err = loadRunComponents[deliveryrun.AcceptanceCase](ctx, database, renderer, TableAcceptanceCases, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.AcceptanceConfirmations, err = loadRunComponents[deliveryrun.AcceptanceConfirmation](ctx, database, renderer, TableAcceptanceConfirmations, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.ReleaseChecks, err = loadRunComponents[deliveryrun.ReleaseCheck](ctx, database, renderer, TableReleaseChecks, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.Releases, err = loadRunComponents[deliveryrun.Release](ctx, database, renderer, TableReleases, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.Activity, err = loadRunComponents[deliveryrun.ActivityEvent](ctx, database, renderer, TableActivity, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	return run, nil
}

func loadRunComponents[T any](ctx context.Context, database sqlhost.DBTX, renderer sqlRenderer, table, workspaceID, runID string) ([]T, error) {
	return loadJSONRows[T](ctx, database,
		ormquery.NewWorkspaceSelectBuilder(renderer, table, workspaceID).
			Columns("value_json").
			Where(ormquery.Equal("run_id", runID)).
			OrderBy(ormquery.Ascending("ordinal")),
	)
}

func (store *Store) insertRunState(ctx context.Context, transaction sqlhost.DBTX, run deliveryrun.DeliveryRun) error {
	productJSON, featureJSON, membersJSON, executableJSON, err := marshalRunHeader(run)
	if err != nil {
		return err
	}
	statement, arguments, err := ormquery.NewWorkspaceInsertBuilder(store.renderer, TableRuns, run.WorkspaceID).
		Columns(
			"run_id", "product_id", "name", "code", "goal", "target_date", "stage", "revision",
			"product_snapshot_json", "feature_snapshot_json", "members_json", "active_delivery_unit_id",
			"executable_revision_json", "created_at", "updated_at",
		).
		Values(
			run.ID, run.Product.ID, run.Name, run.Code, run.Goal, run.TargetDate, run.Stage, run.Revision,
			productJSON, featureJSON, membersJSON, run.ActiveDeliveryUnitID, executableJSON,
			run.CreatedAt.Format(timeFormat), run.UpdatedAt.Format(timeFormat),
		).
		Build()
	if err != nil {
		return storageError(err)
	}
	if _, err := transaction.ExecContext(ctx, statement, arguments...); err != nil {
		return storageError(err)
	}
	return store.persistRunRelations(ctx, transaction, run)
}

func (store *Store) updateRunState(ctx context.Context, transaction sqlhost.DBTX, run deliveryrun.DeliveryRun, expectedRevision uint64) error {
	productJSON, featureJSON, membersJSON, executableJSON, err := marshalRunHeader(run)
	if err != nil {
		return err
	}
	statement, arguments, err := ormquery.NewWorkspaceUpdateBuilder(store.renderer, TableRuns, run.WorkspaceID).
		Set("product_id", run.Product.ID).
		Set("name", run.Name).
		Set("code", run.Code).
		Set("goal", run.Goal).
		Set("target_date", run.TargetDate).
		Set("stage", run.Stage).
		Set("revision", run.Revision).
		Set("product_snapshot_json", productJSON).
		Set("feature_snapshot_json", featureJSON).
		Set("members_json", membersJSON).
		Set("active_delivery_unit_id", run.ActiveDeliveryUnitID).
		Set("executable_revision_json", executableJSON).
		Set("updated_at", run.UpdatedAt.Format(timeFormat)).
		Where(ormquery.And(
			ormquery.Equal("run_id", run.ID),
			ormquery.Equal("revision", expectedRevision),
		)).
		Build()
	if err != nil {
		return storageError(err)
	}
	result, err := transaction.ExecContext(ctx, statement, arguments...)
	if err != nil {
		return storageError(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return storageError(err)
	}
	if changed != 1 {
		return domain.Conflict(expectedRevision)
	}
	return store.persistRunRelations(ctx, transaction, run)
}

func marshalRunHeader(run deliveryrun.DeliveryRun) ([]byte, []byte, []byte, []byte, error) {
	productJSON, err := json.Marshal(run.Product)
	if err != nil {
		return nil, nil, nil, nil, storageError(err)
	}
	featureJSON, err := json.Marshal(run.Feature)
	if err != nil {
		return nil, nil, nil, nil, storageError(err)
	}
	membersJSON, err := json.Marshal(run.Members)
	if err != nil {
		return nil, nil, nil, nil, storageError(err)
	}
	executableJSON, err := nullableJSON(run.ExecutableRevision)
	if err != nil {
		return nil, nil, nil, nil, storageError(err)
	}
	return productJSON, featureJSON, membersJSON, executableJSON, nil
}

func (store *Store) persistRunRelations(ctx context.Context, transaction sqlhost.DBTX, run deliveryrun.DeliveryRun) error {
	if err := store.persistMutableRunComponents(ctx, transaction, TableDeliveryUnits, "unit_id", run.WorkspaceID, run.ID, len(run.DeliveryUnits), func(index int) (string, any) { return run.DeliveryUnits[index].ID, run.DeliveryUnits[index] }); err != nil {
		return err
	}
	if err := store.persistImmutableRunComponents(ctx, transaction, TableTestCases, "test_case_id", run.WorkspaceID, run.ID, len(run.TestCases), func(index int) (string, any) { return run.TestCases[index].ID, run.TestCases[index] }); err != nil {
		return err
	}
	if err := store.persistImmutableRunComponents(ctx, transaction, TableQualityRuns, "quality_run_id", run.WorkspaceID, run.ID, len(run.QualityRuns), func(index int) (string, any) { return run.QualityRuns[index].ID, run.QualityRuns[index] }); err != nil {
		return err
	}
	if err := store.persistImmutableRunComponents(ctx, transaction, TableAcceptanceCases, "acceptance_case_id", run.WorkspaceID, run.ID, len(run.AcceptanceCases), func(index int) (string, any) { return run.AcceptanceCases[index].ID, run.AcceptanceCases[index] }); err != nil {
		return err
	}
	if err := store.persistImmutableRunComponents(ctx, transaction, TableAcceptanceConfirmations, "confirmation_id", run.WorkspaceID, run.ID, len(run.AcceptanceConfirmations), func(index int) (string, any) {
		return run.AcceptanceConfirmations[index].ID, run.AcceptanceConfirmations[index]
	}); err != nil {
		return err
	}
	if err := store.replaceRunComponents(ctx, transaction, TableReleaseChecks, "check_id", run.WorkspaceID, run.ID, len(run.ReleaseChecks), func(index int) (string, any) { return run.ReleaseChecks[index].ID, run.ReleaseChecks[index] }); err != nil {
		return err
	}
	if err := store.persistMutableRunComponents(ctx, transaction, TableReleases, "release_id", run.WorkspaceID, run.ID, len(run.Releases), func(index int) (string, any) { return run.Releases[index].ID, run.Releases[index] }); err != nil {
		return err
	}
	return store.persistImmutableRunComponents(ctx, transaction, TableActivity, "event_id", run.WorkspaceID, run.ID, len(run.Activity), func(index int) (string, any) { return run.Activity[index].ID, run.Activity[index] })
}

type componentAt func(int) (string, any)

func (store *Store) persistImmutableRunComponents(ctx context.Context, transaction sqlhost.DBTX, table, idColumn, workspaceID, runID string, count int, at componentAt) error {
	for ordinal := 0; ordinal < count; ordinal++ {
		id, value := at(ordinal)
		raw, err := json.Marshal(value)
		if err != nil {
			return storageError(err)
		}
		statement, arguments, err := ormquery.NewWorkspaceInsertBuilder(store.renderer, table, workspaceID).
			Columns("run_id", idColumn, "ordinal", "value_json").
			Values(runID, id, ordinal, raw).
			OnConflictDoNothing("workspace_id", "run_id", idColumn).
			Build()
		if err != nil {
			return storageError(err)
		}
		if _, err := transaction.ExecContext(ctx, statement, arguments...); err != nil {
			return storageError(err)
		}
	}
	return nil
}

func (store *Store) persistMutableRunComponents(ctx context.Context, transaction sqlhost.DBTX, table, idColumn, workspaceID, runID string, count int, at componentAt) error {
	for ordinal := 0; ordinal < count; ordinal++ {
		id, value := at(ordinal)
		raw, err := json.Marshal(value)
		if err != nil {
			return storageError(err)
		}
		insert := ormquery.NewWorkspaceInsertBuilder(store.renderer, table, workspaceID).
			Columns("run_id", idColumn, "ordinal", "value_json").
			Values(runID, id, ordinal, raw)
		insert, err = store.profile.ApplyUpsert(insert, []string{"workspace_id", "run_id", idColumn},
			ormquery.AssignExpression("ordinal", ormquery.InsertedValue("ordinal")),
			ormquery.AssignExpression("value_json", ormquery.InsertedValue("value_json")),
		)
		if err != nil {
			return storageError(err)
		}
		statement, arguments, err := insert.Build()
		if err != nil {
			return storageError(err)
		}
		if _, err := transaction.ExecContext(ctx, statement, arguments...); err != nil {
			return storageError(err)
		}
	}
	return nil
}

func (store *Store) replaceRunComponents(ctx context.Context, transaction sqlhost.DBTX, table, idColumn, workspaceID, runID string, count int, at componentAt) error {
	statement, arguments, err := ormquery.NewWorkspaceDeleteBuilder(store.renderer, table, workspaceID).
		Where(ormquery.Equal("run_id", runID)).
		Build()
	if err != nil {
		return storageError(err)
	}
	if _, err := transaction.ExecContext(ctx, statement, arguments...); err != nil {
		return storageError(err)
	}
	return store.persistMutableRunComponents(ctx, transaction, table, idColumn, workspaceID, runID, count, at)
}
