package deliveryrun

import (
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
	if unit == nil || unit.Phase != DeliveryUnitInteractionModeling || len(unit.DevelopmentTodos) != 0 {
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
	todos := make([]DevelopmentTodo, 0, len(payload.Todos))
	lastPhaseOrder := -1
	for index, candidate := range payload.Todos {
		candidate.Category = strings.TrimSpace(candidate.Category)
		candidate.SourceKind = strings.TrimSpace(candidate.SourceKind)
		candidate.SourceID = strings.TrimSpace(candidate.SourceID)
		candidate.Title = strings.TrimSpace(candidate.Title)
		candidate.Detail = strings.TrimSpace(candidate.Detail)
		phaseOrder := developmentPhaseOrder(candidate.Phase)
		if phaseOrder == len(orderedDevelopmentPhases) || phaseOrder < lastPhaseOrder || candidate.Category == "" || len(candidate.Category) > 120 || candidate.SourceKind == "" || len(candidate.SourceKind) > 120 || candidate.SourceID == "" || len(candidate.SourceID) > 500 || candidate.Title == "" || len(candidate.Title) > 500 || candidate.Detail == "" || len(candidate.Detail) > 4000 {
			return Invalid("development_todo_batch_invalid")
		}
		lastPhaseOrder = phaseOrder
		phaseCounts[candidate.Phase]++
		todos = append(todos, DevelopmentTodo{
			ID: fmt.Sprintf("%s:development_todo:%03d", unit.ID, index+1), Sequence: index + 1,
			Phase: candidate.Phase, Category: candidate.Category, SourceKind: candidate.SourceKind, SourceID: candidate.SourceID,
			Title: candidate.Title, Detail: candidate.Detail, Status: DevelopmentTodoNotStarted, Evidence: []DevelopmentTodoEvidence{},
		})
	}
	for _, phase := range orderedDevelopmentPhases {
		if phaseCounts[phase] == 0 {
			return Invalid("development_todo_phase_missing")
		}
	}
	unit.DevelopmentTodos = todos
	syncDevelopmentTodoStatuses(unit)
	appendActivity(run, command.Actor, "development_todos_initialized", "Development Todo batch initialized", fmt.Sprintf("%d ordered Todos", len(todos)), now)
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
	todo := activeDevelopmentTodo(unit)
	if todo == nil || todo.ID != payload.TodoID || todo.Phase != unit.Phase {
		return Invalid("development_todo_conflict")
	}
	todo.Evidence = append(todo.Evidence, DevelopmentTodoEvidence{
		Status: DeliveryGatePassed, GitRevision: payload.GitRevision, Summary: payload.Summary,
		EvidenceRefs: clone(payload.EvidenceRefs), Diagnostics: []GateDiagnostic{}, RecordedBy: command.Actor.ID, RecordedAt: now,
	})
	todo.Status = DevelopmentTodoCompleted
	syncDevelopmentTodoStatuses(unit)
	appendActivity(run, command.Actor, "development_todo_completed", todo.Title+" completed", payload.Summary, now)
	return nil
}

func activeDevelopmentTodo(unit *DeliveryUnit) *DevelopmentTodo {
	for index := range unit.DevelopmentTodos {
		todo := &unit.DevelopmentTodos[index]
		if todo.Phase == unit.Phase && todo.Status == DevelopmentTodoInProgress {
			return todo
		}
	}
	return nil
}

func developmentPhaseTodosCompleted(unit *DeliveryUnit, phase DeliveryUnitPhase) bool {
	found := false
	for _, todo := range unit.DevelopmentTodos {
		if todo.Phase != phase {
			continue
		}
		found = true
		if todo.Status != DevelopmentTodoCompleted {
			return false
		}
	}
	return found
}

func completeSystemPhaseTodos(unit *DeliveryUnit, gate DeliveryGateResult) {
	for index := range unit.DevelopmentTodos {
		todo := &unit.DevelopmentTodos[index]
		if todo.Phase != gate.Phase || todo.Status == DevelopmentTodoCompleted {
			continue
		}
		todo.Status = DevelopmentTodoCompleted
		todo.Evidence = append(todo.Evidence, DevelopmentTodoEvidence{
			Status: gate.Status, GitRevision: gate.GitRevision, Summary: gate.Summary,
			EvidenceRefs: clone(gate.EvidenceRefs), Diagnostics: clone(gate.Diagnostics), BackendGuide: clone(gate.BackendGuide), RecordedBy: gate.RecordedBy, RecordedAt: gate.RecordedAt,
		})
	}
}

func resetDevelopmentTodosFromPhase(unit *DeliveryUnit, phase DeliveryUnitPhase) {
	activeOrder := developmentPhaseOrder(phase)
	if activeOrder == len(orderedDevelopmentPhases) {
		return
	}
	for index := range unit.DevelopmentTodos {
		if developmentPhaseOrder(unit.DevelopmentTodos[index].Phase) >= activeOrder {
			unit.DevelopmentTodos[index].Status = DevelopmentTodoNotStarted
		}
	}
	syncDevelopmentTodoStatuses(unit)
}

func syncDevelopmentTodoStatuses(unit *DeliveryUnit) {
	if unit.Phase == DeliveryUnitComplete {
		for index := range unit.DevelopmentTodos {
			unit.DevelopmentTodos[index].Status = DevelopmentTodoCompleted
		}
		return
	}
	active := false
	for index := range unit.DevelopmentTodos {
		todo := &unit.DevelopmentTodos[index]
		if todo.Phase != unit.Phase || todo.Status == DevelopmentTodoCompleted {
			continue
		}
		if !active {
			todo.Status = DevelopmentTodoInProgress
			active = true
		} else {
			todo.Status = DevelopmentTodoNotStarted
		}
	}
}
