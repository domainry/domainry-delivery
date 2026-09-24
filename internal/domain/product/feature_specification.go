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
	}
	for index := range specification.Impacts {
		impact := &specification.Impacts[index]
		impact.ID = strings.TrimSpace(impact.ID)
		impact.Operation = strings.TrimSpace(impact.Operation)
		impact.TargetKind = strings.TrimSpace(impact.TargetKind)
		impact.TargetID = strings.TrimSpace(impact.TargetID)
		impact.Summary = strings.TrimSpace(impact.Summary)
		impact.Details = cleanStrings(impact.Details)
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
	}
	return specification
}

type featureAuthorizationPolicy struct {
	Mode         string
	KnownRoleIDs map[string]bool
}

func assessFeatureReadiness(title, summary string, discovery FeatureDiscovery, specification FeatureSpecification, decisions []FeatureDecision, nextQuestion string, authorizationPolicy featureAuthorizationPolicy) (FeatureReadiness, error) {
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
			return Invalid("feature_authorization_role_duplicate", "Feature authorization role IDs must be non-empty and unique.")
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
				return Invalid("feature_permission_admin_invalid", "A Feature permission administrator must reference a defined role.")
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
			return Invalid("feature_authorization_grant_duplicate", "Feature authorization grant IDs must be non-empty and unique.")
		}
		grantIDs[grant.ID] = true
		if len(grant.RoleIDs) == 0 || grant.CapabilityID == "" || grant.Capability == "" || len(grant.ScenarioIDs) == 0 || len(grant.DataScopes) == 0 {
			addBlocker("authorization")
		}
		for _, roleID := range grant.RoleIDs {
			if !roleIDs[roleID] {
				return Invalid("feature_authorization_role_unknown", "A Feature authorization grant must reference a role from the Product baseline or this Feature.")
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
					Message: "A Feature authorization grant must reference a Feature scenario.",
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
			return Invalid("feature_scenario_duplicate", "Feature scenario IDs must be unique.")
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
			return Invalid("feature_impact_duplicate", "Feature impact IDs must be unique.")
		}
		ids[impact.ID] = true
		if impact.Operation != "add" && impact.Operation != "change" && impact.Operation != "remove" {
			return Invalid("feature_impact_operation_invalid", "A Feature impact operation must be add, change, or remove.")
		}
		if !validImpactTargetKind(impact.TargetKind) {
			return Invalid("feature_impact_target_invalid", "A Feature impact must target a supported ProductDefinition area.")
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
			return Invalid("feature_acceptance_duplicate", "Feature acceptance scenario IDs must be unique.")
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
