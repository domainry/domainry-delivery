package deliveryrun

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

var orderedDevelopmentPhases = []DeliveryUnitPhase{
	DeliveryUnitInteractionModeling,
	DeliveryUnitDomainModeling,
	DeliveryUnitModelVerification,
	DeliveryUnitBackendImplementation,
	DeliveryUnitFrontendConvergence,
	DeliveryUnitContractVerification,
	DeliveryUnitJourneyTesting,
}

func initializeDevelopmentTodos(run *DeliveryRun, command Command, now time.Time) error {
	unit := activeDeliveryUnit(run)
	if unit == nil || unit.Phase != DeliveryUnitInteractionModeling || len(unit.DevelopmentTodos) != 0 || len(unit.DevelopmentRepairItems) != 0 {
		return Invalid("development_todos_already_initialized")
	}
	var payload developmentTodosInitializePayload
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.DeliveryUnitID = strings.TrimSpace(payload.DeliveryUnitID)
	if payload.DeliveryUnitID != unit.ID || len(payload.Todos) == 0 || len(payload.Todos) > 500 {
		return Invalid("development_todo_batch_invalid")
	}
	phaseCounts := make(map[DeliveryUnitPhase]int, len(orderedDevelopmentPhases))
	capabilityIDs := make(map[string]bool, len(payload.Todos))
	todos := make([]DevelopmentTodo, 0, len(payload.Todos))
	for index, candidate := range payload.Todos {
		candidate.CapabilityID = strings.TrimSpace(candidate.CapabilityID)
		candidate.Category = strings.TrimSpace(candidate.Category)
		candidate.SourceKind = strings.TrimSpace(candidate.SourceKind)
		candidate.SourceID = strings.TrimSpace(candidate.SourceID)
		candidate.Title = strings.TrimSpace(candidate.Title)
		candidate.Detail = strings.TrimSpace(candidate.Detail)
		if candidate.CapabilityID == "" || len(candidate.CapabilityID) > 500 || capabilityIDs[candidate.CapabilityID] || candidate.Category == "" || len(candidate.Category) > 120 || candidate.SourceKind == "" || len(candidate.SourceKind) > 120 || candidate.SourceID == "" || len(candidate.SourceID) > 500 || candidate.Title == "" || len(candidate.Title) > 500 || candidate.Detail == "" || len(candidate.Detail) > 4000 || len(candidate.PhasePlan) == 0 || len(candidate.PhasePlan) > len(orderedDevelopmentPhases) {
			return Invalid("development_todo_batch_invalid")
		}
		capabilityIDs[candidate.CapabilityID] = true
		phaseWorkItems := make([]DevelopmentPhaseWorkItem, 0, len(candidate.PhasePlan))
		lastPhaseOrder := -1
		for _, phase := range candidate.PhasePlan {
			phaseOrder := developmentPhaseOrder(phase)
			if phaseOrder == len(orderedDevelopmentPhases) || phaseOrder <= lastPhaseOrder {
				return Invalid("development_todo_phase_plan_invalid")
			}
			lastPhaseOrder = phaseOrder
			phaseCounts[phase]++
			phaseWorkItems = append(phaseWorkItems, DevelopmentPhaseWorkItem{
				ID:       fmt.Sprintf("%s:capability:%s:phase:%s", unit.ID, candidate.CapabilityID, phase),
				Phase:    phase,
				Status:   DevelopmentTodoNotStarted,
				Evidence: []DevelopmentTodoEvidence{},
			})
		}
		todos = append(todos, DevelopmentTodo{
			ID: candidate.CapabilityID, Sequence: index + 1,
			Category: candidate.Category, SourceKind: candidate.SourceKind, SourceID: candidate.SourceID,
			Title: candidate.Title, Detail: candidate.Detail, Status: DevelopmentTodoNotStarted, PhaseWorkItems: phaseWorkItems,
		})
	}
	for _, phase := range orderedDevelopmentPhases {
		if phaseCounts[phase] == 0 {
			return Invalid("development_todo_phase_missing")
		}
	}
	unit.DevelopmentTodos = todos
	syncDevelopmentWorkItemStatuses(unit)
	appendActivity(run, command.Actor, "development_todos_initialized", "Development capability plan initialized", fmt.Sprintf("%d ordered capabilities", len(todos)), now)
	return nil
}

func developmentPhaseOrder(phase DeliveryUnitPhase) int {
	for index, candidate := range orderedDevelopmentPhases {
		if candidate == phase {
			return index
		}
	}
	return len(orderedDevelopmentPhases)
}

func completeDevelopmentTodo(run *DeliveryRun, command Command, now time.Time) error {
	unit := activeDeliveryUnit(run)
	if unit == nil || unit.ActiveRole == "system" {
		return Invalid("development_todo_inactive")
	}
	var payload developmentTodoCompletePayload
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.DeliveryUnitID = strings.TrimSpace(payload.DeliveryUnitID)
	payload.TodoID = strings.TrimSpace(payload.TodoID)
	payload.GitRevision = strings.TrimSpace(payload.GitRevision)
	payload.Summary = strings.TrimSpace(payload.Summary)
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	if payload.DeliveryUnitID != unit.ID || !validGitRevision(payload.GitRevision) || payload.Summary == "" || len(payload.Summary) > 2000 || len(payload.EvidenceRefs) == 0 || len(payload.EvidenceRefs) > 100 {
		return Invalid("development_todo_evidence_invalid")
	}
	for _, evidenceRef := range payload.EvidenceRefs {
		if len(evidenceRef) > 1024 {
			return Invalid("development_todo_evidence_invalid")
		}
	}
	workItem := activeDevelopmentWorkItem(unit)
	if workItem == nil || workItem.ID != payload.TodoID || workItem.Phase != unit.Phase {
		return Invalid("development_todo_conflict")
	}
	workItem.Evidence = append(workItem.Evidence, DevelopmentTodoEvidence{
		Status: DeliveryGatePassed, GitRevision: payload.GitRevision, Summary: payload.Summary,
		EvidenceRefs: clone(payload.EvidenceRefs), Diagnostics: []GateDiagnostic{}, RecordedBy: command.Actor.ID, RecordedAt: now,
	})
	workItem.Status = DevelopmentTodoCompleted
	syncDevelopmentWorkItemStatuses(unit)
	appendActivity(run, command.Actor, "development_work_item_completed", workItem.ID+" completed", payload.Summary, now)
	return nil
}

func activeDevelopmentWorkItem(unit *DeliveryUnit) *DevelopmentPhaseWorkItem {
	for index := range unit.DevelopmentRepairItems {
		workItem := &unit.DevelopmentRepairItems[index].DevelopmentPhaseWorkItem
		if workItem.Phase == unit.Phase && workItem.Status == DevelopmentTodoInProgress {
			return workItem
		}
	}
	for todoIndex := range unit.DevelopmentTodos {
		for workItemIndex := range unit.DevelopmentTodos[todoIndex].PhaseWorkItems {
			workItem := &unit.DevelopmentTodos[todoIndex].PhaseWorkItems[workItemIndex]
			if workItem.Phase == unit.Phase && workItem.Status == DevelopmentTodoInProgress {
				return workItem
			}
		}
	}
	return nil
}

func developmentPhaseWorkItemsCompleted(unit *DeliveryUnit, phase DeliveryUnitPhase) bool {
	found := false
	for _, repair := range unit.DevelopmentRepairItems {
		if repair.Phase == phase {
			found = true
			if repair.Status != DevelopmentTodoCompleted {
				return false
			}
		}
	}
	for _, todo := range unit.DevelopmentTodos {
		for _, workItem := range todo.PhaseWorkItems {
			if workItem.Phase != phase {
				continue
			}
			found = true
			if workItem.Status != DevelopmentTodoCompleted {
				return false
			}
		}
	}
	return found
}

func completeSystemPhaseWorkItems(unit *DeliveryUnit, gate DeliveryGateResult) {
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
	active := false
	activate := func(workItem *DevelopmentPhaseWorkItem) {
		if workItem.Phase != unit.Phase || workItem.Status == DevelopmentTodoCompleted {
			return
		}
		if !active {
			workItem.Status = DevelopmentTodoInProgress
			active = true
		} else {
			workItem.Status = DevelopmentTodoNotStarted
		}
	}
	for index := range unit.DevelopmentRepairItems {
		activate(&unit.DevelopmentRepairItems[index].DevelopmentPhaseWorkItem)
	}
	for todoIndex := range unit.DevelopmentTodos {
		for workItemIndex := range unit.DevelopmentTodos[todoIndex].PhaseWorkItems {
			activate(&unit.DevelopmentTodos[todoIndex].PhaseWorkItems[workItemIndex])
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
