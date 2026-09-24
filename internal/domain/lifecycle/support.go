package lifecycle

import (
	"encoding/json"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain"
	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/domain/product"
)

type Product = product.Product
type ProductRevision = product.ProductRevision
type ProductDeployment = product.ProductDeployment
type DeliveryRun = deliveryrun.DeliveryRun
type DeliveryRunSpec = deliveryrun.DeliveryRunSpec
type Member = deliveryrun.Member
type Release = deliveryrun.Release
type Command = domain.Command
type Actor = domain.Actor

const (
	ActorAgent           = domain.ActorAgent
	ActorHuman           = domain.ActorHuman
	ActorSystem          = domain.ActorSystem
	EngineeringReady     = product.EngineeringReady
	FeatureDelivering    = product.FeatureDelivering
	FeatureInstalled     = product.FeatureInstalled
	ProductActive        = product.ProductActive
	StageLive            = deliveryrun.StageLive
	ReleaseLive          = deliveryrun.ReleaseLive
	CommandTargetProduct = commanddomain.TargetProduct
)

func validateCommandActor(value Command, target commanddomain.Target) error {
	return commanddomain.ValidateActor(value, target)
}

func decode(payload json.RawMessage, target any) error { return domain.Decode(payload, target) }
func clone[T any](value T) T                           { return domain.Clone(value) }
func Invalid(code, message string) error               { return domain.Invalid(code, message) }
func NotFound(entity, id string) error                 { return domain.NotFound(entity, id) }

func nextFeatureForDelivery(value *Product) *product.Feature {
	return product.NextFeatureForDelivery(value)
}

func findFeature(value *Product, id string) *product.Feature {
	return product.FindFeature(value, id)
}

func NewDeliveryRun(value Product, featureID string, featureRevision uint64, spec DeliveryRunSpec, actor Actor, now time.Time) (DeliveryRun, error) {
	return deliveryrun.NewDeliveryRun(value, featureID, featureRevision, spec, actor, now)
}
