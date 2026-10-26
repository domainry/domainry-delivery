package product

import (
	"encoding/json"
	"strings"
	"time"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

const (
	commandProductCreate                   = commanddomain.ProductCreate
	commandProductDelete                   = commanddomain.ProductDelete
	commandProductRestore                  = commanddomain.ProductRestore
	commandProductEngineeringPrepareStart  = commanddomain.ProductEngineeringPrepareStart
	commandProductEngineeringPrepareFinish = commanddomain.ProductEngineeringPrepareComplete
	commandProductFoundationStart          = commanddomain.ProductFoundationStarted
	commandProductFoundationFinish         = commanddomain.ProductFoundationComplete
	commandProductFoundationFail           = commanddomain.ProductFoundationFailed
	commandFeatureDiscoveryOpen            = commanddomain.FeatureDiscoveryOpen
	commandFeatureDiscoveryReplace         = commanddomain.FeatureDiscoveryReplace
	commandFeatureDelete                   = commanddomain.FeatureDelete
	commandFeatureConfirm                  = commanddomain.FeatureConfirm
	commandFeatureRebase                   = commanddomain.FeatureRebase
	commandFeatureDelivery                 = commanddomain.FeatureDeliveryStart
)

func NewProduct(workspaceID, productID string, command Command, now time.Time) (Product, error) {
	if command.Type != commandProductCreate {
		return Product{}, Invalid("command_unknown")
	}
	if err := validateCommandActor(command, CommandTargetProduct); err != nil {
		return Product{}, err
	}
	if command.Actor.Kind == ActorSystem {
		return Product{}, Forbidden("actor_unauthorized")
	}
	var payload struct {
		Name        string `json:"name"`
		Code        string `json:"code"`
		Goal        string `json:"goal"`
		Industry    string `json:"industry"`
		Engineering struct {
			Mode           EngineeringMode `json:"mode"`
			SourceRevision string          `json:"source_revision"`
			SourceRef      string          `json:"source_ref"`
		} `json:"engineering"`
		Story      ProductStory      `json:"story"`
		Definition ProductDefinition `json:"definition"`
		Decisions  []ProductDecision `json:"decisions"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return Product{}, err
	}
	productID = strings.TrimSpace(productID)
	workspaceID = strings.TrimSpace(workspaceID)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Code = strings.TrimSpace(payload.Code)
	payload.Goal = strings.TrimSpace(payload.Goal)
	payload.Industry = strings.TrimSpace(payload.Industry)
	payload.Engineering.SourceRevision = strings.TrimSpace(payload.Engineering.SourceRevision)
	payload.Engineering.SourceRef = strings.TrimSpace(payload.Engineering.SourceRef)
	payload.Definition = NormalizeProductDefinition(payload.Definition)
	if workspaceID == "" || productID == "" || payload.Name == "" || payload.Code == "" || payload.Goal == "" || payload.Industry == "" {
		return Product{}, Invalid("product_incomplete")
	}
	content := ProductRevisionContent{Story: payload.Story, Definition: payload.Definition, Decisions: payload.Decisions}
	if err := ValidateProductRevisionContent(content, false); err != nil {
		return Product{}, err
	}
	engineering, err := initialProductEngineering(
		payload.Engineering.Mode,
		payload.Engineering.SourceRevision,
		payload.Engineering.SourceRef,
	)
	if err != nil {
		return Product{}, err
	}
	return Product{
		ID:                        productID,
		WorkspaceID:               workspaceID,
		Name:                      payload.Name,
		Code:                      payload.Code,
		Goal:                      payload.Goal,
		Industry:                  payload.Industry,
		Engineering:               engineering,
		Status:                    ProductShaping,
		Revision:                  1,
		CurrentDefinitionRevision: 1,
		CurrentReleaseRevision:    1,
		Revisions: []ProductRevision{{
			Number: 1, Story: clone(content.Story), Definition: clone(content.Definition), Decisions: clone(content.Decisions),
			CreatedBy: command.Actor.ID, CreatedAt: now,
		}},
		Features:  []Feature{},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func initialProductEngineering(mode EngineeringMode, sourceRevision, sourceRef string) (ProductEngineering, error) {
	switch mode {
	case EngineeringInitialize:
		if sourceRevision != "" || sourceRef != "" {
			return ProductEngineering{}, Invalid("product_engineering_source_unexpected")
		}
		return ProductEngineering{Mode: mode, Status: EngineeringPreparationQueued}, nil
	case EngineeringAdoptExistingSource:
		if !validCodeRevision(sourceRevision) || sourceRef == "" {
			return ProductEngineering{}, Invalid("product_engineering_source_incomplete")
		}
		return ProductEngineering{
			Mode:           mode,
			Status:         EngineeringReady,
			SourceRevision: sourceRevision,
			SourceRef:      sourceRef,
		}, nil
	default:
		return ProductEngineering{}, Invalid("product_engineering_mode_invalid")
	}
}

func ApplyProduct(product *Product, command Command, now time.Time) error {
	if err := validateCommandActor(command, CommandTargetProduct); err != nil {
		return err
	}
	if product.Status == ProductArchived && command.Type != commandProductRestore {
		return Invalid("product_archived")
	}
	if product.Status != ProductArchived && command.Type == commandProductRestore {
		return Invalid("product_not_archived")
	}
	var err error
	switch command.Type {
	case commandProductDelete:
		if command.Actor.Kind != ActorHuman {
			return Invalid("human_deletion_required")
		}
		product.Status = ProductArchived
	case commandProductRestore:
		if command.Actor.Kind != ActorHuman {
			return Invalid("human_restore_required")
		}
		if product.CurrentDefinitionRevision > 1 {
			product.Status = ProductActive
		} else {
			product.Status = ProductShaping
		}
	case commandProductEngineeringPrepareStart:
		if command.Actor.Kind != ActorAgent {
			return Invalid("agent_execution_required")
		}
		err = startProductEngineeringPreparation(product, command, now)
	case commandProductEngineeringPrepareFinish:
		if command.Actor.Kind != ActorAgent {
			return Invalid("agent_execution_required")
		}
		err = completeProductEngineeringPreparation(product, command, now)
	case commandProductFoundationStart:
		if command.Actor.Kind != ActorSystem {
			return Invalid("system_execution_required")
		}
		err = startProductFoundation(product, command, now)
	case commandProductFoundationFinish:
		if command.Actor.Kind != ActorSystem {
			return Invalid("system_execution_required")
		}
		err = completeProductFoundation(product, command, now)
	case commandProductFoundationFail:
		if command.Actor.Kind != ActorSystem {
			return Invalid("system_execution_required")
		}
		err = failProductFoundation(product, command)
	case commandFeatureDiscoveryOpen:
		if command.Actor.Kind != ActorHuman {
			return Forbidden("actor_unauthorized")
		}
		err = openFeatureDiscovery(product, command, now)
	case commandFeatureDiscoveryReplace:
		if command.Actor.Kind == ActorSystem {
			return Forbidden("actor_unauthorized")
		}
		err = replaceFeatureDiscovery(product, command, now)
	case commandFeatureDelete:
		if command.Actor.Kind != ActorHuman {
			return Invalid("human_confirmation_required")
		}
		err = deleteFeature(product, command)
	case commandFeatureConfirm:
		if command.Actor.Kind != ActorHuman {
			return Invalid("human_confirmation_required")
		}
		err = confirmFeature(product, command, now)
	case commandFeatureRebase:
		if command.Actor.Kind != ActorHuman {
			return Invalid("human_confirmation_required")
		}
		err = rebaseFeature(product, command, now)
	default:
		err = Invalid("command_unknown")
	}
	if err != nil {
		return err
	}
	product.UpdatedAt = now
	return nil
}

func startProductEngineeringPreparation(product *Product, command Command, now time.Time) error {
	if product.Engineering.Status != EngineeringPreparationQueued {
		return Invalid("product_engineering_not_queued")
	}
	initialScope := confirmedFoundationScope(product)
	if len(initialScope) == 0 {
		return Invalid("product_initial_scope_missing")
	}
	product.Engineering.Status = EngineeringPreparing
	product.Engineering.InitialFeatureRevisions = initialScope
	product.Engineering.PreparationStartedBy = command.Actor.ID
	product.Engineering.PreparationStartedAt = &now
	return nil
}

func completeProductEngineeringPreparation(product *Product, command Command, now time.Time) error {
	if product.Engineering.Status != EngineeringPreparing {
		return Invalid("product_engineering_not_preparing")
	}
	var payload struct {
		CodeRevision      string                       `json:"code_revision"`
		ArtifactRef       string                       `json:"artifact_ref"`
		DesignContractRef string                       `json:"design_contract_ref"`
		LoginEntry        string                       `json:"login_entry"`
		ShellEntry        string                       `json:"shell_entry"`
		PreviewEntry      string                       `json:"preview_entry"`
		Model             json.RawMessage              `json:"model"`
		Capabilities      []string                     `json:"capabilities"`
		ModuleDeployments []FoundationModuleDeployment `json:"module_deployments"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.CodeRevision = strings.TrimSpace(payload.CodeRevision)
	payload.ArtifactRef = strings.TrimSpace(payload.ArtifactRef)
	payload.DesignContractRef = strings.TrimSpace(payload.DesignContractRef)
	payload.LoginEntry = strings.TrimSpace(payload.LoginEntry)
	payload.ShellEntry = strings.TrimSpace(payload.ShellEntry)
	payload.PreviewEntry = strings.TrimSpace(payload.PreviewEntry)
	if payload.CodeRevision == "" || payload.ArtifactRef == "" || payload.DesignContractRef == "" || payload.LoginEntry == "" || payload.ShellEntry == "" || payload.PreviewEntry == "" {
		return Invalid("product_engineering_evidence_incomplete")
	}
	design, err := prepareFoundationDesign(product, foundationDesignInput{
		Model: payload.Model, Capabilities: payload.Capabilities, ModuleDeployments: payload.ModuleDeployments,
	}, command.Actor.ID, now)
	if err != nil {
		return err
	}
	product.Engineering.Status = EngineeringFoundationPending
	product.Engineering.PreparationCodeRevision = payload.CodeRevision
	product.Engineering.PreparationArtifactRef = payload.ArtifactRef
	product.Engineering.DesignContractRef = payload.DesignContractRef
	product.Engineering.LoginEntry = payload.LoginEntry
	product.Engineering.ShellEntry = payload.ShellEntry
	product.Engineering.PreviewEntry = payload.PreviewEntry
	product.Engineering.PreparationCompletedBy = command.Actor.ID
	product.Engineering.PreparationCompletedAt = &now
	product.Engineering.FoundationDesign = design
	return nil
}

func startProductFoundation(product *Product, command Command, now time.Time) error {
	if product.Engineering.Status != EngineeringFoundationPending {
		return Invalid("product_foundation_not_pending")
	}
	var payload struct {
		ApplicationDeliverySHA256 string `json:"application_delivery_sha256"`
		FoundationDesignSHA256    string `json:"foundation_design_sha256"`
		IdempotencyKey            string `json:"idempotency_key"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.ApplicationDeliverySHA256 = strings.ToLower(strings.TrimSpace(payload.ApplicationDeliverySHA256))
	payload.FoundationDesignSHA256 = strings.ToLower(strings.TrimSpace(payload.FoundationDesignSHA256))
	payload.IdempotencyKey = strings.ToLower(strings.TrimSpace(payload.IdempotencyKey))
	if !sha256ValuePattern.MatchString(payload.ApplicationDeliverySHA256) ||
		!sha256ValuePattern.MatchString(payload.FoundationDesignSHA256) ||
		!sha256ValuePattern.MatchString(payload.IdempotencyKey) {
		return Invalid("product_foundation_identity_invalid")
	}
	if product.Engineering.FoundationDesign == nil || product.Engineering.FoundationDesign.DesignSHA256 != payload.FoundationDesignSHA256 {
		return Invalid("product_foundation_design_mismatch")
	}
	if product.Engineering.ApplicationDeliverySHA256 != "" &&
		(product.Engineering.ApplicationDeliverySHA256 != payload.ApplicationDeliverySHA256 || product.Engineering.FoundationIdempotencyKey != payload.IdempotencyKey) &&
		product.Engineering.FoundationFailureCode == "" {
		return Invalid("product_foundation_retry_conflict")
	}
	product.Engineering.Status = EngineeringFoundationInstalling
	product.Engineering.ApplicationDeliverySHA256 = payload.ApplicationDeliverySHA256
	product.Engineering.FoundationIdempotencyKey = payload.IdempotencyKey
	product.Engineering.FoundationStartedBy = command.Actor.ID
	product.Engineering.FoundationStartedAt = &now
	product.Engineering.FoundationFailureCode = ""
	product.Engineering.FoundationFailureMessage = ""
	return nil
}

func completeProductFoundation(product *Product, command Command, now time.Time) error {
	if product.Engineering.Status != EngineeringFoundationInstalling {
		return Invalid("product_foundation_not_installing")
	}
	var payload struct {
		ApplicationDeliverySHA256 string `json:"application_delivery_sha256"`
		FoundationReleaseSHA256   string `json:"foundation_release_sha256"`
		FoundationPackageSHA256   string `json:"foundation_package_sha256"`
		ModelSHA256               string `json:"model_sha256"`
		IdempotencyKey            string `json:"idempotency_key"`
		CodeRevision              string `json:"code_revision"`
		VerificationSHA256        string `json:"verification_sha256"`
		IdentityBaselineResult    string `json:"identity_baseline_result"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.ApplicationDeliverySHA256 = strings.ToLower(strings.TrimSpace(payload.ApplicationDeliverySHA256))
	payload.FoundationReleaseSHA256 = strings.ToLower(strings.TrimSpace(payload.FoundationReleaseSHA256))
	payload.FoundationPackageSHA256 = strings.ToLower(strings.TrimSpace(payload.FoundationPackageSHA256))
	payload.ModelSHA256 = strings.ToLower(strings.TrimSpace(payload.ModelSHA256))
	payload.IdempotencyKey = strings.ToLower(strings.TrimSpace(payload.IdempotencyKey))
	payload.CodeRevision = strings.TrimSpace(payload.CodeRevision)
	payload.VerificationSHA256 = strings.ToLower(strings.TrimSpace(payload.VerificationSHA256))
	payload.IdentityBaselineResult = strings.TrimSpace(payload.IdentityBaselineResult)
	if !sha256ValuePattern.MatchString(payload.ApplicationDeliverySHA256) ||
		!sha256ValuePattern.MatchString(payload.FoundationReleaseSHA256) ||
		!sha256ValuePattern.MatchString(payload.FoundationPackageSHA256) ||
		!sha256ValuePattern.MatchString(payload.ModelSHA256) ||
		!sha256ValuePattern.MatchString(payload.IdempotencyKey) ||
		!sha256ValuePattern.MatchString(payload.VerificationSHA256) ||
		!validCodeRevision(payload.CodeRevision) || payload.IdentityBaselineResult != "passed" {
		return Invalid("product_foundation_evidence_invalid")
	}
	if product.Engineering.ApplicationDeliverySHA256 != payload.ApplicationDeliverySHA256 ||
		product.Engineering.FoundationIdempotencyKey != payload.IdempotencyKey ||
		product.Engineering.FoundationDesign == nil ||
		product.Engineering.FoundationDesign.ModelSHA256 != payload.ModelSHA256 {
		return Invalid("product_foundation_identity_mismatch")
	}
	product.Engineering.Status = EngineeringReady
	product.Engineering.FoundationReleaseSHA256 = payload.FoundationReleaseSHA256
	product.Engineering.FoundationPackageSHA256 = payload.FoundationPackageSHA256
	product.Engineering.FoundationModelSHA256 = payload.ModelSHA256
	product.Engineering.FoundationCodeRevision = payload.CodeRevision
	product.Engineering.FoundationVerificationSHA256 = payload.VerificationSHA256
	product.Engineering.IdentityBaselineResult = payload.IdentityBaselineResult
	product.Engineering.FoundationCompletedBy = command.Actor.ID
	product.Engineering.FoundationCompletedAt = &now
	return nil
}

func failProductFoundation(product *Product, command Command) error {
	if product.Engineering.Status != EngineeringFoundationInstalling {
		return Invalid("product_foundation_not_installing")
	}
	var payload struct {
		Code        string `json:"code"`
		Message     string `json:"message"`
		Disposition string `json:"disposition"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.Code = strings.TrimSpace(payload.Code)
	payload.Message = strings.TrimSpace(payload.Message)
	payload.Disposition = strings.TrimSpace(payload.Disposition)
	if payload.Code == "" || len(payload.Code) > 128 || payload.Message == "" || len(payload.Message) > 2000 ||
		(payload.Disposition != "retry_same_design" && payload.Disposition != "revise_design") {
		return Invalid("product_foundation_failure_invalid")
	}
	product.Engineering.Status = EngineeringFoundationPending
	product.Engineering.ApplicationDeliverySHA256 = ""
	product.Engineering.FoundationIdempotencyKey = ""
	product.Engineering.FoundationFailureCode = payload.Code
	product.Engineering.FoundationFailureMessage = payload.Message
	if payload.Disposition == "revise_design" {
		product.Engineering.FoundationDesign = nil
		product.Engineering.Status = EngineeringPreparing
	}
	return nil
}
