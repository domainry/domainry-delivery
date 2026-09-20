package delivery

import "time"

type ProductStatus string

const (
	ProductShaping  ProductStatus = "shaping"
	ProductActive   ProductStatus = "active"
	ProductArchived ProductStatus = "archived"
)

type FeatureStatus string

const (
	FeatureDraft      FeatureStatus = "draft"
	FeatureConfirmed  FeatureStatus = "confirmed"
	FeatureDelivering FeatureStatus = "delivering"
	FeatureInstalled  FeatureStatus = "installed"
)

type EngineeringStatus string

const (
	EngineeringFrontendQueued       EngineeringStatus = "frontend_queued"
	EngineeringFrontendInitializing EngineeringStatus = "frontend_initializing"
	EngineeringReady                EngineeringStatus = "ready"
)

// Product is the long-lived aggregate. Its story and executable definition are
// versioned together so the human explanation cannot drift from the runtime
// source of truth.
type Product struct {
	ID                        string             `json:"id"`
	WorkspaceID               string             `json:"workspace_id"`
	Name                      string             `json:"name"`
	Code                      string             `json:"code"`
	Goal                      string             `json:"goal"`
	Industry                  string             `json:"industry"`
	Engineering               ProductEngineering `json:"engineering"`
	Status                    ProductStatus      `json:"status"`
	Revision                  uint64             `json:"revision"`
	CurrentDefinitionRevision uint64             `json:"current_definition_revision"`
	CurrentReleaseRevision    uint64             `json:"current_release_revision"`
	CurrentDeployment         *ProductDeployment `json:"current_deployment,omitempty"`
	Revisions                 []ProductRevision  `json:"revisions"`
	Features                  []Feature          `json:"features"`
	CreatedAt                 time.Time          `json:"created_at"`
	UpdatedAt                 time.Time          `json:"updated_at"`
}

// ProductEngineering tracks the one-time technical project initialization
// that RD performs outside the business Feature lifecycle.
type ProductEngineering struct {
	Status               EngineeringStatus `json:"status"`
	FrontendCodeRevision string            `json:"frontend_code_revision,omitempty"`
	FrontendArtifactRef  string            `json:"frontend_artifact_ref,omitempty"`
	DesignContractRef    string            `json:"design_contract_ref,omitempty"`
	LoginEntry           string            `json:"login_entry,omitempty"`
	ShellEntry           string            `json:"shell_entry,omitempty"`
	PreviewEntry         string            `json:"preview_entry,omitempty"`
	StartedBy            string            `json:"started_by,omitempty"`
	StartedAt            *time.Time        `json:"started_at,omitempty"`
	CompletedBy          string            `json:"completed_by,omitempty"`
	CompletedAt          *time.Time        `json:"completed_at,omitempty"`
}

// ProductDeployment is the trusted launch destination of the currently
// installed Product release. It is copied only from a successful deployment
// receipt so clients never infer a public URL from an environment identifier.
type ProductDeployment struct {
	ReleaseID      string    `json:"release_id"`
	Version        string    `json:"version"`
	EnvironmentRef string    `json:"environment_ref"`
	LaunchURL      string    `json:"launch_url"`
	ReceiptRef     string    `json:"receipt_ref"`
	DeployedAt     time.Time `json:"deployed_at"`
}

type ProductRevision struct {
	Number          uint64            `json:"number"`
	Story           ProductStory      `json:"story"`
	Definition      ProductDefinition `json:"definition"`
	Decisions       []ProductDecision `json:"decisions"`
	SourceFeatureID string            `json:"source_feature_id,omitempty"`
	FeatureRevision uint64            `json:"feature_revision,omitempty"`
	DeliveryRunID   string            `json:"delivery_run_id,omitempty"`
	ReleaseID       string            `json:"release_id,omitempty"`
	CodeRevision    string            `json:"code_revision,omitempty"`
	ArtifactRef     string            `json:"artifact_ref,omitempty"`
	DefinitionRef   string            `json:"definition_ref,omitempty"`
	CreatedBy       string            `json:"created_by"`
	CreatedAt       time.Time         `json:"created_at"`
}

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

type Feature struct {
	ID                 string              `json:"id"`
	Code               string              `json:"code"`
	Status             FeatureStatus       `json:"status"`
	CurrentRevision    uint64              `json:"current_revision"`
	ConfirmedRevision  uint64              `json:"confirmed_revision,omitempty"`
	DeliverySequence   uint64              `json:"delivery_sequence,omitempty"`
	QueuedAt           *time.Time          `json:"queued_at,omitempty"`
	DeliveryRunID      string              `json:"delivery_run_id,omitempty"`
	InstalledReleaseID string              `json:"installed_release_id,omitempty"`
	Draft              *FeatureDraftState  `json:"draft,omitempty"`
	Attachments        []FeatureAttachment `json:"attachments"`
	Revisions          []FeatureRevision   `json:"revisions"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

// FeatureDraftState is the mutable product-discovery workspace for one Feature.
// It is deliberately separate from FeatureRevision: conversation turns refine
// evidence and decisions, while only human confirmation freezes a revision.
type FeatureDraftState struct {
	Version                 uint64               `json:"version"`
	Title                   string               `json:"title"`
	Summary                 string               `json:"summary"`
	Priority                string               `json:"priority"`
	BaselineProductRevision uint64               `json:"baseline_product_revision"`
	Discovery               FeatureDiscovery     `json:"discovery"`
	Specification           FeatureSpecification `json:"specification"`
	Decisions               []FeatureDecision    `json:"decisions"`
	Readiness               FeatureReadiness     `json:"readiness"`
	NextQuestion            string               `json:"next_question"`
	Sources                 []FeatureSource      `json:"sources"`
	UpdatedBy               string               `json:"updated_by"`
	UpdatedAt               time.Time            `json:"updated_at"`
}

type FeatureRevision struct {
	Number                  uint64               `json:"number"`
	Title                   string               `json:"title"`
	Summary                 string               `json:"summary"`
	Priority                string               `json:"priority"`
	BaselineProductRevision uint64               `json:"baseline_product_revision"`
	Discovery               FeatureDiscovery     `json:"discovery"`
	Specification           FeatureSpecification `json:"specification"`
	Decisions               []FeatureDecision    `json:"decisions"`
	Readiness               FeatureReadiness     `json:"readiness"`
	Sources                 []FeatureSource      `json:"sources"`
	Attachments             []FeatureAttachment  `json:"attachments"`
	CreatedBy               string               `json:"created_by"`
	CreatedAt               time.Time            `json:"created_at"`
}

// FeatureAttachment is immutable evidence supplied by a human during product
// discovery. Binary content lives in Delivery's attachment store; the Product
// aggregate retains the verifiable identity and a stable content reference.
type FeatureAttachment struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	MediaType  string    `json:"media_type"`
	SizeBytes  int64     `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
	ContentRef string    `json:"content_ref"`
	UploadedBy string    `json:"uploaded_by"`
	CreatedAt  time.Time `json:"created_at"`
}

type FeatureAttachmentContent struct {
	Attachment FeatureAttachment `json:"attachment"`
	Content    []byte            `json:"-"`
}

// FeatureDecision records an explicit business choice, the alternatives the PM
// considered, the recommendation, and the human resolution. Product
// implementation details belong to the delivery run.
type FeatureDecision struct {
	ID                  string           `json:"id"`
	Title               string           `json:"title"`
	Question            string           `json:"question"`
	Impact              string           `json:"impact"`
	Options             []DecisionOption `json:"options"`
	RecommendedOptionID string           `json:"recommended_option_id,omitempty"`
	SelectedOptionID    string           `json:"selected_option_id,omitempty"`
	Resolution          string           `json:"resolution"`
	Rationale           string           `json:"rationale"`
	Tradeoffs           []string         `json:"tradeoffs"`
	Status              string           `json:"status"`
	SourceIDs           []string         `json:"source_ids"`
}

// FeatureDiscovery retains how the requirement was learned. The PM uses it to
// distinguish user evidence from inference, expose conflicts, compare options,
// and choose the next highest-value question without turning the conversation
// into a fixed questionnaire.
type FeatureDiscovery struct {
	Focus         FeatureDiscoveryFocus      `json:"focus"`
	Evidence      []FeatureEvidence          `json:"evidence"`
	Scenarios     []FeatureDiscoveryScenario `json:"scenarios"`
	Assumptions   []FeatureAssumption        `json:"assumptions"`
	Conflicts     []FeatureConflict          `json:"conflicts"`
	Options       []FeatureDesignOption      `json:"options"`
	OpenQuestions []FeatureOpenQuestion      `json:"open_questions"`
}

type FeatureDiscoveryFocus struct {
	Topic        string `json:"topic"`
	ScenarioID   string `json:"scenario_id,omitempty"`
	DialogueMove string `json:"dialogue_move"`
	Rationale    string `json:"rationale"`
}

type FeatureEvidence struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Statement  string   `json:"statement"`
	Status     string   `json:"status"`
	SourceIDs  []string `json:"source_ids"`
	RelatedIDs []string `json:"related_ids"`
}

type FeatureDiscoveryScenario struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	ActorIDs      []string `json:"actor_ids"`
	Trigger       string   `json:"trigger"`
	CurrentFlow   []string `json:"current_flow"`
	PainPoints    []string `json:"pain_points"`
	TargetOutcome string   `json:"target_outcome"`
	FailureModes  []string `json:"failure_modes"`
	Status        string   `json:"status"`
}

type FeatureAssumption struct {
	ID        string   `json:"id"`
	Statement string   `json:"statement"`
	Reason    string   `json:"reason"`
	Risk      string   `json:"risk"`
	Status    string   `json:"status"`
	SourceIDs []string `json:"source_ids"`
}

type FeatureConflict struct {
	ID          string   `json:"id"`
	Summary     string   `json:"summary"`
	EvidenceIDs []string `json:"evidence_ids"`
	Impact      string   `json:"impact"`
	Status      string   `json:"status"`
	Resolution  string   `json:"resolution"`
}

type FeatureDesignOption struct {
	ID          string   `json:"id"`
	DecisionID  string   `json:"decision_id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Benefits    []string `json:"benefits"`
	Risks       []string `json:"risks"`
	Recommended bool     `json:"recommended"`
}

type FeatureOpenQuestion struct {
	ID             string   `json:"id"`
	Question       string   `json:"question"`
	Kind           string   `json:"kind"`
	Blocks         []string `json:"blocks"`
	Impact         string   `json:"impact"`
	Uncertainty    string   `json:"uncertainty"`
	Dependency     string   `json:"dependency"`
	AnswerCost     string   `json:"answer_cost"`
	PriorityScore  int      `json:"priority_score"`
	PriorityReason string   `json:"priority_reason"`
	Status         string   `json:"status"`
}

// FeatureSpecification is the business change contract formed by one PMAgent
// conversation. It describes the delta from an immutable Product baseline and
// carries only business-facing behavior; RD remains responsible for technical
// design and implementation.
type FeatureSpecification struct {
	Intent        FeatureIntent               `json:"intent"`
	Scope         FeatureScope                `json:"scope"`
	Scenarios     []FeatureScenario           `json:"scenarios"`
	Impacts       []FeatureImpact             `json:"impacts"`
	Authorization FeatureAuthorization        `json:"authorization"`
	Acceptance    []FeatureAcceptanceScenario `json:"acceptance"`
}

type FeatureIntent struct {
	Problem         string   `json:"problem"`
	DesiredOutcome  string   `json:"desired_outcome"`
	SuccessMeasures []string `json:"success_measures"`
}

type FeatureScope struct {
	In  []string `json:"in"`
	Out []string `json:"out"`
}

type FeatureScenario struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	ActorIDs       []string `json:"actor_ids"`
	Trigger        string   `json:"trigger"`
	Preconditions  []string `json:"preconditions"`
	MainFlow       []string `json:"main_flow"`
	AlternateFlows []string `json:"alternate_flows"`
	Outcome        string   `json:"outcome"`
}

type FeatureImpact struct {
	ID         string   `json:"id"`
	Operation  string   `json:"operation"`
	TargetKind string   `json:"target_kind"`
	TargetID   string   `json:"target_id"`
	Summary    string   `json:"summary"`
	Details    []string `json:"details"`
}

// FeatureAuthorization makes access an explicit part of every Feature. The
// first Feature establishes the Product-wide role baseline; later Features
// bind their changed capabilities to roles from that baseline (and may add new
// roles when the change genuinely requires them).
type FeatureAuthorization struct {
	Mode               string               `json:"mode"`
	Authentication     string               `json:"authentication"`
	PermissionModel    string               `json:"permission_model"`
	Roles              []ProductRole        `json:"roles"`
	Grants             []FeatureAccessGrant `json:"grants"`
	PermissionAdminIDs []string             `json:"permission_admin_ids"`
	AssignmentRules    []string             `json:"assignment_rules"`
}

type FeatureAccessGrant struct {
	ID           string   `json:"id"`
	RoleIDs      []string `json:"role_ids"`
	CapabilityID string   `json:"capability_id"`
	Capability   string   `json:"capability"`
	ScenarioIDs  []string `json:"scenario_ids"`
	ActionIDs    []string `json:"action_ids"`
	DataScopes   []string `json:"data_scopes"`
	Conditions   []string `json:"conditions"`
}

type FeatureAcceptanceScenario struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Given []string `json:"given"`
	When  string   `json:"when"`
	Then  []string `json:"then"`
}

type FeatureReadiness struct {
	Status           string   `json:"status"`
	BlockingSections []string `json:"blocking_sections"`
	BlockingIssues   []string `json:"blocking_issues"`
}

// FeatureSource freezes the exact PM conversation run that produced a feature
// revision. Delivery owns the reference, while the Agent runtime owns messages
// and run state.
type FeatureSource struct {
	ConversationID string   `json:"conversation_id"`
	RunID          string   `json:"run_id"`
	BeforeStep     int      `json:"before_step,omitempty"`
	SourceIDs      []string `json:"source_ids"`
	DecisionIDs    []string `json:"decision_ids"`
}

type ProductProjection struct {
	Product          Product           `json:"product"`
	AvailableActions []AvailableAction `json:"available_actions"`
}

type ProductAgentContext struct {
	Product              Product           `json:"product"`
	AllowedAgentCommands []string          `json:"allowed_agent_commands"`
	AvailableActions     []AvailableAction `json:"available_actions"`
	HumanOnlyCommands    []string          `json:"human_only_commands"`
	SystemOnlyCommands   []string          `json:"system_only_commands"`
	SourcePolicy         string            `json:"source_policy"`
}
