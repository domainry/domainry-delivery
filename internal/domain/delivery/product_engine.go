package delivery

import (
	"fmt"
	"strings"
	"time"
)

const (
	commandProductCreate           = "product.create"
	commandProductDelete           = "product.delete"
	commandProductFrontendStart    = "product.engineering.frontend.start"
	commandProductFrontendFinish   = "product.engineering.frontend.complete"
	commandFeatureDiscoveryOpen    = "feature.discovery.open"
	commandFeatureDiscoveryReplace = "feature.discovery.replace"
	commandFeatureConfirm          = "feature.confirm"
	commandFeatureDelivery         = "feature.delivery.start"
	commandFeatureInstall          = "feature.install"
)

func NewProduct(workspaceID, productID string, command Command, now time.Time) (Product, error) {
	if command.Type != commandProductCreate {
		return Product{}, Invalid("command_unknown", "Product creation requires the product.create command.")
	}
	if err := validateActor(command.Actor); err != nil {
		return Product{}, err
	}
	if command.Actor.Kind == ActorSystem {
		return Product{}, Invalid("actor_unauthorized", "A system executor cannot create a Product.")
	}
	var payload struct {
		Name       string            `json:"name"`
		Code       string            `json:"code"`
		Goal       string            `json:"goal"`
		Industry   string            `json:"industry"`
		Story      ProductStory      `json:"story"`
		Definition ProductDefinition `json:"definition"`
		Decisions  []ProductDecision `json:"decisions"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return Product{}, err
	}
	productID = strings.TrimSpace(productID)
	workspaceID = strings.TrimSpace(workspaceID)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Code = strings.TrimSpace(payload.Code)
	payload.Goal = strings.TrimSpace(payload.Goal)
	payload.Industry = strings.TrimSpace(payload.Industry)
	payload.Definition = normalizeProductDefinition(payload.Definition)
	if workspaceID == "" || productID == "" || payload.Name == "" || payload.Code == "" || payload.Goal == "" || payload.Industry == "" {
		return Product{}, Invalid("product_incomplete", "A Product requires a Workspace, ID, name, code, goal, and industry.")
	}
	content := ProductRevisionContent{Story: payload.Story, Definition: payload.Definition, Decisions: payload.Decisions}
	if err := validateProductRevisionContent(content, false); err != nil {
		return Product{}, err
	}
	return Product{
		ID:          productID,
		WorkspaceID: workspaceID,
		Name:        payload.Name,
		Code:        payload.Code,
		Goal:        payload.Goal,
		Industry:    payload.Industry,
		Engineering: ProductEngineering{
			Status: EngineeringFrontendQueued,
		},
		Status:                    ProductShaping,
		Revision:                  1,
		CurrentDefinitionRevision: 1,
		CurrentReleaseRevision:    1,
		Revisions: []ProductRevision{{
			Number: 1, Story: clone(content.Story), Definition: clone(content.Definition), Decisions: clone(content.Decisions),
			CreatedBy: command.Actor.ID, CreatedAt: now,
		}},
		Features:  []Feature{},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func ApplyProduct(product *Product, command Command, now time.Time) error {
	if product.Status == ProductArchived {
		return Invalid("product_archived", "An archived Product cannot be changed.")
	}
	if err := validateActor(command.Actor); err != nil {
		return err
	}
	var err error
	switch command.Type {
	case commandProductDelete:
		if command.Actor.Kind != ActorHuman {
			return Invalid("human_deletion_required", "Only an authenticated Product owner can delete a Product.")
		}
		product.Status = ProductArchived
	case commandProductFrontendStart:
		if command.Actor.Kind != ActorAgent {
			return Invalid("agent_execution_required", "A local RD Agent must start Product engineering initialization.")
		}
		err = startProductFrontend(product, command, now)
	case commandProductFrontendFinish:
		if command.Actor.Kind != ActorAgent {
			return Invalid("agent_execution_required", "A local RD Agent must complete Product engineering initialization.")
		}
		err = completeProductFrontend(product, command, now)
	case commandFeatureDiscoveryOpen:
		if command.Actor.Kind != ActorHuman {
			return Invalid("actor_unauthorized", "Only an authenticated Product owner can open a Feature discovery workspace.")
		}
		err = openFeatureDiscovery(product, command, now)
	case commandFeatureDiscoveryReplace:
		if command.Actor.Kind == ActorSystem {
			return Invalid("actor_unauthorized", "A system executor cannot author a Feature.")
		}
		err = replaceFeatureDiscovery(product, command, now)
	case commandFeatureConfirm:
		if command.Actor.Kind != ActorHuman {
			return Invalid("human_confirmation_required", "A Feature must be confirmed by an authenticated Product owner.")
		}
		err = confirmFeature(product, command, now)
	default:
		err = Invalid("command_unknown", "The Product command is not supported.")
	}
	if err != nil {
		return err
	}
	product.UpdatedAt = now
	return nil
}

func startProductFrontend(product *Product, command Command, now time.Time) error {
	if product.Engineering.Status != EngineeringFrontendQueued {
		return Invalid("product_frontend_not_queued", "Only queued frontend initialization can start.")
	}
	product.Engineering.Status = EngineeringFrontendInitializing
	product.Engineering.StartedBy = command.Actor.ID
	product.Engineering.StartedAt = &now
	return nil
}

func completeProductFrontend(product *Product, command Command, now time.Time) error {
	if product.Engineering.Status != EngineeringFrontendInitializing {
		return Invalid("product_frontend_not_initializing", "Only frontend initialization in progress can complete.")
	}
	var payload struct {
		CodeRevision      string `json:"code_revision"`
		ArtifactRef       string `json:"artifact_ref"`
		DesignContractRef string `json:"design_contract_ref"`
		LoginEntry        string `json:"login_entry"`
		ShellEntry        string `json:"shell_entry"`
		PreviewEntry      string `json:"preview_entry"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.CodeRevision = strings.TrimSpace(payload.CodeRevision)
	payload.ArtifactRef = strings.TrimSpace(payload.ArtifactRef)
	payload.DesignContractRef = strings.TrimSpace(payload.DesignContractRef)
	payload.LoginEntry = strings.TrimSpace(payload.LoginEntry)
	payload.ShellEntry = strings.TrimSpace(payload.ShellEntry)
	payload.PreviewEntry = strings.TrimSpace(payload.PreviewEntry)
	if payload.CodeRevision == "" || payload.ArtifactRef == "" || payload.DesignContractRef == "" || payload.LoginEntry == "" || payload.ShellEntry == "" || payload.PreviewEntry == "" {
		return Invalid("product_frontend_evidence_incomplete", "Frontend initialization requires a code revision, build evidence, design contract evidence, login entry, shell entry, and reviewable preview entry.")
	}
	product.Engineering.Status = EngineeringReady
	product.Engineering.FrontendCodeRevision = payload.CodeRevision
	product.Engineering.FrontendArtifactRef = payload.ArtifactRef
	product.Engineering.DesignContractRef = payload.DesignContractRef
	product.Engineering.LoginEntry = payload.LoginEntry
	product.Engineering.ShellEntry = payload.ShellEntry
	product.Engineering.PreviewEntry = payload.PreviewEntry
	product.Engineering.CompletedBy = command.Actor.ID
	product.Engineering.CompletedAt = &now
	return nil
}

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
	if findFeature(product, payload.FeatureID) != nil {
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
		Attachments: []FeatureAttachment{},
		Revisions:   []FeatureRevision{},
		CreatedAt:   now,
		UpdatedAt:   now,
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
	feature := findFeature(product, payload.FeatureID)
	if feature == nil {
		for index := range product.Features {
			if product.Features[index].Code == payload.Code {
				return Invalid("feature_code_duplicate", "Feature codes must be unique within a Product.")
			}
		}
		product.Features = append(product.Features, Feature{
			ID: payload.FeatureID, Code: payload.Code, Status: FeatureDraft, Attachments: []FeatureAttachment{}, Revisions: []FeatureRevision{}, CreatedAt: now,
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

func featureAuthorizationPolicyFor(product *Product, featureID string) featureAuthorizationPolicy {
	firstFeature := len(product.Features) == 0
	knownRoleIDs := map[string]bool{}
	if len(product.Revisions) > 0 {
		for _, role := range product.Revisions[len(product.Revisions)-1].Definition.Access.Roles {
			knownRoleIDs[role.ID] = true
		}
	}
	for index := range product.Features {
		feature := &product.Features[index]
		if feature.ID == featureID {
			firstFeature = index == 0
			continue
		}
		if feature.Draft != nil {
			for _, role := range feature.Draft.Specification.Authorization.Roles {
				knownRoleIDs[role.ID] = true
			}
		}
		if len(feature.Revisions) > 0 {
			for _, role := range feature.Revisions[len(feature.Revisions)-1].Specification.Authorization.Roles {
				knownRoleIDs[role.ID] = true
			}
		}
	}
	mode := "feature_delta"
	if firstFeature {
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
	feature := findFeature(product, strings.TrimSpace(payload.FeatureID))
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
	if draft.Readiness.Status != "ready" || len(draft.Readiness.BlockingSections) > 0 || len(draft.Readiness.BlockingIssues) > 0 {
		return Invalid("feature_specification_incomplete", "Resolve every material requirement concern before confirming the Feature.")
	}
	number := feature.CurrentRevision + 1
	feature.Revisions = append(feature.Revisions, FeatureRevision{
		Number: number, Title: draft.Title, Summary: draft.Summary, Priority: draft.Priority,
		BaselineProductRevision: draft.BaselineProductRevision, Discovery: clone(draft.Discovery),
		Specification: clone(draft.Specification), Decisions: clone(draft.Decisions),
		Readiness: clone(draft.Readiness), Sources: clone(draft.Sources), Attachments: clone(feature.Attachments),
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
	feature := findFeature(product, strings.TrimSpace(payload.FeatureID))
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
	add := func(command, targetID string, actorKind ActorKind) {
		actions = append(actions, AvailableAction{Command: command, TargetID: targetID, ActorKind: actorKind})
	}
	if product.Status != ProductArchived {
		add(commandFeatureDiscoveryOpen, "", ActorHuman)
		add(commandFeatureDiscoveryReplace, "", ActorAgent)
		switch product.Engineering.Status {
		case EngineeringFrontendQueued:
			add(commandProductFrontendStart, product.ID, ActorAgent)
		case EngineeringFrontendInitializing:
			add(commandProductFrontendFinish, product.ID, ActorAgent)
		case EngineeringReady:
		}
		nextDelivery := nextFeatureForDelivery(&product)
		for _, feature := range product.Features {
			switch feature.Status {
			case FeatureDraft:
				if feature.Draft != nil && feature.Draft.Readiness.Status == "ready" {
					add(commandFeatureConfirm, feature.ID, ActorHuman)
				}
			case FeatureConfirmed:
				if product.Engineering.Status == EngineeringReady && nextDelivery != nil && nextDelivery.ID == feature.ID {
					add(commandFeatureDelivery, feature.ID, ActorAgent)
				}
			case FeatureDelivering:
			case FeatureInstalled:
			}
		}
		add(commandProductDelete, product.ID, ActorHuman)
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

func nextFeatureForDelivery(product *Product) *Feature {
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
		HumanOnlyCommands: []string{commandFeatureDiscoveryOpen, commandFeatureConfirm}, SystemOnlyCommands: []string{},
		SourcePolicy: "Product engineering initialization is outside the business Feature lifecycle and is never represented as a synthetic requirement. Its frontend phase creates the industry-informed design contract, sign-in presentation, responsive shell, and real build. Its backend phase creates a Rust service foundation, explicit API contract, authentication and configurable authorization primitives, health boundary, and real tests without inventing business roles or workflows. Feature discovery must distinguish sourced evidence, PM inference, assumptions, conflicts, options, and human decisions. Delivery ranks candidate questions and freezes all contributing Conversation, Run, source, and decision references only when the Product owner confirms the Feature. Product-wide facts belong to ProductDefinition; a Feature records only the business delta. Business implementation belongs to a DeliveryRun owned by RD, QA, and OP.",
	}
}

func validateFeatureDecisions(decisions []FeatureDecision) error {
	ids := map[string]bool{}
	for _, decision := range decisions {
		if decision.ID == "" || decision.Title == "" || decision.Question == "" || !discoveryLevels[decision.Impact] || decision.Rationale == "" || len(decision.SourceIDs) == 0 {
			return Invalid("feature_decision_incomplete", "A Feature decision requires an ID, title, question, impact, rationale, and source references.")
		}
		if decision.Status != "open" && decision.Status != "resolved" {
			return Invalid("feature_decision_status_invalid", "A Feature decision status must be open or resolved.")
		}
		if ids[decision.ID] {
			return Invalid("feature_decision_duplicate", "Feature decision IDs must be unique.")
		}
		ids[decision.ID] = true
		optionIDs := map[string]bool{}
		for _, option := range decision.Options {
			if option.ID == "" || option.Label == "" || option.Description == "" {
				return Invalid("feature_decision_option_incomplete", "Every Feature decision option requires an ID, label, and description.")
			}
			if optionIDs[option.ID] {
				return Invalid("feature_decision_option_duplicate", "Feature decision option IDs must be unique within a decision.")
			}
			optionIDs[option.ID] = true
		}
		if len(decision.Options) < 2 {
			return Invalid("feature_decision_options_missing", "A Feature decision must retain at least two explicit business options.")
		}
		if decision.RecommendedOptionID != "" && !optionIDs[decision.RecommendedOptionID] {
			return Invalid("feature_decision_recommendation_invalid", "A Feature recommendation must reference one of its options.")
		}
		if decision.Status == "resolved" && (decision.Resolution == "" || !optionIDs[decision.SelectedOptionID]) {
			return Invalid("feature_decision_resolution_incomplete", "A resolved Feature decision requires a selected option and resolution.")
		}
	}
	return nil
}

func normalizeFeatureDecisions(decisions []FeatureDecision) []FeatureDecision {
	if decisions == nil {
		return []FeatureDecision{}
	}
	for index := range decisions {
		decision := &decisions[index]
		decision.ID = strings.TrimSpace(decision.ID)
		decision.Title = strings.TrimSpace(decision.Title)
		decision.Question = strings.TrimSpace(decision.Question)
		decision.Impact = strings.TrimSpace(decision.Impact)
		if decision.Options == nil {
			decision.Options = []DecisionOption{}
		}
		for optionIndex := range decision.Options {
			option := &decision.Options[optionIndex]
			option.ID = strings.TrimSpace(option.ID)
			option.Label = strings.TrimSpace(option.Label)
			option.Description = strings.TrimSpace(option.Description)
		}
		decision.RecommendedOptionID = strings.TrimSpace(decision.RecommendedOptionID)
		decision.SelectedOptionID = strings.TrimSpace(decision.SelectedOptionID)
		decision.Resolution = strings.TrimSpace(decision.Resolution)
		decision.Rationale = strings.TrimSpace(decision.Rationale)
		decision.Tradeoffs = cleanStrings(decision.Tradeoffs)
		decision.Status = strings.TrimSpace(decision.Status)
		decision.SourceIDs = cleanStrings(decision.SourceIDs)
	}
	return decisions
}

func appendFeatureSource(sources []FeatureSource, source FeatureSource) []FeatureSource {
	for index := range sources {
		if sources[index].ConversationID == source.ConversationID && sources[index].RunID == source.RunID {
			sources[index] = source
			return sources
		}
	}
	return append(sources, source)
}

func normalizeFeatureSource(source FeatureSource) FeatureSource {
	source.ConversationID = strings.TrimSpace(source.ConversationID)
	source.RunID = strings.TrimSpace(source.RunID)
	source.SourceIDs = cleanStrings(source.SourceIDs)
	source.DecisionIDs = cleanStrings(source.DecisionIDs)
	return source
}

func validateActor(actor Actor) error {
	if strings.TrimSpace(actor.ID) == "" {
		return Invalid("actor_required", "An authenticated actor is required.")
	}
	if actor.Kind != ActorHuman && actor.Kind != ActorAgent && actor.Kind != ActorSystem {
		return Invalid("actor_kind_invalid", "The actor kind is invalid.")
	}
	return nil
}

func validateFeatureSource(source FeatureSource) error {
	if strings.TrimSpace(source.ConversationID) == "" || strings.TrimSpace(source.RunID) == "" || source.BeforeStep < 0 {
		return Invalid("feature_source_incomplete", "Feature discovery must bind an exact Conversation, Run, and valid step boundary.")
	}
	if len(cleanStrings(source.SourceIDs)) == 0 {
		return Invalid("feature_evidence_missing", "Feature discovery must retain at least one business source.")
	}
	return nil
}

func validateProductRevisionContent(content ProductRevisionContent, requireResolvedDecisions bool) error {
	content.Story.Title = strings.TrimSpace(content.Story.Title)
	content.Story.Summary = strings.TrimSpace(content.Story.Summary)
	content.Story.Narrative = strings.TrimSpace(content.Story.Narrative)
	if content.Story.Title == "" || content.Story.Summary == "" || content.Story.Narrative == "" {
		return Invalid("product_story_incomplete", "A ProductStory requires a title, summary, and complete narrative.")
	}
	if err := validateProductDefinition(content.Definition); err != nil {
		return err
	}
	decisionIDs := map[string]bool{}
	for _, decision := range content.Decisions {
		if strings.TrimSpace(decision.ID) == "" || strings.TrimSpace(decision.Title) == "" || strings.TrimSpace(decision.Question) == "" {
			return Invalid("product_decision_incomplete", "A Product decision requires an ID, title, and question.")
		}
		if decisionIDs[decision.ID] {
			return Invalid("product_decision_duplicate", "Product decision IDs must be unique.")
		}
		decisionIDs[decision.ID] = true
		if decision.Status != "open" && decision.Status != "resolved" {
			return Invalid("product_decision_status_invalid", "A Product decision status must be open or resolved.")
		}
		optionIDs := map[string]bool{}
		for _, option := range decision.Options {
			if err := addUniqueID(optionIDs, option.ID, "Decision option"); err != nil {
				return err
			}
			if strings.TrimSpace(option.Label) == "" {
				return Invalid("product_decision_option_incomplete", "A decision option requires a label.")
			}
		}
		if requireResolvedDecisions && decision.Status != "resolved" {
			return Invalid("product_decision_open", "All Product decisions must be resolved before confirming a Feature.")
		}
		if decision.Status == "resolved" && !decisionHasOption(decision) {
			return Invalid("product_decision_selection_invalid", "A resolved decision must select a valid option.")
		}
	}
	return nil
}

func validateProductDefinition(definition ProductDefinition) error {
	if definition.SchemaVersion != 2 {
		return Invalid("product_definition_schema_invalid", "ProductDefinition schema_version must be 2.")
	}
	actors := map[string]bool{}
	for _, actor := range definition.Actors {
		if err := addUniqueID(actors, actor.ID, "Actor"); err != nil {
			return err
		}
		if strings.TrimSpace(actor.Name) == "" || strings.TrimSpace(actor.Responsibility) == "" {
			return Invalid("product_actor_incomplete", "An Actor requires a name and responsibility.")
		}
	}
	scenarios := map[string]bool{}
	for _, scenario := range definition.Scenarios {
		if err := addUniqueID(scenarios, scenario.ID, "Scenario"); err != nil {
			return err
		}
		if strings.TrimSpace(scenario.Title) == "" || strings.TrimSpace(scenario.Trigger) == "" || strings.TrimSpace(scenario.Outcome) == "" {
			return Invalid("product_scenario_incomplete", "A Scenario requires a title, trigger, and outcome.")
		}
		steps := map[string]bool{}
		for _, step := range scenario.Steps {
			if err := addUniqueID(steps, step.ID, "Scenario step"); err != nil {
				return err
			}
			if strings.TrimSpace(step.Title) == "" || (step.ActorID != "" && !actors[step.ActorID]) {
				return Invalid("product_step_invalid", "A Scenario step requires a title and must reference a valid Actor.")
			}
		}
	}
	objects := map[string]BusinessObject{}
	for _, object := range definition.Objects {
		if strings.TrimSpace(object.ID) == "" || strings.TrimSpace(object.Name) == "" {
			return Invalid("business_object_incomplete", "A business object requires an ID and name.")
		}
		if _, exists := objects[object.ID]; exists {
			return Invalid("business_object_duplicate", "Business object IDs must be unique.")
		}
		fields := map[string]bool{}
		for _, field := range object.Fields {
			if err := addUniqueID(fields, field.Key, "Business field"); err != nil {
				return err
			}
			if strings.TrimSpace(field.Label) == "" || strings.TrimSpace(field.Type) == "" {
				return Invalid("business_field_incomplete", "A business field requires a label and type.")
			}
		}
		states := map[string]bool{}
		for _, state := range object.States {
			if err := addUniqueID(states, state.Value, "Business state"); err != nil {
				return err
			}
			if strings.TrimSpace(state.Label) == "" {
				return Invalid("business_state_incomplete", "A business state requires a label.")
			}
		}
		if !fields[object.PrimaryFieldKey] || !fields[object.StateFieldKey] || !states[object.InitialState] {
			return Invalid("business_object_reference_invalid", "A business object must reference a valid primary field, state field, and initial state.")
		}
		objects[object.ID] = object
	}
	rules := map[string]BusinessRule{}
	for _, rule := range definition.Rules {
		if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.Title) == "" || strings.TrimSpace(rule.Statement) == "" {
			return Invalid("business_rule_incomplete", "A business rule requires an ID, title, and statement.")
		}
		if _, exists := rules[rule.ID]; exists {
			return Invalid("business_rule_duplicate", "Business rule IDs must be unique.")
		}
		if rule.Condition != nil && rule.ObjectID == "" {
			return Invalid("business_rule_reference_invalid", "A field-based business rule must reference a business object.")
		}
		if rule.ObjectID != "" {
			object, exists := objects[rule.ObjectID]
			if !exists || (rule.Condition != nil && (!objectHasField(object, rule.Condition.FieldKey) || !validRuleOperator(rule.Condition.Operator))) {
				return Invalid("business_rule_reference_invalid", "A business rule must reference a valid object and field.")
			}
		}
		rules[rule.ID] = rule
	}
	actions := map[string]ProductAction{}
	for _, action := range definition.Actions {
		object, exists := objects[action.ObjectID]
		if strings.TrimSpace(action.ID) == "" || strings.TrimSpace(action.Label) == "" || !validActionKind(action.Kind) || !exists {
			return Invalid("product_action_incomplete", "A Product action requires an ID, label, kind, and valid object reference.")
		}
		if _, duplicate := actions[action.ID]; duplicate {
			return Invalid("product_action_duplicate", "Product action IDs must be unique.")
		}
		for _, fieldKey := range action.RequiredFieldKeys {
			if !objectHasField(object, fieldKey) {
				return Invalid("product_action_field_invalid", "A Product action references an invalid field.")
			}
		}
		for _, ruleID := range action.RuleIDs {
			if _, exists := rules[ruleID]; !exists {
				return Invalid("product_action_rule_invalid", "A Product action references an invalid rule.")
			}
		}
		for _, state := range action.FromStates {
			if !objectHasState(object, state) {
				return Invalid("product_action_state_invalid", "A Product action references an invalid source state.")
			}
		}
		if action.ToState != "" && !objectHasState(object, action.ToState) {
			return Invalid("product_action_state_invalid", "A Product action references an invalid target state.")
		}
		actions[action.ID] = action
	}
	pages := map[string]bool{}
	for _, page := range definition.Pages {
		object, exists := objects[page.ObjectID]
		if err := addUniqueID(pages, page.ID, "Product page"); err != nil {
			return err
		}
		if strings.TrimSpace(page.Title) == "" || !validPageKind(page.Kind) || !exists {
			return Invalid("product_page_incomplete", "A Product page requires a title, kind, and valid object reference.")
		}
		for _, fieldKey := range page.VisibleFieldKeys {
			if !objectHasField(object, fieldKey) {
				return Invalid("product_page_field_invalid", "A Product page references an invalid field.")
			}
		}
		if page.CreateActionID != "" {
			action, exists := actions[page.CreateActionID]
			if !exists || action.ObjectID != page.ObjectID {
				return Invalid("product_page_action_invalid", "A Product page must reference a valid create action for the same business object.")
			}
		}
	}
	exceptions := map[string]bool{}
	for _, exception := range definition.Exceptions {
		if err := addUniqueID(exceptions, exception.ID, "Business exception"); err != nil {
			return err
		}
		if strings.TrimSpace(exception.ID) == "" || strings.TrimSpace(exception.Title) == "" || strings.TrimSpace(exception.Trigger) == "" || strings.TrimSpace(exception.Handling) == "" {
			return Invalid("business_exception_incomplete", "A business exception requires an ID, title, trigger, and handling instruction.")
		}
		if exception.ScenarioID != "" && !scenarios[exception.ScenarioID] {
			return Invalid("business_exception_scenario_invalid", "A business exception references an invalid Scenario.")
		}
	}
	return validateProductDefinitionExtensions(definition, actors)
}

func decisionHasOption(decision ProductDecision) bool {
	for _, option := range decision.Options {
		if option.ID == decision.SelectedOptionID && strings.TrimSpace(option.Label) != "" {
			return true
		}
	}
	return false
}

func addUniqueID(ids map[string]bool, id, label string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return Invalid("product_definition_id_missing", fmt.Sprintf("%s ID cannot be empty.", label))
	}
	if ids[id] {
		return Invalid("product_definition_id_duplicate", fmt.Sprintf("%s IDs must be unique.", label))
	}
	ids[id] = true
	return nil
}

func objectHasField(object BusinessObject, fieldKey string) bool {
	for _, field := range object.Fields {
		if field.Key == fieldKey {
			return true
		}
	}
	return false
}

func objectHasState(object BusinessObject, stateValue string) bool {
	for _, state := range object.States {
		if state.Value == stateValue {
			return true
		}
	}
	return false
}

func validRuleOperator(operator string) bool {
	return operator == "eq" || operator == "neq" || operator == "truthy" || operator == "gte" || operator == "lte"
}

func validActionKind(kind string) bool {
	return kind == "create" || kind == "update" || kind == "transition"
}

func validPageKind(kind string) bool {
	return kind == "list" || kind == "form" || kind == "detail" || kind == "dashboard"
}

func findFeature(product *Product, id string) *Feature {
	for index := range product.Features {
		if product.Features[index].ID == id {
			return &product.Features[index]
		}
	}
	return nil
}
