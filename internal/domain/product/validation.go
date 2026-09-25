package product

import (
	"slices"
	"strings"
)

func validateFeatureDecisions(decisions []FeatureDecision) error {
	ids := map[string]bool{}
	for _, decision := range decisions {
		if decision.ID == "" || decision.Title == "" || decision.Question == "" || !discoveryLevels[decision.Impact] || decision.Rationale == "" || len(decision.SourceIDs) == 0 {
			return Invalid("feature_decision_incomplete")
		}
		if decision.Status != "open" && decision.Status != "resolved" {
			return Invalid("feature_decision_status_invalid")
		}
		if ids[decision.ID] {
			return Invalid("feature_decision_duplicate")
		}
		ids[decision.ID] = true
		optionIDs := map[string]bool{}
		for _, option := range decision.Options {
			if option.ID == "" || option.Label == "" || option.Description == "" {
				return Invalid("feature_decision_option_incomplete")
			}
			if optionIDs[option.ID] {
				return Invalid("feature_decision_option_duplicate")
			}
			optionIDs[option.ID] = true
		}
		if len(decision.Options) < 2 {
			return Invalid("feature_decision_options_missing")
		}
		if decision.RecommendedOptionID != "" && !optionIDs[decision.RecommendedOptionID] {
			return Invalid("feature_decision_recommendation_invalid")
		}
		if decision.Status == "resolved" && (decision.Resolution == "" || !optionIDs[decision.SelectedOptionID]) {
			return Invalid("feature_decision_resolution_incomplete")
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

// validateCurrentFeatureSourceDecisions keeps Delivery-owned decision identity
// out of the Agent source contract. The current source declaration must name
// exactly the decisions whose business evidence points at that source.
func validateCurrentFeatureSourceDecisions(source FeatureSource, decisions []FeatureDecision) error {
	expected := featureDecisionIDsForSources(decisions, source.SourceIDs)
	actual := cleanStrings(source.DecisionIDs)
	slices.Sort(expected)
	slices.Sort(actual)
	if !slices.Equal(expected, actual) {
		return Invalid("feature_source_decision_mismatch")
	}
	return nil
}

func reconcileFeatureSourceDecisions(sources []FeatureSource, decisions []FeatureDecision) []FeatureSource {
	for index := range sources {
		sources[index].DecisionIDs = featureDecisionIDsForSources(decisions, sources[index].SourceIDs)
	}
	return sources
}

func featureDecisionIDsForSources(decisions []FeatureDecision, sourceIDs []string) []string {
	sourceSet := stringSet(sourceIDs)
	decisionIDs := make([]string, 0)
	for _, decision := range decisions {
		for _, sourceID := range decision.SourceIDs {
			if sourceSet[sourceID] {
				decisionIDs = append(decisionIDs, decision.ID)
				break
			}
		}
	}
	return cleanStrings(decisionIDs)
}

// validateFeatureLineageReferences enforces Delivery's provenance boundary:
// every business fact and decision in the draft must close over the immutable
// source identities supplied by the producer. Delivery preserves those opaque
// references and does not read the Agent owner during its write transaction.
func validateFeatureLineageReferences(discovery FeatureDiscovery, specification FeatureSpecification, decisions []FeatureDecision, sources []FeatureSource) error {
	registered := map[string]bool{}
	for _, source := range sources {
		for _, sourceID := range source.SourceIDs {
			registered[sourceID] = true
		}
	}
	used := make([]string, 0)
	for _, evidence := range discovery.Evidence {
		used = append(used, evidence.SourceIDs...)
	}
	for _, assumption := range discovery.Assumptions {
		used = append(used, assumption.SourceIDs...)
	}
	for _, role := range specification.Authorization.Roles {
		used = append(used, role.SourceIDs...)
	}
	for _, scenario := range specification.Scenarios {
		used = append(used, scenario.SourceIDs...)
	}
	for _, impact := range specification.Impacts {
		used = append(used, impact.SourceIDs...)
	}
	for _, acceptance := range specification.Acceptance {
		used = append(used, acceptance.SourceIDs...)
	}
	for _, decision := range decisions {
		used = append(used, decision.SourceIDs...)
	}
	for _, sourceID := range cleanStrings(used) {
		if !registered[sourceID] {
			return Invalid("feature_source_reference_unverified")
		}
	}
	return nil
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range cleanStrings(values) {
		result[value] = true
	}
	return result
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
		return Invalid("actor_required")
	}
	if actor.Kind != ActorHuman && actor.Kind != ActorAgent && actor.Kind != ActorSystem {
		return Invalid("actor_kind_invalid")
	}
	return nil
}

func validateFeatureSource(source FeatureSource) error {
	if strings.TrimSpace(source.ConversationID) == "" || strings.TrimSpace(source.RunID) == "" || source.BeforeStep < 0 {
		return Invalid("feature_source_incomplete")
	}
	if len(cleanStrings(source.SourceIDs)) == 0 {
		return Invalid("feature_evidence_missing")
	}
	return nil
}

func ValidateProductRevisionContent(content ProductRevisionContent, requireResolvedDecisions bool) error {
	content.Story.Title = strings.TrimSpace(content.Story.Title)
	content.Story.Summary = strings.TrimSpace(content.Story.Summary)
	content.Story.Narrative = strings.TrimSpace(content.Story.Narrative)
	if content.Story.Title == "" || content.Story.Summary == "" || content.Story.Narrative == "" {
		return Invalid("product_story_incomplete")
	}
	if err := validateProductDefinition(content.Definition); err != nil {
		return err
	}
	decisionIDs := map[string]bool{}
	for _, decision := range content.Decisions {
		if strings.TrimSpace(decision.ID) == "" || strings.TrimSpace(decision.Title) == "" || strings.TrimSpace(decision.Question) == "" {
			return Invalid("product_decision_incomplete")
		}
		if decisionIDs[decision.ID] {
			return Invalid("product_decision_duplicate")
		}
		decisionIDs[decision.ID] = true
		if decision.Status != "open" && decision.Status != "resolved" {
			return Invalid("product_decision_status_invalid")
		}
		optionIDs := map[string]bool{}
		for _, option := range decision.Options {
			if err := addUniqueID(optionIDs, option.ID, "Decision option"); err != nil {
				return err
			}
			if strings.TrimSpace(option.Label) == "" {
				return Invalid("product_decision_option_incomplete")
			}
		}
		if requireResolvedDecisions && decision.Status != "resolved" {
			return Invalid("product_decision_open")
		}
		if decision.Status == "resolved" && !decisionHasOption(decision) {
			return Invalid("product_decision_selection_invalid")
		}
	}
	return nil
}

func validateProductDefinition(definition ProductDefinition) error {
	if definition.SchemaVersion != 2 {
		return Invalid("product_definition_schema_invalid")
	}
	actors := map[string]bool{}
	for _, actor := range definition.Actors {
		if err := addUniqueID(actors, actor.ID, "Actor"); err != nil {
			return err
		}
		if strings.TrimSpace(actor.Name) == "" || strings.TrimSpace(actor.Responsibility) == "" {
			return Invalid("product_actor_incomplete")
		}
	}
	scenarios := map[string]bool{}
	for _, scenario := range definition.Scenarios {
		if err := addUniqueID(scenarios, scenario.ID, "Scenario"); err != nil {
			return err
		}
		if strings.TrimSpace(scenario.Title) == "" || strings.TrimSpace(scenario.Trigger) == "" || strings.TrimSpace(scenario.Outcome) == "" {
			return Invalid("product_scenario_incomplete")
		}
		steps := map[string]bool{}
		for _, step := range scenario.Steps {
			if err := addUniqueID(steps, step.ID, "Scenario step"); err != nil {
				return err
			}
			if strings.TrimSpace(step.Title) == "" || (step.ActorID != "" && !actors[step.ActorID]) {
				return Invalid("product_step_invalid")
			}
		}
	}
	objects := map[string]BusinessObject{}
	for _, object := range definition.Objects {
		if strings.TrimSpace(object.ID) == "" || strings.TrimSpace(object.Name) == "" {
			return Invalid("business_object_incomplete")
		}
		if _, exists := objects[object.ID]; exists {
			return Invalid("business_object_duplicate")
		}
		fields := map[string]bool{}
		for _, field := range object.Fields {
			if err := addUniqueID(fields, field.Key, "Business field"); err != nil {
				return err
			}
			if strings.TrimSpace(field.Label) == "" || strings.TrimSpace(field.Type) == "" {
				return Invalid("business_field_incomplete")
			}
		}
		states := map[string]bool{}
		for _, state := range object.States {
			if err := addUniqueID(states, state.Value, "Business state"); err != nil {
				return err
			}
			if strings.TrimSpace(state.Label) == "" {
				return Invalid("business_state_incomplete")
			}
		}
		if !fields[object.PrimaryFieldKey] || !fields[object.StateFieldKey] || !states[object.InitialState] {
			return Invalid("business_object_reference_invalid")
		}
		objects[object.ID] = object
	}
	rules := map[string]BusinessRule{}
	for _, rule := range definition.Rules {
		if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.Title) == "" || strings.TrimSpace(rule.Statement) == "" {
			return Invalid("business_rule_incomplete")
		}
		if _, exists := rules[rule.ID]; exists {
			return Invalid("business_rule_duplicate")
		}
		if rule.Condition != nil && rule.ObjectID == "" {
			return Invalid("business_rule_reference_invalid")
		}
		if rule.ObjectID != "" {
			object, exists := objects[rule.ObjectID]
			if !exists || (rule.Condition != nil && (!objectHasField(object, rule.Condition.FieldKey) || !validRuleOperator(rule.Condition.Operator))) {
				return Invalid("business_rule_reference_invalid")
			}
		}
		rules[rule.ID] = rule
	}
	actions := map[string]ProductAction{}
	for _, action := range definition.Actions {
		object, exists := objects[action.ObjectID]
		if strings.TrimSpace(action.ID) == "" || strings.TrimSpace(action.Label) == "" || !validActionKind(action.Kind) || !exists {
			return Invalid("product_action_incomplete")
		}
		if _, duplicate := actions[action.ID]; duplicate {
			return Invalid("product_action_duplicate")
		}
		for _, fieldKey := range action.RequiredFieldKeys {
			if !objectHasField(object, fieldKey) {
				return Invalid("product_action_field_invalid")
			}
		}
		for _, ruleID := range action.RuleIDs {
			if _, exists := rules[ruleID]; !exists {
				return Invalid("product_action_rule_invalid")
			}
		}
		for _, state := range action.FromStates {
			if !objectHasState(object, state) {
				return Invalid("product_action_state_invalid")
			}
		}
		if action.ToState != "" && !objectHasState(object, action.ToState) {
			return Invalid("product_action_state_invalid")
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
			return Invalid("product_page_incomplete")
		}
		for _, fieldKey := range page.VisibleFieldKeys {
			if !objectHasField(object, fieldKey) {
				return Invalid("product_page_field_invalid")
			}
		}
		if page.CreateActionID != "" {
			action, exists := actions[page.CreateActionID]
			if !exists || action.ObjectID != page.ObjectID {
				return Invalid("product_page_action_invalid")
			}
		}
	}
	exceptions := map[string]bool{}
	for _, exception := range definition.Exceptions {
		if err := addUniqueID(exceptions, exception.ID, "Business exception"); err != nil {
			return err
		}
		if strings.TrimSpace(exception.ID) == "" || strings.TrimSpace(exception.Title) == "" || strings.TrimSpace(exception.Trigger) == "" || strings.TrimSpace(exception.Handling) == "" {
			return Invalid("business_exception_incomplete")
		}
		if exception.ScenarioID != "" && !scenarios[exception.ScenarioID] {
			return Invalid("business_exception_scenario_invalid")
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
		return Invalid("product_definition_id_missing")
	}
	if ids[id] {
		return Invalid("product_definition_id_duplicate")
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

func FindFeature(product *Product, id string) *Feature {
	for index := range product.Features {
		if product.Features[index].ID == id {
			return &product.Features[index]
		}
	}
	return nil
}
