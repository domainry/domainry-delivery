package product

import "strings"

func normalizeFeatureSpecification(specification FeatureSpecification) FeatureSpecification {
	if specification.Scenarios == nil {
		specification.Scenarios = []FeatureScenario{}
	}
	if specification.Impacts == nil {
		specification.Impacts = []FeatureImpact{}
	}
	if specification.Authorization.Roles == nil {
		specification.Authorization.Roles = []ProductRole{}
	}
	if specification.Authorization.Grants == nil {
		specification.Authorization.Grants = []FeatureAccessGrant{}
	}
	if specification.Authorization.PermissionAdminIDs == nil {
		specification.Authorization.PermissionAdminIDs = []string{}
	}
	if specification.Authorization.AssignmentRules == nil {
		specification.Authorization.AssignmentRules = []string{}
	}
	if specification.Acceptance == nil {
		specification.Acceptance = []FeatureAcceptanceScenario{}
	}
	specification.Intent.Problem = strings.TrimSpace(specification.Intent.Problem)
	specification.Intent.DesiredOutcome = strings.TrimSpace(specification.Intent.DesiredOutcome)
	specification.Intent.SuccessMeasures = cleanStrings(specification.Intent.SuccessMeasures)
	specification.Scope.In = cleanStrings(specification.Scope.In)
	specification.Scope.Out = cleanStrings(specification.Scope.Out)
	for index := range specification.Scenarios {
		scenario := &specification.Scenarios[index]
		scenario.ID = strings.TrimSpace(scenario.ID)
		scenario.Title = strings.TrimSpace(scenario.Title)
		scenario.ActorIDs = cleanStrings(scenario.ActorIDs)
		scenario.Trigger = strings.TrimSpace(scenario.Trigger)
		scenario.Preconditions = cleanStrings(scenario.Preconditions)
		scenario.MainFlow = cleanStrings(scenario.MainFlow)
		scenario.AlternateFlows = cleanStrings(scenario.AlternateFlows)
		scenario.Outcome = strings.TrimSpace(scenario.Outcome)
		scenario.SourceIDs = cleanStrings(scenario.SourceIDs)
	}
	for index := range specification.Impacts {
		impact := &specification.Impacts[index]
		impact.ID = strings.TrimSpace(impact.ID)
		impact.Operation = strings.TrimSpace(impact.Operation)
		impact.TargetKind = strings.TrimSpace(impact.TargetKind)
		impact.TargetID = strings.TrimSpace(impact.TargetID)
		impact.Summary = strings.TrimSpace(impact.Summary)
		impact.Details = cleanStrings(impact.Details)
		impact.SourceIDs = cleanStrings(impact.SourceIDs)
	}
	specification.Authorization.Mode = strings.TrimSpace(specification.Authorization.Mode)
	specification.Authorization.Authentication = strings.TrimSpace(specification.Authorization.Authentication)
	specification.Authorization.PermissionModel = strings.TrimSpace(specification.Authorization.PermissionModel)
	specification.Authorization.PermissionAdminIDs = cleanStrings(specification.Authorization.PermissionAdminIDs)
	specification.Authorization.AssignmentRules = cleanStrings(specification.Authorization.AssignmentRules)
	for index := range specification.Authorization.Roles {
		role := &specification.Authorization.Roles[index]
		role.ID = strings.TrimSpace(role.ID)
		role.Name = strings.TrimSpace(role.Name)
		role.ActorIDs = cleanStrings(role.ActorIDs)
		role.Responsibilities = cleanStrings(role.Responsibilities)
		role.Capabilities = cleanStrings(role.Capabilities)
		role.DataScopes = cleanStrings(role.DataScopes)
		role.SourceIDs = cleanStrings(role.SourceIDs)
	}
	for index := range specification.Authorization.Grants {
		grant := &specification.Authorization.Grants[index]
		grant.ID = strings.TrimSpace(grant.ID)
		grant.RoleIDs = cleanStrings(grant.RoleIDs)
		grant.CapabilityID = strings.TrimSpace(grant.CapabilityID)
		grant.Capability = strings.TrimSpace(grant.Capability)
		grant.ScenarioIDs = cleanStrings(grant.ScenarioIDs)
		grant.ActionIDs = cleanStrings(grant.ActionIDs)
		grant.DataScopes = cleanStrings(grant.DataScopes)
		grant.Conditions = cleanStrings(grant.Conditions)
	}
	for index := range specification.Acceptance {
		acceptance := &specification.Acceptance[index]
		acceptance.ID = strings.TrimSpace(acceptance.ID)
		acceptance.Title = strings.TrimSpace(acceptance.Title)
		acceptance.Given = cleanStrings(acceptance.Given)
		acceptance.When = strings.TrimSpace(acceptance.When)
		acceptance.Then = cleanStrings(acceptance.Then)
		acceptance.SourceIDs = cleanStrings(acceptance.SourceIDs)
	}
	return specification
}

type featureAuthorizationPolicy struct {
	Mode         string
	KnownRoleIDs map[string]bool
}

func normalizeFeatureSourceCoverage(coverage *FeatureSourceCoverage) *FeatureSourceCoverage {
	if coverage == nil {
		return nil
	}
	coverage.SourceID = strings.TrimSpace(coverage.SourceID)
	coverage.Status = strings.TrimSpace(coverage.Status)
	if coverage.Areas == nil {
		coverage.Areas = []FeatureSourceCoverageArea{}
	}
	for index := range coverage.Areas {
		area := &coverage.Areas[index]
		area.ID = strings.TrimSpace(area.ID)
		area.Title = strings.TrimSpace(area.Title)
		area.Summary = strings.TrimSpace(area.Summary)
		area.Kind = strings.TrimSpace(area.Kind)
		area.SourcePaths = cleanStrings(area.SourcePaths)
		if area.Findings == nil {
			area.Findings = []FeatureSourceFinding{}
		}
		for findingIndex := range area.Findings {
			finding := &area.Findings[findingIndex]
			finding.SourcePath = strings.TrimSpace(finding.SourcePath)
			finding.Summary = strings.TrimSpace(finding.Summary)
			finding.Status = strings.TrimSpace(finding.Status)
			finding.SupportingPaths = cleanStrings(finding.SupportingPaths)
		}
		area.PrimaryScenarioID = strings.TrimSpace(area.PrimaryScenarioID)
		area.PrimaryImpactID = strings.TrimSpace(area.PrimaryImpactID)
		area.PrimaryAcceptanceID = strings.TrimSpace(area.PrimaryAcceptanceID)
		area.ScenarioIDs = cleanStrings(area.ScenarioIDs)
		area.ImpactIDs = cleanStrings(area.ImpactIDs)
		area.GrantIDs = cleanStrings(area.GrantIDs)
		area.AcceptanceIDs = cleanStrings(area.AcceptanceIDs)
		area.Status = strings.TrimSpace(area.Status)
	}
	return coverage
}

func assessFeatureReadiness(title, summary string, discovery FeatureDiscovery, specification FeatureSpecification, sourceCoverage *FeatureSourceCoverage, decisions []FeatureDecision, nextQuestion string, authorizationPolicy featureAuthorizationPolicy) (FeatureReadiness, error) {
	blocking := make([]string, 0)
	blocked := map[string]bool{}
	addBlocker := func(value string) {
		if !blocked[value] {
			blocking = append(blocking, value)
			blocked[value] = true
		}
	}
	if title == "" || summary == "" {
		addBlocker("identity")
	}
	if len(discovery.Evidence) == 0 {
		addBlocker("evidence")
	}
	if specification.Intent.Problem == "" || specification.Intent.DesiredOutcome == "" || len(specification.Intent.SuccessMeasures) == 0 {
		addBlocker("intent")
	}
	if len(specification.Scope.In) == 0 || len(specification.Scope.Out) == 0 {
		addBlocker("scope")
	}
	if len(specification.Scenarios) == 0 {
		addBlocker("scenarios")
	}
	if err := assessFeatureScenarios(specification.Scenarios, addBlocker); err != nil {
		return FeatureReadiness{}, err
	}
	if err := assessFeatureImpacts(specification.Impacts, addBlocker); err != nil {
		return FeatureReadiness{}, err
	}
	if err := assessFeatureAuthorization(specification.Authorization, specification.Scenarios, authorizationPolicy, addBlocker); err != nil {
		return FeatureReadiness{}, err
	}
	if err := assessFeatureAcceptance(specification.Acceptance, addBlocker); err != nil {
		return FeatureReadiness{}, err
	}
	if sourceCoverage != nil {
		if err := assessFeatureSourceCoverage(*sourceCoverage, specification, addBlocker); err != nil {
			return FeatureReadiness{}, err
		}
	}
	if nextQuestion != "" {
		addBlocker("open_question")
	}
	issues := discoveryReadinessBlockers(discovery, decisions)
	status := "shaping"
	if len(blocking) == 0 && len(issues) == 0 {
		status = "ready"
	}
	return FeatureReadiness{Status: status, BlockingSections: blocking, BlockingIssues: issues}, nil
}

func assessFeatureSourceCoverage(coverage FeatureSourceCoverage, specification FeatureSpecification, addBlocker func(string)) error {
	if coverage.SourceID == "" || len(coverage.Areas) == 0 {
		addBlocker("source_coverage")
	}
	if coverage.Status != "complete" && coverage.Status != "incomplete" {
		return Invalid("feature_source_coverage_status_invalid")
	}
	scenarioIDs := map[string]bool{}
	impactIDs := map[string]bool{}
	grantIDs := map[string]bool{}
	acceptanceIDs := map[string]bool{}
	for _, scenario := range specification.Scenarios {
		scenarioIDs[scenario.ID] = true
	}
	for _, impact := range specification.Impacts {
		impactIDs[impact.ID] = true
	}
	for _, grant := range specification.Authorization.Grants {
		grantIDs[grant.ID] = true
	}
	for _, acceptance := range specification.Acceptance {
		acceptanceIDs[acceptance.ID] = true
	}
	areaIDs := map[string]bool{}
	primaryScenarioIDs := map[string]bool{}
	primaryImpactIDs := map[string]bool{}
	primaryAcceptanceIDs := map[string]bool{}
	hasUncoveredArea := false
	for _, area := range coverage.Areas {
		if area.ID == "" || areaIDs[area.ID] {
			return Invalid("feature_source_coverage_area_duplicate")
		}
		areaIDs[area.ID] = true
		if area.Title == "" || area.Summary == "" || !validSourceCoverageKind(area.Kind) || len(area.SourcePaths) == 0 || len(area.SourcePaths) > 8 {
			return Invalid("feature_source_coverage_area_invalid")
		}
		if !validSourceFindings(area) {
			return Invalid("feature_source_coverage_findings_invalid")
		}
		if !primarySourceMappingValid(area.PrimaryScenarioID, area.ScenarioIDs, scenarioIDs, primaryScenarioIDs) ||
			!primarySourceMappingValid(area.PrimaryImpactID, area.ImpactIDs, impactIDs, primaryImpactIDs) ||
			!primarySourceMappingValid(area.PrimaryAcceptanceID, area.AcceptanceIDs, acceptanceIDs, primaryAcceptanceIDs) {
			return Invalid("feature_source_coverage_primary_mapping_invalid")
		}
		areaHasUnresolved := false
		for _, finding := range area.Findings {
			areaHasUnresolved = areaHasUnresolved || finding.Status == "unresolved"
		}
		switch area.Status {
		case "covered":
			if areaHasUnresolved || !sourceCoverageAreaMapped(area) {
				addBlocker("source_coverage")
			}
		case "uncovered":
			if !areaHasUnresolved {
				return Invalid("feature_source_coverage_unresolved_mismatch")
			}
			hasUncoveredArea = true
			addBlocker("source_coverage")
		default:
			return Invalid("feature_source_coverage_area_status_invalid")
		}
		if !referencesKnown(area.ScenarioIDs, scenarioIDs) || !referencesKnown(area.ImpactIDs, impactIDs) || !referencesKnown(area.GrantIDs, grantIDs) || !referencesKnown(area.AcceptanceIDs, acceptanceIDs) {
			return Invalid("feature_source_coverage_reference_unknown")
		}
	}
	if coverage.Status == "incomplete" || hasUncoveredArea {
		addBlocker("source_coverage")
	}
	if coverage.Status == "complete" && hasUncoveredArea {
		return Invalid("feature_source_coverage_completion_invalid")
	}
	return nil
}

func validSourceFindings(area FeatureSourceCoverageArea) bool {
	if len(area.Findings) != len(area.SourcePaths) {
		return false
	}
	areaPaths := map[string]bool{}
	for _, path := range area.SourcePaths {
		areaPaths[path] = true
	}
	findingPaths := map[string]bool{}
	for _, finding := range area.Findings {
		if finding.SourcePath == "" || finding.Summary == "" || !areaPaths[finding.SourcePath] || findingPaths[finding.SourcePath] {
			return false
		}
		findingPaths[finding.SourcePath] = true
		switch finding.Status {
		case "confirmed", "unresolved", "context":
		case "superseded":
			if len(finding.SupportingPaths) == 0 {
				return false
			}
			for _, supportingPath := range finding.SupportingPaths {
				if supportingPath == finding.SourcePath {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

func primarySourceMappingValid(primary string, mapped []string, known map[string]bool, used map[string]bool) bool {
	if primary == "" || !known[primary] || used[primary] {
		return false
	}
	found := false
	for _, id := range mapped {
		if !known[id] {
			return false
		}
		found = found || id == primary
	}
	if !found {
		return false
	}
	used[primary] = true
	return true
}

func validSourceCoverageKind(kind string) bool {
	return kind == "documentation" || kind == "page" || kind == "action" || kind == "workflow" || kind == "rule" || kind == "exception" || kind == "access" || kind == "integration" || kind == "automation" || kind == "quality"
}

func sourceCoverageAreaMapped(area FeatureSourceCoverageArea) bool {
	switch area.Kind {
	case "rule", "exception", "integration", "automation":
		return len(area.ScenarioIDs) > 0 && len(area.ImpactIDs) > 0 && len(area.AcceptanceIDs) > 0
	case "access":
		return len(area.ScenarioIDs) > 0 && len(area.ImpactIDs) > 0 && len(area.GrantIDs) > 0 && len(area.AcceptanceIDs) > 0
	case "quality":
		return len(area.ScenarioIDs) > 0 && len(area.ImpactIDs) > 0 && len(area.AcceptanceIDs) > 0
	case "documentation", "page", "action", "workflow":
		return len(area.ScenarioIDs) > 0 && len(area.ImpactIDs) > 0 && len(area.AcceptanceIDs) > 0
	default:
		return false
	}
}

func referencesKnown(values []string, known map[string]bool) bool {
	for _, value := range values {
		if !known[value] {
			return false
		}
	}
	return true
}

func assessFeatureAuthorization(authorization FeatureAuthorization, scenarios []FeatureScenario, policy featureAuthorizationPolicy, addBlocker func(string)) error {
	if authorization.Mode != policy.Mode {
		addBlocker("authorization")
	}
	roleIDs := make(map[string]bool, len(policy.KnownRoleIDs)+len(authorization.Roles))
	declaredRoleIDs := map[string]bool{}
	for roleID := range policy.KnownRoleIDs {
		roleIDs[roleID] = true
	}
	for _, role := range authorization.Roles {
		if role.ID == "" || declaredRoleIDs[role.ID] {
			return Invalid("feature_authorization_role_duplicate")
		}
		declaredRoleIDs[role.ID] = true
		roleIDs[role.ID] = true
		if role.Name == "" || len(role.Responsibilities) == 0 || len(role.Capabilities) == 0 || len(role.DataScopes) == 0 {
			addBlocker("authorization")
		}
	}
	if policy.Mode == "establish_baseline" {
		if !validAuthentication(authorization.Authentication) || !validPermissionModel(authorization.PermissionModel) || len(authorization.Roles) == 0 {
			addBlocker("authorization")
		}
		if (authorization.PermissionModel == "configurable" || authorization.PermissionModel == "hybrid") && (len(authorization.PermissionAdminIDs) == 0 || len(authorization.AssignmentRules) == 0) {
			addBlocker("authorization")
		}
		for _, roleID := range authorization.PermissionAdminIDs {
			if !roleIDs[roleID] {
				return Invalid("feature_permission_admin_invalid")
			}
		}
		coveredActors := map[string]bool{}
		for _, role := range authorization.Roles {
			for _, actorID := range role.ActorIDs {
				coveredActors[actorID] = true
			}
		}
		for _, scenario := range scenarios {
			for _, actorID := range scenario.ActorIDs {
				if !coveredActors[actorID] {
					addBlocker("authorization")
				}
			}
		}
	}
	if len(authorization.Grants) == 0 {
		addBlocker("authorization")
		return nil
	}
	scenarioIDs := make(map[string]bool, len(scenarios))
	coveredScenarios := make(map[string]bool, len(scenarios))
	for _, scenario := range scenarios {
		scenarioIDs[scenario.ID] = true
	}
	grantIDs := map[string]bool{}
	for _, grant := range authorization.Grants {
		if grant.ID == "" || grantIDs[grant.ID] {
			return Invalid("feature_authorization_grant_duplicate")
		}
		grantIDs[grant.ID] = true
		if len(grant.RoleIDs) == 0 || grant.CapabilityID == "" || grant.Capability == "" || len(grant.ScenarioIDs) == 0 || len(grant.DataScopes) == 0 {
			addBlocker("authorization")
		}
		for _, roleID := range grant.RoleIDs {
			if !roleIDs[roleID] {
				return Invalid("feature_authorization_role_unknown")
			}
		}
		for _, scenarioID := range grant.ScenarioIDs {
			if len(scenarioIDs) == 0 {
				addBlocker("authorization")
				continue
			}
			if !scenarioIDs[scenarioID] {
				return &Error{
					Code:    "feature_authorization_scenario_unknown",
					Details: map[string]any{"grant_id": grant.ID, "scenario_id": scenarioID},
				}
			}
			coveredScenarios[scenarioID] = true
		}
	}
	for scenarioID := range scenarioIDs {
		if !coveredScenarios[scenarioID] {
			addBlocker("authorization")
		}
	}
	return nil
}

func validAuthentication(authentication string) bool {
	return authentication == "required" || authentication == "optional" || authentication == "not_required" || authentication == "mixed"
}

func validPermissionModel(permissionModel string) bool {
	return permissionModel == "fixed" || permissionModel == "configurable" || permissionModel == "hybrid" || permissionModel == "not_applicable"
}

func assessFeatureScenarios(scenarios []FeatureScenario, addBlocker func(string)) error {
	ids := map[string]bool{}
	for _, scenario := range scenarios {
		if scenario.ID != "" && ids[scenario.ID] {
			return Invalid("feature_scenario_duplicate")
		}
		ids[scenario.ID] = true
		if scenario.ID == "" || scenario.Title == "" || len(scenario.ActorIDs) == 0 || scenario.Trigger == "" || len(scenario.MainFlow) == 0 || scenario.Outcome == "" {
			addBlocker("scenarios")
		}
	}
	return nil
}

func assessFeatureImpacts(impacts []FeatureImpact, addBlocker func(string)) error {
	if len(impacts) == 0 {
		addBlocker("impact")
	}
	ids := map[string]bool{}
	for _, impact := range impacts {
		if impact.ID != "" && ids[impact.ID] {
			return Invalid("feature_impact_duplicate")
		}
		ids[impact.ID] = true
		if impact.Operation != "add" && impact.Operation != "change" && impact.Operation != "remove" {
			return Invalid("feature_impact_operation_invalid")
		}
		if !validImpactTargetKind(impact.TargetKind) {
			return Invalid("feature_impact_target_invalid")
		}
		if impact.ID == "" || impact.TargetID == "" || impact.Summary == "" || len(impact.Details) == 0 {
			addBlocker("impact")
		}
	}
	return nil
}

func assessFeatureAcceptance(acceptance []FeatureAcceptanceScenario, addBlocker func(string)) error {
	if len(acceptance) == 0 {
		addBlocker("acceptance")
	}
	ids := map[string]bool{}
	for _, scenario := range acceptance {
		if scenario.ID != "" && ids[scenario.ID] {
			return Invalid("feature_acceptance_duplicate")
		}
		ids[scenario.ID] = true
		if scenario.ID == "" || scenario.Title == "" || len(scenario.Given) == 0 || scenario.When == "" || len(scenario.Then) == 0 {
			addBlocker("acceptance")
		}
	}
	return nil
}

func validImpactTargetKind(kind string) bool {
	return kind == "actor" || kind == "scenario" || kind == "object" || kind == "rule" || kind == "exception" || kind == "action" || kind == "page" || kind == "access" || kind == "integration" || kind == "automation" || kind == "configuration" || kind == "quality"
}
