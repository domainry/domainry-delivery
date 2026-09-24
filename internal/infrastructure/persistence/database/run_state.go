package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-orm/sqlhost"
)

func loadRunState(ctx context.Context, database sqlhost.DBTX, workspaceID, runID string) (deliveryrun.DeliveryRun, error) {
	var run deliveryrun.DeliveryRun
	var productJSON, featureJSON, membersJSON, executableJSON []byte
	var createdAt, updatedAt string
	err := database.QueryRowContext(ctx, `
SELECT run_id, workspace_id, name, code, goal, target_date, stage, revision, product_snapshot_json,
       feature_snapshot_json, members_json, active_delivery_unit_id, executable_revision_json, created_at, updated_at
FROM delivery_runs WHERE workspace_id = ? AND run_id = ?
`, workspaceID, runID).Scan(
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
	if run.DeliveryUnits, err = loadRunComponents[deliveryrun.DeliveryUnit](ctx, database, TableDeliveryUnits, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.TestCases, err = loadRunComponents[deliveryrun.TestCase](ctx, database, TableTestCases, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.QualityRuns, err = loadRunComponents[deliveryrun.QualityRun](ctx, database, TableQualityRuns, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.AcceptanceCases, err = loadRunComponents[deliveryrun.AcceptanceCase](ctx, database, TableAcceptanceCases, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.AcceptanceConfirmations, err = loadRunComponents[deliveryrun.AcceptanceConfirmation](ctx, database, TableAcceptanceConfirmations, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.ReleaseChecks, err = loadRunComponents[deliveryrun.ReleaseCheck](ctx, database, TableReleaseChecks, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.Releases, err = loadRunComponents[deliveryrun.Release](ctx, database, TableReleases, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	if run.Activity, err = loadRunComponents[deliveryrun.ActivityEvent](ctx, database, TableActivity, workspaceID, runID); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	return run, nil
}

func loadRunComponents[T any](ctx context.Context, database sqlhost.DBTX, table, workspaceID, runID string) ([]T, error) {
	return loadJSONRows[T](ctx, database, "SELECT value_json FROM "+table+" WHERE workspace_id = ? AND run_id = ? ORDER BY ordinal", workspaceID, runID)
}

func (store *Store) insertRunState(ctx context.Context, transaction sqlhost.DBTX, run deliveryrun.DeliveryRun) error {
	productJSON, featureJSON, membersJSON, executableJSON, err := marshalRunHeader(run)
	if err != nil {
		return err
	}
	_, err = transaction.ExecContext(ctx, `
INSERT INTO delivery_runs (
  workspace_id, run_id, product_id, name, code, goal, target_date, stage, revision,
  product_snapshot_json, feature_snapshot_json, members_json, active_delivery_unit_id,
  executable_revision_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, run.WorkspaceID, run.ID, run.Product.ID, run.Name, run.Code, run.Goal, run.TargetDate, run.Stage, run.Revision,
		productJSON, featureJSON, membersJSON, run.ActiveDeliveryUnitID, executableJSON,
		run.CreatedAt.Format(timeFormat), run.UpdatedAt.Format(timeFormat))
	if err != nil {
		return storageError(err)
	}
	return store.persistRunRelations(ctx, transaction, run)
}

func (store *Store) updateRunState(ctx context.Context, transaction sqlhost.DBTX, run deliveryrun.DeliveryRun, expectedRevision uint64) error {
	productJSON, featureJSON, membersJSON, executableJSON, err := marshalRunHeader(run)
	if err != nil {
		return err
	}
	result, err := transaction.ExecContext(ctx, `
UPDATE delivery_runs
SET product_id = ?, name = ?, code = ?, goal = ?, target_date = ?, stage = ?, revision = ?,
    product_snapshot_json = ?, feature_snapshot_json = ?, members_json = ?, active_delivery_unit_id = ?,
    executable_revision_json = ?, updated_at = ?
WHERE workspace_id = ? AND run_id = ? AND revision = ?
`, run.Product.ID, run.Name, run.Code, run.Goal, run.TargetDate, run.Stage, run.Revision,
		productJSON, featureJSON, membersJSON, run.ActiveDeliveryUnitID, executableJSON, run.UpdatedAt.Format(timeFormat),
		run.WorkspaceID, run.ID, expectedRevision)
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
		statement := insertIgnoreForDialect("INSERT INTO "+table+" (workspace_id, run_id, "+idColumn+", ordinal, value_json) VALUES (?, ?, ?, ?, ?)", store.dialect)
		if _, err := transaction.ExecContext(ctx, statement, workspaceID, runID, id, ordinal, raw); err != nil {
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
		statement := "INSERT INTO " + table + " (workspace_id, run_id, " + idColumn + ", ordinal, value_json) VALUES (?, ?, ?, ?, ?)"
		if store.dialect == "mysql" {
			statement += " ON DUPLICATE KEY UPDATE ordinal=VALUES(ordinal), value_json=VALUES(value_json)"
		} else {
			statement += " ON CONFLICT(workspace_id, run_id, " + idColumn + ") DO UPDATE SET ordinal=excluded.ordinal, value_json=excluded.value_json"
		}
		if _, err := transaction.ExecContext(ctx, statement, workspaceID, runID, id, ordinal, raw); err != nil {
			return storageError(err)
		}
	}
	return nil
}

func (store *Store) replaceRunComponents(ctx context.Context, transaction sqlhost.DBTX, table, idColumn, workspaceID, runID string, count int, at componentAt) error {
	if _, err := transaction.ExecContext(ctx, "DELETE FROM "+table+" WHERE workspace_id = ? AND run_id = ?", workspaceID, runID); err != nil {
		return storageError(err)
	}
	return store.persistMutableRunComponents(ctx, transaction, table, idColumn, workspaceID, runID, count, at)
}
