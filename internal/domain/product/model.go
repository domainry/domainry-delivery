package product

import (
	"encoding/json"
	"time"
)

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
	EngineeringPreparationQueued    EngineeringStatus = "preparation_queued"
	EngineeringPreparing            EngineeringStatus = "preparing"
	EngineeringFoundationPending    EngineeringStatus = "foundation_pending"
	EngineeringFoundationInstalling EngineeringStatus = "foundation_installing"
	EngineeringReady                EngineeringStatus = "ready"
)

type EngineeringMode string

const (
	EngineeringInitialize          EngineeringMode = "initialize"
	EngineeringAdoptExistingSource EngineeringMode = "adopt_existing_source"
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

// ProductEngineering records whether Delivery initializes an empty project or
// adopts an existing source baseline before the business Feature lifecycle.
type ProductEngineering struct {
	Mode                         EngineeringMode             `json:"mode"`
	Status                       EngineeringStatus           `json:"status"`
	SourceRevision               string                      `json:"source_revision,omitempty"`
	SourceRef                    string                      `json:"source_ref,omitempty"`
	PreparationCodeRevision      string                      `json:"preparation_code_revision,omitempty"`
	PreparationArtifactRef       string                      `json:"preparation_artifact_ref,omitempty"`
	DesignContractRef            string                      `json:"design_contract_ref,omitempty"`
	LoginEntry                   string                      `json:"login_entry,omitempty"`
	ShellEntry                   string                      `json:"shell_entry,omitempty"`
	PreviewEntry                 string                      `json:"preview_entry,omitempty"`
	PreparationStartedBy         string                      `json:"preparation_started_by,omitempty"`
	PreparationStartedAt         *time.Time                  `json:"preparation_started_at,omitempty"`
	PreparationCompletedBy       string                      `json:"preparation_completed_by,omitempty"`
	PreparationCompletedAt       *time.Time                  `json:"preparation_completed_at,omitempty"`
	InitialFeatureRevisions      []FoundationFeatureRevision `json:"initial_feature_revisions,omitempty"`
	FoundationDesign             *FoundationDesign           `json:"foundation_design,omitempty"`
	ApplicationDeliverySHA256    string                      `json:"application_delivery_sha256,omitempty"`
	FoundationReleaseSHA256      string                      `json:"foundation_release_sha256,omitempty"`
	FoundationPackageSHA256      string                      `json:"foundation_package_sha256,omitempty"`
	FoundationModelSHA256        string                      `json:"foundation_model_sha256,omitempty"`
	FoundationIdempotencyKey     string                      `json:"foundation_idempotency_key,omitempty"`
	FoundationCodeRevision       string                      `json:"foundation_code_revision,omitempty"`
	FoundationVerificationSHA256 string                      `json:"foundation_verification_sha256,omitempty"`
	IdentityBaselineResult       string                      `json:"identity_baseline_result,omitempty"`
	FoundationStartedBy          string                      `json:"foundation_started_by,omitempty"`
	FoundationStartedAt          *time.Time                  `json:"foundation_started_at,omitempty"`
	FoundationCompletedBy        string                      `json:"foundation_completed_by,omitempty"`
	FoundationCompletedAt        *time.Time                  `json:"foundation_completed_at,omitempty"`
	FoundationFailureCode        string                      `json:"foundation_failure_code,omitempty"`
	FoundationFailureMessage     string                      `json:"foundation_failure_message,omitempty"`
}

// FoundationFeatureRevision freezes the confirmed business scope used by the
// one-time Source Foundation without copying Feature content into engineering state.
type FoundationFeatureRevision struct {
	FeatureID       string `json:"feature_id"`
	FeatureRevision uint64 `json:"feature_revision"`
}

// FoundationDesign is the immutable input selected for one Foundation attempt.
// It is replaceable only while Foundation is pending and is not a long-lived
// Product capability registry.
type FoundationDesign struct {
	ProductRevision         uint64                       `json:"product_revision"`
	InitialFeatureRevisions []FoundationFeatureRevision  `json:"initial_feature_revisions"`
	Model                   json.RawMessage              `json:"model"`
	ModelSHA256             string                       `json:"model_sha256"`
	Capabilities            []string                     `json:"capabilities"`
	ModuleDeployments       []FoundationModuleDeployment `json:"module_deployments"`
	DesignSHA256            string                       `json:"design_sha256"`
	RecordedBy              string                       `json:"recorded_by"`
	RecordedAt              time.Time                    `json:"recorded_at"`
}

type FoundationModuleDeployment struct {
	Capability string `json:"capability"`
	Mode       string `json:"mode"`
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
