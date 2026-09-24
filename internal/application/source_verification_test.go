package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	agentsdk "github.com/domainry/domainry-agent-sdk"

	"github.com/domainry/domainry-delivery/internal/domain"
	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

type sourceVerifierFunc func(context.Context, agentsdk.ConversationSourceVerificationRequest) (agentsdk.ConversationSourceVerificationReceipt, error)

func (function sourceVerifierFunc) VerifyConversationSources(ctx context.Context, request agentsdk.ConversationSourceVerificationRequest) (agentsdk.ConversationSourceVerificationReceipt, error) {
	return function(ctx, request)
}

func TestFeatureDiscoveryFailsClosedWithoutSourceOwner(t *testing.T) {
	service := NewService(Ports{SourceRuntimeID: "agent-test"})
	command := featureSourceCommand(t)
	err := service.verifyProductCommandSources(t.Context(), "workspace-a", domain.Actor{ID: "pm-a", Kind: domain.ActorAgent}, command)
	assertDomainCode(t, err, "feature_source_verifier_unavailable")
}

func TestFeatureDiscoveryRequiresExactSourceOwnerReceipt(t *testing.T) {
	command := featureSourceCommand(t)
	var captured agentsdk.ConversationSourceVerificationRequest
	service := NewService(Ports{
		SourceRuntimeID: "agent-test",
		Sources: sourceVerifierFunc(func(_ context.Context, request agentsdk.ConversationSourceVerificationRequest) (agentsdk.ConversationSourceVerificationReceipt, error) {
			captured = request
			return agentsdk.ConversationSourceVerificationReceipt{
				WorkspaceID: "workspace-other", References: request.References, SourceIDs: request.SourceIDs,
				DecisionIDs: request.DecisionIDs, VerifiedAt: time.Now().UTC(),
			}, nil
		}),
	})
	err := service.verifyProductCommandSources(t.Context(), "workspace-a", domain.Actor{ID: "pm-a", Kind: domain.ActorAgent}, command)
	assertDomainCode(t, err, "feature_source_receipt_invalid")
	if captured.Reader.RuntimeID != "agent-test" || captured.Reader.WorkspaceID != "workspace-a" || captured.Reader.UserID != "pm-a" || captured.References[0].BeforeStep != 3 {
		t.Fatalf("source owner request lost immutable authority or boundary: %#v", captured)
	}
}

func TestFeatureDiscoveryMapsOwnerFailureToStableError(t *testing.T) {
	service := NewService(Ports{
		SourceRuntimeID: "agent-test",
		Sources: sourceVerifierFunc(func(context.Context, agentsdk.ConversationSourceVerificationRequest) (agentsdk.ConversationSourceVerificationReceipt, error) {
			return agentsdk.ConversationSourceVerificationReceipt{}, errors.New("private owner detail")
		}),
	})
	err := service.verifyProductCommandSources(t.Context(), "workspace-a", domain.Actor{ID: "pm-a", Kind: domain.ActorAgent}, featureSourceCommand(t))
	assertDomainCode(t, err, "feature_source_unverified")
}

func featureSourceCommand(t *testing.T) domain.Command {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"source": map[string]any{
		"conversation_id": "conversation-a", "run_id": "run-a", "before_step": 3,
		"source_ids": []string{"source-a"}, "decision_ids": []string{"decision-a"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return domain.Command{Type: commanddomain.FeatureDiscoveryReplace, Payload: payload}
}

func assertDomainCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != code {
		t.Fatalf("error=%v, want domain code %s", err, code)
	}
}
