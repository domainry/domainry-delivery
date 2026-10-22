package application

import (
	"context"
	"strconv"
	"strings"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/conversation"
	"github.com/domainry/domainry-foundation/entityid"
)

type AppendFeatureMessage struct {
	ClientID         string   `json:"client_id"`
	DeviceID         string   `json:"device_id"`
	ExpectedRevision uint64   `json:"expected_revision"`
	TurnID           string   `json:"turn_id"`
	Role             string   `json:"role"`
	Text             string   `json:"text"`
	AttachmentIDs    []string `json:"attachment_ids"`
}

func (service *Service) AppendFeatureMessage(ctx context.Context, workspaceID, productID, featureID string, input AppendFeatureMessage) (conversation.Message, error) {
	actor, err := authenticatedActor(ctx, workspaceID, PermissionProductWrite, nil)
	if err != nil {
		return conversation.Message{}, err
	}
	if !conversation.ValidID(productID) || !conversation.ValidID(featureID) || !conversation.ValidID(input.ClientID) || !conversation.ValidID(input.DeviceID) || !conversation.ValidID(input.TurnID) || input.ExpectedRevision != 0 {
		return conversation.Message{}, domain.Invalid("request_invalid")
	}
	if input.Role != "user" && input.Role != "assistant" {
		return conversation.Message{}, domain.Invalid("request_invalid")
	}
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > conversation.MaxMessageBytes {
		return conversation.Message{}, domain.Invalid("request_invalid")
	}
	if len(input.AttachmentIDs) > 10 {
		return conversation.Message{}, domain.Invalid("request_invalid")
	}
	if _, err := service.ports.Products.GetProduct(ctx, workspaceID, productID); err != nil {
		return conversation.Message{}, err
	}
	conversationID := conversation.ConversationID(workspaceID, productID, featureID)
	messageID, err := entityid.Derive("delmsg", "delivery.feature-message", workspaceID, productID, featureID, input.ClientID)
	if err != nil {
		return conversation.Message{}, domain.Invalid("request_invalid")
	}
	message := conversation.Message{
		ID: messageID, ConversationID: conversationID, TurnID: input.TurnID,
		Role: input.Role, Text: input.Text, AttachmentIDs: append([]string{}, input.AttachmentIDs...),
		SourceID: conversation.SourceID(conversationID, messageID), CreatedBy: actor.ID, DeviceID: input.DeviceID,
		CreatedAt: service.now().UnixMilli(),
	}
	if _, err := service.ports.Conversations.GetMessage(ctx, workspaceID, productID, featureID, message.ID); err == nil {
		return service.ports.Conversations.PutMessage(ctx, workspaceID, productID, featureID, message)
	} else if !isNotFound(err) {
		return conversation.Message{}, err
	}
	for _, id := range input.AttachmentIDs {
		if !conversation.ValidID(id) {
			return conversation.Message{}, domain.Invalid("request_invalid")
		}
		metadata, err := service.ports.AttachmentRecords.GetAttachment(ctx, workspaceID, productID, featureID, id)
		if err != nil || !metadata.Active {
			return conversation.Message{}, domain.Invalid("feature_source_reference_unverified")
		}
	}
	return service.ports.Conversations.PutMessage(ctx, workspaceID, productID, featureID, message)
}

type FeatureMessagesPage struct {
	ConversationID string                 `json:"conversation_id"`
	Data           []conversation.Message `json:"data"`
	NextCursor     string                 `json:"next_cursor,omitempty"`
}

func (service *Service) ListFeatureMessages(ctx context.Context, workspaceID, productID, featureID, cursor string, limit int) (FeatureMessagesPage, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductRead, nil); err != nil {
		return FeatureMessagesPage{}, err
	}
	if !conversation.ValidID(productID) || !conversation.ValidID(featureID) || limit < 0 || limit > 100 {
		return FeatureMessagesPage{}, domain.Invalid("request_invalid")
	}
	afterID := int64(0)
	if cursor = strings.TrimSpace(cursor); cursor != "" {
		var err error
		afterID, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil || afterID < 1 {
			return FeatureMessagesPage{}, domain.Invalid("request_invalid")
		}
	}
	if _, err := service.ports.Products.GetProduct(ctx, workspaceID, productID); err != nil {
		return FeatureMessagesPage{}, err
	}
	if limit == 0 {
		limit = 100
	}
	messages, err := service.ports.Conversations.ListMessages(ctx, workspaceID, productID, featureID, afterID, limit)
	if err != nil {
		return FeatureMessagesPage{}, err
	}
	page := FeatureMessagesPage{ConversationID: conversation.ConversationID(workspaceID, productID, featureID), Data: messages.Messages}
	if messages.HasMore {
		if messages.NextAfterID < 1 {
			return FeatureMessagesPage{}, domain.Invalid("message_cursor_invalid")
		}
		page.NextCursor = strconv.FormatInt(messages.NextAfterID, 10)
	}
	return page, nil
}
