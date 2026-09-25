package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/conversation"
	"github.com/domainry/domainry-delivery/internal/domain/product"
)

func (service *Service) verifyFeatureSources(ctx context.Context, workspaceID, productID string, command domain.Command) error {
	if command.Type != "feature.discovery.replace" {
		return nil
	}
	var payload struct {
		FeatureID string                `json:"feature_id"`
		Source    product.FeatureSource `json:"source"`
	}
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		return domain.Invalid("payload_invalid")
	}
	if !conversation.ValidID(payload.FeatureID) || len(payload.Source.SourceIDs) == 0 {
		return domain.Invalid("feature_source_reference_unverified")
	}
	conversationID := conversation.ConversationID(workspaceID, productID, payload.FeatureID)
	if payload.Source.ConversationID != conversationID || !conversation.ValidID(payload.Source.RunID) {
		return domain.Invalid("feature_source_reference_unverified")
	}
	baseline, err := service.ports.Products.GetProduct(ctx, workspaceID, productID)
	if err != nil {
		return err
	}
	baselineSource := fmt.Sprintf("product://%s/revision/%d", productID, baseline.CurrentDefinitionRevision)
	for _, sourceID := range payload.Source.SourceIDs {
		if messageID, ok := strings.CutPrefix(sourceID, "conversation://"+conversationID+"/message/"); ok {
			if !conversation.ValidID(messageID) {
				return domain.Invalid("feature_source_reference_unverified")
			}
			message, err := service.ports.Conversations.GetMessage(ctx, workspaceID, productID, payload.FeatureID, messageID)
			if err != nil || message.TurnID != payload.Source.RunID {
				return domain.Invalid("feature_source_reference_unverified")
			}
			continue
		}
		if attachmentID, ok := strings.CutPrefix(sourceID, "attachment://"); ok {
			if !conversation.ValidID(attachmentID) {
				return domain.Invalid("feature_source_reference_unverified")
			}
			metadata, err := service.ports.AttachmentRecords.GetAttachment(ctx, workspaceID, productID, payload.FeatureID, attachmentID)
			if err != nil || !metadata.Active {
				return domain.Invalid("feature_source_reference_unverified")
			}
			continue
		}
		if sourceID != baselineSource {
			return domain.Invalid("feature_source_reference_unverified")
		}
	}
	return nil
}
