package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain"
	productdomain "github.com/domainry/domainry-delivery/internal/domain/product"
	ormquery "github.com/domainry/domainry-orm/query"
	"github.com/domainry/domainry-orm/sqlhost"
)

func loadProductState(ctx context.Context, database sqlhost.DBTX, renderer sqlRenderer, workspaceID, productID string) (productdomain.Product, error) {
	statement, arguments, err := ormquery.NewWorkspaceSelectBuilder(renderer, TableProducts, workspaceID).
		Columns(
			"product_id", "workspace_id", "name", "code", "goal", "industry", "engineering_json", "status", "revision",
			"current_definition_revision", "current_release_revision", "current_deployment_json", "created_at", "updated_at",
		).
		Where(ormquery.Equal("product_id", productID)).
		Build()
	if err != nil {
		return productdomain.Product{}, storageError(err)
	}
	var product productdomain.Product
	var engineeringJSON []byte
	var deploymentJSON []byte
	var createdAt, updatedAt string
	err = database.QueryRowContext(ctx, statement, arguments...).Scan(
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
	if product.Revisions, err = loadJSONRows[productdomain.ProductRevision](ctx, database,
		ormquery.NewWorkspaceSelectBuilder(renderer, TableProductRevisions, workspaceID).
			Columns("revision_json").
			Where(ormquery.Equal("product_id", productID)).
			OrderBy(ormquery.Ascending("revision_number")),
	); err != nil {
		return productdomain.Product{}, err
	}
	if product.Features, err = loadFeatures(ctx, database, renderer, workspaceID, productID); err != nil {
		return productdomain.Product{}, err
	}
	return product, nil
}

func loadFeatures(ctx context.Context, database sqlhost.DBTX, renderer sqlRenderer, workspaceID, productID string) ([]productdomain.Feature, error) {
	statement, arguments, err := ormquery.NewWorkspaceSelectBuilder(renderer, TableFeatures, workspaceID).
		Columns(
			"feature_id", "code", "status", "current_revision", "confirmed_revision", "delivery_sequence",
			"queued_at", "delivery_run_id", "installed_release_id", "draft_json", "created_at", "updated_at",
		).
		Where(ormquery.Equal("product_id", productID)).
		OrderBy(ormquery.Ascending("created_at"), ormquery.Ascending("feature_id")).
		Build()
	if err != nil {
		return nil, storageError(err)
	}
	rows, err := database.QueryContext(ctx, statement, arguments...)
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
		features[index].Revisions, err = loadJSONRows[productdomain.FeatureRevision](ctx, database,
			ormquery.NewWorkspaceSelectBuilder(renderer, TableFeatureRevisions, workspaceID).
				Columns("revision_json").
				Where(ormquery.And(
					ormquery.Equal("product_id", productID),
					ormquery.Equal("feature_id", features[index].ID),
				)).
				OrderBy(ormquery.Ascending("revision_number")),
		)
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
	statement, arguments, err := ormquery.NewWorkspaceInsertBuilder(store.renderer, TableProducts, product.WorkspaceID).
		Columns(
			"product_id", "name", "code", "goal", "industry", "engineering_json", "status", "revision",
			"current_definition_revision", "current_release_revision", "current_deployment_json", "created_at", "updated_at",
		).
		Values(
			product.ID, product.Name, product.Code, product.Goal, product.Industry, engineeringJSON, product.Status, product.Revision,
			product.CurrentDefinitionRevision, product.CurrentReleaseRevision, deploymentJSON,
			product.CreatedAt.Format(timeFormat), product.UpdatedAt.Format(timeFormat),
		).
		Build()
	if err != nil {
		return storageError(err)
	}
	if _, err := transaction.ExecContext(ctx, statement, arguments...); err != nil {
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
	statement, arguments, err := ormquery.NewWorkspaceUpdateBuilder(store.renderer, TableProducts, product.WorkspaceID).
		Set("name", product.Name).
		Set("code", product.Code).
		Set("goal", product.Goal).
		Set("industry", product.Industry).
		Set("engineering_json", engineeringJSON).
		Set("status", product.Status).
		Set("revision", product.Revision).
		Set("current_definition_revision", product.CurrentDefinitionRevision).
		Set("current_release_revision", product.CurrentReleaseRevision).
		Set("current_deployment_json", deploymentJSON).
		Set("updated_at", product.UpdatedAt.Format(timeFormat)).
		Where(ormquery.And(
			ormquery.Equal("product_id", product.ID),
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
	return store.persistProductRelations(ctx, transaction, product)
}

func (store *Store) persistProductRelations(ctx context.Context, transaction sqlhost.DBTX, product productdomain.Product) error {
	for _, revision := range product.Revisions {
		valueJSON, err := json.Marshal(revision)
		if err != nil {
			return storageError(err)
		}
		statement, arguments, err := ormquery.NewWorkspaceInsertBuilder(store.renderer, TableProductRevisions, product.WorkspaceID).
			Columns("product_id", "revision_number", "revision_json", "created_at").
			Values(product.ID, revision.Number, valueJSON, revision.CreatedAt.Format(timeFormat)).
			OnConflictDoNothing("workspace_id", "product_id", "revision_number").
			Build()
		if err != nil {
			return storageError(err)
		}
		if _, err := transaction.ExecContext(ctx, statement, arguments...); err != nil {
			return storageError(err)
		}
	}
	for _, feature := range product.Features {
		if err := store.persistFeature(ctx, transaction, product, feature); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) persistFeature(ctx context.Context, transaction sqlhost.DBTX, product productdomain.Product, feature productdomain.Feature) error {
	draftJSON, err := nullableJSON(feature.Draft)
	if err != nil {
		return storageError(err)
	}
	insert := ormquery.NewWorkspaceInsertBuilder(store.renderer, TableFeatures, product.WorkspaceID).
		Columns(
			"product_id", "feature_id", "code", "status", "current_revision", "confirmed_revision", "delivery_sequence",
			"queued_at", "delivery_run_id", "installed_release_id", "draft_json", "created_at", "updated_at",
		).
		Values(
			product.ID, feature.ID, feature.Code, feature.Status, feature.CurrentRevision, feature.ConfirmedRevision, feature.DeliverySequence,
			nullableTime(feature.QueuedAt), feature.DeliveryRunID, feature.InstalledReleaseID, draftJSON,
			feature.CreatedAt.Format(timeFormat), feature.UpdatedAt.Format(timeFormat),
		)
	insert, err = store.profile.ApplyUpsert(insert, []string{"workspace_id", "product_id", "feature_id"},
		ormquery.AssignExpression("code", ormquery.InsertedValue("code")),
		ormquery.AssignExpression("status", ormquery.InsertedValue("status")),
		ormquery.AssignExpression("current_revision", ormquery.InsertedValue("current_revision")),
		ormquery.AssignExpression("confirmed_revision", ormquery.InsertedValue("confirmed_revision")),
		ormquery.AssignExpression("delivery_sequence", ormquery.InsertedValue("delivery_sequence")),
		ormquery.AssignExpression("queued_at", ormquery.InsertedValue("queued_at")),
		ormquery.AssignExpression("delivery_run_id", ormquery.InsertedValue("delivery_run_id")),
		ormquery.AssignExpression("installed_release_id", ormquery.InsertedValue("installed_release_id")),
		ormquery.AssignExpression("draft_json", ormquery.InsertedValue("draft_json")),
		ormquery.AssignExpression("updated_at", ormquery.InsertedValue("updated_at")),
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
	for _, revision := range feature.Revisions {
		valueJSON, err := json.Marshal(revision)
		if err != nil {
			return storageError(err)
		}
		statement, arguments, err := ormquery.NewWorkspaceInsertBuilder(store.renderer, TableFeatureRevisions, product.WorkspaceID).
			Columns("product_id", "feature_id", "revision_number", "revision_json", "created_at").
			Values(product.ID, feature.ID, revision.Number, valueJSON, revision.CreatedAt.Format(timeFormat)).
			OnConflictDoNothing("workspace_id", "product_id", "feature_id", "revision_number").
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

func loadJSONRows[T any](ctx context.Context, database sqlhost.DBTX, builder *ormquery.SelectBuilder) ([]T, error) {
	statement, arguments, err := builder.Build()
	if err != nil {
		return nil, storageError(err)
	}
	rows, err := database.QueryContext(ctx, statement, arguments...)
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
