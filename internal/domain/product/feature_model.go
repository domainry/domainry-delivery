package product

import "time"

type Feature struct {
	ID                 string             `json:"id"`
	Code               string             `json:"code"`
	Status             FeatureStatus      `json:"status"`
	CurrentRevision    uint64             `json:"current_revision"`
	ConfirmedRevision  uint64             `json:"confirmed_revision,omitempty"`
	DeliverySequence   uint64             `json:"delivery_sequence,omitempty"`
	QueuedAt           *time.Time         `json:"queued_at,omitempty"`
	DeliveryRunID      string             `json:"delivery_run_id,omitempty"`
	InstalledReleaseID string             `json:"installed_release_id,omitempty"`
	Draft              *FeatureDraftState `json:"draft,omitempty"`
	Revisions          []FeatureRevision  `json:"revisions"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
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
	CreatedBy               string               `json:"created_by"`
	CreatedAt               time.Time            `json:"created_at"`
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
