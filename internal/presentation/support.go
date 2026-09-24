package presentation

import (
	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

type Error = domain.Error
type DeliveryRun = deliveryrun.DeliveryRun
type Projection = deliveryrun.Projection
type FeatureSnapshot = deliveryrun.FeatureSnapshot
type FeatureRevisionRef = deliveryrun.FeatureRevisionRef
type DeliveryUnit = deliveryrun.DeliveryUnit
type ReleaseGate = deliveryrun.ReleaseGate

func ProjectionFor(run DeliveryRun) Projection { return deliveryrun.ProjectionFor(run) }
