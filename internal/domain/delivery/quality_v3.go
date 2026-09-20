package delivery

import (
	"strings"
	"time"
)

const commandQualityRecord = "quality.record"

func qualityActions(run *DeliveryRun) []AvailableAction {
	if run.Stage != StageTesting {
		return []AvailableAction{}
	}
	revision, ok := v3VerificationRevision(run)
	if !ok {
		return []AvailableAction{}
	}
	actions := make([]AvailableAction, 0, len(run.TestCases))
	for _, testCase := range run.TestCases {
		if latestQualityResult(run, revision, testCase.ID) == ResultPass {
			continue
		}
		actions = append(actions, AvailableAction{
			Command: commandQualityRecord, TargetID: testCase.ID, ActorKind: ActorAgent,
		})
	}
	return actions
}

func recordQuality(run *DeliveryRun, command Command, now time.Time) error {
	if run.Stage != StageTesting || activeDeliveryUnit(run) != nil {
		return Invalid("quality_stage_invalid", "Independent QA can run only after every DeliveryUnit Journey passes.")
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
	revision, ok := v3VerificationRevision(run)
	if !ok || strings.TrimSpace(payload.GitRevision) != revision {
		return Invalid("quality_revision_conflict", "QA evidence must match the latest Git revision that passed every DeliveryUnit Journey.")
	}
	payload.Note = strings.TrimSpace(payload.Note)
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	payload.FailureOwner = strings.TrimSpace(payload.FailureOwner)
	if !validResult(payload.Result, true) || payload.Note == "" || !evidenceRefsMatchRevision(payload.EvidenceRefs, revision) {
		return Invalid("quality_evidence_missing", "A QA result requires an outcome, actual observation, and at least one Git-bound evidence reference.")
	}
	if payload.Result == ResultFail && !validQualityFailureOwner(payload.FailureOwner) {
		return Invalid("quality_failure_owner_invalid", "A failed QA result must route to frontend, backend, framework, or unlocated.")
	}
	if payload.Result != ResultFail && payload.FailureOwner != "" {
		return Invalid("quality_failure_owner_invalid", "Only a failed QA result can declare a failure owner.")
	}
	if payload.Result == ResultFail && (payload.FailureOwner == "frontend" || payload.FailureOwner == "backend") && deliveryUnitByID(run, testCase.FeatureID) == nil {
		return Invalid("quality_delivery_unit_missing", "The failed QA case has no DeliveryUnit to reopen.")
	}
	run.QualityRuns = append(run.QualityRuns, QualityRun{
		ID: newID("quality"), GitRevision: revision, TestCaseID: testCase.ID,
		Result: payload.Result, Note: payload.Note, EvidenceRefs: payload.EvidenceRefs,
		FailureOwner: payload.FailureOwner, ExecutedBy: command.Actor.ID, ExecutedAt: now,
	})
	appendActivity(run, command.Actor, "quality_recorded", testCase.ID+" independent QA result recorded", payload.Note, now)
	if payload.Result == ResultFail {
		return routeQualityFailure(run, testCase.FeatureID, payload.FailureOwner)
	}
	return nil
}

func routeQualityFailure(run *DeliveryRun, deliveryUnitID, owner string) error {
	return routeV3Failure(run, deliveryUnitID, owner)
}

func routeV3Failure(run *DeliveryRun, deliveryUnitID, owner string) error {
	if owner == "framework" || owner == "unlocated" {
		run.ActiveDeliveryUnitID = ""
		run.Stage = StageBugs
		return nil
	}
	unit := deliveryUnitByID(run, deliveryUnitID)
	if unit == nil {
		return Invalid("quality_delivery_unit_missing", "The failed QA case has no DeliveryUnit to reopen.")
	}
	unit.JourneyStatus = DeliveryGatePending
	unit.ImplementationGitRevision = ""
	switch owner {
	case "frontend":
		unit.FrontendStatus = DeliveryGateNeedsChange
		unit.BackendStatus = DeliveryGatePending
		setDeliveryUnitPhase(unit, DeliveryUnitFrontendConvergence)
	case "backend":
		unit.BackendStatus = DeliveryGateNeedsChange
		setDeliveryUnitPhase(unit, DeliveryUnitBackendImplementation)
	}
	run.ActiveDeliveryUnitID = unit.ID
	run.Stage = StageDevelopment
	return nil
}

func deriveV3Stage(run *DeliveryRun) Stage {
	if activeDeliveryUnit(run) != nil {
		return StageDevelopment
	}
	revision, ok := v3VerificationRevision(run)
	if !ok {
		return StageDevelopment
	}
	for _, testCase := range run.TestCases {
		result, owner := latestQualityObservation(run, revision, testCase.ID)
		if result == ResultFail && (owner == "framework" || owner == "unlocated") {
			return StageBugs
		}
		if result != ResultPass {
			return StageTesting
		}
	}
	for _, acceptanceCase := range run.AcceptanceCases {
		result, owner := latestAcceptanceConfirmation(run, revision, acceptanceCase.ID)
		if result == ResultFail && (owner == "framework" || owner == "unlocated") {
			return StageBugs
		}
		if result != ResultPass {
			return StageAcceptance
		}
	}
	return StageRelease
}

func v3VerificationRevision(run *DeliveryRun) (string, bool) {
	if len(run.DeliveryUnits) == 0 || strings.TrimSpace(run.ActiveDeliveryUnitID) != "" {
		return "", false
	}
	for _, unit := range run.DeliveryUnits {
		if unit.Phase != DeliveryUnitComplete || unit.JourneyStatus != DeliveryGatePassed || strings.TrimSpace(unit.ImplementationGitRevision) == "" {
			return "", false
		}
	}
	revision := strings.TrimSpace(run.DeliveryUnits[len(run.DeliveryUnits)-1].ImplementationGitRevision)
	return revision, validGitRevision(revision)
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
	revision, ready := v3VerificationRevision(run)
	return ReleaseGate{
		Code: "independent_quality_passed", Label: "Independent QA passed",
		Detail: revision, OK: ready && allQualityPass(run, revision),
	}
}

func validQualityFailureOwner(owner string) bool {
	return owner == "frontend" || owner == "backend" || owner == "framework" || owner == "unlocated"
}

func evidenceRefsMatchRevision(references []string, revision string) bool {
	if len(references) == 0 {
		return false
	}
	prefix := "git:" + revision + "#evidence:"
	for _, reference := range references {
		if !strings.HasPrefix(reference, prefix) || strings.TrimPrefix(reference, prefix) == "" {
			return false
		}
	}
	return true
}
