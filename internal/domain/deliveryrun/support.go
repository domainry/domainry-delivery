package deliveryrun

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/domainry/domainry-delivery/internal/domain"
	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
	"github.com/domainry/domainry-delivery/internal/domain/product"
)

type Actor = domain.Actor
type ActorKind = domain.ActorKind
type Command = domain.Command
type Session = domain.Session
type Error = domain.Error
type AvailableAction = domain.AvailableAction

type Product = product.Product
type ProductRevision = product.ProductRevision
type ProductRevisionContent = product.ProductRevisionContent
type ProductStory = product.ProductStory
type ProductDefinition = product.ProductDefinition
type ProductDecision = product.ProductDecision
type Feature = product.Feature
type FeatureRevision = product.FeatureRevision
type FeatureDiscovery = product.FeatureDiscovery
type FeatureSpecification = product.FeatureSpecification
type FeatureDecision = product.FeatureDecision
type FeatureReadiness = product.FeatureReadiness
type FeatureSource = product.FeatureSource
type FeatureAcceptanceScenario = product.FeatureAcceptanceScenario

const (
	ActorHuman       = domain.ActorHuman
	ActorAgent       = domain.ActorAgent
	ActorSystem      = domain.ActorSystem
	FeatureConfirmed = product.FeatureConfirmed
)

const CommandTargetDeliveryRun = commanddomain.TargetDeliveryRun

func validateCommandActor(value Command, target commanddomain.Target) error {
	return commanddomain.ValidateActor(value, target)
}

func catalogAction(key, targetID string) AvailableAction {
	action, ok := commanddomain.Action(key, targetID)
	if !ok {
		panic("invalid DeliveryRun command catalog action: " + key)
	}
	return action
}

func Invalid(code, message string) error             { return domain.Invalid(code, message) }
func NotFound(entity, id string) error               { return domain.NotFound(entity, id) }
func Conflict(actual uint64) error                   { return domain.Conflict(actual) }
func validateActor(actor Actor) error                { return commanddomain.ValidateIdentity(actor) }
func findFeature(value *Product, id string) *Feature { return product.FindFeature(value, id) }
func normalizeProductDefinition(value ProductDefinition) ProductDefinition {
	return product.NormalizeProductDefinition(value)
}
func validateProductRevisionContent(value ProductRevisionContent, requireResolvedDecisions bool) error {
	return product.ValidateProductRevisionContent(value, requireResolvedDecisions)
}

func findRelease(run *DeliveryRun, id string) *Release {
	for index := range run.Releases {
		if run.Releases[index].ID == id {
			return &run.Releases[index]
		}
	}
	return nil
}

func findTestCase(run *DeliveryRun, id string) *TestCase {
	for index := range run.TestCases {
		if run.TestCases[index].ID == id {
			return &run.TestCases[index]
		}
	}
	return nil
}

func findAcceptanceCase(run *DeliveryRun, id string) *AcceptanceCase {
	for index := range run.AcceptanceCases {
		if run.AcceptanceCases[index].ID == id {
			return &run.AcceptanceCases[index]
		}
	}
	return nil
}

func findReleaseCheck(run *DeliveryRun, id string) *ReleaseCheck {
	for index := range run.ReleaseChecks {
		if run.ReleaseChecks[index].ID == id {
			return &run.ReleaseChecks[index]
		}
	}
	return nil
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func decode(payload json.RawMessage, target any) error {
	if len(payload) == 0 {
		payload = json.RawMessage([]byte("{}"))
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return Invalid("payload_invalid", "The command payload is invalid: "+err.Error())
	}
	return nil
}

func clone[T any](value T) T {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var copied T
	if err := json.Unmarshal(data, &copied); err != nil {
		panic(err)
	}
	return copied
}

func newID(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}
