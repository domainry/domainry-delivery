package deliveryrun

import (
	"strings"
	"time"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

const commandQualityRecord = commanddomain.QualityRecord

func qualityActions(run *DeliveryRun) []AvailableAction {
	if (run.Stage != StageTesting && run.Stage != StageAcceptance) || run.ExecutableRevision == nil || activeDeliveryUnit(run) != nil {
		return []AvailableAction{}
	}
	revision, ok := verifiedJourneyRevision(run)
	if !ok {
		return []AvailableAction{}
	}
	actions := make([]AvailableAction, 0, len(run.TestCases))
	for _, testCase := range run.TestCases {
		if !qualityCaseRequired(run, testCase.ID) {
			continue
		}
		result := latestQualityResult(run, revision, testCase.ID)
		if result == ResultPass || run.Mode == DeliveryModeExistingSourceVerification && result == ResultFail {
			continue
		}
		actions = append(actions, catalogAction(commandQualityRecord, testCase.ID))
	}
	return actions
}

func recordQuality(run *DeliveryRun, command Command, now time.Time) error {
	if (run.Stage != StageTesting && run.Stage != StageAcceptance) || activeDeliveryUnit(run) != nil || run.ExecutableRevision == nil {
		return Invalid("quality_stage_invalid")
	}
	var payload struct {
		TestCaseID   string      `json:"test_case_id"`
		GitRevision  string      `json:"git_revision"`
		Result       CheckResult `json:"result"`
		Note         string      `json:"note"`
		EvidenceRefs []string    `json:"evidence_refs"`
		FailureOwner string      `json:"failure_owner"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	testCase := findTestCase(run, strings.TrimSpace(payload.TestCaseID))
	if testCase == nil {
		return NotFound("TestCase", payload.TestCaseID)
	}
	revision, ok := verifiedJourneyRevision(run)
	if !ok || strings.TrimSpace(payload.GitRevision) != revision {
		return Invalid("quality_revision_conflict")
	}
	payload.Note = strings.TrimSpace(payload.Note)
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	payload.FailureOwner = strings.TrimSpace(payload.FailureOwner)
	if !validResult(payload.Result, true) || payload.Note == "" || !evidenceRefsMatchRevision(payload.EvidenceRefs, revision) {
		return Invalid("quality_evidence_missing")
	}
	if payload.Result == ResultFail && !validQualityFailureOwner(payload.FailureOwner) {
		return Invalid("quality_failure_owner_invalid")
	}
	if payload.Result != ResultFail && payload.FailureOwner != "" {
		return Invalid("quality_failure_owner_invalid")
	}
	if payload.Result == ResultFail && (payload.FailureOwner == "frontend" || payload.FailureOwner == "backend") && deliveryUnitByID(run, testCase.FeatureID) == nil {
		return Invalid("quality_delivery_unit_missing")
	}
	run.QualityRuns = append(run.QualityRuns, QualityRun{
		ID: newID("quality"), GitRevision: revision, TestCaseID: testCase.ID,
		Result: payload.Result, Note: payload.Note, EvidenceRefs: payload.EvidenceRefs,
		FailureOwner: payload.FailureOwner, ExecutedBy: command.Actor.ID, ExecutedAt: now,
	})
	appendActivity(run, command.Actor, "quality_recorded", testCase.ID+" independent QA result recorded", payload.Note, now)
	if payload.Result == ResultFail {
		if run.Mode == DeliveryModeExistingSourceVerification {
			appendActivity(run, command.Actor, "historical_source_verification_failed", testCase.ID+" historical source verification failed", "The adopted source remains read-only; no implementation repair was created.", now)
			return nil
		}
		return routeQualityFailure(run, testCase, payload.FailureOwner, payload.Note)
	}
	return nil
}

func routeQualityFailure(run *DeliveryRun, testCase *TestCase, owner, detail string) error {
	return routeVerificationFailure(run, testCase.FeatureID, owner, []developmentRepairTarget{{
		SourceKind: "quality_case",
		SourceID:   testCase.ID,
		Title:      "Repair failed quality case: " + testCase.Title,
		Detail:     detail,
	}})
}

func routeVerificationFailure(run *DeliveryRun, deliveryUnitID, owner string, repairTargets []developmentRepairTarget) error {
	unit := deliveryUnitByID(run, deliveryUnitID)
	if unit == nil {
		return Invalid("quality_delivery_unit_missing")
	}
	switch owner {
	case "interaction", "model", "frontend", "backend":
		routeDeliveryUnitGap(unit, owner, repairTargets)
	}
	run.ActiveDeliveryUnitID = unit.ID
	return nil
}

func deriveStage(run *DeliveryRun) Stage {
	if activeDeliveryUnit(run) != nil {
		if run.AcceptanceReview != nil {
			return StageAcceptance
		}
		return StageDevelopment
	}
	revision, ok := verifiedJourneyRevision(run)
	if !ok {
		return StageDevelopment
	}
	if run.ExecutableRevision == nil || run.ExecutableRevision.CodeRevision != revision {
		return StageTesting
	}
	for _, testCase := range run.TestCases {
		if !qualityCaseRequired(run, testCase.ID) {
			continue
		}
		result, _ := latestQualityObservation(run, revision, testCase.ID)
		if result != ResultPass && (run.Mode != DeliveryModeExistingSourceVerification || result != ResultFail) {
			return StageTesting
		}
	}
	if run.AcceptanceReview != nil {
		if acceptanceReviewCurrent(run) {
			if run.Mode == DeliveryModeExistingSourceVerification {
				return StageVerified
			}
			return StageRelease
		}
		return StageAcceptance
	}
	return StageAcceptance
}

func verifiedJourneyRevision(run *DeliveryRun) (string, bool) {
	if len(run.DeliveryUnits) == 0 || strings.TrimSpace(run.ActiveDeliveryUnitID) != "" {
		return "", false
	}
	for _, unit := range run.DeliveryUnits {
		if unit.Phase != DeliveryUnitComplete || unit.JourneyStatus != DeliveryGatePassed || strings.TrimSpace(unit.IntegratedGitRevision) == "" {
			return "", false
		}
	}
	revision := strings.TrimSpace(run.DeliveryUnits[len(run.DeliveryUnits)-1].IntegratedGitRevision)
	return revision, validCodeRevision(revision)
}

func latestQualityResult(run *DeliveryRun, revision, testCaseID string) CheckResult {
	result, _ := latestQualityObservation(run, revision, testCaseID)
	return result
}

func latestQualityObservation(run *DeliveryRun, revision, testCaseID string) (CheckResult, string) {
	for index := len(run.QualityRuns) - 1; index >= 0; index-- {
		result := run.QualityRuns[index]
		if result.GitRevision == revision && result.TestCaseID == testCaseID {
			return result.Result, result.FailureOwner
		}
	}
	return "", ""
}

func allQualityPass(run *DeliveryRun, revision string) bool {
	if run.ExecutableRevision == nil {
		return false
	}
	if len(run.TestCases) == 0 {
		return false
	}
	for _, testCase := range run.TestCases {
		if !qualityCaseRequired(run, testCase.ID) {
			continue
		}
		if latestQualityResult(run, revision, testCase.ID) != ResultPass {
			return false
		}
	}
	return true
}

func qualityReviewComplete(run *DeliveryRun, revision string) bool {
	if run.Mode != DeliveryModeExistingSourceVerification {
		return allQualityPass(run, revision)
	}
	if run.ExecutableRevision == nil || len(run.TestCases) == 0 {
		return false
	}
	for _, testCase := range run.TestCases {
		if !qualityCaseRequired(run, testCase.ID) {
			continue
		}
		result := latestQualityResult(run, revision, testCase.ID)
		if result != ResultPass && result != ResultFail {
			return false
		}
	}
	return true
}

func qualityCaseRequired(run *DeliveryRun, testCaseID string) bool {
	if run.ExecutableRevision == nil {
		return false
	}
	if run.ExecutableRevision.RequiredTestCaseIDs == nil {
		return true
	}
	for _, requiredID := range run.ExecutableRevision.RequiredTestCaseIDs {
		if requiredID == testCaseID {
			return true
		}
	}
	return false
}

func requiredQualityCaseIDs(run *DeliveryRun, revision string) []string {
	if run.ExecutableRevision == nil {
		return allTestCaseIDs(run)
	}
	previousRevision := run.ExecutableRevision.CodeRevision
	required := make(map[string]bool)
	foundTargetedRepair := false
	for unitIndex := range run.DeliveryUnits {
		unit := &run.DeliveryUnits[unitIndex]
		for _, repair := range unit.DevelopmentRepairItems {
			if repair.Status != DevelopmentTodoCompleted || !repairHasRevision(repair, revision) {
				continue
			}
			switch repair.SourceKind {
			case "quality_case":
				foundTargetedRepair = true
				required[repair.SourceID] = true
			case "acceptance_bug":
				foundTargetedRepair = true
				if run.AcceptanceReview != nil {
					if bug := findAcceptanceBug(run.AcceptanceReview, repair.SourceID); bug != nil && bug.RetestCaseID != "" {
						required[bug.RetestCaseID] = true
					}
				}
			case "cloud_deployment":
				foundTargetedRepair = true
				for _, testCase := range run.TestCases {
					if testCase.FeatureID == unit.ID {
						required[testCase.ID] = true
					}
				}
			case "verification_gap", "model_change":
				return allTestCaseIDs(run)
			case "repair_validation":
			}
		}
	}
	if !foundTargetedRepair {
		return allTestCaseIDs(run)
	}
	for _, testCase := range run.TestCases {
		if latestQualityResult(run, previousRevision, testCase.ID) != ResultPass {
			required[testCase.ID] = true
		}
	}
	ids := make([]string, 0, len(required))
	for _, testCase := range run.TestCases {
		if required[testCase.ID] {
			ids = append(ids, testCase.ID)
		}
	}
	return ids
}

func repairHasRevision(repair DevelopmentRepairWorkItem, revision string) bool {
	for _, evidence := range repair.Evidence {
		if evidence.GitRevision == revision {
			return true
		}
	}
	return false
}

func allTestCaseIDs(run *DeliveryRun) []string {
	ids := make([]string, 0, len(run.TestCases))
	for _, testCase := range run.TestCases {
		ids = append(ids, testCase.ID)
	}
	return ids
}

func qualityReleaseGate(run *DeliveryRun) ReleaseGate {
	revision, ready := verifiedJourneyRevision(run)
	label := "Independent QA passed"
	if run.Mode == DeliveryModeExistingSourceVerification {
		label = "Historical source evidence recorded"
	}
	return ReleaseGate{
		Code: "independent_quality_passed", Label: label,
		Detail: revision, OK: ready && qualityReviewComplete(run, revision),
	}
}

func validQualityFailureOwner(owner string) bool {
	return validDeliveryGapOwner(owner)
}

func evidenceRefsMatchRevision(references []string, revision string) bool {
	if len(references) == 0 {
		return false
	}
	prefix := "workspace:" + revision + "#evidence:"
	for _, reference := range references {
		if !strings.HasPrefix(reference, prefix) || strings.TrimPrefix(reference, prefix) == "" {
			return false
		}
	}
	return true
}
