package deliveryrun

func activeRoleForPhase(phase DeliveryUnitPhase) string {
	switch phase {
	case DeliveryUnitInteractionModeling, DeliveryUnitFrontendConvergence:
		return "frontend"
	case DeliveryUnitDomainModeling, DeliveryUnitBackendImplementation:
		return "backend"
	case DeliveryUnitModelVerification, DeliveryUnitContractVerification, DeliveryUnitJourneyTesting:
		return "system"
	case DeliveryUnitPlanned, DeliveryUnitComplete:
		return ""
	}
	return ""
}

func commandForPhase(phase DeliveryUnitPhase) (string, ActorKind) {
	switch phase {
	case DeliveryUnitInteractionModeling:
		return commandInteractionComplete, ActorAgent
	case DeliveryUnitDomainModeling:
		return commandModelComplete, ActorAgent
	case DeliveryUnitModelVerification:
		return commandModelVerify, ActorSystem
	case DeliveryUnitBackendImplementation:
		return commandBackendComplete, ActorAgent
	case DeliveryUnitFrontendConvergence:
		return commandFrontendComplete, ActorAgent
	case DeliveryUnitContractVerification:
		return commandContractVerify, ActorSystem
	case DeliveryUnitJourneyTesting:
		return commandJourneyComplete, ActorSystem
	case DeliveryUnitPlanned, DeliveryUnitComplete:
		return "", ""
	}
	return "", ""
}
