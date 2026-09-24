package product

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
	EngineeringFoundationPending    EngineeringStatus = "foundation_pending"
	EngineeringFoundationInstalling EngineeringStatus = "foundation_installing"
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
	Status                       EngineeringStatus `json:"status"`
	FrontendCodeRevision         string            `json:"frontend_code_revision,omitempty"`
	FrontendArtifactRef          string            `json:"frontend_artifact_ref,omitempty"`
	DesignContractRef            string            `json:"design_contract_ref,omitempty"`
	LoginEntry                   string            `json:"login_entry,omitempty"`
	ShellEntry                   string            `json:"shell_entry,omitempty"`
	PreviewEntry                 string            `json:"preview_entry,omitempty"`
	FrontendStartedBy            string            `json:"frontend_started_by,omitempty"`
	FrontendStartedAt            *time.Time        `json:"frontend_started_at,omitempty"`
	FrontendCompletedBy          string            `json:"frontend_completed_by,omitempty"`
	FrontendCompletedAt          *time.Time        `json:"frontend_completed_at,omitempty"`
	ApplicationDeliverySHA256    string            `json:"application_delivery_sha256,omitempty"`
	FoundationReleaseSHA256      string            `json:"foundation_release_sha256,omitempty"`
	FoundationPackageSHA256      string            `json:"foundation_package_sha256,omitempty"`
	FoundationModelSHA256        string            `json:"foundation_model_sha256,omitempty"`
	FoundationIdempotencyKey     string            `json:"foundation_idempotency_key,omitempty"`
	FoundationCodeRevision       string            `json:"foundation_code_revision,omitempty"`
	FoundationGitStatus          string            `json:"foundation_git_status,omitempty"`
	FoundationVerificationSHA256 string            `json:"foundation_verification_sha256,omitempty"`
	IdentityBaselineResult       string            `json:"identity_baseline_result,omitempty"`
	FoundationStartedBy          string            `json:"foundation_started_by,omitempty"`
	FoundationStartedAt          *time.Time        `json:"foundation_started_at,omitempty"`
	FoundationCompletedBy        string            `json:"foundation_completed_by,omitempty"`
	FoundationCompletedAt        *time.Time        `json:"foundation_completed_at,omitempty"`
	FoundationFailureCode        string            `json:"foundation_failure_code,omitempty"`
	FoundationFailureMessage     string            `json:"foundation_failure_message,omitempty"`
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
	DefinitionRef   string            `json:"definition_ref,omitempty"`
	ModelSHA256     string            `json:"model_sha256,omitempty"`
	CreatedBy       string            `json:"created_by"`
	CreatedAt       time.Time         `json:"created_at"`
}
