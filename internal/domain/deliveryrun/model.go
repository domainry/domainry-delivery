package deliveryrun

import (
	"time"
)

type Stage string

const (
	StageDevelopment Stage = "development"
	StageTesting     Stage = "testing"
	StageBugs        Stage = "bugs"
	StageAcceptance  Stage = "acceptance"
	StageRelease     Stage = "release"
	StageLive        Stage = "live"
)

type ProductSnapshot struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Code            string `json:"code"`
	ProductRevision uint64 `json:"product_revision"`
}

type Member struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Roles    []string `json:"roles"`
	Initials string   `json:"initials"`
}

const (
	RoleProductOwner     = "product_owner"
	RoleDevelopmentLead  = "development_lead"
	RoleQualityLead      = "quality_lead"
	RoleBusinessAcceptor = "business_acceptor"
	RoleReleaseApprover  = "release_approver"
)

type DeliveryRun struct {
	ID                      string                     `json:"id"`
	WorkspaceID             string                     `json:"workspace_id"`
	Product                 ProductSnapshot            `json:"product"`
	Feature                 FeatureSnapshot            `json:"feature"`
	Name                    string                     `json:"name"`
	Code                    string                     `json:"code"`
	Goal                    string                     `json:"goal"`
	TargetDate              string                     `json:"target_date"`
	Revision                uint64                     `json:"revision"`
	Stage                   Stage                      `json:"stage"`
	Members                 []Member                   `json:"members"`
	DeliveryUnits           []DeliveryUnit             `json:"delivery_units"`
	ActiveDeliveryUnitID    string                     `json:"active_delivery_unit_id,omitempty"`
	ExecutableRevision      *ExecutableProductRevision `json:"executable_product_revision,omitempty"`
	TestCases               []TestCase                 `json:"test_cases"`
	QualityRuns             []QualityRun               `json:"quality_runs"`
	AcceptanceCases         []AcceptanceCase           `json:"acceptance_cases"`
	AcceptanceConfirmations []AcceptanceConfirmation   `json:"acceptance_confirmations"`
	ReleaseChecks           []ReleaseCheck             `json:"release_checks"`
	Releases                []Release                  `json:"releases"`
	Activity                []ActivityEvent            `json:"activity"`
	CreatedAt               time.Time                  `json:"created_at"`
	UpdatedAt               time.Time                  `json:"updated_at"`
}

// ExecutableProductRevision is the complete runtime definition produced by RD
// for this DeliveryRun. It is installed into Product only when the exact bound
// build receives a successful trusted deployment receipt.
type ExecutableProductRevision struct {
	BaseRevision   uint64                 `json:"base_revision"`
	TargetRevision uint64                 `json:"target_revision"`
	Content        ProductRevisionContent `json:"content"`
	EvidenceRef    string                 `json:"evidence_ref"`
	CodeRevision   string                 `json:"code_revision"`
	ModelSHA256    string                 `json:"model_sha256"`
	RecordedBy     string                 `json:"recorded_by"`
	RecordedAt     time.Time              `json:"recorded_at"`
}

type FeatureSnapshot struct {
	ID                      string               `json:"id"`
	Title                   string               `json:"title"`
	Summary                 string               `json:"summary"`
	Priority                string               `json:"priority"`
	BaselineProductRevision uint64               `json:"baseline_product_revision"`
	Discovery               FeatureDiscovery     `json:"discovery"`
	Specification           FeatureSpecification `json:"specification"`
	Decisions               []FeatureDecision    `json:"decisions"`
	Readiness               FeatureReadiness     `json:"readiness"`
	Source                  FeatureRevisionRef   `json:"source"`
}

type FeatureRevisionRef struct {
	ProductID       string        `json:"product_id"`
	ProductRevision uint64        `json:"product_revision"`
	FeatureID       string        `json:"feature_id"`
	FeatureRevision uint64        `json:"feature_revision"`
	ConversationIDs []string      `json:"conversation_ids"`
	AgentRuns       []AgentRunRef `json:"agent_runs"`
	SourceIDs       []string      `json:"source_ids"`
	DecisionIDs     []string      `json:"decision_ids"`
}

// AgentRunRef mirrors the stable Conversation source identity without making
// Delivery the owner of Agent runs. BeforeStep follows the Agent convention:
// zero means the complete run; positive values freeze a pre-step boundary.
type AgentRunRef struct {
	ConversationID string `json:"conversation_id"`
	RunID          string `json:"run_id"`
	BeforeStep     int    `json:"before_step,omitempty"`
}

type CheckResult string

const (
	ResultPass    CheckResult = "pass"
	ResultFail    CheckResult = "fail"
	ResultBlocked CheckResult = "blocked"
)

type TestCase struct {
	ID        string `json:"id"`
	FeatureID string `json:"feature_id"`
	Title     string `json:"title"`
	Expected  string `json:"expected"`
	Revision  uint64 `json:"revision"`
}

// QualityRun is an independent QA observation bound to the exact Git revision
// that passed the Runtime Journey gate. It does not depend on a separate build
// artifact or an Agent-supplied deployment claim.
type QualityRun struct {
	ID           string      `json:"id"`
	GitRevision  string      `json:"git_revision"`
	TestCaseID   string      `json:"test_case_id"`
	Result       CheckResult `json:"result"`
	Note         string      `json:"note"`
	EvidenceRefs []string    `json:"evidence_refs"`
	FailureOwner string      `json:"failure_owner,omitempty"`
	ExecutedBy   string      `json:"executed_by"`
	ExecutedAt   time.Time   `json:"executed_at"`
}

type AcceptanceCase struct {
	ID            string             `json:"id"`
	FeatureID     string             `json:"feature_id"`
	Title         string             `json:"title"`
	BusinessValue string             `json:"business_value"`
	Source        FeatureRevisionRef `json:"source"`
}

type AcceptanceConfirmation struct {
	ID               string      `json:"id"`
	GitRevision      string      `json:"git_revision"`
	AcceptanceCaseID string      `json:"acceptance_case_id"`
	Result           CheckResult `json:"result"`
	Note             string      `json:"note"`
	EvidenceRefs     []string    `json:"evidence_refs"`
	FailureOwner     string      `json:"failure_owner,omitempty"`
	ConfirmedBy      string      `json:"confirmed_by"`
	ConfirmedAt      time.Time   `json:"confirmed_at"`
}

type ReleaseStatus string

const (
	ReleaseDraft               ReleaseStatus = "draft"
	ReleaseApproved            ReleaseStatus = "approved"
	ReleaseLive                ReleaseStatus = "live"
	ReleaseFailed              ReleaseStatus = "failed"
	ReleaseNeedsReconciliation ReleaseStatus = "needs_reconciliation"
)

type DeploymentOutcome string

const (
	DeploymentSuccess DeploymentOutcome = "success"
	DeploymentFailure DeploymentOutcome = "failure"
	DeploymentUnknown DeploymentOutcome = "unknown"
)

type DeploymentAttempt struct {
	ID             string            `json:"id"`
	Outcome        DeploymentOutcome `json:"outcome"`
	EnvironmentRef string            `json:"environment_ref"`
	LaunchURL      string            `json:"launch_url,omitempty"`
	ReceiptRef     string            `json:"receipt_ref,omitempty"`
	StartedAt      time.Time         `json:"started_at"`
	ResolvedAt     *time.Time        `json:"resolved_at,omitempty"`
}

type Release struct {
	ID                 string             `json:"id"`
	Version            string             `json:"version"`
	CodeRevision       string             `json:"code_revision"`
	ProductRevision    uint64             `json:"product_revision"`
	ProductRevisionRef string             `json:"product_revision_ref"`
	ModelSHA256        string             `json:"model_sha256"`
	EnvironmentRef     string             `json:"environment_ref"`
	Status             ReleaseStatus      `json:"status"`
	ApprovedBy         string             `json:"approved_by,omitempty"`
	ApprovedAt         *time.Time         `json:"approved_at,omitempty"`
	DeploymentAttempt  *DeploymentAttempt `json:"deployment_attempt,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
}

type ReleaseCheckStatus string

const (
	ReleaseCheckPending ReleaseCheckStatus = "pending"
	ReleaseCheckPassed  ReleaseCheckStatus = "passed"
	ReleaseCheckFailed  ReleaseCheckStatus = "failed"
)

type ReleaseCheck struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	EnvironmentRef string             `json:"environment_ref"`
	Required       bool               `json:"required"`
	Status         ReleaseCheckStatus `json:"status"`
	Note           string             `json:"note,omitempty"`
	EvidenceRefs   []string           `json:"evidence_refs"`
	UpdatedBy      string             `json:"updated_by,omitempty"`
	UpdatedAt      *time.Time         `json:"updated_at,omitempty"`
}

type ActivityEvent struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail"`
	ActorID    string    `json:"actor_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

type AgentContext struct {
	DeliveryRun          DeliveryRun       `json:"run"`
	AllowedAgentCommands []string          `json:"allowed_agent_commands"`
	AvailableActions     []AvailableAction `json:"available_actions"`
	HumanOnlyCommands    []string          `json:"human_only_commands"`
	SystemOnlyCommands   []string          `json:"system_only_commands"`
	SourcePolicy         string            `json:"source_policy"`
}

// Projection is the complete read model consumed by Deck. The server owns both
// lifecycle state and the actions that are currently valid; clients only render
// this result and submit commands.
type Projection struct {
	DeliveryRun
	Workflow Workflow `json:"workflow"`
}

type Workflow struct {
	AvailableActions []AvailableAction `json:"available_actions"`
	ReleaseGates     []ReleaseGate     `json:"release_gates"`
}

type ReleaseGate struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
	OK     bool   `json:"ok"`
}
