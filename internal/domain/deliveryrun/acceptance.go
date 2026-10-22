package deliveryrun

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

const (
	commandAcceptanceEnvironmentReady = commanddomain.AcceptanceEnvironmentReady
	commandAcceptanceBugReport        = commanddomain.AcceptanceBugReport
	commandAcceptanceBugTriage        = commanddomain.AcceptanceBugTriage
	commandAcceptanceBugFixReady      = commanddomain.AcceptanceBugFixReady
	commandAcceptanceBugResolve       = commanddomain.AcceptanceBugResolve
	commandAcceptanceBugReopen        = commanddomain.AcceptanceBugReopen
	commandAcceptanceConfirm          = commanddomain.AcceptanceConfirm
	maxAcceptanceBugs                 = 100
	maxAcceptanceTitleRunes           = 120
	maxAcceptanceTextRunes            = 4_000
)

func ensureAcceptanceReview(run *DeliveryRun, actor Actor, now time.Time) {
	if activeDeliveryUnit(run) != nil || run.ExecutableRevision == nil {
		return
	}
	revision, ready := verifiedJourneyRevision(run)
	if !ready || run.ExecutableRevision.CodeRevision != revision || !qualityReviewComplete(run, revision) {
		return
	}
	if run.AcceptanceReview != nil {
		review := run.AcceptanceReview
		if !acceptanceReviewRequiresRenewal(run) {
			return
		}
		review.Status = AcceptanceReviewOpen
		review.CandidateGitRevision = revision
		review.EnvironmentRevision = ""
		review.EnvironmentRef = ""
		review.RuntimeURL = ""
		if run.Mode == DeliveryModeExistingSourceVerification {
			review.EnvironmentRevision = revision
			review.EnvironmentRef = run.SourceRef
		}
		review.AcceptedBy = ""
		review.AcceptedAt = nil
		appendAcceptanceEvent(review, actor, "", "acceptance_reopened", "Verified product changes require renewed business acceptance.", revision, nil, now)
		appendActivity(run, actor, "acceptance_reopened", "User acceptance reopened", revision, now)
		return
	}
	review := &AcceptanceReview{
		ID: newID("acceptance"), Status: AcceptanceReviewOpen, CandidateGitRevision: revision,
		Bugs: []AcceptanceBug{}, Events: []AcceptanceEvent{}, OpenedAt: now,
	}
	if run.Mode == DeliveryModeExistingSourceVerification {
		review.EnvironmentRevision = revision
		review.EnvironmentRef = run.SourceRef
	}
	run.AcceptanceReview = review
	note := "Acceptance environment is awaiting startup."
	if run.Mode == DeliveryModeExistingSourceVerification {
		note = "The verified historical source evidence is ready for confirmation."
	}
	appendAcceptanceEvent(review, actor, "", "acceptance_opened", note, revision, nil, now)
	appendActivity(run, actor, "acceptance_opened", "User acceptance opened", revision, now)
}

func acceptanceActions(run *DeliveryRun) []AvailableAction {
	review := run.AcceptanceReview
	if run.Stage != StageAcceptance || review == nil {
		return []AvailableAction{}
	}
	if review.Status == AcceptanceReviewAccepted {
		if acceptanceReviewRequiresRenewal(run) {
			return []AvailableAction{catalogAction(commandAcceptanceEnvironmentReady, review.ID)}
		}
		return []AvailableAction{}
	}
	if review.Status != AcceptanceReviewOpen {
		return []AvailableAction{}
	}
	if activeDeliveryUnit(run) != nil {
		return []AvailableAction{}
	}
	revision, verified := verifiedJourneyRevision(run)
	if !verified || run.ExecutableRevision == nil || run.ExecutableRevision.CodeRevision != revision {
		return []AvailableAction{}
	}
	if !qualityReviewComplete(run, revision) {
		return qualityActions(run)
	}
	if run.Mode == DeliveryModeExistingSourceVerification {
		if review.CandidateGitRevision == revision && review.EnvironmentRevision == revision && review.EnvironmentRef == run.SourceRef && allAcceptanceBugsResolved(review) {
			return []AvailableAction{catalogAction(commandAcceptanceConfirm, review.ID)}
		}
		return []AvailableAction{}
	}
	actions := make([]AvailableAction, 0, len(review.Bugs)+3)
	if revision != review.CandidateGitRevision {
		for _, bug := range review.Bugs {
			if bug.Status == AcceptanceBugFixing {
				actions = append(actions, catalogAction(commandAcceptanceBugFixReady, bug.ID))
			}
		}
		return actions
	}
	if review.EnvironmentRevision != revision || review.EnvironmentRef == "" || review.RuntimeURL == "" {
		return []AvailableAction{catalogAction(commandAcceptanceEnvironmentReady, review.ID)}
	}
	actions = append(actions, catalogAction(commandAcceptanceBugReport, review.ID))
	for _, bug := range review.Bugs {
		switch bug.Status {
		case AcceptanceBugReported:
			actions = append(actions, catalogAction(commandAcceptanceBugTriage, bug.ID))
		case AcceptanceBugReadyForRetest:
			actions = append(actions, catalogAction(commandAcceptanceBugResolve, bug.ID))
		case AcceptanceBugResolved:
			actions = append(actions, catalogAction(commandAcceptanceBugReopen, bug.ID))
		case AcceptanceBugFixing:
		}
	}
	if allAcceptanceBugsResolved(review) {
		actions = append(actions, catalogAction(commandAcceptanceConfirm, review.ID))
	}
	return actions
}

func recordAcceptanceEnvironment(run *DeliveryRun, command Command, now time.Time) error {
	if run.Mode == DeliveryModeExistingSourceVerification {
		return Invalid("historical_source_acceptance_runtime_forbidden")
	}
	ensureAcceptanceReview(run, command.Actor, now)
	review, err := openAcceptanceReview(run)
	if err != nil {
		return err
	}
	var payload struct {
		GitRevision    string `json:"git_revision"`
		EnvironmentRef string `json:"environment_ref"`
		RuntimeURL     string `json:"runtime_url"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.GitRevision = strings.TrimSpace(payload.GitRevision)
	payload.EnvironmentRef = strings.TrimSpace(payload.EnvironmentRef)
	payload.RuntimeURL = strings.TrimSpace(payload.RuntimeURL)
	parsed, parseErr := url.Parse(payload.RuntimeURL)
	if payload.GitRevision != review.CandidateGitRevision || payload.EnvironmentRef == "" || parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Invalid("acceptance_environment_invalid")
	}
	revision, ready := verifiedJourneyRevision(run)
	if !ready || revision != payload.GitRevision || run.ExecutableRevision == nil || run.ExecutableRevision.CodeRevision != revision || !allQualityPass(run, revision) {
		return Invalid("acceptance_environment_revision_stale")
	}
	review.EnvironmentRevision = revision
	review.EnvironmentRef = payload.EnvironmentRef
	review.RuntimeURL = parsed.String()
	appendAcceptanceEvent(review, command.Actor, "", "environment_ready", payload.EnvironmentRef, revision, nil, now)
	appendActivity(run, command.Actor, "acceptance_environment_ready", "Acceptance environment ready", payload.EnvironmentRef, now)
	return nil
}

func confirmAcceptance(run *DeliveryRun, command Command, now time.Time) error {
	review, err := readyAcceptanceReview(run)
	if err != nil {
		return err
	}
	var payload struct {
		GitRevision string `json:"git_revision"`
		Note        string `json:"note"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.GitRevision = strings.TrimSpace(payload.GitRevision)
	payload.Note = strings.TrimSpace(payload.Note)
	if payload.GitRevision != review.CandidateGitRevision || payload.Note == "" || !allAcceptanceBugsResolved(review) {
		return Invalid("acceptance_not_confirmable")
	}
	revision, ready := verifiedJourneyRevision(run)
	if !ready || revision != payload.GitRevision || run.ExecutableRevision == nil || run.ExecutableRevision.CodeRevision != revision || !qualityReviewComplete(run, revision) {
		return Invalid("acceptance_revision_stale")
	}
	review.Status = AcceptanceReviewAccepted
	review.AcceptedBy = command.Actor.ID
	review.AcceptedAt = &now
	appendAcceptanceEvent(review, command.Actor, "", "acceptance_confirmed", payload.Note, revision, nil, now)
	appendActivity(run, command.Actor, "acceptance_confirmed", "User acceptance confirmed", payload.Note, now)
	return nil
}

func openAcceptanceReview(run *DeliveryRun) (*AcceptanceReview, error) {
	if run.Stage != StageAcceptance || activeDeliveryUnit(run) != nil || run.AcceptanceReview == nil || run.AcceptanceReview.Status != AcceptanceReviewOpen {
		return nil, Invalid("acceptance_stage_invalid")
	}
	return run.AcceptanceReview, nil
}

func readyAcceptanceReview(run *DeliveryRun) (*AcceptanceReview, error) {
	review, err := openAcceptanceReview(run)
	if err != nil {
		return nil, err
	}
	if run.Mode == DeliveryModeExistingSourceVerification {
		if review.EnvironmentRevision != review.CandidateGitRevision || review.EnvironmentRef != run.SourceRef || review.RuntimeURL != "" {
			return nil, Invalid("historical_source_acceptance_evidence_not_ready")
		}
		return review, nil
	}
	if review.EnvironmentRevision != review.CandidateGitRevision || review.EnvironmentRef == "" || review.RuntimeURL == "" {
		return nil, Invalid("acceptance_environment_not_ready")
	}
	return review, nil
}

func allAcceptanceBugsResolved(review *AcceptanceReview) bool {
	for _, bug := range review.Bugs {
		if bug.Status != AcceptanceBugResolved {
			return false
		}
	}
	return true
}

func acceptanceReleaseGate(run *DeliveryRun) ReleaseGate {
	review := run.AcceptanceReview
	resolved := 0
	total := 0
	accepted := false
	detail := "not opened"
	if review != nil {
		total = len(review.Bugs)
		for _, bug := range review.Bugs {
			if bug.Status == AcceptanceBugResolved {
				resolved++
			}
		}
		accepted = acceptanceReviewCurrent(run)
		detail = fmt.Sprintf("%d / %d bugs resolved · %s", resolved, total, review.CandidateGitRevision)
	}
	return ReleaseGate{Code: "business_acceptance_passed", Label: "Business acceptance passed", Detail: detail, OK: accepted}
}

func acceptanceReviewCurrent(run *DeliveryRun) bool {
	review := run.AcceptanceReview
	if review == nil || review.Status != AcceptanceReviewAccepted || !allAcceptanceBugsResolved(review) || review.EnvironmentRevision != review.CandidateGitRevision {
		return false
	}
	revision, ready := verifiedJourneyRevision(run)
	if !ready || run.ExecutableRevision == nil || run.ExecutableRevision.CodeRevision != revision || !qualityReviewComplete(run, revision) {
		return false
	}
	return revision == review.CandidateGitRevision || deploymentRepairRevision(run, revision)
}

func deploymentRepairRevision(run *DeliveryRun, revision string) bool {
	foundDeploymentRepair := false
	for unitIndex := range run.DeliveryUnits {
		for _, repair := range run.DeliveryUnits[unitIndex].DevelopmentRepairItems {
			if !repairHasRevision(repair, revision) {
				continue
			}
			switch repair.SourceKind {
			case "cloud_deployment":
				foundDeploymentRepair = true
			case "repair_validation":
			default:
				return false
			}
		}
	}
	return foundDeploymentRepair
}

func appendAcceptanceEvent(review *AcceptanceReview, actor Actor, bugID, kind, note, revision string, evidenceRefs []string, now time.Time) {
	review.Events = append(review.Events, AcceptanceEvent{
		ID: newID("accevt"), BugID: bugID, Kind: kind, Note: note,
		GitRevision: revision, EvidenceRefs: clone(evidenceRefs), ActorID: actor.ID, OccurredAt: now,
	})
}
