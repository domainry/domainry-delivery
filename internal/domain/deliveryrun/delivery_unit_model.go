package deliveryrun

import (
	"time"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

type DeliveryUnitPhase string

const (
	DeliveryUnitPlanned               DeliveryUnitPhase = "planned"
	DeliveryUnitInteractionModeling   DeliveryUnitPhase = "interaction_modeling"
	DeliveryUnitDomainModeling        DeliveryUnitPhase = "domain_modeling"
	DeliveryUnitModelVerification     DeliveryUnitPhase = "model_verification"
	DeliveryUnitBackendImplementation DeliveryUnitPhase = "backend_implementation"
	DeliveryUnitFrontendConvergence   DeliveryUnitPhase = "frontend_convergence"
	DeliveryUnitContractVerification  DeliveryUnitPhase = "contract_verification"
	DeliveryUnitJourneyTesting        DeliveryUnitPhase = "journey_testing"
	DeliveryUnitComplete              DeliveryUnitPhase = "complete"
)

type DeliveryGateStatus string

const (
	DeliveryGatePending     DeliveryGateStatus = "pending"
	DeliveryGatePassed      DeliveryGateStatus = "passed"
	DeliveryGateNeedsChange DeliveryGateStatus = "needs_change"
)

type DevelopmentTodoStatus string

const (
	DevelopmentTodoNotStarted DevelopmentTodoStatus = "not_started"
	DevelopmentTodoInProgress DevelopmentTodoStatus = "in_progress"
	DevelopmentTodoCompleted  DevelopmentTodoStatus = "completed"
)

const (
	commandInteractionComplete     = commanddomain.DeliveryUnitInteractionComplete
	commandModelComplete           = commanddomain.DeliveryUnitModelComplete
	commandModelVerify             = commanddomain.DeliveryUnitModelVerify
	commandBackendComplete         = commanddomain.DeliveryUnitBackendComplete
	commandFrontendComplete        = commanddomain.DeliveryUnitFrontendComplete
	commandContractVerify          = commanddomain.DeliveryUnitContractVerify
	commandGapReport               = commanddomain.DeliveryUnitGapReport
	commandJourneyComplete         = commanddomain.DeliveryUnitJourneyComplete
	commandDevelopmentTodosInit    = commanddomain.DevelopmentTodosInitialize
	commandDevelopmentTodoComplete = commanddomain.DevelopmentTodoComplete
)

type DeliveryUnit struct {
	ID                               string                `json:"id"`
	Title                            string                `json:"title"`
	DependsOn                        []string              `json:"depends_on"`
	Phase                            DeliveryUnitPhase     `json:"phase"`
	ActiveRole                       string                `json:"active_role,omitempty"`
	ModelGitRevision                 string                `json:"model_git_revision,omitempty"`
	BackendGitRevision               string                `json:"backend_git_revision,omitempty"`
	IntegratedGitRevision            string                `json:"integrated_git_revision,omitempty"`
	InvalidatedIntegratedGitRevision string                `json:"invalidated_integrated_git_revision,omitempty"`
	InteractionStatus                DeliveryGateStatus    `json:"interaction_status"`
	ModelStatus                      DeliveryGateStatus    `json:"model_status"`
	BackendStatus                    DeliveryGateStatus    `json:"backend_status"`
	FrontendStatus                   DeliveryGateStatus    `json:"frontend_status"`
	ContractStatus                   DeliveryGateStatus    `json:"contract_status"`
	JourneyStatus                    DeliveryGateStatus    `json:"journey_status"`
	ModelEvidence                    *BackendGuideEvidence `json:"model_evidence,omitempty"`
	ContractEvidence                 *BackendGuideEvidence `json:"contract_evidence,omitempty"`
	JourneyEvidence                  *BackendGuideEvidence `json:"journey_evidence,omitempty"`
	LatestGate                       *DeliveryGateResult   `json:"latest_gate,omitempty"`
	DevelopmentTodos                 []DevelopmentTodo     `json:"development_todos"`
}

// DevelopmentTodo is one ordered, authoritative unit of development work.
// Category is batch-provided data rather than a fixed enum. Phase controls the
// technical gate that owns the item and is not the Todo classification.
type DevelopmentTodo struct {
	ID         string                    `json:"id"`
	Sequence   int                       `json:"sequence"`
	Phase      DeliveryUnitPhase         `json:"phase"`
	Category   string                    `json:"category"`
	SourceKind string                    `json:"source_kind"`
	SourceID   string                    `json:"source_id"`
	Title      string                    `json:"title"`
	Detail     string                    `json:"detail"`
	Status     DevelopmentTodoStatus     `json:"status"`
	Evidence   []DevelopmentTodoEvidence `json:"evidence"`
}

type DevelopmentTodoEvidence struct {
	Status       DeliveryGateStatus    `json:"status"`
	GitRevision  string                `json:"git_revision"`
	Summary      string                `json:"summary"`
	EvidenceRefs []string              `json:"evidence_refs"`
	Diagnostics  []GateDiagnostic      `json:"diagnostics"`
	BackendGuide *BackendGuideEvidence `json:"backend_guide,omitempty"`
	RecordedBy   string                `json:"recorded_by"`
	RecordedAt   time.Time             `json:"recorded_at"`
}

type DeliveryGateResult struct {
	Phase        DeliveryUnitPhase     `json:"phase"`
	Status       DeliveryGateStatus    `json:"status"`
	GitRevision  string                `json:"git_revision"`
	Summary      string                `json:"summary"`
	EvidenceRefs []string              `json:"evidence_refs"`
	Diagnostics  []GateDiagnostic      `json:"diagnostics"`
	FailureOwner string                `json:"failure_owner,omitempty"`
	BackendGuide *BackendGuideEvidence `json:"backend_guide,omitempty"`
	RecordedBy   string                `json:"recorded_by"`
	RecordedAt   time.Time             `json:"recorded_at"`
}

// BackendGuideEvidence is the machine-verifiable subset of the Plane backend
// guide. Free-form summaries remain explanatory and never satisfy a gate.
type BackendGuideEvidence struct {
	WorkspaceID                string    `json:"workspace_id"`
	ProductID                  string    `json:"product_id"`
	FeatureRevision            uint64    `json:"feature_revision"`
	RepositoryIdentity         string    `json:"repository_identity"`
	GitRevision                string    `json:"git_revision"`
	GitStatus                  string    `json:"git_status"`
	CheckSuite                 string    `json:"check_suite"`
	CheckVersion               string    `json:"check_version"`
	ExecutedBy                 string    `json:"executed_by"`
	OccurredAt                 time.Time `json:"occurred_at"`
	EvidenceRefs               []string  `json:"evidence_refs"`
	ModelPath                  string    `json:"model_path"`
	ModelSHA256                string    `json:"model_sha256"`
	SingleBackendModel         bool      `json:"single_backend_model"`
	StrictModelValidation      bool      `json:"strict_model_validation"`
	CrossReferencesResolved    bool      `json:"cross_references_resolved"`
	GoBehaviorRegistry         bool      `json:"go_behavior_registry"`
	NoExecutableBehaviorJSON   bool      `json:"no_executable_behavior_json"`
	ProjectHTTP                bool      `json:"project_http"`
	HandlerUnitOfWork          bool      `json:"handler_unit_of_work"`
	HandlerIdempotency         bool      `json:"handler_idempotency"`
	DefinitionsRegistered      bool      `json:"definitions_registered"`
	RuntimeBootstrap           bool      `json:"runtime_bootstrap"`
	NoProjectSchemaSQL         bool      `json:"no_project_schema_sql"`
	BackendGoModOnly           bool      `json:"backend_go_mod_only"`
	NoRootGoMod                bool      `json:"no_root_go_mod"`
	NoGoWork                   bool      `json:"no_go_work"`
	NoCompilerBuilder          bool      `json:"no_compiler_builder"`
	NoGeneratedRuntimeContract bool      `json:"no_generated_runtime_contract"`
	AuthWorkspacePermission    bool      `json:"auth_workspace_permission"`
	DataAuditPersistence       bool      `json:"data_audit_persistence"`
	EmptyDatabaseInitPassed    bool      `json:"empty_database_init_passed"`
	SameModelRestartPassed     bool      `json:"same_model_restart_passed"`
	ChangedModelRejected       bool      `json:"changed_model_rejected"`
	MockJourneyPassed          bool      `json:"mock_journey_passed"`
	RuntimeJourneyPassed       bool      `json:"runtime_journey_passed"`
}

type GateDiagnostic struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Path        string `json:"path"`
	Message     string `json:"message"`
	Owner       string `json:"owner"`
	Category    string `json:"category"`
	Remediation string `json:"remediation,omitempty"`
}

type deliveryUnitImplementationPayload struct {
	DeliveryUnitID string            `json:"delivery_unit_id"`
	Phase          DeliveryUnitPhase `json:"phase"`
	GitRevision    string            `json:"git_revision"`
	Summary        string            `json:"summary"`
	EvidenceRefs   []string          `json:"evidence_refs"`
}

type deliveryUnitVerificationPayload struct {
	DeliveryUnitID string                `json:"delivery_unit_id"`
	Phase          DeliveryUnitPhase     `json:"phase"`
	GitRevision    string                `json:"git_revision"`
	Summary        string                `json:"summary"`
	EvidenceRefs   []string              `json:"evidence_refs"`
	BackendGuide   *BackendGuideEvidence `json:"backend_guide"`
}

type deliveryUnitGapPayload struct {
	DeliveryUnitID string            `json:"delivery_unit_id"`
	Phase          DeliveryUnitPhase `json:"phase"`
	GitRevision    string            `json:"git_revision"`
	Summary        string            `json:"summary"`
	EvidenceRefs   []string          `json:"evidence_refs"`
	Diagnostics    []GateDiagnostic  `json:"diagnostics"`
	FailureOwner   string            `json:"failure_owner"`
}

type developmentTodoCompletePayload struct {
	DeliveryUnitID string   `json:"delivery_unit_id"`
	TodoID         string   `json:"todo_id"`
	GitRevision    string   `json:"git_revision"`
	Summary        string   `json:"summary"`
	EvidenceRefs   []string `json:"evidence_refs"`
}

type developmentTodoInitializeItem struct {
	Phase      DeliveryUnitPhase `json:"phase"`
	Category   string            `json:"category"`
	SourceKind string            `json:"source_kind"`
	SourceID   string            `json:"source_id"`
	Title      string            `json:"title"`
	Detail     string            `json:"detail"`
}

type developmentTodosInitializePayload struct {
	DeliveryUnitID string                          `json:"delivery_unit_id"`
	Todos          []developmentTodoInitializeItem `json:"todos"`
}

// deliveryUnitResult is the normalized domain input after one of the three
// strict wire payloads above has been decoded. It is not a transport DTO.
type deliveryUnitResult struct {
	DeliveryUnitID string
	Phase          DeliveryUnitPhase
	GitRevision    string
	Summary        string
	EvidenceRefs   []string
	Diagnostics    []GateDiagnostic
	FailureOwner   string
	BackendGuide   *BackendGuideEvidence
}
