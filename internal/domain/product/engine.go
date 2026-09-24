package product

import (
	"strings"
	"time"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

const (
	commandProductCreate           = commanddomain.ProductCreate
	commandProductDelete           = commanddomain.ProductDelete
	commandProductFrontendStart    = commanddomain.ProductFrontendStart
	commandProductFrontendFinish   = commanddomain.ProductFrontendComplete
	commandProductFoundationStart  = commanddomain.ProductFoundationStarted
	commandProductFoundationFinish = commanddomain.ProductFoundationComplete
	commandProductFoundationFail   = commanddomain.ProductFoundationFailed
	commandFeatureDiscoveryOpen    = commanddomain.FeatureDiscoveryOpen
	commandFeatureDiscoveryReplace = commanddomain.FeatureDiscoveryReplace
	commandFeatureConfirm          = commanddomain.FeatureConfirm
	commandFeatureDelivery         = commanddomain.FeatureDeliveryStart
)

func NewProduct(workspaceID, productID string, command Command, now time.Time) (Product, error) {
	if command.Type != commandProductCreate {
		return Product{}, Invalid("command_unknown")
	}
	if err := validateCommandActor(command, CommandTargetProduct); err != nil {
		return Product{}, err
	}
	if command.Actor.Kind == ActorSystem {
		return Product{}, Invalid("actor_unauthorized")
	}
	var payload struct {
		Name       string            `json:"name"`
		Code       string            `json:"code"`
		Goal       string            `json:"goal"`
		Industry   string            `json:"industry"`
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
	payload.Definition = NormalizeProductDefinition(payload.Definition)
	if workspaceID == "" || productID == "" || payload.Name == "" || payload.Code == "" || payload.Goal == "" || payload.Industry == "" {
		return Product{}, Invalid("product_incomplete")
	}
	content := ProductRevisionContent{Story: payload.Story, Definition: payload.Definition, Decisions: payload.Decisions}
	if err := ValidateProductRevisionContent(content, false); err != nil {
		return Product{}, err
	}
	return Product{
		ID:          productID,
		WorkspaceID: workspaceID,
		Name:        payload.Name,
		Code:        payload.Code,
		Goal:        payload.Goal,
		Industry:    payload.Industry,
		Engineering: ProductEngineering{
			Status: EngineeringFrontendQueued,
		},
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

func ApplyProduct(product *Product, command Command, now time.Time) error {
	if product.Status == ProductArchived {
		return Invalid("product_archived")
	}
	if err := validateCommandActor(command, CommandTargetProduct); err != nil {
		return err
	}
	var err error
	switch command.Type {
	case commandProductDelete:
		if command.Actor.Kind != ActorHuman {
			return Invalid("human_deletion_required")
		}
		product.Status = ProductArchived
	case commandProductFrontendStart:
		if command.Actor.Kind != ActorAgent {
			return Invalid("agent_execution_required")
		}
		err = startProductFrontend(product, command, now)
	case commandProductFrontendFinish:
		if command.Actor.Kind != ActorAgent {
			return Invalid("agent_execution_required")
		}
		err = completeProductFrontend(product, command, now)
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
			return Invalid("actor_unauthorized")
		}
		err = openFeatureDiscovery(product, command, now)
	case commandFeatureDiscoveryReplace:
		if command.Actor.Kind == ActorSystem {
			return Invalid("actor_unauthorized")
		}
		err = replaceFeatureDiscovery(product, command, now)
	case commandFeatureConfirm:
		if command.Actor.Kind != ActorHuman {
			return Invalid("human_confirmation_required")
		}
		err = confirmFeature(product, command, now)
	default:
		err = Invalid("command_unknown")
	}
	if err != nil {
		return err
	}
	product.UpdatedAt = now
	return nil
}

func startProductFrontend(product *Product, command Command, now time.Time) error {
	if product.Engineering.Status != EngineeringFrontendQueued {
		return Invalid("product_frontend_not_queued")
	}
	product.Engineering.Status = EngineeringFrontendInitializing
	product.Engineering.FrontendStartedBy = command.Actor.ID
	product.Engineering.FrontendStartedAt = &now
	return nil
}

func completeProductFrontend(product *Product, command Command, now time.Time) error {
	if product.Engineering.Status != EngineeringFrontendInitializing {
		return Invalid("product_frontend_not_initializing")
	}
	var payload struct {
		CodeRevision      string `json:"code_revision"`
		ArtifactRef       string `json:"artifact_ref"`
		DesignContractRef string `json:"design_contract_ref"`
		LoginEntry        string `json:"login_entry"`
		ShellEntry        string `json:"shell_entry"`
		PreviewEntry      string `json:"preview_entry"`
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
		return Invalid("product_frontend_evidence_incomplete")
	}
	product.Engineering.Status = EngineeringFoundationPending
	product.Engineering.FrontendCodeRevision = payload.CodeRevision
	product.Engineering.FrontendArtifactRef = payload.ArtifactRef
	product.Engineering.DesignContractRef = payload.DesignContractRef
	product.Engineering.LoginEntry = payload.LoginEntry
	product.Engineering.ShellEntry = payload.ShellEntry
	product.Engineering.PreviewEntry = payload.PreviewEntry
	product.Engineering.FrontendCompletedBy = command.Actor.ID
	product.Engineering.FrontendCompletedAt = &now
	return nil
}

func startProductFoundation(product *Product, command Command, now time.Time) error {
	if product.Engineering.Status != EngineeringFoundationPending {
		return Invalid("product_foundation_not_pending")
	}
	var payload struct {
		ApplicationDeliverySHA256 string `json:"application_delivery_sha256"`
		IdempotencyKey            string `json:"idempotency_key"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.ApplicationDeliverySHA256 = strings.ToLower(strings.TrimSpace(payload.ApplicationDeliverySHA256))
	payload.IdempotencyKey = strings.ToLower(strings.TrimSpace(payload.IdempotencyKey))
	if !sha256ValuePattern.MatchString(payload.ApplicationDeliverySHA256) || !sha256ValuePattern.MatchString(payload.IdempotencyKey) {
		return Invalid("product_foundation_identity_invalid")
	}
	if product.Engineering.ApplicationDeliverySHA256 != "" &&
		(product.Engineering.ApplicationDeliverySHA256 != payload.ApplicationDeliverySHA256 || product.Engineering.FoundationIdempotencyKey != payload.IdempotencyKey) {
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
		GitStatus                 string `json:"git_status"`
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
	payload.GitStatus = strings.TrimSpace(payload.GitStatus)
	payload.VerificationSHA256 = strings.ToLower(strings.TrimSpace(payload.VerificationSHA256))
	payload.IdentityBaselineResult = strings.TrimSpace(payload.IdentityBaselineResult)
	if !sha256ValuePattern.MatchString(payload.ApplicationDeliverySHA256) ||
		!sha256ValuePattern.MatchString(payload.FoundationReleaseSHA256) ||
		!sha256ValuePattern.MatchString(payload.FoundationPackageSHA256) ||
		!sha256ValuePattern.MatchString(payload.ModelSHA256) ||
		!sha256ValuePattern.MatchString(payload.IdempotencyKey) ||
		!sha256ValuePattern.MatchString(payload.VerificationSHA256) ||
		!validGitRevision(payload.CodeRevision) || payload.GitStatus != "clean" || payload.IdentityBaselineResult != "passed" {
		return Invalid("product_foundation_evidence_invalid")
	}
	if product.Engineering.ApplicationDeliverySHA256 != payload.ApplicationDeliverySHA256 || product.Engineering.FoundationIdempotencyKey != payload.IdempotencyKey {
		return Invalid("product_foundation_identity_mismatch")
	}
	product.Engineering.Status = EngineeringReady
	product.Engineering.FoundationReleaseSHA256 = payload.FoundationReleaseSHA256
	product.Engineering.FoundationPackageSHA256 = payload.FoundationPackageSHA256
	product.Engineering.FoundationModelSHA256 = payload.ModelSHA256
	product.Engineering.FoundationCodeRevision = payload.CodeRevision
	product.Engineering.FoundationGitStatus = payload.GitStatus
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
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.Code = strings.TrimSpace(payload.Code)
	payload.Message = strings.TrimSpace(payload.Message)
	if payload.Code == "" || len(payload.Code) > 128 || payload.Message == "" || len(payload.Message) > 2000 {
		return Invalid("product_foundation_failure_invalid")
	}
	product.Engineering.Status = EngineeringFoundationPending
	product.Engineering.FoundationFailureCode = payload.Code
	product.Engineering.FoundationFailureMessage = payload.Message
	return nil
}
