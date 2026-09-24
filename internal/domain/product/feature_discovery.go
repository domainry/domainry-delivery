package product

import "strings"

var featureEvidenceKinds = map[string]bool{
	"fact": true, "actor": true, "problem": true, "goal": true, "rule": true, "constraint": true,
	"idea": true, "decision": true, "correction": true,
}

var featureEvidenceStatuses = map[string]bool{
	"stated": true, "inferred": true, "confirmed": true, "rejected": true,
}

var discoveryLevels = map[string]bool{"low": true, "medium": true, "high": true}

func normalizeFeatureDiscovery(discovery FeatureDiscovery) FeatureDiscovery {
	discovery.Focus.Topic = strings.TrimSpace(discovery.Focus.Topic)
	discovery.Focus.ScenarioID = strings.TrimSpace(discovery.Focus.ScenarioID)
	discovery.Focus.DialogueMove = strings.TrimSpace(discovery.Focus.DialogueMove)
	discovery.Focus.Rationale = strings.TrimSpace(discovery.Focus.Rationale)
	if discovery.Evidence == nil {
		discovery.Evidence = []FeatureEvidence{}
	}
	if discovery.Scenarios == nil {
		discovery.Scenarios = []FeatureDiscoveryScenario{}
	}
	if discovery.Assumptions == nil {
		discovery.Assumptions = []FeatureAssumption{}
	}
	if discovery.Conflicts == nil {
		discovery.Conflicts = []FeatureConflict{}
	}
	if discovery.Options == nil {
		discovery.Options = []FeatureDesignOption{}
	}
	if discovery.OpenQuestions == nil {
		discovery.OpenQuestions = []FeatureOpenQuestion{}
	}
	for index := range discovery.Evidence {
		item := &discovery.Evidence[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Kind = strings.TrimSpace(item.Kind)
		item.Statement = strings.TrimSpace(item.Statement)
		item.Status = strings.TrimSpace(item.Status)
		item.SourceIDs = cleanStrings(item.SourceIDs)
		item.RelatedIDs = cleanStrings(item.RelatedIDs)
	}
	for index := range discovery.Scenarios {
		item := &discovery.Scenarios[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Title = strings.TrimSpace(item.Title)
		item.ActorIDs = cleanStrings(item.ActorIDs)
		item.Trigger = strings.TrimSpace(item.Trigger)
		item.CurrentFlow = cleanStrings(item.CurrentFlow)
		item.PainPoints = cleanStrings(item.PainPoints)
		item.TargetOutcome = strings.TrimSpace(item.TargetOutcome)
		item.FailureModes = cleanStrings(item.FailureModes)
		item.Status = strings.TrimSpace(item.Status)
	}
	for index := range discovery.Assumptions {
		item := &discovery.Assumptions[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Statement = strings.TrimSpace(item.Statement)
		item.Reason = strings.TrimSpace(item.Reason)
		item.Risk = strings.TrimSpace(item.Risk)
		item.Status = strings.TrimSpace(item.Status)
		item.SourceIDs = cleanStrings(item.SourceIDs)
	}
	for index := range discovery.Conflicts {
		item := &discovery.Conflicts[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Summary = strings.TrimSpace(item.Summary)
		item.EvidenceIDs = cleanStrings(item.EvidenceIDs)
		item.Impact = strings.TrimSpace(item.Impact)
		item.Status = strings.TrimSpace(item.Status)
		item.Resolution = strings.TrimSpace(item.Resolution)
	}
	for index := range discovery.Options {
		item := &discovery.Options[index]
		item.ID = strings.TrimSpace(item.ID)
		item.DecisionID = strings.TrimSpace(item.DecisionID)
		item.Label = strings.TrimSpace(item.Label)
		item.Description = strings.TrimSpace(item.Description)
		item.Benefits = cleanStrings(item.Benefits)
		item.Risks = cleanStrings(item.Risks)
	}
	for index := range discovery.OpenQuestions {
		item := &discovery.OpenQuestions[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Question = strings.TrimSpace(item.Question)
		item.Kind = strings.TrimSpace(item.Kind)
		item.Blocks = cleanStrings(item.Blocks)
		item.Impact = strings.TrimSpace(item.Impact)
		item.Uncertainty = strings.TrimSpace(item.Uncertainty)
		item.Dependency = strings.TrimSpace(item.Dependency)
		item.AnswerCost = strings.TrimSpace(item.AnswerCost)
		item.PriorityReason = strings.TrimSpace(item.PriorityReason)
		item.Status = strings.TrimSpace(item.Status)
	}
	return discovery
}

func validateFeatureDiscovery(discovery FeatureDiscovery) error {
	if discovery.Focus.Topic == "" || discovery.Focus.DialogueMove == "" || discovery.Focus.Rationale == "" {
		return Invalid("feature_discovery_focus_incomplete", "Feature discovery requires a current topic, dialogue move, and rationale.")
	}
	if len(discovery.Evidence) == 0 {
		return Invalid("feature_discovery_evidence_missing", "Feature discovery must retain at least one stated or inferred business observation.")
	}
	if err := validateFeatureEvidence(discovery.Evidence); err != nil {
		return err
	}
	if err := validateDiscoveryScenarios(discovery.Scenarios); err != nil {
		return err
	}
	if err := validateFeatureAssumptions(discovery.Assumptions); err != nil {
		return err
	}
	if err := validateFeatureConflicts(discovery.Conflicts); err != nil {
		return err
	}
	if err := validateFeatureOptions(discovery.Options); err != nil {
		return err
	}
	return validateFeatureOpenQuestions(discovery.OpenQuestions)
}

func validateFeatureEvidence(items []FeatureEvidence) error {
	ids := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || item.Statement == "" || !featureEvidenceKinds[item.Kind] || !featureEvidenceStatuses[item.Status] || len(item.SourceIDs) == 0 {
			return Invalid("feature_evidence_incomplete", "Every discovery observation requires an ID, supported kind and status, statement, and source reference.")
		}
		if ids[item.ID] {
			return Invalid("feature_evidence_duplicate", "Feature discovery observation IDs must be unique.")
		}
		ids[item.ID] = true
	}
	return nil
}

func validateDiscoveryScenarios(items []FeatureDiscoveryScenario) error {
	ids := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || item.Title == "" || item.Status == "" {
			return Invalid("feature_discovery_scenario_incomplete", "Every discovery scenario requires an ID, title, and status.")
		}
		if item.Status != "exploring" && item.Status != "confirmed" {
			return Invalid("feature_discovery_scenario_status_invalid", "A discovery scenario status must be exploring or confirmed.")
		}
		if ids[item.ID] {
			return Invalid("feature_discovery_scenario_duplicate", "Feature discovery scenario IDs must be unique.")
		}
		ids[item.ID] = true
	}
	return nil
}

func validateFeatureAssumptions(items []FeatureAssumption) error {
	ids := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || item.Statement == "" || item.Reason == "" || !discoveryLevels[item.Risk] {
			return Invalid("feature_assumption_incomplete", "Every assumption requires an ID, statement, reason, and risk level.")
		}
		if item.Status != "open" && item.Status != "confirmed" && item.Status != "rejected" {
			return Invalid("feature_assumption_status_invalid", "An assumption status must be open, confirmed, or rejected.")
		}
		if ids[item.ID] {
			return Invalid("feature_assumption_duplicate", "Feature assumption IDs must be unique.")
		}
		ids[item.ID] = true
	}
	return nil
}

func validateFeatureConflicts(items []FeatureConflict) error {
	ids := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || item.Summary == "" || !discoveryLevels[item.Impact] || len(item.EvidenceIDs) < 2 {
			return Invalid("feature_conflict_incomplete", "Every conflict requires an ID, summary, impact, and at least two evidence references.")
		}
		if item.Status != "open" && item.Status != "resolved" && item.Status != "dismissed" {
			return Invalid("feature_conflict_status_invalid", "A conflict status must be open, resolved, or dismissed.")
		}
		if item.Status == "resolved" && item.Resolution == "" {
			return Invalid("feature_conflict_resolution_missing", "A resolved conflict requires its business resolution.")
		}
		if ids[item.ID] {
			return Invalid("feature_conflict_duplicate", "Feature conflict IDs must be unique.")
		}
		ids[item.ID] = true
	}
	return nil
}

func validateFeatureOptions(items []FeatureDesignOption) error {
	ids := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || item.DecisionID == "" || item.Label == "" || item.Description == "" {
			return Invalid("feature_option_incomplete", "Every product option requires an ID, decision reference, label, and description.")
		}
		if ids[item.ID] {
			return Invalid("feature_option_duplicate", "Feature product option IDs must be unique.")
		}
		ids[item.ID] = true
	}
	return nil
}

func validateFeatureOpenQuestions(items []FeatureOpenQuestion) error {
	ids := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || item.Question == "" || item.Kind == "" || !discoveryLevels[item.Impact] || !discoveryLevels[item.Uncertainty] || !discoveryLevels[item.Dependency] || !discoveryLevels[item.AnswerCost] || item.PriorityScore < 1 || item.PriorityScore > 100 || item.PriorityReason == "" {
			return Invalid("feature_question_incomplete", "Every candidate question requires an ID, question, kind, complete ranking factors, and a Rust-computed priority.")
		}
		if item.Status != "open" && item.Status != "answered" && item.Status != "dismissed" {
			return Invalid("feature_question_status_invalid", "A candidate question status must be open, answered, or dismissed.")
		}
		if ids[item.ID] {
			return Invalid("feature_question_duplicate", "Feature candidate question IDs must be unique.")
		}
		ids[item.ID] = true
	}
	return nil
}

func validateFeatureDiscoveryReferences(discovery FeatureDiscovery, decisions []FeatureDecision) error {
	evidenceIDs := make(map[string]bool, len(discovery.Evidence))
	for _, evidence := range discovery.Evidence {
		evidenceIDs[evidence.ID] = true
	}
	scenarioIDs := make(map[string]bool, len(discovery.Scenarios))
	for _, scenario := range discovery.Scenarios {
		scenarioIDs[scenario.ID] = true
	}
	if discovery.Focus.ScenarioID != "" && !scenarioIDs[discovery.Focus.ScenarioID] {
		return Invalid("feature_discovery_focus_scenario_invalid", "The discovery focus must reference an existing scenario.")
	}
	for _, conflict := range discovery.Conflicts {
		for _, evidenceID := range conflict.EvidenceIDs {
			if !evidenceIDs[evidenceID] {
				return Invalid("feature_conflict_evidence_invalid", "A Feature conflict must reference existing discovery evidence.")
			}
		}
	}
	decisionOptions := make(map[string]map[string]bool, len(decisions))
	for _, decision := range decisions {
		options := make(map[string]bool, len(decision.Options))
		for _, option := range decision.Options {
			options[option.ID] = true
		}
		decisionOptions[decision.ID] = options
	}
	for _, option := range discovery.Options {
		options, ok := decisionOptions[option.DecisionID]
		if !ok || !options[option.ID] {
			return Invalid("feature_option_decision_invalid", "A product option must reference an existing decision and one of that decision's options.")
		}
	}
	return nil
}

func selectNextFeatureQuestion(questions []FeatureOpenQuestion) string {
	bestScore := -1
	bestQuestion := ""
	for _, question := range questions {
		if question.Status != "open" {
			continue
		}
		score := question.PriorityScore
		if score > bestScore {
			bestScore = score
			bestQuestion = question.Question
		}
	}
	return bestQuestion
}

func discoveryReadinessBlockers(discovery FeatureDiscovery, decisions []FeatureDecision) []string {
	blockers := make([]string, 0)
	for _, conflict := range discovery.Conflicts {
		if conflict.Status == "open" {
			blockers = append(blockers, "open_conflict")
			break
		}
	}
	for _, assumption := range discovery.Assumptions {
		if assumption.Status == "open" && assumption.Risk == "high" {
			blockers = append(blockers, "high_risk_assumption")
			break
		}
	}
	for _, decision := range decisions {
		if decision.Status == "open" {
			blockers = append(blockers, "open_decision")
			break
		}
	}
	return blockers
}
