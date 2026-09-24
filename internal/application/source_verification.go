package application

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	agentsdk "github.com/domainry/domainry-agent-sdk"

	"github.com/domainry/domainry-delivery/internal/domain"
	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
	"github.com/domainry/domainry-delivery/internal/domain/product"
)

func (service *Service) verifyProductCommandSources(ctx context.Context, workspaceID string, actor domain.Actor, command domain.Command) error {
	if command.Type != commanddomain.FeatureDiscoveryReplace {
		return nil
	}
	var payload struct {
		Source product.FeatureSource `json:"source"`
	}
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		return domain.Invalid("payload_invalid", "The command payload is invalid.")
	}
	source := payload.Source
	source.ConversationID = strings.TrimSpace(source.ConversationID)
	source.RunID = strings.TrimSpace(source.RunID)
	source.SourceIDs = normalizedIdentities(source.SourceIDs)
	source.DecisionIDs = normalizedIdentities(source.DecisionIDs)
	if service.ports.Sources == nil || strings.TrimSpace(service.ports.SourceRuntimeID) == "" {
		return domain.Invalid("feature_source_verifier_unavailable", "The Agent source owner is not configured; Feature lineage cannot be committed.")
	}
	request := agentsdk.ConversationSourceVerificationRequest{
		References: []agentsdk.ConversationRunReference{{ConversationID: source.ConversationID, RunID: source.RunID, BeforeStep: source.BeforeStep}},
		SourceIDs:  source.SourceIDs,
		Reader: agentsdk.ConversationAuthority{
			Known: true, RuntimeID: strings.TrimSpace(service.ports.SourceRuntimeID), WorkspaceID: strings.TrimSpace(workspaceID),
			UserID: strings.TrimSpace(actor.ID), RoleKey: string(actor.Kind),
		},
	}
	receipt, err := service.ports.Sources.VerifyConversationSources(ctx, request)
	if err != nil {
		return domain.Invalid("feature_source_unverified", "The Agent source owner rejected the Feature lineage or current reader access.")
	}
	if strings.TrimSpace(receipt.WorkspaceID) != request.Reader.WorkspaceID || receipt.VerifiedAt.IsZero() || len(receipt.References) != 1 || receipt.References[0] != request.References[0] || !sameIdentities(receipt.SourceIDs, request.SourceIDs) {
		return domain.Invalid("feature_source_receipt_invalid", "The Agent source verification receipt does not match the submitted Feature lineage.")
	}
	return nil
}

func normalizedIdentities(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func sameIdentities(left, right []string) bool {
	left = normalizedIdentities(left)
	right = normalizedIdentities(right)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}
