package delivery

import (
	"fmt"
	"strings"
	"time"
)

type DeliveryRunSpec struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Code       string   `json:"code"`
	Goal       string   `json:"goal"`
	TargetDate string   `json:"target_date"`
	Members    []Member `json:"members"`
}

// NewDeliveryRun freezes one confirmed FeatureRevision as the complete scope of
// one delivery. Development work is deliberately not generated here: the RD
// Agent must explicitly record either an empty simple-feature plan or a real
// dependency graph before creating a build.
func NewDeliveryRun(product Product, featureID string, featureRevision uint64, spec DeliveryRunSpec, actor Actor, now time.Time) (DeliveryRun, error) {
	if err := validateActor(actor); err != nil {
		return DeliveryRun{}, err
	}
	if actor.Kind != ActorAgent {
		return DeliveryRun{}, Invalid("agent_execution_required", "A local Agent Runtime must create a DeliveryRun.")
	}
	feature := findFeature(&product, strings.TrimSpace(featureID))
	if feature == nil {
		return DeliveryRun{}, NotFound("Feature", featureID)
	}
	if feature.Status != FeatureConfirmed || feature.ConfirmedRevision != featureRevision || feature.CurrentRevision != featureRevision {
		return DeliveryRun{}, Invalid("feature_not_confirmed", "A DeliveryRun must bind the currently confirmed FeatureRevision.")
	}
	featureRevisionValue := feature.Revisions[len(feature.Revisions)-1]
	if err := validateDeliveryRunSpec(spec); err != nil {
		return DeliveryRun{}, err
	}
	conversationIDs := make([]string, 0, len(featureRevisionValue.Sources))
	agentRuns := make([]AgentRunRef, 0, len(featureRevisionValue.Sources))
	sourceIDs := make([]string, 0)
	decisionIDs := make([]string, 0)
	seenConversations := map[string]bool{}
	for _, featureSource := range featureRevisionValue.Sources {
		if !seenConversations[featureSource.ConversationID] {
			conversationIDs = append(conversationIDs, featureSource.ConversationID)
			seenConversations[featureSource.ConversationID] = true
		}
		agentRuns = append(agentRuns, AgentRunRef{
			ConversationID: featureSource.ConversationID,
			RunID:          featureSource.RunID,
			BeforeStep:     featureSource.BeforeStep,
		})
		sourceIDs = append(sourceIDs, featureSource.SourceIDs...)
		decisionIDs = append(decisionIDs, featureSource.DecisionIDs...)
	}
	source := FeatureRevisionRef{
		ProductID:       product.ID,
		ProductRevision: product.CurrentReleaseRevision,
		FeatureID:       feature.ID,
		FeatureRevision: featureRevisionValue.Number,
		ConversationIDs: conversationIDs,
		AgentRuns:       agentRuns,
		SourceIDs:       cleanStrings(sourceIDs),
		DecisionIDs:     cleanStrings(decisionIDs),
	}
	featureSnapshot := FeatureSnapshot{
		ID: feature.ID, Title: featureRevisionValue.Title, Summary: featureRevisionValue.Summary,
		Priority: featureRevisionValue.Priority, BaselineProductRevision: featureRevisionValue.BaselineProductRevision,
		Discovery:     clone(featureRevisionValue.Discovery),
		Specification: clone(featureRevisionValue.Specification), Decisions: clone(featureRevisionValue.Decisions),
		Readiness: clone(featureRevisionValue.Readiness), Source: source,
	}
	deliveryUnits, activeDeliveryUnitID := newDeliveryUnits(featureSnapshot)
	testCases := make([]TestCase, 0, len(featureRevisionValue.Specification.Acceptance))
	acceptanceCases := make([]AcceptanceCase, 0, len(featureRevisionValue.Specification.Acceptance))
	for index, criterion := range featureRevisionValue.Specification.Acceptance {
		testCases = append(testCases, TestCase{
			ID: fmt.Sprintf("T-%02d", index+1), FeatureID: feature.ID, Title: criterion.Title,
			Expected: formatAcceptanceScenario(criterion), Revision: 1,
		})
		acceptanceCases = append(acceptanceCases, AcceptanceCase{
			ID: fmt.Sprintf("UAT-%02d", index+1), FeatureID: feature.ID, Title: criterion.Title,
			BusinessValue: featureRevisionValue.Specification.Intent.DesiredOutcome, Source: clone(source),
		})
	}
	return DeliveryRun{
		ID: spec.ID, WorkspaceID: product.WorkspaceID,
		Product: ProductSnapshot{ID: product.ID, Name: product.Name, Code: product.Code, ProductRevision: product.CurrentReleaseRevision},
		Feature: featureSnapshot, Name: spec.Name, Code: spec.Code, Goal: spec.Goal, TargetDate: spec.TargetDate,
		Revision: 1, Stage: StageDevelopment, Members: clone(spec.Members),
		DeliveryUnits: deliveryUnits, ActiveDeliveryUnitID: activeDeliveryUnitID,
		WorkItems: []WorkItem{}, Builds: []Build{}, TestCases: testCases, TestRuns: []TestRun{}, QualityRuns: []QualityRun{}, Issues: []Issue{},
		AcceptanceCases:   acceptanceCases,
		AcceptanceResults: []AcceptanceResult{}, AcceptanceConfirmations: []AcceptanceConfirmation{},
		ReleaseChecks: []ReleaseCheck{}, Releases: []Release{},
		Activity: []ActivityEvent{{
			ID: newID("event"), Kind: "delivery_started", Title: feature.Code + " delivery started",
			Detail: "The DeliveryRun froze the exact released Product baseline, confirmed FeatureRevision, and Agent source.", ActorID: actor.ID, OccurredAt: now,
		}},
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func formatAcceptanceScenario(scenario FeatureAcceptanceScenario) string {
	return "Given " + strings.Join(scenario.Given, "; ") + " | When " + scenario.When + " | Then " + strings.Join(scenario.Then, "; ")
}

func validateDeliveryRunSpec(spec DeliveryRunSpec) error {
	if strings.TrimSpace(spec.ID) == "" || strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Code) == "" || strings.TrimSpace(spec.Goal) == "" {
		return Invalid("delivery_run_incomplete", "A DeliveryRun requires an ID, name, code, and goal.")
	}
	members := map[string]bool{}
	roles := map[string]bool{}
	for _, member := range spec.Members {
		if strings.TrimSpace(member.ID) == "" || strings.TrimSpace(member.Name) == "" || len(member.Roles) == 0 {
			return Invalid("delivery_member_incomplete", "Every DeliveryRun member requires an ID, a name, and at least one role.")
		}
		if members[member.ID] {
			return Invalid("delivery_member_duplicate", "DeliveryRun member IDs must be unique.")
		}
		members[member.ID] = true
		memberRoles := map[string]bool{}
		for _, role := range member.Roles {
			role = strings.TrimSpace(role)
			if role == "" || memberRoles[role] {
				return Invalid("delivery_member_role_invalid", "Member roles must be non-empty and unique.")
			}
			memberRoles[role] = true
			roles[role] = true
		}
	}
	for _, role := range []string{RoleProductOwner, RoleBusinessAcceptor, RoleReleaseApprover} {
		if !roles[role] {
			return Invalid("delivery_role_missing", "DeliveryRun is missing required role: "+role)
		}
	}
	return nil
}
