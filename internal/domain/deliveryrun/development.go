package deliveryrun

import (
	"fmt"
	"strings"
	"time"
)

func newDeliveryUnits(feature FeatureSnapshot) ([]DeliveryUnit, string) {
	unit := DeliveryUnit{
		ID:                feature.ID,
		Title:             feature.Title,
		DependsOn:         []string{},
		Phase:             DeliveryUnitInteractionModeling,
		ActiveRole:        activeRoleForPhase(DeliveryUnitInteractionModeling),
		InteractionStatus: DeliveryGatePending,
		ModelStatus:       DeliveryGatePending,
		BackendStatus:     DeliveryGatePending,
		FrontendStatus:    DeliveryGatePending,
		ContractStatus:    DeliveryGatePending,
		JourneyStatus:     DeliveryGatePending,
		DevelopmentTodos:  newDevelopmentTodos(feature),
	}
	return []DeliveryUnit{unit}, unit.ID
}

func deliveryUnitActions(run *DeliveryRun) ([]AvailableAction, bool) {
	if len(run.DeliveryUnits) == 0 {
		return nil, false
	}
	unit := activeDeliveryUnit(run)
	if unit == nil {
		return lifecycleActions(run), true
	}
	command, actorKind := commandForPhase(unit.Phase)
	if command == "" {
		return []AvailableAction{}, true
	}
	_ = actorKind // catalog is the single source of projected actor metadata.
	actions := []AvailableAction{}
	if todo := activeDevelopmentTodo(unit); todo != nil && unit.ActiveRole != "system" {
		actions = append(actions, catalogAction(commandDevelopmentTodoComplete, todo.ID))
	}
	if unit.ActiveRole == "system" || developmentPhaseTodosCompleted(unit, unit.Phase) {
		actions = append(actions, catalogAction(command, unit.ID))
	}
	if phaseCanReportGap(unit.Phase) {
		actions = append(actions, catalogAction(commandGapReport, unit.ID))
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
		{Code: "feature_frozen", Label: "FeatureRevision frozen", Detail: run.Feature.ID, OK: run.Feature.ID != "" && run.Feature.Source.FeatureRevision > 0},
		{Code: "delivery_units_completed", Label: "Delivery units completed", Detail: fmt.Sprintf("%d / %d", completed, len(run.DeliveryUnits)), OK: len(run.DeliveryUnits) > 0 && completed == len(run.DeliveryUnits)},
		{Code: "journeys_passed", Label: "Real journeys passed", Detail: fmt.Sprintf("%d / %d", journeys, len(run.DeliveryUnits)), OK: len(run.DeliveryUnits) > 0 && journeys == len(run.DeliveryUnits)},
		{Code: "backend_guide_verified", Label: "Plane backend guide verified", Detail: fmt.Sprintf("%d delivery units", len(run.DeliveryUnits)), OK: backendGuideEvidenceReady(run)},
		{Code: "product_revision_bound", Label: "Executable ProductRevision bound", Detail: executableRevisionDetail(run), OK: executableRevisionReady(run)},
	}
	gates = append(gates, qualityReleaseGate(run), acceptanceReleaseGate(run), releaseChecksGate(run))
	return gates
}

func backendGuideEvidenceReady(run *DeliveryRun) bool {
	if len(run.DeliveryUnits) == 0 {
		return false
	}
	for _, unit := range run.DeliveryUnits {
		if validateBackendGuideEvidenceForRun(run, commandModelVerify, unit.ModelEvidence, unit.ModelGitRevision) != nil ||
			validateBackendGuideEvidenceForRun(run, commandContractVerify, unit.ContractEvidence, unit.IntegratedGitRevision) != nil ||
			validateBackendGuideEvidenceForRun(run, commandJourneyComplete, unit.JourneyEvidence, unit.IntegratedGitRevision) != nil {
			return false
		}
	}
	return true
}

func executableRevisionReady(run *DeliveryRun) bool {
	revision, ready := verifiedJourneyRevision(run)
	return ready && run.ExecutableRevision != nil && run.ExecutableRevision.CodeRevision == revision && run.ExecutableRevision.BaseRevision == run.Feature.BaselineProductRevision && run.ExecutableRevision.TargetRevision == run.Feature.BaselineProductRevision+1 && sha256ValuePattern.MatchString(run.ExecutableRevision.ModelSHA256)
}

func executableRevisionDetail(run *DeliveryRun) string {
	if run.ExecutableRevision == nil {
		return "not recorded"
	}
	return fmt.Sprintf("R%d · %s", run.ExecutableRevision.TargetRevision, run.ExecutableRevision.CodeRevision)
}

func lifecycleActions(run *DeliveryRun) []AvailableAction {
	switch run.Stage {
	case StageTesting:
		return qualityActions(run)
	case StageAcceptance:
		return acceptanceActions(run)
	case StageRelease:
		return releaseActions(run)
	case StageDevelopment, StageBugs, StageLive:
		return []AvailableAction{}
	}
	return []AvailableAction{}
}

func applyDeliveryUnitCommand(run *DeliveryRun, command Command, now time.Time) (bool, error) {
	if command.Type == commandDevelopmentTodoComplete {
		return true, completeDevelopmentTodo(run, command, now)
	}
	if !isDeliveryUnitCommand(command.Type) {
		return false, nil
	}
	unit := activeDeliveryUnit(run)
	if unit == nil {
		return true, Invalid("delivery_unit_inactive")
	}
	expectedCommand, expectedActor := commandForPhase(unit.Phase)
	if command.Type == commandGapReport && phaseCanReportGap(unit.Phase) {
		expectedCommand = commandGapReport
		expectedActor = ActorSystem
	}
	if command.Type != expectedCommand {
		return true, Invalid("delivery_unit_command_invalid")
	}
	if command.Type != commandGapReport && unit.ActiveRole != "system" && !developmentPhaseTodosCompleted(unit, unit.Phase) {
		return true, Invalid("development_todos_incomplete")
	}
	if command.Actor.Kind != expectedActor {
		return true, Invalid("delivery_unit_actor_invalid")
	}
	payload, err := decodeDeliveryUnitResult(command.Type, command.Payload)
	if err != nil {
		return true, err
	}
	if payload.BackendGuide != nil {
		// Execution identity and time are trusted command context, never
		// client-authored evidence. Overwrite both before domain validation and
		// persistence so a system adapter cannot impersonate another verifier.
		payload.BackendGuide.ExecutedBy = command.Actor.ID
		payload.BackendGuide.OccurredAt = now
	}
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	if err := validateDeliveryUnitPayload(run, unit, command, payload); err != nil {
		return true, err
	}
	if unit.Phase == DeliveryUnitJourneyTesting && strings.TrimSpace(payload.GitRevision) != unit.IntegratedGitRevision {
		return true, Invalid("delivery_unit_integrated_revision_conflict")
	}
	if command.Type == commandContractVerify && strings.TrimSpace(payload.GitRevision) == unit.InvalidatedIntegratedGitRevision {
		return true, Invalid("delivery_unit_revision_not_advanced")
	}
	status := DeliveryGatePassed
	if command.Type == commandGapReport {
		status = DeliveryGateNeedsChange
	}
	gate := DeliveryGateResult{
		Phase: unit.Phase, Status: status, GitRevision: strings.TrimSpace(payload.GitRevision),
		Summary: strings.TrimSpace(payload.Summary), EvidenceRefs: clone(payload.EvidenceRefs), Diagnostics: clone(payload.Diagnostics), FailureOwner: strings.TrimSpace(payload.FailureOwner), BackendGuide: clone(payload.BackendGuide),
		RecordedBy: command.Actor.ID, RecordedAt: now,
	}
	unit.LatestGate = &gate
	if unit.ActiveRole == "system" && command.Type != commandGapReport {
		completeSystemPhaseTodos(unit, gate)
	}
	advanceDeliveryUnit(run, unit, command.Type, payload)
	appendActivity(run, command.Actor, "delivery_unit_phase_recorded", unit.ID+" phase recorded", string(payload.Phase)+": "+strings.TrimSpace(payload.Summary), now)
	return true, nil
}

func decodeDeliveryUnitResult(command string, raw []byte) (deliveryUnitResult, error) {
	switch command {
	case commandInteractionComplete, commandModelComplete, commandBackendComplete, commandFrontendComplete:
		var payload deliveryUnitImplementationPayload
		if err := decode(raw, &payload); err != nil {
			return deliveryUnitResult{}, err
		}
		return deliveryUnitResult{
			DeliveryUnitID: payload.DeliveryUnitID, Phase: payload.Phase,
			GitRevision: payload.GitRevision, Summary: payload.Summary, EvidenceRefs: payload.EvidenceRefs,
		}, nil
	case commandModelVerify, commandContractVerify, commandJourneyComplete:
		var payload deliveryUnitVerificationPayload
		if err := decode(raw, &payload); err != nil {
			return deliveryUnitResult{}, err
		}
		return deliveryUnitResult{
			DeliveryUnitID: payload.DeliveryUnitID, Phase: payload.Phase,
			GitRevision: payload.GitRevision, Summary: payload.Summary, EvidenceRefs: payload.EvidenceRefs, BackendGuide: payload.BackendGuide,
		}, nil
	case commandGapReport:
		var payload deliveryUnitGapPayload
		if err := decode(raw, &payload); err != nil {
			return deliveryUnitResult{}, err
		}
		return deliveryUnitResult{
			DeliveryUnitID: payload.DeliveryUnitID, Phase: payload.Phase,
			GitRevision: payload.GitRevision, Summary: payload.Summary,
			EvidenceRefs: payload.EvidenceRefs, Diagnostics: payload.Diagnostics, FailureOwner: payload.FailureOwner,
		}, nil
	default:
		return deliveryUnitResult{}, Invalid("delivery_unit_command_invalid")
	}
}

func validateDeliveryUnitPayload(run *DeliveryRun, unit *DeliveryUnit, command Command, payload deliveryUnitResult) error {
	if strings.TrimSpace(payload.DeliveryUnitID) != unit.ID || payload.Phase != unit.Phase {
		return Invalid("delivery_unit_phase_conflict")
	}
	payload.GitRevision = strings.TrimSpace(payload.GitRevision)
	payload.Summary = strings.TrimSpace(payload.Summary)
	payload.FailureOwner = strings.TrimSpace(payload.FailureOwner)
	if !validGitRevision(payload.GitRevision) || payload.Summary == "" || len(payload.Summary) > 2000 {
		return Invalid("delivery_unit_gate_invalid")
	}
	if len(payload.EvidenceRefs) == 0 || len(payload.EvidenceRefs) > 100 {
		return Invalid("delivery_unit_evidence_invalid")
	}
	for _, evidenceRef := range payload.EvidenceRefs {
		if strings.TrimSpace(evidenceRef) == "" || len(evidenceRef) > 1024 {
			return Invalid("delivery_unit_evidence_invalid")
		}
	}
	if len(payload.Diagnostics) > 100 {
		return Invalid("delivery_unit_diagnostics_invalid")
	}
	for _, diagnostic := range payload.Diagnostics {
		diagnostic.Code = strings.TrimSpace(diagnostic.Code)
		diagnostic.Severity = strings.TrimSpace(diagnostic.Severity)
		diagnostic.Path = strings.TrimSpace(diagnostic.Path)
		diagnostic.Message = strings.TrimSpace(diagnostic.Message)
		diagnostic.Owner = strings.TrimSpace(diagnostic.Owner)
		diagnostic.Category = strings.TrimSpace(diagnostic.Category)
		diagnostic.Remediation = strings.TrimSpace(diagnostic.Remediation)
		if diagnostic.Code == "" || len(diagnostic.Code) > 128 || diagnostic.Severity != "error" || diagnostic.Path == "" || len(diagnostic.Path) > 1024 || diagnostic.Message == "" || len(diagnostic.Message) > 2000 || diagnostic.Owner == "" || len(diagnostic.Owner) > 128 || diagnostic.Category == "" || len(diagnostic.Category) > 128 || len(diagnostic.Remediation) > 2000 {
			return Invalid("delivery_unit_diagnostics_invalid")
		}
	}
	if command.Type == commandGapReport {
		if !validDeliveryGapOwner(payload.FailureOwner) || len(payload.Diagnostics) == 0 {
			return Invalid("delivery_unit_gap_invalid")
		}
	} else if payload.FailureOwner != "" {
		return Invalid("delivery_unit_gap_invalid")
	}
	if err := validateBackendGuideEvidence(command.Type, payload.BackendGuide); err != nil {
		return err
	}
	if payload.BackendGuide != nil {
		evidence := payload.BackendGuide
		if err := validateBackendGuideEvidenceForRun(run, command.Type, evidence, payload.GitRevision); err != nil {
			return err
		}
		if (command.Type == commandContractVerify || command.Type == commandJourneyComplete) &&
			(unit.ModelEvidence == nil || !strings.EqualFold(strings.TrimSpace(evidence.ModelSHA256), strings.TrimSpace(unit.ModelEvidence.ModelSHA256))) {
			return Invalid("backend_evidence_model_mismatch")
		}
	}
	return nil
}

func validateBackendGuideEvidenceForRun(run *DeliveryRun, command string, evidence *BackendGuideEvidence, gitRevision string) error {
	if err := validateBackendGuideEvidence(command, evidence); err != nil {
		return err
	}
	if evidence == nil {
		return nil
	}
	gitRevision = strings.TrimSpace(gitRevision)
	if strings.TrimSpace(evidence.WorkspaceID) != run.WorkspaceID || strings.TrimSpace(evidence.ProductID) != run.Product.ID || evidence.FeatureRevision != run.Feature.Source.FeatureRevision || strings.TrimSpace(evidence.GitRevision) != gitRevision {
		return Invalid("backend_evidence_scope_invalid")
	}
	if strings.TrimSpace(evidence.RepositoryIdentity) == "" || strings.TrimSpace(evidence.GitStatus) != "clean" || strings.TrimSpace(evidence.CheckSuite) == "" || strings.TrimSpace(evidence.CheckVersion) == "" || strings.TrimSpace(evidence.ExecutedBy) == "" || evidence.OccurredAt.IsZero() || len(evidence.EvidenceRefs) == 0 || !evidenceRefsMatchRevision(evidence.EvidenceRefs, gitRevision) {
		return Invalid("backend_evidence_provenance_invalid")
	}
	return nil
}

func validateBackendGuideEvidence(command string, evidence *BackendGuideEvidence) error {
	switch command {
	case commandModelVerify:
		if evidence == nil || strings.TrimSpace(evidence.ModelPath) != "backend/model.json" || !sha256ValuePattern.MatchString(strings.ToLower(strings.TrimSpace(evidence.ModelSHA256))) || !evidence.SingleBackendModel || !evidence.StrictModelValidation || !evidence.CrossReferencesResolved || !evidence.GoBehaviorRegistry || !evidence.NoExecutableBehaviorJSON {
			return Invalid("backend_model_evidence_invalid")
		}
	case commandContractVerify:
		if evidence == nil || strings.TrimSpace(evidence.ModelPath) != "backend/model.json" || !sha256ValuePattern.MatchString(strings.ToLower(strings.TrimSpace(evidence.ModelSHA256))) || !evidence.ProjectHTTP || !evidence.HandlerUnitOfWork || !evidence.HandlerIdempotency || !evidence.DefinitionsRegistered || !evidence.RuntimeBootstrap || !evidence.NoProjectSchemaSQL || !evidence.BackendGoModOnly || !evidence.NoRootGoMod || !evidence.NoGoWork || !evidence.NoCompilerBuilder || !evidence.NoGeneratedRuntimeContract || !evidence.AuthWorkspacePermission || !evidence.DataAuditPersistence || !evidence.EmptyDatabaseInitPassed || !evidence.SameModelRestartPassed || !evidence.ChangedModelRejected {
			return Invalid("backend_contract_evidence_invalid")
		}
	case commandJourneyComplete:
		if evidence == nil || strings.TrimSpace(evidence.ModelPath) != "backend/model.json" || !sha256ValuePattern.MatchString(strings.ToLower(strings.TrimSpace(evidence.ModelSHA256))) || !evidence.MockJourneyPassed || !evidence.RuntimeJourneyPassed {
			return Invalid("backend_journey_evidence_invalid")
		}
	case commandGapReport:
		// Failed gates carry diagnostics; passing evidence is intentionally absent.
		if evidence != nil {
			return Invalid("backend_guide_evidence_unexpected")
		}
	default:
		if evidence != nil {
			return Invalid("backend_guide_evidence_unexpected")
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

func advanceDeliveryUnit(run *DeliveryRun, unit *DeliveryUnit, command string, payload deliveryUnitResult) {
	revision := strings.TrimSpace(payload.GitRevision)
	switch command {
	case commandInteractionComplete:
		unit.InteractionStatus = DeliveryGatePassed
		setDeliveryUnitPhase(unit, DeliveryUnitDomainModeling)
	case commandModelComplete:
		unit.ModelStatus = DeliveryGatePending
		setDeliveryUnitPhase(unit, DeliveryUnitModelVerification)
	case commandModelVerify:
		unit.ModelStatus = DeliveryGatePassed
		unit.ModelGitRevision = revision
		unit.ModelEvidence = clone(payload.BackendGuide)
		setDeliveryUnitPhase(unit, DeliveryUnitBackendImplementation)
	case commandBackendComplete:
		unit.BackendStatus = DeliveryGatePassed
		unit.BackendGitRevision = revision
		setDeliveryUnitPhase(unit, DeliveryUnitFrontendConvergence)
	case commandFrontendComplete:
		unit.FrontendStatus = DeliveryGatePassed
		setDeliveryUnitPhase(unit, DeliveryUnitContractVerification)
	case commandContractVerify:
		unit.ContractStatus = DeliveryGatePassed
		unit.IntegratedGitRevision = revision
		unit.InvalidatedIntegratedGitRevision = ""
		unit.ContractEvidence = clone(payload.BackendGuide)
		setDeliveryUnitPhase(unit, DeliveryUnitJourneyTesting)
	case commandGapReport:
		routeDeliveryUnitGap(unit, payload.FailureOwner)
	case commandJourneyComplete:
		unit.JourneyStatus = DeliveryGatePassed
		unit.JourneyEvidence = clone(payload.BackendGuide)
		setDeliveryUnitPhase(unit, DeliveryUnitComplete)
		activateNextDeliveryUnit(run)
	}
}

func routeDeliveryUnitGap(unit *DeliveryUnit, owner string) {
	if unit.IntegratedGitRevision != "" {
		unit.InvalidatedIntegratedGitRevision = unit.IntegratedGitRevision
	}
	unit.JourneyStatus = DeliveryGatePending
	switch owner {
	case "interaction":
		unit.InteractionStatus = DeliveryGateNeedsChange
		unit.ModelStatus = DeliveryGatePending
		unit.BackendStatus = DeliveryGatePending
		unit.FrontendStatus = DeliveryGatePending
		unit.ContractStatus = DeliveryGatePending
		unit.ModelGitRevision = ""
		unit.ModelEvidence = nil
		unit.ContractEvidence = nil
		unit.JourneyEvidence = nil
		unit.BackendGitRevision = ""
		unit.IntegratedGitRevision = ""
		setDeliveryUnitPhase(unit, DeliveryUnitInteractionModeling)
	case "model":
		unit.ModelStatus = DeliveryGateNeedsChange
		unit.BackendStatus = DeliveryGatePending
		unit.FrontendStatus = DeliveryGatePending
		unit.ContractStatus = DeliveryGatePending
		unit.ModelGitRevision = ""
		unit.ModelEvidence = nil
		unit.ContractEvidence = nil
		unit.JourneyEvidence = nil
		unit.BackendGitRevision = ""
		unit.IntegratedGitRevision = ""
		unit.ContractEvidence = nil
		unit.JourneyEvidence = nil
		setDeliveryUnitPhase(unit, DeliveryUnitDomainModeling)
	case "backend":
		unit.BackendStatus = DeliveryGateNeedsChange
		unit.FrontendStatus = DeliveryGatePending
		unit.ContractStatus = DeliveryGatePending
		unit.BackendGitRevision = ""
		unit.IntegratedGitRevision = ""
		unit.ContractEvidence = nil
		unit.JourneyEvidence = nil
		setDeliveryUnitPhase(unit, DeliveryUnitBackendImplementation)
	case "frontend":
		unit.FrontendStatus = DeliveryGateNeedsChange
		unit.ContractStatus = DeliveryGatePending
		unit.IntegratedGitRevision = ""
		setDeliveryUnitPhase(unit, DeliveryUnitFrontendConvergence)
	}
	resetDevelopmentTodosFromPhase(unit, unit.Phase)
}

func validDeliveryGapOwner(owner string) bool {
	return owner == "interaction" || owner == "model" || owner == "backend" || owner == "frontend"
}

func phaseCanReportGap(phase DeliveryUnitPhase) bool {
	return phase == DeliveryUnitModelVerification || phase == DeliveryUnitBackendImplementation || phase == DeliveryUnitContractVerification || phase == DeliveryUnitJourneyTesting
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
	syncDevelopmentTodoStatuses(unit)
}

func isDeliveryUnitCommand(command string) bool {
	switch command {
	case commandInteractionComplete, commandModelComplete, commandModelVerify,
		commandBackendComplete, commandFrontendComplete, commandContractVerify,
		commandGapReport, commandJourneyComplete:
		return true
	default:
		return false
	}
}
