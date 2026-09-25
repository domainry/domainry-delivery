package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/conversation"
	ormquery "github.com/domainry/domainry-orm/query"
)

func (store *Store) PutMessage(ctx context.Context, workspaceID, productID, featureID string, message conversation.Message) (conversation.Message, error) {
	previous, err := store.GetMessage(ctx, workspaceID, productID, featureID, message.ID)
	if err == nil {
		return matchingMessage(previous, message)
	}
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != "not_found" {
		return conversation.Message{}, err
	}
	attachmentIDs, err := json.Marshal(message.AttachmentIDs)
	if err != nil {
		return conversation.Message{}, storageError(err)
	}
	statement, arguments, err := ormquery.NewInsertBuilder(store.renderer, TableFeatureMessages).
		Columns("workspace_id", "product_id", "feature_id", "message_id", "conversation_id", "turn_id", "role", "text", "attachment_ids_json", "created_by", "device_id", "created_at").
		Values(workspaceID, productID, featureID, message.ID, message.ConversationID, message.TurnID, message.Role, message.Text, attachmentIDs, message.CreatedBy, message.DeviceID, message.CreatedAt).Build()
	if err != nil {
		return conversation.Message{}, storageError(err)
	}
	if _, err := store.db.ExecContext(ctx, statement, arguments...); err != nil {
		previous, lookupErr := store.GetMessage(ctx, workspaceID, productID, featureID, message.ID)
		if lookupErr == nil {
			return matchingMessage(previous, message)
		}
		return conversation.Message{}, storageError(err)
	}
	return message, nil
}

func matchingMessage(previous, requested conversation.Message) (conversation.Message, error) {
	if previous.ID != requested.ID || previous.ConversationID != requested.ConversationID || previous.TurnID != requested.TurnID || previous.Role != requested.Role || previous.Text != requested.Text || previous.CreatedBy != requested.CreatedBy || previous.DeviceID != requested.DeviceID || !reflect.DeepEqual(previous.AttachmentIDs, requested.AttachmentIDs) {
		return conversation.Message{}, domain.Invalid("idempotency_key_reused")
	}
	return previous, nil
}

func (store *Store) GetMessage(ctx context.Context, workspaceID, productID, featureID, messageID string) (conversation.Message, error) {
	statement, arguments, err := ormquery.NewWorkspaceSelectBuilder(store.renderer, TableFeatureMessages, workspaceID).
		Columns("message_id", "conversation_id", "turn_id", "role", "text", "attachment_ids_json", "created_by", "device_id", "created_at").
		Where(ormquery.And(ormquery.Equal("product_id", productID), ormquery.Equal("feature_id", featureID), ormquery.Equal("message_id", messageID))).Build()
	if err != nil {
		return conversation.Message{}, storageError(err)
	}
	message, err := scanConversationMessage(store.db.QueryRowContext(ctx, statement, arguments...))
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.Message{}, domain.NotFound("feature_message", messageID)
	}
	if err != nil {
		return conversation.Message{}, storageError(err)
	}
	return message, nil
}

func (store *Store) ListMessages(ctx context.Context, workspaceID, productID, featureID, cursor string, limit int) ([]conversation.Message, error) {
	predicate := ormquery.And(ormquery.Equal("product_id", productID), ormquery.Equal("feature_id", featureID))
	if cursor != "" {
		previous, err := store.GetMessage(ctx, workspaceID, productID, featureID, cursor)
		if err != nil {
			return nil, err
		}
		createdAt := previous.CreatedAt
		predicate = ormquery.And(predicate, ormquery.Or(
			ormquery.GreaterThan("created_at", createdAt),
			ormquery.And(ormquery.Equal("created_at", createdAt), ormquery.GreaterThan("message_id", cursor)),
		))
	}
	statement, arguments, err := ormquery.NewWorkspaceSelectBuilder(store.renderer, TableFeatureMessages, workspaceID).
		Columns("message_id", "conversation_id", "turn_id", "role", "text", "attachment_ids_json", "created_by", "device_id", "created_at").
		Where(predicate).
		OrderBy(ormquery.Ascending("created_at"), ormquery.Ascending("message_id")).Limit(limit).Build()
	if err != nil {
		return nil, storageError(err)
	}
	rows, err := store.db.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	messages := make([]conversation.Message, 0)
	for rows.Next() {
		message, err := scanConversationMessage(rows)
		if err != nil {
			return nil, storageError(err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError(err)
	}
	return messages, nil
}

type messageScanner interface{ Scan(...any) error }

func scanConversationMessage(row messageScanner) (conversation.Message, error) {
	var message conversation.Message
	var attachmentIDs []byte
	if err := row.Scan(&message.ID, &message.ConversationID, &message.TurnID, &message.Role, &message.Text, &attachmentIDs, &message.CreatedBy, &message.DeviceID, &message.CreatedAt); err != nil {
		return conversation.Message{}, err
	}
	if err := json.Unmarshal(attachmentIDs, &message.AttachmentIDs); err != nil {
		return conversation.Message{}, err
	}
	message.SourceID = conversation.SourceID(message.ConversationID, message.ID)
	return message, nil
}
