package product

import (
	"fmt"
	"strings"
	"time"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

func openFeatureDiscovery(product *Product, command Command, now time.Time) error {
	var payload struct {
		FeatureID string `json:"feature_id"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.FeatureID = strings.TrimSpace(payload.FeatureID)
	if payload.FeatureID == "" {
		return Invalid("feature_identity_incomplete", "A Feature discovery workspace requires a stable ID.")
	}
	if FindFeature(product, payload.FeatureID) != nil {
		return Invalid("feature_id_duplicate", "Feature IDs must be unique within a Product.")
	}
	code := nextFeatureCode(product)
	product.Features = append(product.Features, Feature{
		ID:     payload.FeatureID,
		Code:   code,
		Status: FeatureDraft,
		Draft: &FeatureDraftState{
			Version:                 1,
			BaselineProductRevision: product.CurrentDefinitionRevision,
			Discovery:               normalizeFeatureDiscovery(FeatureDiscovery{}),
			Specification:           normalizeFeatureSpecification(FeatureSpecification{}),
			Decisions:               []FeatureDecision{},
			Readiness: FeatureReadiness{
				Status:           "discovering",
				BlockingSections: []string{"intent", "scenario", "scope", "authorization", "acceptance"},
				BlockingIssues:   []string{},
			},
			Sources:   []FeatureSource{},
			UpdatedBy: command.Actor.ID,
			UpdatedAt: now,
		},
		Revisions: []FeatureRevision{},
		CreatedAt: now,
		UpdatedAt: now,
	})
	return nil
}

func nextFeatureCode(product *Product) string {
	used := make(map[string]bool, len(product.Features))
	for _, feature := range product.Features {
		used[feature.Code] = true
	}
	for number := 1; ; number++ {
		code := fmt.Sprintf("F-%03d", number)
		if !used[code] {
			return code
		}
	}
}

func replaceFeatureDiscovery(product *Product, command Command, now time.Time) error {
	var payload struct {
		FeatureID               string               `json:"feature_id"`
		Code                    string               `json:"code"`
		Title                   string               `json:"title"`
		Summary                 string               `json:"summary"`
		Priority                string               `json:"priority"`
		BaselineProductRevision uint64               `json:"baseline_product_revision"`
		Discovery               FeatureDiscovery     `json:"discovery"`
		Specification           FeatureSpecification `json:"specification"`
		Decisions               []FeatureDecision    `json:"decisions"`
		Source                  FeatureSource        `json:"source"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.FeatureID = strings.TrimSpace(payload.FeatureID)
	payload.Code = strings.TrimSpace(payload.Code)
	payload.Title = strings.TrimSpace(payload.Title)
	payload.Summary = strings.TrimSpace(payload.Summary)
	payload.Priority = strings.TrimSpace(payload.Priority)
	payload.Discovery = normalizeFeatureDiscovery(payload.Discovery)
	payload.Specification = normalizeFeatureSpecification(payload.Specification)
	payload.Decisions = normalizeFeatureDecisions(payload.Decisions)
	payload.Source = normalizeFeatureSource(payload.Source)
	if payload.FeatureID == "" || payload.Code == "" {
		return Invalid("feature_identity_incomplete", "A Feature draft requires a stable ID and code.")
	}
	if payload.BaselineProductRevision == 0 || payload.BaselineProductRevision != product.CurrentDefinitionRevision {
		return Invalid("feature_baseline_stale", "A Feature must use the current Product definition revision as its baseline.")
	}
	if err := validateFeatureSource(payload.Source); err != nil {
		return err
	}
	if err := validateFeatureDiscovery(payload.Discovery); err != nil {
		return err
	}
	if err := validateFeatureDecisions(payload.Decisions); err != nil {
		return err
	}
	if err := validateFeatureDiscoveryReferences(payload.Discovery, payload.Decisions); err != nil {
		return err
	}
	nextQuestion := selectNextFeatureQuestion(payload.Discovery.OpenQuestions)
	readiness, err := assessFeatureReadiness(payload.Title, payload.Summary, payload.Discovery, payload.Specification, payload.Decisions, nextQuestion, featureAuthorizationPolicyFor(product, payload.FeatureID))
	if err != nil {
		return err
	}
	if readiness.Status != "ready" && nextQuestion == "" {
		return Invalid("feature_next_question_required", "Incomplete Feature discovery must retain at least one ranked open business question.")
	}
	feature := FindFeature(product, payload.FeatureID)
	if feature == nil {
		for index := range product.Features {
			if product.Features[index].Code == payload.Code {
				return Invalid("feature_code_duplicate", "Feature codes must be unique within a Product.")
			}
		}
		product.Features = append(product.Features, Feature{
			ID: payload.FeatureID, Code: payload.Code, Status: FeatureDraft, Revisions: []FeatureRevision{}, CreatedAt: now,
		})
		feature = &product.Features[len(product.Features)-1]
	} else if feature.Status != FeatureDraft {
		return Invalid("feature_locked", "A confirmed, delivering, or installed Feature cannot be replaced; create a new Feature for new requirements.")
	} else if feature.Code != payload.Code {
		return Invalid("feature_code_immutable", "A Feature code is immutable after creation.")
	}
	draftVersion := uint64(1)
	sources := []FeatureSource{}
	if feature.Draft != nil {
		draftVersion = feature.Draft.Version + 1
		sources = clone(feature.Draft.Sources)
	}
	sources = appendFeatureSource(sources, payload.Source)
	feature.Draft = &FeatureDraftState{
		Version: draftVersion, Title: payload.Title, Summary: payload.Summary, Priority: payload.Priority,
		BaselineProductRevision: payload.BaselineProductRevision, Discovery: clone(payload.Discovery),
		Specification: clone(payload.Specification), Decisions: clone(payload.Decisions), Readiness: readiness,
		NextQuestion: nextQuestion, Sources: sources, UpdatedBy: command.Actor.ID, UpdatedAt: now,
	}
	feature.Status = FeatureDraft
	feature.UpdatedAt = now
	return nil
}

func featureAuthorizationPolicyFor(product *Product, _ string) featureAuthorizationPolicy {
	knownRoleIDs := map[string]bool{}
	if len(product.Revisions) > 0 {
		for _, role := range product.Revisions[len(product.Revisions)-1].Definition.Access.Roles {
			knownRoleIDs[role.ID] = true
		}
	}
	mode := "feature_delta"
	if len(knownRoleIDs) == 0 {
		mode = "establish_baseline"
	}
	return featureAuthorizationPolicy{Mode: mode, KnownRoleIDs: knownRoleIDs}
}

func confirmFeature(product *Product, command Command, now time.Time) error {
	var payload struct {
		FeatureID    string `json:"feature_id"`
		DraftVersion uint64 `json:"draft_version"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	feature := FindFeature(product, strings.TrimSpace(payload.FeatureID))
	if feature == nil {
		return NotFound("Feature", payload.FeatureID)
	}
	if feature.Status != FeatureDraft || feature.Draft == nil {
		return Invalid("feature_not_draft", "Only a Feature with active discovery can be confirmed.")
	}
	if payload.DraftVersion == 0 || payload.DraftVersion != feature.Draft.Version {
		return Invalid("feature_draft_stale", "Feature discovery changed; read the latest draft before confirming it.")
	}
	draft := feature.Draft
	if draft.BaselineProductRevision != product.CurrentDefinitionRevision || draft.BaselineProductRevision != product.CurrentReleaseRevision {
		return Invalid("feature_baseline_stale", "Feature discovery is based on an old ProductRevision; reopen it against the current Product baseline before confirmation.")
	}
	if draft.Readiness.Status != "ready" || len(draft.Readiness.BlockingSections) > 0 || len(draft.Readiness.BlockingIssues) > 0 {
		return Invalid("feature_specification_incomplete", "Resolve every material requirement concern before confirming the Feature.")
	}
	number := feature.CurrentRevision + 1
	feature.Revisions = append(feature.Revisions, FeatureRevision{
		Number: number, Title: draft.Title, Summary: draft.Summary, Priority: draft.Priority,
		BaselineProductRevision: draft.BaselineProductRevision, Discovery: clone(draft.Discovery),
		Specification: clone(draft.Specification), Decisions: clone(draft.Decisions),
		Readiness: clone(draft.Readiness), Sources: clone(draft.Sources),
		CreatedBy: command.Actor.ID, CreatedAt: now,
	})
	feature.CurrentRevision = number
	feature.ConfirmedRevision = number
	feature.DeliverySequence = nextDeliverySequence(product)
	feature.QueuedAt = &now
	feature.Draft = nil
	feature.Status = FeatureConfirmed
	feature.UpdatedAt = now
	return nil
}

type productFeaturePayload struct {
	FeatureID       string `json:"feature_id"`
	FeatureRevision uint64 `json:"feature_revision"`
	DeliveryRunID   string `json:"delivery_run_id,omitempty"`
	ReleaseID       string `json:"release_id,omitempty"`
}

func decodeProductFeaturePayload(command Command) (productFeaturePayload, error) {
	var payload productFeaturePayload
	err := decode(command.Payload, &payload)
	if err != nil {
		return productFeaturePayload{}, err
	}
	return payload, nil
}

func productFeatureRevision(product *Product, payload productFeaturePayload) (*Feature, *FeatureRevision, error) {
	feature := FindFeature(product, strings.TrimSpace(payload.FeatureID))
	if feature == nil {
		return nil, nil, NotFound("Feature", payload.FeatureID)
	}
	if payload.FeatureRevision == 0 || payload.FeatureRevision != feature.CurrentRevision {
		return nil, nil, Invalid("feature_revision_stale", "The Feature revision changed; read it again before submitting.")
	}
	return feature, &feature.Revisions[len(feature.Revisions)-1], nil
}

func ProductProjectionFor(product Product) ProductProjection {
	actions := make([]AvailableAction, 0)
	add := func(command, targetID string) {
		actions = append(actions, catalogAction(command, targetID))
	}
	if product.Status != ProductArchived {
		add(commandFeatureDiscoveryOpen, "")
		add(commandFeatureDiscoveryReplace, "")
		switch product.Engineering.Status {
		case EngineeringFrontendQueued:
			add(commandProductFrontendStart, product.ID)
		case EngineeringFrontendInitializing:
			add(commandProductFrontendFinish, product.ID)
		case EngineeringFoundationPending:
			add(commandProductFoundationStart, product.ID)
		case EngineeringFoundationInstalling:
			add(commandProductFoundationFinish, product.ID)
			add(commandProductFoundationFail, product.ID)
		case EngineeringReady:
		}
		nextDelivery := NextFeatureForDelivery(&product)
		for _, feature := range product.Features {
			switch feature.Status {
			case FeatureDraft:
				if feature.Draft != nil && feature.Draft.Readiness.Status == "ready" {
					add(commandFeatureConfirm, feature.ID)
				}
			case FeatureConfirmed:
				if product.Engineering.Status == EngineeringReady && nextDelivery != nil && nextDelivery.ID == feature.ID {
					add(commandFeatureDelivery, feature.ID)
				}
			case FeatureDelivering:
			case FeatureInstalled:
			}
		}
		add(commandProductDelete, product.ID)
	}
	return ProductProjection{Product: product, AvailableActions: actions}
}

func nextDeliverySequence(product *Product) uint64 {
	var highest uint64
	for _, feature := range product.Features {
		if feature.DeliverySequence > highest {
			highest = feature.DeliverySequence
		}
	}
	return highest + 1
}

func NextFeatureForDelivery(product *Product) *Feature {
	for index := range product.Features {
		if product.Features[index].Status == FeatureDelivering {
			return nil
		}
	}
	var next *Feature
	for index := range product.Features {
		feature := &product.Features[index]
		if feature.Status != FeatureConfirmed || feature.DeliverySequence == 0 {
			continue
		}
		if next == nil || feature.DeliverySequence < next.DeliverySequence {
			next = feature
		}
	}
	return next
}

func ProductAgentContextFor(product Product) ProductAgentContext {
	projection := ProductProjectionFor(product)
	actions := make([]AvailableAction, 0)
	commands := make([]string, 0)
	seen := map[string]bool{}
	for _, action := range projection.AvailableActions {
		if action.ActorKind != ActorAgent {
			continue
		}
		actions = append(actions, action)
		if !seen[action.Command] {
			commands = append(commands, action.Command)
			seen[action.Command] = true
		}
	}
	return ProductAgentContext{
		Product: product, AllowedAgentCommands: commands, AvailableActions: actions,
		HumanOnlyCommands: commanddomain.KeysFor(commanddomain.TargetProduct, ActorHuman), SystemOnlyCommands: commanddomain.KeysFor(commanddomain.TargetProduct, ActorSystem),
		SourcePolicy: "Product engineering initialization is outside the business Feature lifecycle and is never represented as a synthetic requirement. Its frontend phase creates the industry-informed design contract, sign-in presentation, responsive shell, and real build. Its one-time foundation phase installs a signed Go source foundation around the single backend/model.json and verifies the exact Application Delivery, release, package, model, clean Git revision, local check, and Identity baseline evidence. Plane is not used again after this installation. Feature discovery must distinguish sourced evidence, PM inference, assumptions, conflicts, options, and human decisions. Delivery ranks candidate questions and freezes all contributing Conversation, Run, source, and decision references only when the Product owner confirms the Feature. Product-wide facts belong to ProductDefinition; a Feature records only the business delta. Business implementation belongs to a DeliveryRun owned by RD, QA, and OP.",
	}
}
