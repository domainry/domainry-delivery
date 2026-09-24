package product

import "strings"

func NormalizeProductDefinition(definition ProductDefinition) ProductDefinition {
	if definition.Actors == nil {
		definition.Actors = []ProductActor{}
	}
	if definition.Scenarios == nil {
		definition.Scenarios = []ProductScenario{}
	}
	if definition.Objects == nil {
		definition.Objects = []BusinessObject{}
	}
	if definition.Rules == nil {
		definition.Rules = []BusinessRule{}
	}
	if definition.Exceptions == nil {
		definition.Exceptions = []BusinessException{}
	}
	if definition.Actions == nil {
		definition.Actions = []ProductAction{}
	}
	if definition.Pages == nil {
		definition.Pages = []ProductPage{}
	}
	if definition.Access.Roles == nil {
		definition.Access.Roles = []ProductRole{}
	}
	if definition.Access.PermissionAdmins == nil {
		definition.Access.PermissionAdmins = []string{}
	}
	if definition.Access.AssignmentRules == nil {
		definition.Access.AssignmentRules = []string{}
	}
	if definition.Integrations == nil {
		definition.Integrations = []ProductIntegration{}
	}
	if definition.Automations == nil {
		definition.Automations = []ProductAutomation{}
	}
	if definition.Configuration == nil {
		definition.Configuration = []ProductConfiguration{}
	}
	if definition.QualityConstraints == nil {
		definition.QualityConstraints = []ProductQualityConstraint{}
	}
	return definition
}

func validateProductDefinitionExtensions(definition ProductDefinition, actors map[string]bool) error {
	roleIDs, err := validateProductAccessModel(definition.Access, actors)
	if err != nil {
		return err
	}
	if err := validateProductIntegrations(definition.Integrations); err != nil {
		return err
	}
	if err := validateProductAutomations(definition.Automations); err != nil {
		return err
	}
	if err := validateProductConfiguration(definition.Configuration, roleIDs); err != nil {
		return err
	}
	return validateProductQualityConstraints(definition.QualityConstraints)
}

func validateProductAccessModel(access ProductAccessModel, actors map[string]bool) (map[string]bool, error) {
	if access.Authentication != "" && access.Authentication != "required" && access.Authentication != "optional" && access.Authentication != "not_required" && access.Authentication != "mixed" {
		return nil, Invalid("product_authentication_invalid")
	}
	if access.PermissionModel != "" && access.PermissionModel != "fixed" && access.PermissionModel != "configurable" && access.PermissionModel != "hybrid" && access.PermissionModel != "not_applicable" {
		return nil, Invalid("product_permission_model_invalid")
	}
	roleIDs := map[string]bool{}
	for _, role := range access.Roles {
		if err := addUniqueID(roleIDs, role.ID, "Product role"); err != nil {
			return nil, err
		}
		if strings.TrimSpace(role.Name) == "" || len(cleanStrings(role.Responsibilities)) == 0 || len(cleanStrings(role.Capabilities)) == 0 || len(cleanStrings(role.DataScopes)) == 0 {
			return nil, Invalid("product_role_incomplete")
		}
		for _, actorID := range cleanStrings(role.ActorIDs) {
			if !actors[actorID] {
				return nil, Invalid("product_role_actor_invalid")
			}
		}
	}
	for _, roleID := range cleanStrings(access.PermissionAdmins) {
		if !roleIDs[roleID] {
			return nil, Invalid("product_permission_admin_invalid")
		}
	}
	if (access.PermissionModel == "configurable" || access.PermissionModel == "hybrid") && (len(cleanStrings(access.PermissionAdmins)) == 0 || len(cleanStrings(access.AssignmentRules)) == 0) {
		return nil, Invalid("product_permission_administration_incomplete")
	}
	return roleIDs, nil
}

func validateProductIntegrations(integrations []ProductIntegration) error {
	ids := map[string]bool{}
	for _, integration := range integrations {
		if err := addUniqueID(ids, integration.ID, "Product integration"); err != nil {
			return err
		}
		if strings.TrimSpace(integration.Name) == "" || strings.TrimSpace(integration.Contract) == "" || strings.TrimSpace(integration.Authentication) == "" || strings.TrimSpace(integration.FailurePolicy) == "" {
			return Invalid("product_integration_incomplete")
		}
		if integration.Direction != "inbound" && integration.Direction != "outbound" && integration.Direction != "bidirectional" {
			return Invalid("product_integration_direction_invalid")
		}
		if integration.Mode != "synchronous" && integration.Mode != "asynchronous" && integration.Mode != "batch" {
			return Invalid("product_integration_mode_invalid")
		}
	}
	return nil
}

func validateProductAutomations(automations []ProductAutomation) error {
	ids := map[string]bool{}
	for _, automation := range automations {
		if err := addUniqueID(ids, automation.ID, "Product automation"); err != nil {
			return err
		}
		if strings.TrimSpace(automation.Name) == "" || strings.TrimSpace(automation.Trigger) == "" || strings.TrimSpace(automation.Outcome) == "" || strings.TrimSpace(automation.FailureHandling) == "" {
			return Invalid("product_automation_incomplete")
		}
		if (strings.TrimSpace(automation.Schedule) == "") != (strings.TrimSpace(automation.Timezone) == "") {
			return Invalid("product_automation_schedule_invalid")
		}
	}
	return nil
}

func validateProductConfiguration(configuration []ProductConfiguration, roles map[string]bool) error {
	ids := map[string]bool{}
	for _, setting := range configuration {
		if err := addUniqueID(ids, setting.ID, "Product configuration"); err != nil {
			return err
		}
		if strings.TrimSpace(setting.Name) == "" || strings.TrimSpace(setting.Scope) == "" || strings.TrimSpace(setting.ValueType) == "" {
			return Invalid("product_configuration_incomplete")
		}
		for _, roleID := range cleanStrings(setting.ManagedByRoleIDs) {
			if !roles[roleID] {
				return Invalid("product_configuration_role_invalid")
			}
		}
	}
	return nil
}

func validateProductQualityConstraints(constraints []ProductQualityConstraint) error {
	ids := map[string]bool{}
	for _, constraint := range constraints {
		if err := addUniqueID(ids, constraint.ID, "Product quality constraint"); err != nil {
			return err
		}
		if strings.TrimSpace(constraint.Category) == "" || strings.TrimSpace(constraint.Requirement) == "" || strings.TrimSpace(constraint.Measure) == "" {
			return Invalid("product_quality_constraint_incomplete")
		}
	}
	return nil
}
