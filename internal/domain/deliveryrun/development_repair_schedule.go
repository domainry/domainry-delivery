package deliveryrun

// repairPhaseForOwner selects responsibility without invalidating dependencies.
func repairPhaseForOwner(owner string) DeliveryUnitPhase {
	switch owner {
	case "interaction":
		return DeliveryUnitInteractionModeling
	case "model":
		return DeliveryUnitDomainModeling
	case "frontend":
		return DeliveryUnitFrontendImplementation
	case "backend":
		return DeliveryUnitBackendImplementation
	}
	return ""
}

func invalidateRepairPhase(unit *DeliveryUnit, phase DeliveryUnitPhase) {
	switch phase {
	case DeliveryUnitInteractionModeling:
		unit.InteractionStatus = DeliveryGateNeedsChange
	case DeliveryUnitDomainModeling:
		unit.ModelStatus = DeliveryGateNeedsChange
	case DeliveryUnitFrontendImplementation:
		unit.FrontendStatus = DeliveryGateNeedsChange
	case DeliveryUnitBackendImplementation:
		unit.BackendStatus = DeliveryGateNeedsChange
	}
	resetUnfinishedDevelopmentPhaseProgress(unit, phase)
}

func nextRequiredDevelopmentPhase(unit *DeliveryUnit) DeliveryUnitPhase {
	for _, required := range []struct {
		phase  DeliveryUnitPhase
		status DeliveryGateStatus
	}{
		{DeliveryUnitInteractionModeling, unit.InteractionStatus},
		{DeliveryUnitDomainModeling, unit.ModelStatus},
		{DeliveryUnitFrontendImplementation, unit.FrontendStatus},
		{DeliveryUnitBackendImplementation, unit.BackendStatus},
	} {
		if required.status != DeliveryGatePassed {
			return required.phase
		}
	}
	for _, phase := range orderedDevelopmentPhases {
		for _, repair := range unit.DevelopmentRepairItems {
			if repair.Phase == phase && repair.Status != DevelopmentTodoCompleted {
				return phase
			}
		}
	}
	return DeliveryUnitContractVerification
}

// Already completed verification Todos remain unchanged. Reverification gets
// its own record, linked to the revision that started this repair.
func queueRepairValidation(unit *DeliveryUnit, phase DeliveryUnitPhase, revision string) {
	completed := false
	for _, todo := range unit.DevelopmentTodos {
		for _, item := range todo.PhaseWorkItems {
			if item.Phase == phase && item.Status == DevelopmentTodoCompleted {
				completed = true
			}
		}
	}
	if !completed {
		return
	}
	reopenDevelopmentRepairTodos(unit, phase, []developmentRepairTarget{{
		SourceKind: "repair_validation", SourceID: revision,
		Title:  "Reverify repaired source: " + string(phase),
		Detail: "Verify the repaired integrated source; retain the original receipt at " + revision,
	}})
}

// Only a System Runner model verification can establish this change. An Agent
// cannot declare an unchanged model and bypass the required implementation work.
func queueModelChangeRepairs(unit *DeliveryUnit, modelHash string) {
	for _, phase := range []DeliveryUnitPhase{DeliveryUnitFrontendImplementation, DeliveryUnitBackendImplementation} {
		invalidateRepairPhase(unit, phase)
		reopenDevelopmentRepairTodos(unit, phase, []developmentRepairTarget{{
			SourceKind: "model_change", SourceID: modelHash,
			Title:  "Align implementation with verified model: " + string(phase),
			Detail: "The System Runner verified a changed model hash: " + modelHash,
		}})
	}
}
