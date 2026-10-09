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
		if latestQualityResult(run, revision, testCase.ID) == ResultPass {
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
	if run.AcceptanceReview != nil {
		if acceptanceReviewCurrent(run) {
			return StageRelease
		}
		return StageAcceptance
	}
	if activeDeliveryUnit(run) != nil {
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
		result, _ := latestQualityObservation(run, revision, testCase.ID)
		if result != ResultPass {
			return StageTesting
		}
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
	if len(run.TestCases) == 0 {
		return false
	}
	for _, testCase := range run.TestCases {
		if latestQualityResult(run, revision, testCase.ID) != ResultPass {
			return false
		}
	}
	return true
}

func qualityReleaseGate(run *DeliveryRun) ReleaseGate {
	revision, ready := verifiedJourneyRevision(run)
	return ReleaseGate{
		Code: "independent_quality_passed", Label: "Independent QA passed",
		Detail: revision, OK: ready && allQualityPass(run, revision),
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
