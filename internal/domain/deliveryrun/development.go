package deliveryrun

import (
	"fmt"
	"strings"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain"
)

func newDeliveryUnits(feature FeatureSnapshot, mode DeliveryMode, sourceRevision string) ([]DeliveryUnit, string) {
	phase := DeliveryUnitImplementation
	implementationStatus := DeliveryGatePending
	integratedRevision := ""
	todos := newDevelopmentTodos(feature.ID)
	if mode == DeliveryModeExistingSourceVerification {
		phase = DeliveryUnitVerification
		implementationStatus = DeliveryGatePassed
		integratedRevision = sourceRevision
		todos = existingSourceVerificationTodos(feature.ID)
	}
	unit := DeliveryUnit{
		ID: feature.ID, Title: feature.Title, DependsOn: []string{},
		Phase: phase, ActiveRole: activeRoleForPhase(phase),
		IntegratedGitRevision:  integratedRevision,
		ImplementationStatus:   implementationStatus,
		VerificationStatus:     DeliveryGatePending,
		DevelopmentTodos:       todos,
		DevelopmentRepairItems: []DevelopmentRepairWorkItem{},
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
	command, _ := commandForPhase(unit.Phase)
	if command == "" {
		return []AvailableAction{}, true
	}
	actions := []AvailableAction{catalogAction(command, unit.ID)}
	if phaseCanReportGap(unit.Phase) {
		actions = append(actions, catalogAction(commandGapReport, unit.ID))
	}
	return actions, true
}

func deliveryUnitReleaseGates(run *DeliveryRun) []ReleaseGate {
	completed := 0
	verified := 0
	for _, unit := range run.DeliveryUnits {
		if unit.Phase == DeliveryUnitComplete {
			completed++
		}
		if unit.VerificationStatus == DeliveryGatePassed {
			verified++
		}
	}
	gates := []ReleaseGate{
		{Code: "feature_frozen", Label: "FeatureRevision frozen", Detail: run.Feature.ID, OK: run.Feature.ID != "" && run.Feature.Source.FeatureRevision > 0},
		{Code: "delivery_units_completed", Label: "Delivery units completed", Detail: fmt.Sprintf("%d / %d", completed, len(run.DeliveryUnits)), OK: len(run.DeliveryUnits) > 0 && completed == len(run.DeliveryUnits)},
		{Code: "journeys_passed", Label: "Integrated journeys passed", Detail: fmt.Sprintf("%d / %d", verified, len(run.DeliveryUnits)), OK: len(run.DeliveryUnits) > 0 && verified == len(run.DeliveryUnits)},
		{Code: "backend_guide_verified", Label: "Domainry source contract verified", Detail: fmt.Sprintf("%d delivery units", len(run.DeliveryUnits)), OK: backendGuideEvidenceReady(run)},
		{Code: "product_revision_bound", Label: "Executable ProductRevision bound", Detail: executableRevisionDetail(run), OK: executableRevisionReady(run)},
	}
	gates = append(gates, qualityReleaseGate(run), acceptanceReleaseGate(run))
	if run.Mode != DeliveryModeExistingSourceVerification {
		gates = append(gates, releaseChecksGate(run))
	}
	return gates
}

func backendGuideEvidenceReady(run *DeliveryRun) bool {
	if len(run.DeliveryUnits) == 0 {
		return false
	}
	for _, unit := range run.DeliveryUnits {
		if validateBackendGuideEvidenceForRun(run, unit.VerificationEvidence, unit.IntegratedGitRevision) != nil {
			return false
		}
	}
	return true
}

func executableRevisionReady(run *DeliveryRun) bool {
	revision, ready := verifiedJourneyRevision(run)
	base := executableRevisionBase(run, revision)
	return ready && run.ExecutableRevision != nil && run.ExecutableRevision.CodeRevision == revision && run.ExecutableRevision.BaseRevision == base && run.ExecutableRevision.TargetRevision == base+1 && sha256ValuePattern.MatchString(run.ExecutableRevision.ModelSHA256)
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
	case StageDevelopment, StageVerified, StageLive:
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
	if command.Actor.Kind != expectedActor {
		return true, Invalid("delivery_unit_actor_invalid")
	}
	payload, err := decodeDeliveryUnitResult(command.Type, command.Payload)
	if err != nil {
		return true, err
	}
	if payload.BackendGuide != nil {
		payload.BackendGuide.ExecutedBy = command.Actor.ID
		payload.BackendGuide.OccurredAt = now
	}
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	if err := validateDeliveryUnitPayload(run, unit, command, payload); err != nil {
		return true, err
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
	if command.Type != commandGapReport {
		completeDevelopmentPhaseWorkItems(unit, gate)
	}
	advanceDeliveryUnit(run, unit, command.Type, payload)
	appendActivity(run, command.Actor, "delivery_unit_phase_recorded", unit.ID+" phase recorded", string(payload.Phase)+": "+strings.TrimSpace(payload.Summary), now)
	return true, nil
}

func decodeDeliveryUnitResult(command string, raw []byte) (deliveryUnitResult, error) {
	switch command {
	case commandImplementationComplete:
		var payload deliveryUnitImplementationPayload
		if err := decode(raw, &payload); err != nil {
			return deliveryUnitResult{}, err
		}
		return deliveryUnitResult{DeliveryUnitID: payload.DeliveryUnitID, Phase: payload.Phase, GitRevision: payload.GitRevision, Summary: payload.Summary, EvidenceRefs: payload.EvidenceRefs}, nil
	case commandVerificationComplete:
		var payload deliveryUnitVerificationPayload
		if err := decode(raw, &payload); err != nil {
			return deliveryUnitResult{}, err
		}
		return deliveryUnitResult{DeliveryUnitID: payload.DeliveryUnitID, Phase: payload.Phase, GitRevision: payload.GitRevision, Summary: payload.Summary, EvidenceRefs: payload.EvidenceRefs, BackendGuide: payload.BackendGuide}, nil
	case commandGapReport:
		var payload deliveryUnitGapPayload
		if err := decode(raw, &payload); err != nil {
			return deliveryUnitResult{}, err
		}
		return deliveryUnitResult{DeliveryUnitID: payload.DeliveryUnitID, Phase: payload.Phase, GitRevision: payload.GitRevision, Summary: payload.Summary, EvidenceRefs: payload.EvidenceRefs, Diagnostics: payload.Diagnostics, FailureOwner: payload.FailureOwner}, nil
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
	if run.Mode == DeliveryModeExistingSourceVerification && payload.GitRevision != run.SourceRevision {
		return Invalid("delivery_source_revision_changed")
	}
	if !validCodeRevision(payload.GitRevision) || payload.Summary == "" || len(payload.Summary) > 2000 {
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
		if strings.TrimSpace(diagnostic.Code) == "" || len(strings.TrimSpace(diagnostic.Code)) > 128 || strings.TrimSpace(diagnostic.Severity) != "error" || strings.TrimSpace(diagnostic.Path) == "" || len(strings.TrimSpace(diagnostic.Path)) > 1024 || strings.TrimSpace(diagnostic.Message) == "" || len(strings.TrimSpace(diagnostic.Message)) > 2000 || strings.TrimSpace(diagnostic.Owner) == "" || len(strings.TrimSpace(diagnostic.Owner)) > 128 || strings.TrimSpace(diagnostic.Category) == "" || len(strings.TrimSpace(diagnostic.Category)) > 128 || len(strings.TrimSpace(diagnostic.Remediation)) > 2000 {
			return Invalid("delivery_unit_diagnostics_invalid")
		}
	}
	if command.Type == commandGapReport {
		if !validDeliveryGapOwner(payload.FailureOwner) || len(payload.Diagnostics) == 0 || payload.BackendGuide != nil {
			return Invalid("delivery_unit_gap_invalid")
		}
		return nil
	}
	if payload.FailureOwner != "" || len(payload.Diagnostics) != 0 {
		return Invalid("delivery_unit_gap_invalid")
	}
	if command.Type == commandVerificationComplete {
		if payload.GitRevision != unit.IntegratedGitRevision {
			return Invalid("delivery_unit_integrated_revision_conflict")
		}
		return validateBackendGuideEvidenceForRun(run, payload.BackendGuide, payload.GitRevision)
	}
	if payload.BackendGuide != nil {
		return Invalid("backend_guide_evidence_unexpected")
	}
	return nil
}

func validateBackendGuideEvidenceForRun(run *DeliveryRun, evidence *BackendGuideEvidence, gitRevision string) error {
	if evidence == nil || strings.TrimSpace(evidence.ModelPath) != "backend/model.json" || !sha256ValuePattern.MatchString(strings.ToLower(strings.TrimSpace(evidence.ModelSHA256))) || !evidence.SingleBackendModel || !evidence.StrictModelValidation || !evidence.CrossReferencesResolved || !evidence.GoBehaviorRegistry || !evidence.NoExecutableBehaviorJSON || !evidence.ProjectHTTP || !evidence.HandlerUnitOfWork || !evidence.HandlerIdempotency || !evidence.DefinitionsRegistered || !evidence.RuntimeBootstrap || !evidence.NoProjectSchemaSQL || !evidence.BackendGoModOnly || !evidence.NoRootGoMod || !evidence.NoGoWork || !evidence.NoCompilerBuilder || !evidence.NoGeneratedRuntimeContract || !evidence.AuthWorkspacePermission || !evidence.DataAuditPersistence || !evidence.EmptyDatabaseInitPassed || !evidence.SameModelRestartPassed || !evidence.IncrementalModelRestartPassed || !evidence.MockJourneyPassed || !evidence.RuntimeJourneyPassed {
		return Invalid("integrated_verification_evidence_invalid")
	}
	if run.Mode == DeliveryModeExistingSourceVerification && (!evidence.HistoricalSourceContract || !evidence.ModelContractPresent || !evidence.RuntimeEntrypointPresent || !evidence.BackendModulePresent || !evidence.FrontendPackagePresent || !evidence.FrontendProjectClientPresent || !evidence.SourceRevisionUnchanged) {
		return Invalid("historical_source_contract_evidence_invalid")
	}
	gitRevision = strings.TrimSpace(gitRevision)
	if strings.TrimSpace(evidence.WorkspaceID) != run.WorkspaceID || strings.TrimSpace(evidence.ProductID) != run.Product.ID || evidence.FeatureRevision != run.Feature.Source.FeatureRevision || strings.TrimSpace(evidence.GitRevision) != gitRevision {
		return Invalid("backend_evidence_scope_invalid")
	}
	if strings.TrimSpace(evidence.RepositoryIdentity) == "" || strings.TrimSpace(evidence.CheckSuite) == "" || strings.TrimSpace(evidence.CheckVersion) == "" || strings.TrimSpace(evidence.ExecutedBy) == "" || evidence.OccurredAt.IsZero() || len(evidence.EvidenceRefs) == 0 || !evidenceRefsMatchRevision(evidence.EvidenceRefs, gitRevision) {
		return Invalid("backend_evidence_provenance_invalid")
	}
	return nil
}

func validCodeRevision(revision string) bool {
	return domain.ValidCodeRevision(revision)
}

func advanceDeliveryUnit(run *DeliveryRun, unit *DeliveryUnit, command string, payload deliveryUnitResult) {
	switch command {
	case commandImplementationComplete:
		unit.ImplementationStatus = DeliveryGatePassed
		unit.IntegratedGitRevision = strings.TrimSpace(payload.GitRevision)
		setDeliveryUnitPhase(unit, DeliveryUnitVerification)
	case commandVerificationComplete:
		unit.VerificationStatus = DeliveryGatePassed
		unit.VerificationEvidence = clone(payload.BackendGuide)
		setDeliveryUnitPhase(unit, DeliveryUnitComplete)
		activateNextDeliveryUnit(run)
	case commandGapReport:
		routeDeliveryUnitGap(unit, payload.FailureOwner, repairTargetsFromDiagnostics(payload.Diagnostics, payload.FailureOwner))
	}
}

func routeDeliveryUnitGap(unit *DeliveryUnit, owner string, repairTargets []developmentRepairTarget) {
	unit.ImplementationStatus = DeliveryGateNeedsChange
	unit.VerificationStatus = DeliveryGateNeedsChange
	for _, target := range repairTargets {
		reopenDevelopmentRepairTodos(unit, DeliveryUnitImplementation, []developmentRepairTarget{target})
	}
	if len(repairTargets) == 0 {
		reopenDevelopmentRepairTodos(unit, DeliveryUnitImplementation, []developmentRepairTarget{{
			Phase: DeliveryUnitImplementation, SourceKind: "verification_gap", SourceID: owner,
			Title: "Repair integrated verification gap", Detail: "The integrated verification must be repaired by the same Engineer.",
		}})
	}
	setDeliveryUnitPhase(unit, DeliveryUnitImplementation)
}

func validDeliveryGapOwner(owner string) bool {
	return owner == "engineer"
}

func phaseCanReportGap(phase DeliveryUnitPhase) bool {
	return phase == DeliveryUnitVerification
}

func activateNextDeliveryUnit(run *DeliveryRun) {
	for index := range run.DeliveryUnits {
		unit := &run.DeliveryUnits[index]
		if unit.Phase != DeliveryUnitPlanned || !deliveryUnitDependenciesComplete(run, unit) {
			continue
		}
		setDeliveryUnitPhase(unit, DeliveryUnitImplementation)
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
	syncDevelopmentWorkItemStatuses(unit)
}

func isDeliveryUnitCommand(command string) bool {
	return command == commandImplementationComplete || command == commandVerificationComplete || command == commandGapReport
}
