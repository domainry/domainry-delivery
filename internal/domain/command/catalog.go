package command

import (
	"slices"
	"strings"

	"github.com/domainry/domainry-delivery/internal/domain"
)

type Target string

const (
	TargetProduct     Target = "product"
	TargetDeliveryRun Target = "delivery_run"
)

const (
	ProductCreate             = "product.create"
	ProductDelete             = "product.delete"
	ProductFrontendStart      = "product.engineering.frontend.start"
	ProductFrontendComplete   = "product.engineering.frontend.complete"
	ProductFrontendApprove    = "product.engineering.frontend.approve"
	ProductFrontendRevise     = "product.engineering.frontend.revise"
	ProductFoundationStarted  = "product.engineering.foundation.started"
	ProductFoundationComplete = "product.engineering.foundation.completed"
	ProductFoundationFailed   = "product.engineering.foundation.failed"
	FeatureDiscoveryOpen      = "feature.discovery.open"
	FeatureDiscoveryReplace   = "feature.discovery.replace"
	FeatureConfirm            = "feature.confirm"
	FeatureDeliveryStart      = "feature.delivery.start"

	DeliveryUnitInteractionComplete = "delivery_unit.interaction.complete"
	DeliveryUnitModelComplete       = "delivery_unit.model.complete"
	DeliveryUnitModelVerify         = "delivery_unit.model.verify"
	DeliveryUnitBackendComplete     = "delivery_unit.backend.complete"
	DeliveryUnitFrontendComplete    = "delivery_unit.frontend.complete"
	DeliveryUnitContractVerify      = "delivery_unit.contract.verify"
	DeliveryUnitGapReport           = "delivery_unit.gap.report"
	DeliveryUnitJourneyComplete     = "delivery_unit.journey.complete"
	DevelopmentTodosInitialize      = "development_todos.initialize"
	DevelopmentTodoComplete         = "development_todo.complete"
	ProductRevisionRecord           = "product_revision.record"
	QualityRecord                   = "quality.record"
	AcceptanceConfirm               = "acceptance.confirm"
	ReleaseChecksReplace            = "release_checks.replace"
	ReleaseCheckRecord              = "release_check.record"
	ReleasePrepare                  = "release.prepare"
	ReleaseApprove                  = "release.approve"
	ReleaseDeployResult             = "release.deploy_result"
	ReleaseReconcile                = "release.reconcile"
)

type Definition struct {
	Key           string             `json:"key"`
	Target        Target             `json:"target"`
	PayloadType   string             `json:"payload_type"`
	ReceiptType   string             `json:"receipt_type"`
	Permission    string             `json:"permission"`
	AllowedActors []domain.ActorKind `json:"allowed_actors"`
	LegalStates   []string           `json:"legal_states"`
}

var catalog = buildCatalog()

func buildCatalog() map[string]Definition {
	result := map[string]Definition{}
	register := func(target Target, permission, receiptType string, actors []domain.ActorKind, states []string, commands map[string]string) {
		for key, payloadType := range commands {
			result[key] = Definition{Key: key, Target: target, PayloadType: payloadType, ReceiptType: receiptType, Permission: permission, AllowedActors: actors, LegalStates: states}
		}
	}
	register(TargetDeliveryRun, "delivery_run.write", "DeliveryRunProjection", []domain.ActorKind{domain.ActorAgent}, []string{"development", "testing", "bugs", "release"}, map[string]string{
		DeliveryUnitInteractionComplete: "DeliveryUnitPhaseResult",
		DeliveryUnitModelComplete:       "DeliveryUnitPhaseResult",
		DeliveryUnitBackendComplete:     "DeliveryUnitPhaseResult",
		DeliveryUnitFrontendComplete:    "DeliveryUnitPhaseResult",
		DevelopmentTodosInitialize:      "DevelopmentTodoBatch",
		DevelopmentTodoComplete:         "DevelopmentTodoEvidence",
		ProductRevisionRecord:           "ProductRevisionRecord",
		QualityRecord:                   "QualityResult",
		ReleaseChecksReplace:            "ReleaseCheckSet",
		ReleaseCheckRecord:              "ReleaseCheckResult",
	})
	register(TargetDeliveryRun, "delivery_deployment.record", "DeliveryRunProjection", []domain.ActorKind{domain.ActorSystem}, []string{"development", "release"}, map[string]string{
		DeliveryUnitModelVerify:     "ModelVerification",
		DeliveryUnitContractVerify:  "ContractVerification",
		DeliveryUnitGapReport:       "VerificationGap",
		DeliveryUnitJourneyComplete: "JourneyVerification",
		ReleaseDeployResult:         "DeploymentResult",
	})
	register(TargetDeliveryRun, "delivery_run.write", "DeliveryRunProjection", []domain.ActorKind{domain.ActorHuman}, []string{"acceptance", "release"}, map[string]string{
		AcceptanceConfirm: "AcceptanceConfirmation",
		ReleasePrepare:    "ReleasePreparation",
		ReleaseApprove:    "EntityReference",
		ReleaseReconcile:  "DeploymentReconciliation",
	})
	register(TargetProduct, "delivery_product.write", "ProductProjection", []domain.ActorKind{domain.ActorAgent, domain.ActorHuman}, []string{"new"}, map[string]string{
		ProductCreate: "ProductCreate",
	})
	register(TargetProduct, "delivery_product.write", "ProductProjection", []domain.ActorKind{domain.ActorAgent}, []string{"active"}, map[string]string{
		FeatureDiscoveryReplace: "FeatureDiscoveryRevision",
	})
	register(TargetProduct, "delivery_product.write", "ProductProjection", []domain.ActorKind{domain.ActorAgent}, []string{"active"}, map[string]string{
		ProductFrontendStart:    "Empty",
		ProductFrontendComplete: "FrontendFoundationEvidence",
		FeatureDeliveryStart:    "DeliveryStart",
	})
	register(TargetProduct, "delivery_product.write", "ProductProjection", []domain.ActorKind{domain.ActorHuman}, []string{"active"}, map[string]string{
		ProductDelete:          "ProductDelete",
		ProductFrontendApprove: "FrontendApproval",
		ProductFrontendRevise:  "FrontendReviewFeedback",
		FeatureDiscoveryOpen:   "FeatureDiscoveryOpen",
		FeatureConfirm:         "FeatureConfirmation",
	})
	register(TargetProduct, "delivery_deployment.record", "ProductProjection", []domain.ActorKind{domain.ActorSystem}, []string{"active"}, map[string]string{
		ProductFoundationStarted:  "FoundationStart",
		ProductFoundationComplete: "FoundationCompletion",
		ProductFoundationFailed:   "FoundationFailure",
	})
	return result
}

func DefinitionFor(key string) (Definition, bool) {
	definition, ok := catalog[strings.TrimSpace(key)]
	if !ok {
		return Definition{}, false
	}
	definition.AllowedActors = domain.Clone(definition.AllowedActors)
	definition.LegalStates = domain.Clone(definition.LegalStates)
	return definition, true
}

// Definitions returns the complete command catalog in stable key order. It is
// the only mutation inventory used by projections, transports and architecture
// gates; callers cannot mutate the registered definitions.
func Definitions() []Definition {
	definitions := make([]Definition, 0, len(catalog))
	for _, definition := range catalog {
		copy, _ := DefinitionFor(definition.Key)
		definitions = append(definitions, copy)
	}
	slices.SortFunc(definitions, func(left, right Definition) int {
		return strings.Compare(left.Key, right.Key)
	})
	return definitions
}

func Action(key, targetID string) (domain.AvailableAction, bool) {
	definition, ok := DefinitionFor(key)
	if !ok || len(definition.AllowedActors) != 1 {
		return domain.AvailableAction{}, false
	}
	return domain.AvailableAction{Command: definition.Key, TargetID: strings.TrimSpace(targetID), ActorKind: definition.AllowedActors[0]}, true
}

func KeysFor(target Target, actor domain.ActorKind) []string {
	keys := make([]string, 0)
	for key, definition := range catalog {
		if definition.Target == target && containsActorKind(definition.AllowedActors, actor) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func ValidateActor(value domain.Command, target Target) error {
	if err := ValidateIdentity(value.Actor); err != nil {
		return err
	}
	definition, ok := DefinitionFor(value.Type)
	if !ok || definition.Target != target {
		return domain.Invalid("command_unknown")
	}
	if containsActorKind(definition.AllowedActors, value.Actor.Kind) {
		return nil
	}
	if containsActorKind(definition.AllowedActors, domain.ActorHuman) {
		return domain.Invalid("human_confirmation_required")
	}
	if containsActorKind(definition.AllowedActors, domain.ActorSystem) {
		return domain.Invalid("system_execution_required")
	}
	return domain.Invalid("agent_execution_required")
}

func ValidateIdentity(actor domain.Actor) error {
	if strings.TrimSpace(actor.ID) == "" {
		return domain.Invalid("actor_required")
	}
	if actor.Kind != domain.ActorHuman && actor.Kind != domain.ActorAgent && actor.Kind != domain.ActorSystem {
		return domain.Invalid("actor_kind_invalid")
	}
	return nil
}

func containsActorKind(values []domain.ActorKind, value domain.ActorKind) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
