package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain"
	productdomain "github.com/domainry/domainry-delivery/internal/domain/product"
	"github.com/domainry/domainry-orm/sqlhost"
)

func loadProductState(ctx context.Context, database sqlhost.DBTX, workspaceID, productID string) (productdomain.Product, error) {
	var product productdomain.Product
	var engineeringJSON []byte
	var deploymentJSON []byte
	var createdAt, updatedAt string
	err := database.QueryRowContext(ctx, `
SELECT product_id, workspace_id, name, code, goal, industry, engineering_json, status, revision,
       current_definition_revision, current_release_revision, current_deployment_json, created_at, updated_at
FROM delivery_products WHERE workspace_id = ? AND product_id = ?
`, workspaceID, productID).Scan(
		&product.ID, &product.WorkspaceID, &product.Name, &product.Code, &product.Goal, &product.Industry,
		&engineeringJSON, &product.Status, &product.Revision, &product.CurrentDefinitionRevision,
		&product.CurrentReleaseRevision, &deploymentJSON, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return productdomain.Product{}, domain.NotFound("product", productID)
	}
	if err != nil {
		return productdomain.Product{}, storageError(err)
	}
	if err := json.Unmarshal(engineeringJSON, &product.Engineering); err != nil {
		return productdomain.Product{}, storageError(err)
	}
	if len(deploymentJSON) > 0 {
		product.CurrentDeployment = &productdomain.ProductDeployment{}
		if err := json.Unmarshal(deploymentJSON, product.CurrentDeployment); err != nil {
			return productdomain.Product{}, storageError(err)
		}
	}
	if product.CreatedAt, err = parseStoredTime(createdAt); err != nil {
		return productdomain.Product{}, storageError(err)
	}
	if product.UpdatedAt, err = parseStoredTime(updatedAt); err != nil {
		return productdomain.Product{}, storageError(err)
	}
	if product.Revisions, err = loadJSONRows[productdomain.ProductRevision](ctx, database, `
SELECT revision_json FROM delivery_product_revisions
WHERE workspace_id = ? AND product_id = ? ORDER BY revision_number
`, workspaceID, productID); err != nil {
		return productdomain.Product{}, err
	}
	if product.Features, err = loadFeatures(ctx, database, workspaceID, productID); err != nil {
		return productdomain.Product{}, err
	}
	return product, nil
}

func loadFeatures(ctx context.Context, database sqlhost.DBTX, workspaceID, productID string) ([]productdomain.Feature, error) {
	rows, err := database.QueryContext(ctx, `
SELECT feature_id, code, status, current_revision, confirmed_revision, delivery_sequence,
       queued_at, delivery_run_id, installed_release_id, draft_json, created_at, updated_at
FROM delivery_features WHERE workspace_id = ? AND product_id = ?
ORDER BY created_at, feature_id
`, workspaceID, productID)
	if err != nil {
		return nil, storageError(err)
	}
	features := make([]productdomain.Feature, 0)
	for rows.Next() {
		var feature productdomain.Feature
		var queuedAt sql.NullString
		var draftJSON []byte
		var createdAt, updatedAt string
		if err := rows.Scan(&feature.ID, &feature.Code, &feature.Status, &feature.CurrentRevision, &feature.ConfirmedRevision, &feature.DeliverySequence, &queuedAt, &feature.DeliveryRunID, &feature.InstalledReleaseID, &draftJSON, &createdAt, &updatedAt); err != nil {
			return nil, storageError(err)
		}
		if queuedAt.Valid {
			parsed, err := parseStoredTime(queuedAt.String)
			if err != nil {
				return nil, storageError(err)
			}
			feature.QueuedAt = &parsed
		}
		if len(draftJSON) > 0 {
			feature.Draft = &productdomain.FeatureDraftState{}
			if err := json.Unmarshal(draftJSON, feature.Draft); err != nil {
				return nil, storageError(err)
			}
		}
		if feature.CreatedAt, err = parseStoredTime(createdAt); err != nil {
			return nil, storageError(err)
		}
		if feature.UpdatedAt, err = parseStoredTime(updatedAt); err != nil {
			return nil, storageError(err)
		}
		features = append(features, feature)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, storageError(err)
	}
	if err := rows.Close(); err != nil {
		return nil, storageError(err)
	}
	for index := range features {
		features[index].Revisions, err = loadJSONRows[productdomain.FeatureRevision](ctx, database, `
SELECT revision_json FROM delivery_feature_revisions
WHERE workspace_id = ? AND product_id = ? AND feature_id = ? ORDER BY revision_number
`, workspaceID, productID, features[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return features, nil
}

func (store *Store) insertProductState(ctx context.Context, transaction sqlhost.DBTX, product productdomain.Product) error {
	engineeringJSON, err := json.Marshal(product.Engineering)
	if err != nil {
		return storageError(err)
	}
	deploymentJSON, err := nullableJSON(product.CurrentDeployment)
	if err != nil {
		return storageError(err)
	}
	_, err = transaction.ExecContext(ctx, `
INSERT INTO delivery_products (
  workspace_id, product_id, name, code, goal, industry, engineering_json, status, revision,
  current_definition_revision, current_release_revision, current_deployment_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, product.WorkspaceID, product.ID, product.Name, product.Code, product.Goal, product.Industry, engineeringJSON,
		product.Status, product.Revision, product.CurrentDefinitionRevision, product.CurrentReleaseRevision,
		deploymentJSON, product.CreatedAt.Format(timeFormat), product.UpdatedAt.Format(timeFormat))
	if err != nil {
		return storageError(err)
	}
	return store.persistProductRelations(ctx, transaction, product)
}

func (store *Store) updateProductState(ctx context.Context, transaction sqlhost.DBTX, product productdomain.Product, expectedRevision uint64) error {
	engineeringJSON, err := json.Marshal(product.Engineering)
	if err != nil {
		return storageError(err)
	}
	deploymentJSON, err := nullableJSON(product.CurrentDeployment)
	if err != nil {
		return storageError(err)
	}
	result, err := transaction.ExecContext(ctx, `
UPDATE delivery_products
SET name = ?, code = ?, goal = ?, industry = ?, engineering_json = ?, status = ?, revision = ?,
    current_definition_revision = ?, current_release_revision = ?, current_deployment_json = ?, updated_at = ?
WHERE workspace_id = ? AND product_id = ? AND revision = ?
`, product.Name, product.Code, product.Goal, product.Industry, engineeringJSON, product.Status, product.Revision,
		product.CurrentDefinitionRevision, product.CurrentReleaseRevision, deploymentJSON, product.UpdatedAt.Format(timeFormat),
		product.WorkspaceID, product.ID, expectedRevision)
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
	return store.persistProductRelations(ctx, transaction, product)
}

func (store *Store) persistProductRelations(ctx context.Context, transaction sqlhost.DBTX, product productdomain.Product) error {
	return persistProductRelationsWithDialect(ctx, transaction, product, store.dialect)
}

func persistProductRelationsWithDialect(ctx context.Context, transaction sqlhost.DBTX, product productdomain.Product, dialect string) error {
	for _, revision := range product.Revisions {
		valueJSON, err := json.Marshal(revision)
		if err != nil {
			return storageError(err)
		}
		if _, err := transaction.ExecContext(ctx, insertIgnoreForDialect(`
INSERT INTO delivery_product_revisions (workspace_id, product_id, revision_number, revision_json, created_at)
VALUES (?, ?, ?, ?, ?)
`, dialect), product.WorkspaceID, product.ID, revision.Number, valueJSON, revision.CreatedAt.Format(timeFormat)); err != nil {
			return storageError(err)
		}
	}
	for _, feature := range product.Features {
		if err := persistFeatureWithDialect(ctx, transaction, product, feature, dialect); err != nil {
			return err
		}
	}
	return nil
}

func persistFeatureWithDialect(ctx context.Context, transaction sqlhost.DBTX, product productdomain.Product, feature productdomain.Feature, dialect string) error {
	draftJSON, err := nullableJSON(feature.Draft)
	if err != nil {
		return storageError(err)
	}
	queuedAt := nullableTime(feature.QueuedAt)
	statement := `
INSERT INTO delivery_features (
  workspace_id, product_id, feature_id, code, status, current_revision, confirmed_revision, delivery_sequence,
  queued_at, delivery_run_id, installed_release_id, draft_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if dialect == "mysql" {
		statement += ` ON DUPLICATE KEY UPDATE code=VALUES(code), status=VALUES(status), current_revision=VALUES(current_revision), confirmed_revision=VALUES(confirmed_revision), delivery_sequence=VALUES(delivery_sequence), queued_at=VALUES(queued_at), delivery_run_id=VALUES(delivery_run_id), installed_release_id=VALUES(installed_release_id), draft_json=VALUES(draft_json), updated_at=VALUES(updated_at)`
	} else {
		statement += ` ON CONFLICT(workspace_id, product_id, feature_id) DO UPDATE SET code=excluded.code, status=excluded.status, current_revision=excluded.current_revision, confirmed_revision=excluded.confirmed_revision, delivery_sequence=excluded.delivery_sequence, queued_at=excluded.queued_at, delivery_run_id=excluded.delivery_run_id, installed_release_id=excluded.installed_release_id, draft_json=excluded.draft_json, updated_at=excluded.updated_at`
	}
	if _, err := transaction.ExecContext(ctx, statement, product.WorkspaceID, product.ID, feature.ID, feature.Code, feature.Status, feature.CurrentRevision, feature.ConfirmedRevision, feature.DeliverySequence, queuedAt, feature.DeliveryRunID, feature.InstalledReleaseID, draftJSON, feature.CreatedAt.Format(timeFormat), feature.UpdatedAt.Format(timeFormat)); err != nil {
		return storageError(err)
	}
	for _, revision := range feature.Revisions {
		valueJSON, err := json.Marshal(revision)
		if err != nil {
			return storageError(err)
		}
		if _, err := transaction.ExecContext(ctx, insertIgnoreForDialect(`
INSERT INTO delivery_feature_revisions (workspace_id, product_id, feature_id, revision_number, revision_json, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, dialect), product.WorkspaceID, product.ID, feature.ID, revision.Number, valueJSON, revision.CreatedAt.Format(timeFormat)); err != nil {
			return storageError(err)
		}
	}
	return nil
}

func insertIgnoreForDialect(statement, dialect string) string {
	statement = strings.TrimSpace(statement)
	if dialect == "mysql" {
		return strings.Replace(statement, "INSERT INTO", "INSERT IGNORE INTO", 1)
	}
	return statement + " ON CONFLICT DO NOTHING"
}

func nullableJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		return nil, nil
	}
	return raw, nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(timeFormat)
}

func loadJSONRows[T any](ctx context.Context, database sqlhost.DBTX, query string, arguments ...any) ([]T, error) {
	rows, err := database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, storageError(err)
	}
	defer func() { _ = rows.Close() }()
	values := make([]T, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, storageError(err)
		}
		var value T
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, storageError(fmt.Errorf("decode relational JSON payload: %w", err))
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError(err)
	}
	return values, nil
}
