package deliveryrun

import (
	"strings"
	"time"
)

type developmentTodoSeed struct {
	kind   string
	id     string
	title  string
	detail string
}

func newDevelopmentTodos(feature FeatureSnapshot) []DevelopmentTodo {
	todos := make([]DevelopmentTodo, 0)
	appendTodos := func(phase DeliveryUnitPhase, seeds []developmentTodoSeed) {
		if len(seeds) == 0 {
			seeds = []developmentTodoSeed{{kind: "feature", id: feature.ID, title: feature.Title, detail: feature.Summary}}
		}
		for _, seed := range seeds {
			todos = append(todos, DevelopmentTodo{
				ID:         feature.ID + ":" + string(phase) + ":" + seed.kind + ":" + seed.id,
				Phase:      phase,
				SourceKind: seed.kind,
				SourceID:   seed.id,
				Title:      seed.title,
				Detail:     seed.detail,
				Status:     DevelopmentTodoNotStarted,
				Evidence:   []DevelopmentTodoEvidence{},
			})
		}
	}
	scenarios := make([]developmentTodoSeed, 0, len(feature.Specification.Scenarios))
	for _, scenario := range feature.Specification.Scenarios {
		scenarios = append(scenarios, developmentTodoSeed{kind: "scenario", id: scenario.ID, title: scenario.Title, detail: scenario.Trigger + " → " + scenario.Outcome})
	}
	impacts := make([]developmentTodoSeed, 0, len(feature.Specification.Impacts))
	for _, impact := range feature.Specification.Impacts {
		impacts = append(impacts, developmentTodoSeed{kind: "impact", id: impact.ID, title: impact.Summary, detail: strings.Join(impact.Details, " · ")})
	}
	acceptance := make([]developmentTodoSeed, 0, len(feature.Specification.Acceptance))
	for _, criterion := range feature.Specification.Acceptance {
		acceptance = append(acceptance, developmentTodoSeed{kind: "acceptance", id: criterion.ID, title: criterion.Title, detail: formatAcceptanceScenario(criterion)})
	}
	appendTodos(DeliveryUnitInteractionModeling, scenarios)
	appendTodos(DeliveryUnitDomainModeling, impacts)
	appendTodos(DeliveryUnitModelVerification, impacts)
	appendTodos(DeliveryUnitBackendImplementation, scenarios)
	appendTodos(DeliveryUnitFrontendConvergence, scenarios)
	appendTodos(DeliveryUnitContractVerification, acceptance)
	appendTodos(DeliveryUnitJourneyTesting, acceptance)
	for index := range todos {
		if todos[index].Phase == DeliveryUnitInteractionModeling {
			todos[index].Status = DevelopmentTodoInProgress
			break
		}
	}
	return todos
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
	phaseOrder := map[DeliveryUnitPhase]int{
		DeliveryUnitInteractionModeling:   0,
		DeliveryUnitDomainModeling:        1,
		DeliveryUnitModelVerification:     2,
		DeliveryUnitBackendImplementation: 3,
		DeliveryUnitFrontendConvergence:   4,
		DeliveryUnitContractVerification:  5,
		DeliveryUnitJourneyTesting:        6,
	}
	activeOrder, ok := phaseOrder[phase]
	if !ok {
		return
	}
	for index := range unit.DevelopmentTodos {
		if phaseOrder[unit.DevelopmentTodos[index].Phase] >= activeOrder {
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
