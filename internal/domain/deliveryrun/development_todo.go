package deliveryrun

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

var orderedDevelopmentPhases = []DeliveryUnitPhase{
	DeliveryUnitInteractionModeling,
	DeliveryUnitDomainModeling,
	DeliveryUnitModelVerification,
	DeliveryUnitFrontendImplementation,
	DeliveryUnitBackendImplementation,
	DeliveryUnitContractVerification,
	DeliveryUnitJourneyTesting,
}

func newDevelopmentTodos(unitID string) []DevelopmentTodo {
	todos := make([]DevelopmentTodo, 0, len(orderedDevelopmentPhases))
	for index, phase := range orderedDevelopmentPhases {
		status := DevelopmentTodoNotStarted
		if index == 0 {
			status = DevelopmentTodoInProgress
		}
		workItem := DevelopmentPhaseWorkItem{
			ID:       fmt.Sprintf("%s:phase:%s", unitID, phase),
			Phase:    phase,
			Status:   status,
			Evidence: []DevelopmentTodoEvidence{},
		}
		todos = append(todos, DevelopmentTodo{
			ID:             workItem.ID,
			Sequence:       index + 1,
			Category:       "development_phase",
			SourceKind:     "delivery_phase",
			SourceID:       string(phase),
			Title:          string(phase),
			Detail:         string(phase),
			Status:         status,
			PhaseWorkItems: []DevelopmentPhaseWorkItem{workItem},
		})
	}
	return todos
}

func developmentPhaseOrder(phase DeliveryUnitPhase) int {
	for index, candidate := range orderedDevelopmentPhases {
		if candidate == phase {
			return index
		}
	}
	return len(orderedDevelopmentPhases)
}

func completeDevelopmentPhaseWorkItems(unit *DeliveryUnit, gate DeliveryGateResult) {
	complete := func(workItem *DevelopmentPhaseWorkItem) {
		if workItem.Phase != gate.Phase || workItem.Status == DevelopmentTodoCompleted {
			return
		}
		workItem.Status = DevelopmentTodoCompleted
		workItem.Evidence = append(workItem.Evidence, DevelopmentTodoEvidence{
			Status: gate.Status, GitRevision: gate.GitRevision, Summary: gate.Summary,
			EvidenceRefs: clone(gate.EvidenceRefs), Diagnostics: clone(gate.Diagnostics), BackendGuide: clone(gate.BackendGuide), RecordedBy: gate.RecordedBy, RecordedAt: gate.RecordedAt,
		})
	}
	for index := range unit.DevelopmentRepairItems {
		complete(&unit.DevelopmentRepairItems[index].DevelopmentPhaseWorkItem)
	}
	for todoIndex := range unit.DevelopmentTodos {
		for workItemIndex := range unit.DevelopmentTodos[todoIndex].PhaseWorkItems {
			complete(&unit.DevelopmentTodos[todoIndex].PhaseWorkItems[workItemIndex])
		}
	}
	syncDevelopmentTodoStatuses(unit)
}

type developmentRepairTarget struct {
	SourceKind string
	SourceID   string
	Title      string
	Detail     string
}

func repairTargetsFromDiagnostics(diagnostics []GateDiagnostic, failureOwner string) []developmentRepairTarget {
	targets := make([]developmentRepairTarget, 0, len(diagnostics))
	seen := make(map[string]bool, len(diagnostics))
	for _, diagnostic := range diagnostics {
		owner := strings.TrimSpace(diagnostic.Owner)
		if owner != failureOwner && !(failureOwner == "model" && owner == "project_model") {
			continue
		}
		category := strings.TrimSpace(diagnostic.Category)
		path := strings.TrimSpace(diagnostic.Path)
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s", failureOwner, strings.TrimSpace(diagnostic.Code), category, path)
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
		if seen[digest] {
			continue
		}
		seen[digest] = true
		targets = append(targets, developmentRepairTarget{
			SourceKind: "verification_gap",
			SourceID:   digest,
			Title:      "Repair " + category + " verification gap",
			Detail:     strings.TrimSpace(diagnostic.Message) + "\nEvidence path: " + path,
		})
	}
	if len(targets) > 0 {
		return targets
	}
	for _, diagnostic := range diagnostics {
		category := strings.TrimSpace(diagnostic.Category)
		path := strings.TrimSpace(diagnostic.Path)
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s", failureOwner, strings.TrimSpace(diagnostic.Code), category, path)
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
		if seen[digest] {
			continue
		}
		seen[digest] = true
		targets = append(targets, developmentRepairTarget{
			SourceKind: "verification_gap",
			SourceID:   digest,
			Title:      "Repair " + category + " verification gap",
			Detail:     strings.TrimSpace(diagnostic.Message) + "\nEvidence path: " + path,
		})
	}
	return targets
}

func reopenDevelopmentRepairTodos(unit *DeliveryUnit, phase DeliveryUnitPhase, targets []developmentRepairTarget) {
	for _, target := range targets {
		matched := false
		for index := range unit.DevelopmentRepairItems {
			repair := &unit.DevelopmentRepairItems[index]
			if repair.Phase != phase || repair.SourceKind != target.SourceKind || repair.SourceID != target.SourceID {
				continue
			}
			repair.Title = target.Title
			repair.Detail = target.Detail
			repair.Status = DevelopmentTodoNotStarted
			matched = true
			break
		}
		if matched {
			continue
		}
		unit.DevelopmentRepairItems = append(unit.DevelopmentRepairItems, DevelopmentRepairWorkItem{
			DevelopmentPhaseWorkItem: DevelopmentPhaseWorkItem{
				ID:       fmt.Sprintf("%s:repair:%s", unit.ID, target.SourceID),
				Phase:    phase,
				Status:   DevelopmentTodoNotStarted,
				Evidence: []DevelopmentTodoEvidence{},
			},
			SourceKind: target.SourceKind,
			SourceID:   target.SourceID,
			Title:      target.Title,
			Detail:     target.Detail,
		})
	}
	syncDevelopmentWorkItemStatuses(unit)
}

func resetDevelopmentPhaseProgress(unit *DeliveryUnit, phases ...DeliveryUnitPhase) {
	reset := make(map[DeliveryUnitPhase]bool, len(phases))
	for _, phase := range phases {
		reset[phase] = true
	}
	for todoIndex := range unit.DevelopmentTodos {
		for workItemIndex := range unit.DevelopmentTodos[todoIndex].PhaseWorkItems {
			workItem := &unit.DevelopmentTodos[todoIndex].PhaseWorkItems[workItemIndex]
			if reset[workItem.Phase] {
				workItem.Status = DevelopmentTodoNotStarted
			}
		}
	}
	syncDevelopmentTodoStatuses(unit)
}

func syncDevelopmentWorkItemStatuses(unit *DeliveryUnit) {
	if unit.Phase == DeliveryUnitComplete {
		for index := range unit.DevelopmentRepairItems {
			unit.DevelopmentRepairItems[index].Status = DevelopmentTodoCompleted
		}
		for todoIndex := range unit.DevelopmentTodos {
			for workItemIndex := range unit.DevelopmentTodos[todoIndex].PhaseWorkItems {
				unit.DevelopmentTodos[todoIndex].PhaseWorkItems[workItemIndex].Status = DevelopmentTodoCompleted
			}
		}
		syncDevelopmentTodoStatuses(unit)
		return
	}
	for index := range unit.DevelopmentRepairItems {
		if unit.DevelopmentRepairItems[index].Phase != unit.Phase && unit.DevelopmentRepairItems[index].Status == DevelopmentTodoInProgress {
			unit.DevelopmentRepairItems[index].Status = DevelopmentTodoNotStarted
		}
	}
	for todoIndex := range unit.DevelopmentTodos {
		for workItemIndex := range unit.DevelopmentTodos[todoIndex].PhaseWorkItems {
			workItem := &unit.DevelopmentTodos[todoIndex].PhaseWorkItems[workItemIndex]
			if workItem.Phase != unit.Phase && workItem.Status == DevelopmentTodoInProgress {
				workItem.Status = DevelopmentTodoNotStarted
			}
		}
	}
	repairActive := false
	for index := range unit.DevelopmentRepairItems {
		workItem := &unit.DevelopmentRepairItems[index].DevelopmentPhaseWorkItem
		if workItem.Phase != unit.Phase || workItem.Status == DevelopmentTodoCompleted {
			continue
		}
		if !repairActive {
			workItem.Status = DevelopmentTodoInProgress
			repairActive = true
		} else {
			workItem.Status = DevelopmentTodoNotStarted
		}
	}
	for todoIndex := range unit.DevelopmentTodos {
		for workItemIndex := range unit.DevelopmentTodos[todoIndex].PhaseWorkItems {
			workItem := &unit.DevelopmentTodos[todoIndex].PhaseWorkItems[workItemIndex]
			if workItem.Phase == unit.Phase && workItem.Status != DevelopmentTodoCompleted {
				workItem.Status = DevelopmentTodoInProgress
			}
		}
	}
	syncDevelopmentTodoStatuses(unit)
}

func syncDevelopmentTodoStatuses(unit *DeliveryUnit) {
	for todoIndex := range unit.DevelopmentTodos {
		todo := &unit.DevelopmentTodos[todoIndex]
		if len(todo.PhaseWorkItems) == 0 {
			todo.Status = DevelopmentTodoNotStarted
			continue
		}
		completed := 0
		inProgress := false
		for _, workItem := range todo.PhaseWorkItems {
			if workItem.Status == DevelopmentTodoCompleted {
				completed++
			} else if workItem.Status == DevelopmentTodoInProgress {
				inProgress = true
			}
		}
		switch {
		case completed == len(todo.PhaseWorkItems):
			todo.Status = DevelopmentTodoCompleted
		case completed > 0 || inProgress:
			todo.Status = DevelopmentTodoInProgress
		default:
			todo.Status = DevelopmentTodoNotStarted
		}
	}
}
