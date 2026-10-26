package deliveryrun

func activeRoleForPhase(phase DeliveryUnitPhase) string {
	switch phase {
	case DeliveryUnitImplementation:
		return "engineer"
	case DeliveryUnitVerification:
		return "system"
	case DeliveryUnitPlanned, DeliveryUnitComplete:
		return ""
	}
	return ""
}

func commandForPhase(phase DeliveryUnitPhase) (string, ActorKind) {
	switch phase {
	case DeliveryUnitImplementation:
		return commandImplementationComplete, ActorAgent
	case DeliveryUnitVerification:
		return commandVerificationComplete, ActorSystem
	case DeliveryUnitPlanned, DeliveryUnitComplete:
		return "", ""
	}
	return "", ""
}
