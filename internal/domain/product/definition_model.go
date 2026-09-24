package product

type ProductRevisionContent struct {
	Story      ProductStory      `json:"story"`
	Definition ProductDefinition `json:"definition"`
	Decisions  []ProductDecision `json:"decisions"`
}

type ProductStory struct {
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Narrative string `json:"narrative"`
}

type ProductDefinition struct {
	SchemaVersion      int                        `json:"schema_version"`
	Actors             []ProductActor             `json:"actors"`
	Scenarios          []ProductScenario          `json:"scenarios"`
	Objects            []BusinessObject           `json:"objects"`
	Rules              []BusinessRule             `json:"rules"`
	Exceptions         []BusinessException        `json:"exceptions"`
	Actions            []ProductAction            `json:"actions"`
	Pages              []ProductPage              `json:"pages"`
	Access             ProductAccessModel         `json:"access"`
	Integrations       []ProductIntegration       `json:"integrations"`
	Automations        []ProductAutomation        `json:"automations"`
	Configuration      []ProductConfiguration     `json:"configuration"`
	QualityConstraints []ProductQualityConstraint `json:"quality_constraints"`
}

type ProductActor struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Responsibility string   `json:"responsibility"`
	SourceIDs      []string `json:"source_ids"`
}

type ProductScenario struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Trigger     string         `json:"trigger"`
	Outcome     string         `json:"outcome"`
	Steps       []ScenarioStep `json:"steps"`
	SourceIDs   []string       `json:"source_ids"`
}

type ScenarioStep struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	ActorID   string   `json:"actor_id,omitempty"`
	Detail    string   `json:"detail"`
	SourceIDs []string `json:"source_ids"`
}

type BusinessObject struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	SingularName    string          `json:"singular_name"`
	Description     string          `json:"description"`
	PrimaryFieldKey string          `json:"primary_field_key"`
	StateFieldKey   string          `json:"state_field_key"`
	InitialState    string          `json:"initial_state"`
	Fields          []BusinessField `json:"fields"`
	States          []BusinessState `json:"states"`
	SourceIDs       []string        `json:"source_ids"`
}

type BusinessField struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Type      string   `json:"type"`
	Required  bool     `json:"required"`
	Options   []string `json:"options,omitempty"`
	SourceIDs []string `json:"source_ids"`
}

type BusinessState struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Tone  string `json:"tone"`
}

type RuleCondition struct {
	FieldKey string `json:"field_key"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

type BusinessRule struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Statement    string         `json:"statement"`
	ObjectID     string         `json:"object_id,omitempty"`
	Condition    *RuleCondition `json:"condition,omitempty"`
	ErrorMessage string         `json:"error_message"`
	SourceIDs    []string       `json:"source_ids"`
}

type BusinessException struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Trigger    string   `json:"trigger"`
	Handling   string   `json:"handling"`
	ScenarioID string   `json:"scenario_id,omitempty"`
	SourceIDs  []string `json:"source_ids"`
}

type ProductAction struct {
	ID                string   `json:"id"`
	ObjectID          string   `json:"object_id"`
	Label             string   `json:"label"`
	Kind              string   `json:"kind"`
	FromStates        []string `json:"from_states"`
	ToState           string   `json:"to_state,omitempty"`
	RequiredFieldKeys []string `json:"required_field_keys"`
	RuleIDs           []string `json:"rule_ids"`
}

type ProductPage struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	ObjectID         string   `json:"object_id"`
	Kind             string   `json:"kind"`
	VisibleFieldKeys []string `json:"visible_field_keys"`
	CreateActionID   string   `json:"create_action_id,omitempty"`
}

// ProductAccessModel is the product-wide access baseline. A Feature records
// only changes to this model instead of redefining roles on every conversation.
type ProductAccessModel struct {
	Authentication   string        `json:"authentication"`
	PermissionModel  string        `json:"permission_model"`
	Roles            []ProductRole `json:"roles"`
	PermissionAdmins []string      `json:"permission_admins"`
	AssignmentRules  []string      `json:"assignment_rules"`
}

type ProductRole struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	ActorIDs         []string `json:"actor_ids"`
	Responsibilities []string `json:"responsibilities"`
	Capabilities     []string `json:"capabilities"`
	DataScopes       []string `json:"data_scopes"`
	SourceIDs        []string `json:"source_ids"`
}

type ProductIntegration struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Direction      string   `json:"direction"`
	Mode           string   `json:"mode"`
	Contract       string   `json:"contract"`
	Authentication string   `json:"authentication"`
	FailurePolicy  string   `json:"failure_policy"`
	SourceIDs      []string `json:"source_ids"`
}

type ProductAutomation struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Trigger         string   `json:"trigger"`
	Schedule        string   `json:"schedule"`
	Timezone        string   `json:"timezone"`
	Outcome         string   `json:"outcome"`
	FailureHandling string   `json:"failure_handling"`
	SourceIDs       []string `json:"source_ids"`
}

type ProductConfiguration struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Scope            string   `json:"scope"`
	ValueType        string   `json:"value_type"`
	ManagedByRoleIDs []string `json:"managed_by_role_ids"`
	SourceIDs        []string `json:"source_ids"`
}

type ProductQualityConstraint struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Requirement string   `json:"requirement"`
	Measure     string   `json:"measure"`
	SourceIDs   []string `json:"source_ids"`
}

type ProductDecision struct {
	ID               string           `json:"id"`
	Title            string           `json:"title"`
	Question         string           `json:"question"`
	Impact           string           `json:"impact"`
	Options          []DecisionOption `json:"options"`
	SelectedOptionID string           `json:"selected_option_id,omitempty"`
	Status           string           `json:"status"`
	SourceIDs        []string         `json:"source_ids"`
}

type DecisionOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}
