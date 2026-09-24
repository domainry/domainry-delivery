package deliveryrun

import (
	"fmt"
	"strings"
	"time"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

const commandAcceptanceConfirm = commanddomain.AcceptanceConfirm

func acceptanceActions(run *DeliveryRun) []AvailableAction {
	revision, ok := verifiedJourneyRevision(run)
	if run.Stage != StageAcceptance || !ok || !allQualityPass(run, revision) {
		return []AvailableAction{}
	}
	actions := make([]AvailableAction, 0, len(run.AcceptanceCases))
	for _, acceptanceCase := range run.AcceptanceCases {
		result, _ := latestAcceptanceConfirmation(run, revision, acceptanceCase.ID)
		if result == ResultPass {
			continue
		}
		actions = append(actions, catalogAction(commandAcceptanceConfirm, acceptanceCase.ID))
	}
	return actions
}

func confirmAcceptance(run *DeliveryRun, command Command, now time.Time) error {
	if run.Stage != StageAcceptance || activeDeliveryUnit(run) != nil {
		return Invalid("acceptance_stage_invalid", "Business acceptance can run only after independent QA passes.")
	}
	var payload struct {
		AcceptanceCaseID string      `json:"acceptance_case_id"`
		GitRevision      string      `json:"git_revision"`
		Result           CheckResult `json:"result"`
		Note             string      `json:"note"`
		EvidenceRefs     []string    `json:"evidence_refs"`
		FailureOwner     string      `json:"failure_owner"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	acceptanceCase := findAcceptanceCase(run, strings.TrimSpace(payload.AcceptanceCaseID))
	if acceptanceCase == nil {
		return NotFound("AcceptanceCase", payload.AcceptanceCaseID)
	}
	revision, ok := verifiedJourneyRevision(run)
	if !ok || strings.TrimSpace(payload.GitRevision) != revision || !allQualityPass(run, revision) {
		return Invalid("acceptance_quality_incomplete", "Business acceptance requires independent QA for the current Git revision.")
	}
	payload.Note = strings.TrimSpace(payload.Note)
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	payload.FailureOwner = strings.TrimSpace(payload.FailureOwner)
	if payload.Result != ResultPass && payload.Result != ResultFail {
		return Invalid("acceptance_result_invalid", "Business acceptance accepts only pass or fail.")
	}
	if payload.Note == "" || !evidenceRefsMatchRevision(payload.EvidenceRefs, revision) {
		return Invalid("acceptance_evidence_missing", "Business acceptance requires an actual conclusion and Git-bound evidence.")
	}
	if payload.Result == ResultFail && !validQualityFailureOwner(payload.FailureOwner) {
		return Invalid("acceptance_failure_owner_invalid", "Failed business acceptance must route to frontend, backend, framework, or unlocated.")
	}
	if payload.Result != ResultFail && payload.FailureOwner != "" {
		return Invalid("acceptance_failure_owner_invalid", "Only failed business acceptance can declare a failure owner.")
	}
	if payload.Result == ResultFail && (payload.FailureOwner == "frontend" || payload.FailureOwner == "backend") && deliveryUnitByID(run, acceptanceCase.FeatureID) == nil {
		return Invalid("acceptance_delivery_unit_missing", "The failed acceptance case has no DeliveryUnit to reopen.")
	}
	run.AcceptanceConfirmations = append(run.AcceptanceConfirmations, AcceptanceConfirmation{
		ID: newID("acceptance"), GitRevision: revision, AcceptanceCaseID: acceptanceCase.ID,
		Result: payload.Result, Note: payload.Note, EvidenceRefs: payload.EvidenceRefs,
		FailureOwner: payload.FailureOwner, ConfirmedBy: command.Actor.ID, ConfirmedAt: now,
	})
	appendActivity(run, command.Actor, "acceptance_confirmed", acceptanceCase.ID+" business acceptance recorded", payload.Note, now)
	if payload.Result == ResultFail {
		return routeVerificationFailure(run, acceptanceCase.FeatureID, payload.FailureOwner)
	}
	return nil
}

func latestAcceptanceConfirmation(run *DeliveryRun, revision, acceptanceCaseID string) (CheckResult, string) {
	for index := len(run.AcceptanceConfirmations) - 1; index >= 0; index-- {
		confirmation := run.AcceptanceConfirmations[index]
		if confirmation.GitRevision == revision && confirmation.AcceptanceCaseID == acceptanceCaseID {
			return confirmation.Result, confirmation.FailureOwner
		}
	}
	return "", ""
}

func allAcceptancePass(run *DeliveryRun, revision string) bool {
	if len(run.AcceptanceCases) == 0 {
		return false
	}
	for _, acceptanceCase := range run.AcceptanceCases {
		result, _ := latestAcceptanceConfirmation(run, revision, acceptanceCase.ID)
		if result != ResultPass {
			return false
		}
	}
	return true
}

func acceptanceReleaseGate(run *DeliveryRun) ReleaseGate {
	revision, ready := verifiedJourneyRevision(run)
	accepted := 0
	if ready {
		for _, acceptanceCase := range run.AcceptanceCases {
			result, _ := latestAcceptanceConfirmation(run, revision, acceptanceCase.ID)
			if result == ResultPass {
				accepted++
			}
		}
	}
	return ReleaseGate{
		Code: "business_acceptance_passed", Label: "Business acceptance passed",
		Detail: fmt.Sprintf("%d / %d · %s", accepted, len(run.AcceptanceCases), revision),
		OK:     ready && allAcceptancePass(run, revision),
	}
}
