package delivery

import (
	"fmt"
	"strings"
	"time"
)

type DeliveryUnitPhase string

const (
	DeliveryUnitPlanned               DeliveryUnitPhase = "planned"
	DeliveryUnitInteractionModeling   DeliveryUnitPhase = "interaction_modeling"
	DeliveryUnitDomainModeling        DeliveryUnitPhase = "domain_modeling"
	DeliveryUnitCompiling             DeliveryUnitPhase = "compiling"
	DeliveryUnitFrontendConvergence   DeliveryUnitPhase = "frontend_convergence"
	DeliveryUnitContractGap           DeliveryUnitPhase = "contract_gap"
	DeliveryUnitContractFrozen        DeliveryUnitPhase = "contract_frozen"
	DeliveryUnitBackendImplementation DeliveryUnitPhase = "backend_implementation"
	DeliveryUnitJourneyTesting        DeliveryUnitPhase = "journey_testing"
	DeliveryUnitComplete              DeliveryUnitPhase = "complete"
)

type DeliveryGateStatus string

const (
	DeliveryGatePending     DeliveryGateStatus = "pending"
	DeliveryGatePassed      DeliveryGateStatus = "passed"
	DeliveryGateNeedsChange DeliveryGateStatus = "needs_change"
)

const (
	commandInteractionComplete = "delivery_unit.interaction.complete"
	commandModelComplete       = "delivery_unit.model.complete"
	commandCompileComplete     = "delivery_unit.compile.complete"
	commandFrontendComplete    = "delivery_unit.frontend.complete"
	commandContractGapReport   = "delivery_unit.contract_gap.report"
	commandContractFreeze      = "delivery_unit.contract.freeze"
	commandBackendComplete     = "delivery_unit.backend.complete"
	commandBackendReopen       = "delivery_unit.backend.reopen"
	commandContractReopen      = "delivery_unit.contract.reopen"
	commandJourneyComplete     = "delivery_unit.journey.complete"
)

type DeliveryUnit struct {
	ID                        string              `json:"id"`
	Title                     string              `json:"title"`
	DependsOn                 []string            `json:"depends_on"`
	Phase                     DeliveryUnitPhase   `json:"phase"`
	ActiveRole                string              `json:"active_role,omitempty"`
	ContractGitRevision       string              `json:"contract_git_revision,omitempty"`
	ImplementationGitRevision string              `json:"implementation_git_revision,omitempty"`
	InteractionStatus         DeliveryGateStatus  `json:"interaction_status"`
	CompilerStatus            DeliveryGateStatus  `json:"compiler_status"`
	FrontendStatus            DeliveryGateStatus  `json:"frontend_status"`
	BackendStatus             DeliveryGateStatus  `json:"backend_status"`
	JourneyStatus             DeliveryGateStatus  `json:"journey_status"`
	LatestGate                *DeliveryGateResult `json:"latest_gate,omitempty"`
}

type DeliveryGateResult struct {
	Phase       DeliveryUnitPhase  `json:"phase"`
	Status      DeliveryGateStatus `json:"status"`
	GitRevision string             `json:"git_revision"`
	Summary     string             `json:"summary"`
	Diagnostics []GateDiagnostic   `json:"diagnostics"`
	RecordedBy  string             `json:"recorded_by"`
	RecordedAt  time.Time          `json:"recorded_at"`
}

type GateDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type deliveryUnitCommandPayload struct {
	DeliveryUnitID string            `json:"delivery_unit_id"`
	Phase          DeliveryUnitPhase `json:"phase"`
	GitRevision    string            `json:"git_revision"`
	Summary        string            `json:"summary"`
	Diagnostics    []GateDiagnostic  `json:"diagnostics"`
}

func newDeliveryUnits(feature FeatureSnapshot) ([]DeliveryUnit, string) {
	unit := DeliveryUnit{
		ID:                feature.ID,
		Title:             feature.Title,
		DependsOn:         []string{},
		Phase:             DeliveryUnitInteractionModeling,
		ActiveRole:        activeRoleForPhase(DeliveryUnitInteractionModeling),
		InteractionStatus: DeliveryGatePending,
		CompilerStatus:    DeliveryGatePending,
		FrontendStatus:    DeliveryGatePending,
		BackendStatus:     DeliveryGatePending,
		JourneyStatus:     DeliveryGatePending,
	}
	return []DeliveryUnit{unit}, unit.ID
}

func deliveryUnitActions(run *DeliveryRun) ([]AvailableAction, bool) {
	if len(run.DeliveryUnits) == 0 {
		return nil, false
	}
	unit := activeDeliveryUnit(run)
	if unit == nil {
		return v3LifecycleActions(run), true
	}
	command, actorKind := commandForPhase(unit.Phase)
	if command == "" {
		return []AvailableAction{}, true
	}
	actions := []AvailableAction{{Command: command, TargetID: unit.ID, ActorKind: actorKind}}
	if unit.Phase == DeliveryUnitFrontendConvergence || unit.Phase == DeliveryUnitBackendImplementation {
		actions = append(actions, AvailableAction{
			Command: commandContractGapReport, TargetID: unit.ID, ActorKind: ActorAgent,
		})
	}
	return actions, true
}

func deliveryUnitReleaseGates(run *DeliveryRun) []ReleaseGate {
	completed := 0
	journeys := 0
	for _, unit := range run.DeliveryUnits {
		if unit.Phase == DeliveryUnitComplete {
			completed++
		}
		if unit.JourneyStatus == DeliveryGatePassed {
			journeys++
		}
	}
	gates := []ReleaseGate{
		{
			Code: "feature_frozen", Label: "FeatureRevision frozen",
			Detail: run.Feature.ID, OK: run.Feature.ID != "" && run.Feature.Source.FeatureRevision > 0,
		},
		{
			Code: "delivery_units_completed", Label: "Delivery units completed",
			Detail: fmt.Sprintf("%d / %d", completed, len(run.DeliveryUnits)), OK: len(run.DeliveryUnits) > 0 && completed == len(run.DeliveryUnits),
		},
		{
			Code: "journeys_passed", Label: "Real journeys passed",
			Detail: fmt.Sprintf("%d / %d", journeys, len(run.DeliveryUnits)), OK: len(run.DeliveryUnits) > 0 && journeys == len(run.DeliveryUnits),
		},
	}
	gates = append(gates, qualityReleaseGate(run), acceptanceReleaseGate(run), v3ReleaseChecksGate(run))
	return gates
}

func v3LifecycleActions(run *DeliveryRun) []AvailableAction {
	switch run.Stage {
	case StageTesting:
		return qualityActions(run)
	case StageAcceptance:
		return acceptanceActions(run)
	case StageRelease:
		return v3ReleaseActions(run)
	case StageDevelopment, StageBugs, StageLive:
		return []AvailableAction{}
	}
	return []AvailableAction{}
}

func applyDeliveryUnitCommand(run *DeliveryRun, command Command, now time.Time) (bool, error) {
	if !isDeliveryUnitCommand(command.Type) {
		return false, nil
	}
	unit := activeDeliveryUnit(run)
	if unit == nil {
		return true, Invalid("delivery_unit_inactive", "The DeliveryRun has no active DeliveryUnit.")
	}
	expectedCommand, expectedActor := commandForPhase(unit.Phase)
	if command.Type == commandContractGapReport && (unit.Phase == DeliveryUnitFrontendConvergence || unit.Phase == DeliveryUnitBackendImplementation) {
		expectedCommand = commandContractGapReport
		expectedActor = ActorAgent
	}
	if unit.Phase == DeliveryUnitJourneyTesting && (command.Type == commandBackendReopen || command.Type == commandContractReopen) {
		expectedCommand = command.Type
		expectedActor = ActorSystem
	}
	if command.Type != expectedCommand {
		return true, Invalid("delivery_unit_command_invalid", "The command does not match the active DeliveryUnit phase.")
	}
	if command.Actor.Kind != expectedActor {
		return true, Invalid("delivery_unit_actor_invalid", "The active DeliveryUnit phase requires a different trusted actor kind.")
	}
	var payload deliveryUnitCommandPayload
	if err := decode(command.Payload, &payload); err != nil {
		return true, err
	}
	if err := validateDeliveryUnitPayload(unit, payload); err != nil {
		return true, err
	}
	if command.Type == commandJourneyComplete && strings.TrimSpace(payload.GitRevision) != unit.ImplementationGitRevision {
		return true, Invalid("delivery_unit_implementation_revision_conflict", "Journey evidence must match the Backend implementation Git revision.")
	}
	status := DeliveryGatePassed
	if command.Type == commandContractGapReport {
		status = DeliveryGateNeedsChange
	}
	unit.LatestGate = &DeliveryGateResult{
		Phase: unit.Phase, Status: status, GitRevision: strings.TrimSpace(payload.GitRevision),
		Summary: strings.TrimSpace(payload.Summary), Diagnostics: clone(payload.Diagnostics),
		RecordedBy: command.Actor.ID, RecordedAt: now,
	}
	advanceDeliveryUnit(run, unit, command.Type, payload.GitRevision)
	appendActivity(run, command.Actor, "delivery_unit_phase_recorded", unit.ID+" phase recorded", string(payload.Phase)+": "+strings.TrimSpace(payload.Summary), now)
	return true, nil
}

func validateDeliveryUnitPayload(unit *DeliveryUnit, payload deliveryUnitCommandPayload) error {
	if strings.TrimSpace(payload.DeliveryUnitID) != unit.ID || payload.Phase != unit.Phase {
		return Invalid("delivery_unit_phase_conflict", "The submitted DeliveryUnit identity or phase is stale.")
	}
	payload.GitRevision = strings.TrimSpace(payload.GitRevision)
	payload.Summary = strings.TrimSpace(payload.Summary)
	if !validGitRevision(payload.GitRevision) || payload.Summary == "" || len(payload.Summary) > 2000 {
		return Invalid("delivery_unit_gate_invalid", "A phase result requires a Git revision and a bounded summary.")
	}
	if len(payload.Diagnostics) > 100 {
		return Invalid("delivery_unit_diagnostics_invalid", "A phase result cannot contain more than 100 diagnostics.")
	}
	for _, diagnostic := range payload.Diagnostics {
		if strings.TrimSpace(diagnostic.Code) == "" || len(diagnostic.Code) > 128 || strings.TrimSpace(diagnostic.Message) == "" || len(diagnostic.Message) > 2000 {
			return Invalid("delivery_unit_diagnostics_invalid", "Every phase diagnostic requires a bounded code and message.")
		}
	}
	return nil
}

func validGitRevision(revision string) bool {
	if len(revision) < 7 || len(revision) > 128 {
		return false
	}
	for _, character := range revision {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}

func advanceDeliveryUnit(run *DeliveryRun, unit *DeliveryUnit, command, gitRevision string) {
	switch command {
	case commandInteractionComplete:
		unit.InteractionStatus = DeliveryGatePassed
		setDeliveryUnitPhase(unit, DeliveryUnitDomainModeling)
	case commandModelComplete:
		unit.CompilerStatus = DeliveryGatePending
		unit.FrontendStatus = DeliveryGatePending
		setDeliveryUnitPhase(unit, DeliveryUnitCompiling)
	case commandCompileComplete:
		unit.CompilerStatus = DeliveryGatePassed
		setDeliveryUnitPhase(unit, DeliveryUnitFrontendConvergence)
	case commandFrontendComplete:
		unit.FrontendStatus = DeliveryGatePassed
		setDeliveryUnitPhase(unit, DeliveryUnitContractFrozen)
	case commandContractGapReport:
		unit.CompilerStatus = DeliveryGatePending
		unit.FrontendStatus = DeliveryGateNeedsChange
		unit.BackendStatus = DeliveryGatePending
		unit.JourneyStatus = DeliveryGatePending
		unit.ContractGitRevision = ""
		unit.ImplementationGitRevision = ""
		setDeliveryUnitPhase(unit, DeliveryUnitContractGap)
	case commandContractFreeze:
		unit.ContractGitRevision = strings.TrimSpace(gitRevision)
		setDeliveryUnitPhase(unit, DeliveryUnitBackendImplementation)
	case commandBackendComplete:
		unit.BackendStatus = DeliveryGatePassed
		unit.ImplementationGitRevision = strings.TrimSpace(gitRevision)
		setDeliveryUnitPhase(unit, DeliveryUnitJourneyTesting)
	case commandBackendReopen:
		unit.BackendStatus = DeliveryGatePending
		unit.JourneyStatus = DeliveryGatePending
		unit.ImplementationGitRevision = ""
		setDeliveryUnitPhase(unit, DeliveryUnitBackendImplementation)
	case commandContractReopen:
		unit.CompilerStatus = DeliveryGatePending
		unit.FrontendStatus = DeliveryGateNeedsChange
		unit.BackendStatus = DeliveryGatePending
		unit.JourneyStatus = DeliveryGatePending
		unit.ContractGitRevision = ""
		unit.ImplementationGitRevision = ""
		setDeliveryUnitPhase(unit, DeliveryUnitContractGap)
	case commandJourneyComplete:
		unit.JourneyStatus = DeliveryGatePassed
		setDeliveryUnitPhase(unit, DeliveryUnitComplete)
		activateNextDeliveryUnit(run)
	}
}

func activateNextDeliveryUnit(run *DeliveryRun) {
	for index := range run.DeliveryUnits {
		unit := &run.DeliveryUnits[index]
		if unit.Phase != DeliveryUnitPlanned || !deliveryUnitDependenciesComplete(run, unit) {
			continue
		}
		setDeliveryUnitPhase(unit, DeliveryUnitInteractionModeling)
		run.ActiveDeliveryUnitID = unit.ID
		return
	}
	run.ActiveDeliveryUnitID = ""
	run.Stage = StageTesting
}

func deliveryUnitDependenciesComplete(run *DeliveryRun, unit *DeliveryUnit) bool {
	for _, dependencyID := range unit.DependsOn {
		dependency := deliveryUnitByID(run, dependencyID)
		if dependency == nil || dependency.Phase != DeliveryUnitComplete {
			return false
		}
	}
	return true
}

func activeDeliveryUnit(run *DeliveryRun) *DeliveryUnit {
	if strings.TrimSpace(run.ActiveDeliveryUnitID) == "" {
		return nil
	}
	return deliveryUnitByID(run, run.ActiveDeliveryUnitID)
}

func deliveryUnitByID(run *DeliveryRun, id string) *DeliveryUnit {
	for index := range run.DeliveryUnits {
		if run.DeliveryUnits[index].ID == id {
			return &run.DeliveryUnits[index]
		}
	}
	return nil
}

func setDeliveryUnitPhase(unit *DeliveryUnit, phase DeliveryUnitPhase) {
	unit.Phase = phase
	unit.ActiveRole = activeRoleForPhase(phase)
}

func activeRoleForPhase(phase DeliveryUnitPhase) string {
	switch phase {
	case DeliveryUnitInteractionModeling, DeliveryUnitFrontendConvergence:
		return "frontend"
	case DeliveryUnitDomainModeling, DeliveryUnitContractGap, DeliveryUnitBackendImplementation:
		return "backend"
	case DeliveryUnitCompiling, DeliveryUnitContractFrozen:
		return "system"
	case DeliveryUnitJourneyTesting:
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
	case DeliveryUnitDomainModeling, DeliveryUnitContractGap:
		return commandModelComplete, ActorAgent
	case DeliveryUnitCompiling:
		return commandCompileComplete, ActorSystem
	case DeliveryUnitFrontendConvergence:
		return commandFrontendComplete, ActorAgent
	case DeliveryUnitContractFrozen:
		return commandContractFreeze, ActorSystem
	case DeliveryUnitBackendImplementation:
		return commandBackendComplete, ActorAgent
	case DeliveryUnitJourneyTesting:
		return commandJourneyComplete, ActorSystem
	case DeliveryUnitPlanned, DeliveryUnitComplete:
		return "", ""
	}
	return "", ""
}

func isDeliveryUnitCommand(command string) bool {
	switch command {
	case commandInteractionComplete, commandModelComplete, commandCompileComplete,
		commandFrontendComplete, commandContractGapReport, commandContractFreeze,
		commandBackendComplete, commandBackendReopen, commandContractReopen,
		commandJourneyComplete:
		return true
	default:
		return false
	}
}
