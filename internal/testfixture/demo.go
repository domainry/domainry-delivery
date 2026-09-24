// Package testfixture contains deterministic aggregate fixtures used only by
// domain and persistence acceptance tests. Production assembly never imports
// this package and therefore cannot bypass the command mutation boundary.
package testfixture

import (
	"strings"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	delivery "github.com/domainry/domainry-delivery/internal/domain/product"
)

const (
	DemoWorkspaceID   = "workspace-demo"
	DemoProductID     = "product-greenfit"
	DemoDeliveryRunID = "run-18"
)

func DemoProduct(now time.Time) delivery.Product {
	story := delivery.ProductStory{
		Title:     "GreenFit Location Operations",
		Summary:   "Unify member booking, capacity control, cancellation, and location exceptions in one executable product.",
		Narrative: "Members select and book sessions, while server-side capacity determines availability. Members may cancel before the deadline to release capacity; location managers review late exceptions with a complete audit trail.",
	}
	definition := demoProductDefinition()
	product := delivery.Product{
		ID: DemoProductID, WorkspaceID: DemoWorkspaceID, Name: "GreenFit Location Operations", Code: "GREENFIT",
		Goal: "Make booking, cancellation, capacity control, and exception approval executable and traceable.", Industry: "Fitness and wellness",
		Engineering: delivery.ProductEngineering{
			Status: delivery.EngineeringReady, FrontendCodeRevision: "git:demo-engineering", FrontendArtifactRef: "deck-artifact://sha256/demo-engineering",
			DesignContractRef: "deck-evidence://sha256/demo-design", LoginEntry: "frontend/src/pages/Login.tsx", ShellEntry: "frontend/src/AppShell.tsx", PreviewEntry: "frontend/dist/index.html",
			ApplicationDeliverySHA256: strings.Repeat("a", 64), FoundationReleaseSHA256: strings.Repeat("b", 64), FoundationPackageSHA256: strings.Repeat("c", 64),
			FoundationModelSHA256: strings.Repeat("d", 64), FoundationIdempotencyKey: strings.Repeat("e", 64), FoundationCodeRevision: strings.Repeat("f", 40),
			FoundationGitStatus: "clean", FoundationVerificationSHA256: strings.Repeat("1", 64), IdentityBaselineResult: "passed",
		},
		Status:   delivery.ProductShaping,
		Revision: 1, CurrentDefinitionRevision: 1, CurrentReleaseRevision: 1,
		Revisions: []delivery.ProductRevision{{Number: 1, Story: story, Definition: definition, Decisions: []delivery.ProductDecision{}, CreatedBy: "product-owner", CreatedAt: now}},
		Features:  []delivery.Feature{}, CreatedAt: now, UpdatedAt: now,
	}
	featureSpecs := []struct {
		id, code, title, summary, conversationID, runID string
		acceptance                                      []string
	}{
		{"F-001", "CANCEL", "Members can cancel before a session", "Release capacity before a shared cutoff and return a clear result to the member.", "conversation-cancellation", "run-conversation-cancellation-definition", []string{"Cancellation more than two hours before a session succeeds and releases capacity", "Cancellation after the cutoff returns an understandable reason"}},
		{"F-002", "CAPACITY", "Concurrent bookings cannot oversell", "Remaining capacity for a session must be determined by the server transaction.", "conversation-booking", "run-conversation-booking-definition", []string{"Only one request can claim the last available place", "Conflicting requests receive a consistent capacity-exhausted result"}},
		{"F-003", "OVERRIDE", "Managers can review late cancellation exceptions", "Late cancellations enter exception approval and retain the operator and reason.", "conversation-cancellation", "run-conversation-cancellation-definition", []string{"Regular staff cannot execute an exception cancellation directly", "The approval outcome and reason are recorded in the audit trail"}},
	}
	for index, spec := range featureSpecs {
		featureRevision := uint64(1)
		product.Features = append(product.Features, delivery.Feature{
			ID: spec.id, Code: spec.code, Status: delivery.FeatureConfirmed, CurrentRevision: featureRevision,
			ConfirmedRevision: featureRevision, DeliverySequence: uint64(index + 1), QueuedAt: &now,
			Revisions: []delivery.FeatureRevision{{
				Number: featureRevision, Title: spec.title, Summary: spec.summary, Priority: "high", BaselineProductRevision: 1,
				Discovery:     demoFeatureDiscovery(spec.id, spec.title, spec.summary),
				Specification: demoFeatureSpecification(spec.id, spec.title, spec.summary, spec.acceptance),
				Decisions:     []delivery.FeatureDecision{},
				Readiness:     delivery.FeatureReadiness{Status: "ready", BlockingSections: []string{}, BlockingIssues: []string{}},
				Sources:       []delivery.FeatureSource{{ConversationID: spec.conversationID, RunID: spec.runID, SourceIDs: []string{"source-" + spec.id}, DecisionIDs: []string{}}},
				CreatedBy:     "pm-agent", CreatedAt: now,
			}},
			CreatedAt: now, UpdatedAt: now,
		})
	}
	return product
}

func demoFeatureDiscovery(featureID, title, summary string) delivery.FeatureDiscovery {
	sourceIDs := []string{"source-" + featureID}
	return delivery.FeatureDiscovery{
		Focus: delivery.FeatureDiscoveryFocus{Topic: title, DialogueMove: "summarize_and_confirm", Rationale: "The business scenario and expected outcome are confirmed."},
		Evidence: []delivery.FeatureEvidence{{
			ID: featureID + "-evidence", Kind: "fact", Statement: summary, Status: "confirmed", SourceIDs: sourceIDs, RelatedIDs: []string{featureID},
		}},
		Scenarios: []delivery.FeatureDiscoveryScenario{{
			ID: featureID + "-discovery-scenario", Title: title, ActorIDs: []string{"member"}, Trigger: summary,
			CurrentFlow: []string{title}, PainPoints: []string{summary}, TargetOutcome: summary,
			FailureModes: []string{"The requested business outcome is not completed"}, Status: "confirmed",
		}},
		Assumptions: []delivery.FeatureAssumption{}, Conflicts: []delivery.FeatureConflict{},
		Options: []delivery.FeatureDesignOption{}, OpenQuestions: []delivery.FeatureOpenQuestion{},
	}
}

func demoProductDefinition() delivery.ProductDefinition {
	source := []string{"source-greenfit-operations"}
	booking := delivery.BusinessObject{
		ID: "booking", Name: "Bookings", SingularName: "Booking",
		Description: "A member reservation for one scheduled session.", PrimaryFieldKey: "member_name",
		StateFieldKey: "status", InitialState: "draft", SourceIDs: source,
		Fields: []delivery.BusinessField{
			{Key: "member_name", Label: "Member", Type: "text", Required: true, SourceIDs: source},
			{Key: "session", Label: "Session", Type: "select", Required: true, Options: []string{"Morning strength", "Lunch mobility", "Evening conditioning"}, SourceIDs: source},
			{Key: "starts_at", Label: "Starts at", Type: "datetime", Required: true, SourceIDs: source},
			{Key: "within_cutoff", Label: "Within cancellation cutoff", Type: "boolean", Required: false, SourceIDs: source},
			{Key: "notes", Label: "Notes", Type: "textarea", Required: false, SourceIDs: source},
			{Key: "status", Label: "Status", Type: "select", Required: true, Options: []string{"draft", "confirmed", "cancelled", "exception_pending"}, SourceIDs: source},
		},
		States: []delivery.BusinessState{
			{Value: "draft", Label: "Draft", Tone: "neutral"},
			{Value: "confirmed", Label: "Confirmed", Tone: "positive"},
			{Value: "cancelled", Label: "Cancelled", Tone: "neutral"},
			{Value: "exception_pending", Label: "Exception pending", Tone: "warning"},
		},
	}
	location := delivery.BusinessObject{
		ID: "location", Name: "Locations", SingularName: "Location",
		Description: "A facility that hosts bookable sessions.", PrimaryFieldKey: "name",
		StateFieldKey: "status", InitialState: "draft", SourceIDs: source,
		Fields: []delivery.BusinessField{
			{Key: "name", Label: "Name", Type: "text", Required: true, SourceIDs: source},
			{Key: "city", Label: "City", Type: "text", Required: true, SourceIDs: source},
			{Key: "capacity", Label: "Capacity", Type: "number", Required: true, SourceIDs: source},
			{Key: "status", Label: "Status", Type: "select", Required: true, Options: []string{"draft", "active", "inactive"}, SourceIDs: source},
		},
		States: []delivery.BusinessState{
			{Value: "draft", Label: "Draft", Tone: "neutral"},
			{Value: "active", Label: "Active", Tone: "positive"},
			{Value: "inactive", Label: "Inactive", Tone: "neutral"},
		},
	}
	return delivery.ProductDefinition{
		SchemaVersion: 2,
		Actors: []delivery.ProductActor{
			{ID: "member", Name: "Member", Responsibility: "Book and cancel sessions.", SourceIDs: source},
			{ID: "location_manager", Name: "Location manager", Responsibility: "Review capacity and cancellation exceptions.", SourceIDs: source},
		},
		Scenarios: []delivery.ProductScenario{{
			ID: "manage_booking", Title: "Manage a session booking", Description: "Create, confirm, and cancel a member booking.",
			Trigger: "A member chooses an available session.", Outcome: "The booking and capacity outcome are recorded.", SourceIDs: source,
			Steps: []delivery.ScenarioStep{
				{ID: "choose_session", Title: "Choose a session", ActorID: "member", Detail: "Select a location and start time.", SourceIDs: source},
				{ID: "confirm_booking", Title: "Confirm the booking", ActorID: "member", Detail: "Reserve capacity and show the confirmed state.", SourceIDs: source},
				{ID: "review_exception", Title: "Review an exception", ActorID: "location_manager", Detail: "Handle a cancellation outside the cutoff.", SourceIDs: source},
			},
		}},
		Objects: []delivery.BusinessObject{booking, location},
		Rules: []delivery.BusinessRule{{
			ID: "cancellation_cutoff", Title: "Cancellation cutoff", Statement: "A normal cancellation must be inside the shared cutoff.",
			ObjectID: "booking", Condition: &delivery.RuleCondition{FieldKey: "within_cutoff", Operator: "eq", Value: true},
			ErrorMessage: "The booking is outside the cancellation cutoff. Request an exception instead.", SourceIDs: source,
		}},
		Exceptions: []delivery.BusinessException{{
			ID: "late_cancellation", Title: "Late cancellation", Trigger: "A confirmed booking is outside the cancellation cutoff.",
			Handling: "Move the booking to exception pending for manager review.", ScenarioID: "manage_booking", SourceIDs: source,
		}},
		Actions: []delivery.ProductAction{
			{ID: "booking.create", ObjectID: "booking", Label: "New booking", Kind: "create", RequiredFieldKeys: []string{"member_name", "session", "starts_at"}},
			{ID: "booking.confirm", ObjectID: "booking", Label: "Confirm", Kind: "transition", FromStates: []string{"draft"}, ToState: "confirmed"},
			{ID: "booking.cancel", ObjectID: "booking", Label: "Cancel booking", Kind: "transition", FromStates: []string{"confirmed"}, ToState: "cancelled", RuleIDs: []string{"cancellation_cutoff"}},
			{ID: "booking.request_exception", ObjectID: "booking", Label: "Request exception", Kind: "transition", FromStates: []string{"confirmed"}, ToState: "exception_pending"},
			{ID: "location.create", ObjectID: "location", Label: "New location", Kind: "create", RequiredFieldKeys: []string{"name", "city", "capacity"}},
			{ID: "location.activate", ObjectID: "location", Label: "Activate", Kind: "transition", FromStates: []string{"draft", "inactive"}, ToState: "active"},
			{ID: "location.deactivate", ObjectID: "location", Label: "Deactivate", Kind: "transition", FromStates: []string{"active"}, ToState: "inactive"},
		},
		Pages: []delivery.ProductPage{
			{ID: "bookings", Title: "Bookings", Description: "Create reservations and move each booking through its real state flow.", ObjectID: "booking", Kind: "list", VisibleFieldKeys: []string{"member_name", "session", "starts_at", "status"}, CreateActionID: "booking.create"},
			{ID: "locations", Title: "Locations", Description: "Maintain the facilities that host bookable sessions.", ObjectID: "location", Kind: "list", VisibleFieldKeys: []string{"name", "city", "capacity", "status"}, CreateActionID: "location.create"},
		},
		Access: delivery.ProductAccessModel{
			Authentication: "required", PermissionModel: "fixed",
			Roles: []delivery.ProductRole{
				{ID: "member", Name: "Member", ActorIDs: []string{"member"}, Responsibilities: []string{"Manage personal bookings"}, Capabilities: []string{"Create and cancel an eligible booking"}, DataScopes: []string{"Own bookings"}, SourceIDs: source},
				{ID: "location_manager", Name: "Location manager", ActorIDs: []string{"location_manager"}, Responsibilities: []string{"Operate a location"}, Capabilities: []string{"Review capacity and cancellation exceptions"}, DataScopes: []string{"Assigned locations"}, SourceIDs: source},
			},
			PermissionAdmins: []string{},
			AssignmentRules:  []string{"Authenticated members receive the Member role; assigned managers receive the Location manager role"},
		},
		Integrations:  []delivery.ProductIntegration{},
		Automations:   []delivery.ProductAutomation{},
		Configuration: []delivery.ProductConfiguration{},
		QualityConstraints: []delivery.ProductQualityConstraint{{
			ID: "capacity-consistency", Category: "consistency", Requirement: "Concurrent booking changes must not oversell capacity", Measure: "No accepted booking exceeds configured capacity", SourceIDs: source,
		}},
	}
}

func demoFeatureSpecification(featureID, title, summary string, acceptance []string) delivery.FeatureSpecification {
	acceptanceScenarios := make([]delivery.FeatureAcceptanceScenario, 0, len(acceptance))
	for index, expected := range acceptance {
		acceptanceScenarios = append(acceptanceScenarios, delivery.FeatureAcceptanceScenario{
			ID: featureID + "-acceptance-" + string(rune('1'+index)), Title: expected,
			Given: []string{"The Product is in the required starting state"}, When: title, Then: []string{expected},
		})
	}
	authorizationMode := "feature_delta"
	authentication := ""
	permissionModel := ""
	roles := []delivery.ProductRole{}
	roleID := "member"
	if featureID == "F-001" {
		authorizationMode = "establish_baseline"
		authentication = "required"
		permissionModel = "fixed"
		roles = demoProductDefinition().Access.Roles
	}
	if featureID == "F-003" {
		roleID = "location_manager"
	}
	return delivery.FeatureSpecification{
		Intent: delivery.FeatureIntent{Problem: summary, DesiredOutcome: summary, SuccessMeasures: cloneStrings(acceptance)},
		Scope:  delivery.FeatureScope{In: []string{title}, Out: []string{"Unrelated product behavior"}},
		Scenarios: []delivery.FeatureScenario{{
			ID: featureID + "-scenario", Title: title, ActorIDs: []string{"member"}, Trigger: summary,
			Preconditions: []string{"The Product is available"}, MainFlow: []string{title}, AlternateFlows: []string{"Return an explicit business error"}, Outcome: summary,
		}},
		Impacts: []delivery.FeatureImpact{{ID: featureID + "-impact", Operation: "change", TargetKind: "action", TargetID: featureID, Summary: summary, Details: []string{title}}},
		Authorization: delivery.FeatureAuthorization{
			Mode: authorizationMode, Authentication: authentication, PermissionModel: permissionModel,
			Roles: roles, Grants: []delivery.FeatureAccessGrant{{ID: featureID + "-grant", RoleIDs: []string{roleID}, CapabilityID: featureID, Capability: title, ScenarioIDs: []string{featureID + "-scenario"}, ActionIDs: []string{featureID}, DataScopes: []string{"Authorized records in the role's Product scope"}, Conditions: []string{}}},
			PermissionAdminIDs: []string{}, AssignmentRules: []string{},
		},
		Acceptance: acceptanceScenarios,
	}
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func DemoDeliveryRun(now time.Time) deliveryrun.DeliveryRun {
	run, err := deliveryrun.NewDeliveryRun(DemoProduct(now), "F-001", 1, deliveryrun.DeliveryRunSpec{
		ID: DemoDeliveryRunID, Name: "Booking Cancellation Delivery", Code: "RUN-18",
		Goal: "Bring booking cancellation to a releasable, accepted, and traceable state.", TargetDate: now.AddDate(0, 0, 21).Format("2006-01-02"),
		Members: []deliveryrun.Member{
			{ID: "m-product", Name: "Product Owner", Roles: []string{deliveryrun.RoleProductOwner}, Initials: "PO"},
			{ID: "m-dev", Name: "Development Lead", Roles: []string{deliveryrun.RoleDevelopmentLead}, Initials: "DL"},
			{ID: "m-qa", Name: "Quality Lead", Roles: []string{deliveryrun.RoleQualityLead}, Initials: "QL"},
			{ID: "m-business", Name: "Business Acceptor", Roles: []string{deliveryrun.RoleBusinessAcceptor}, Initials: "BA"},
			{ID: "m-release", Name: "Release Approver", Roles: []string{deliveryrun.RoleReleaseApprover}, Initials: "RA"},
		},
	}, domain.Actor{ID: "rd-agent", Kind: domain.ActorAgent}, now)
	if err != nil {
		panic(err)
	}
	return run
}
