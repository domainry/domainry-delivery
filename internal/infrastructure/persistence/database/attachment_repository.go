package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/attachment"
	ormquery "github.com/domainry/domainry-orm/query"
)

func (store *Store) PutAttachment(ctx context.Context, workspaceID, productID, featureID string, metadata attachment.Metadata) (attachment.Metadata, error) {
	previous, err := store.GetAttachment(ctx, workspaceID, productID, featureID, metadata.ID)
	if err == nil {
		if previous.Filename == metadata.Filename && previous.SHA256 == metadata.SHA256 && previous.CreatedBy == metadata.CreatedBy && previous.DeviceID == metadata.DeviceID {
			return previous, nil
		}
		return attachment.Metadata{}, domain.Invalid("idempotency_key_reused")
	}
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != "not_found" {
		return attachment.Metadata{}, err
	}
	statement, arguments, err := ormquery.NewWorkspaceInsertBuilder(store.renderer, TableFeatureAttachments, workspaceID).
		Columns("product_id", "feature_id", "attachment_id", "filename", "content_type", "bytes", "sha256", "content_ref", "active", "revision", "remove_client_id", "remove_device_id", "created_by", "device_id", "created_at").
		Values(productID, featureID, metadata.ID, metadata.Filename, metadata.ContentType, metadata.Bytes, metadata.SHA256, metadata.ContentRef, metadata.Active, metadata.Revision, metadata.RemoveClientID, metadata.RemoveDeviceID, metadata.CreatedBy, metadata.DeviceID, metadata.CreatedAt).Build()
	if err != nil {
		return attachment.Metadata{}, storageError(err)
	}
	if _, err := store.db.ExecContext(ctx, statement, arguments...); err != nil {
		previous, lookupErr := store.GetAttachment(ctx, workspaceID, productID, featureID, metadata.ID)
		if lookupErr == nil && previous.Filename == metadata.Filename && previous.SHA256 == metadata.SHA256 && previous.CreatedBy == metadata.CreatedBy && previous.DeviceID == metadata.DeviceID {
			return previous, nil
		}
		return attachment.Metadata{}, storageError(err)
	}
	return metadata, nil
}

func (store *Store) GetAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string) (attachment.Metadata, error) {
	statement, arguments, err := ormquery.NewWorkspaceSelectBuilder(store.renderer, TableFeatureAttachments, workspaceID).
		Columns("attachment_id", "filename", "content_type", "bytes", "sha256", "content_ref", "active", "revision", "remove_client_id", "remove_device_id", "created_by", "device_id", "created_at").
		Where(ormquery.And(ormquery.Equal("product_id", productID), ormquery.Equal("feature_id", featureID), ormquery.Equal("attachment_id", attachmentID))).Build()
	if err != nil {
		return attachment.Metadata{}, storageError(err)
	}
	metadata, err := scanAttachment(store.db.QueryRowContext(ctx, statement, arguments...))
	if errors.Is(err, sql.ErrNoRows) {
		return attachment.Metadata{}, domain.NotFound("feature_attachment", attachmentID)
	}
	if err != nil {
		return attachment.Metadata{}, storageError(err)
	}
	return metadata, nil
}

func (store *Store) ListAttachments(ctx context.Context, workspaceID, productID, featureID string) ([]attachment.Metadata, error) {
	statement, arguments, err := ormquery.NewWorkspaceSelectBuilder(store.renderer, TableFeatureAttachments, workspaceID).
		Columns("attachment_id", "filename", "content_type", "bytes", "sha256", "content_ref", "active", "revision", "remove_client_id", "remove_device_id", "created_by", "device_id", "created_at").
		Where(ormquery.And(ormquery.Equal("product_id", productID), ormquery.Equal("feature_id", featureID), ormquery.Equal("active", true))).
		OrderBy(ormquery.Ascending("created_at"), ormquery.Ascending("attachment_id")).Build()
	if err != nil {
		return nil, storageError(err)
	}
	rows, err := store.db.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	result := make([]attachment.Metadata, 0)
	for rows.Next() {
		metadata, err := scanAttachment(rows)
		if err != nil {
			return nil, storageError(err)
		}
		result = append(result, metadata)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError(err)
	}
	return result, nil
}

func (store *Store) RemoveAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID, clientID, deviceID string, expectedRevision uint64) (attachment.Metadata, error) {
	statement, arguments, err := ormquery.NewWorkspaceUpdateBuilder(store.renderer, TableFeatureAttachments, workspaceID).
		Set("active", false).Set("revision", expectedRevision+1).Set("remove_client_id", clientID).Set("remove_device_id", deviceID).
		Where(ormquery.And(ormquery.Equal("product_id", productID), ormquery.Equal("feature_id", featureID), ormquery.Equal("attachment_id", attachmentID), ormquery.Equal("revision", expectedRevision), ormquery.Equal("active", true))).Build()
	if err != nil {
		return attachment.Metadata{}, storageError(err)
	}
	result, err := store.db.ExecContext(ctx, statement, arguments...)
	if err != nil {
		return attachment.Metadata{}, storageError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return attachment.Metadata{}, storageError(err)
	}
	metadata, err := store.GetAttachment(ctx, workspaceID, productID, featureID, attachmentID)
	if err != nil {
		return attachment.Metadata{}, err
	}
	if affected != 1 {
		if !metadata.Active && metadata.RemoveClientID == clientID && metadata.RemoveDeviceID == deviceID {
			return metadata, nil
		}
		return attachment.Metadata{}, domain.Conflict(metadata.Revision)
	}
	return metadata, nil
}

func scanAttachment(row messageScanner) (attachment.Metadata, error) {
	var metadata attachment.Metadata
	if err := row.Scan(&metadata.ID, &metadata.Filename, &metadata.ContentType, &metadata.Bytes, &metadata.SHA256, &metadata.ContentRef, &metadata.Active, &metadata.Revision, &metadata.RemoveClientID, &metadata.RemoveDeviceID, &metadata.CreatedBy, &metadata.DeviceID, &metadata.CreatedAt); err != nil {
		return attachment.Metadata{}, err
	}
	return metadata, nil
}
