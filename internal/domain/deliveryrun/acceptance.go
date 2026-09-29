package deliveryrun

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

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
	if run.AcceptanceReview != nil || activeDeliveryUnit(run) != nil || run.ExecutableRevision == nil {
		return
	}
	revision, ready := verifiedJourneyRevision(run)
	if !ready || run.ExecutableRevision.CodeRevision != revision || !allQualityPass(run, revision) {
		return
	}
	review := &AcceptanceReview{
		ID: newID("acceptance"), Status: AcceptanceReviewOpen, CandidateGitRevision: revision,
		Bugs: []AcceptanceBug{}, Events: []AcceptanceEvent{}, OpenedAt: now,
	}
	run.AcceptanceReview = review
	appendAcceptanceEvent(review, actor, "", "acceptance_opened", "Acceptance environment is awaiting startup.", revision, nil, now)
	appendActivity(run, actor, "acceptance_opened", "User acceptance opened", revision, now)
}

func acceptanceActions(run *DeliveryRun) []AvailableAction {
	review := run.AcceptanceReview
	if run.Stage != StageAcceptance || review == nil || review.Status != AcceptanceReviewOpen {
		return []AvailableAction{}
	}
	if activeDeliveryUnit(run) != nil {
		return []AvailableAction{}
	}
	revision, verified := verifiedJourneyRevision(run)
	if !verified || run.ExecutableRevision == nil || run.ExecutableRevision.CodeRevision != revision {
		return []AvailableAction{}
	}
	if !allQualityPass(run, revision) {
		return qualityActions(run)
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

func reportAcceptanceBug(run *DeliveryRun, command Command, now time.Time) error {
	review, err := readyAcceptanceReview(run)
	if err != nil {
		return err
	}
	var payload struct {
		GitRevision  string   `json:"git_revision"`
		Title        string   `json:"title"`
		Description  string   `json:"description"`
		Expected     string   `json:"expected"`
		Actual       string   `json:"actual"`
		Severity     string   `json:"severity"`
		EvidenceRefs []string `json:"evidence_refs"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.GitRevision = strings.TrimSpace(payload.GitRevision)
	payload.Title = strings.TrimSpace(payload.Title)
	payload.Description = strings.TrimSpace(payload.Description)
	payload.Expected = strings.TrimSpace(payload.Expected)
	payload.Actual = strings.TrimSpace(payload.Actual)
	payload.Severity = strings.TrimSpace(payload.Severity)
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	if payload.GitRevision != review.CandidateGitRevision || len(review.Bugs) >= maxAcceptanceBugs || !boundedAcceptanceText(payload.Title, maxAcceptanceTitleRunes) || !boundedAcceptanceText(payload.Description, maxAcceptanceTextRunes) || !boundedAcceptanceText(payload.Expected, maxAcceptanceTextRunes) || !boundedAcceptanceText(payload.Actual, maxAcceptanceTextRunes) || !validAcceptanceSeverity(payload.Severity) || !validAcceptanceScreenshotRefs(payload.EvidenceRefs) {
		return Invalid("acceptance_bug_incomplete")
	}
	bug := AcceptanceBug{
		ID: newID("bug"), Title: payload.Title, Description: payload.Description,
		Expected: payload.Expected, Actual: payload.Actual, Severity: payload.Severity,
		Status: AcceptanceBugReported, ReportedAgainstRevision: payload.GitRevision,
		EvidenceRefs: payload.EvidenceRefs, ReportedBy: command.Actor.ID, ReportedAt: now, UpdatedAt: now,
	}
	review.Bugs = append(review.Bugs, bug)
	appendAcceptanceEvent(review, command.Actor, bug.ID, "bug_reported", payload.Description, payload.GitRevision, payload.EvidenceRefs, now)
	appendActivity(run, command.Actor, "acceptance_bug_reported", bug.ID+" reported", payload.Title, now)
	return nil
}

func triageAcceptanceBug(run *DeliveryRun, command Command, now time.Time) error {
	review, err := openAcceptanceReview(run)
	if err != nil {
		return err
	}
	var payload struct {
		BugID          string   `json:"bug_id"`
		Outcome        string   `json:"outcome"`
		Owner          string   `json:"owner"`
		DeliveryUnitID string   `json:"delivery_unit_id"`
		Note           string   `json:"note"`
		EvidenceRefs   []string `json:"evidence_refs"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	bug := findAcceptanceBug(review, strings.TrimSpace(payload.BugID))
	if bug == nil {
		return NotFound("AcceptanceBug", payload.BugID)
	}
	payload.Outcome = strings.TrimSpace(payload.Outcome)
	payload.Owner = strings.TrimSpace(payload.Owner)
	payload.DeliveryUnitID = strings.TrimSpace(payload.DeliveryUnitID)
	payload.Note = strings.TrimSpace(payload.Note)
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	if bug.Status != AcceptanceBugReported || !boundedAcceptanceText(payload.Note, maxAcceptanceTextRunes) || len(payload.EvidenceRefs) == 0 {
		return Invalid("acceptance_bug_triage_invalid")
	}
	if payload.Outcome == "not_reproduced" {
		if payload.Owner != "" || payload.DeliveryUnitID != "" {
			return Invalid("acceptance_bug_triage_invalid")
		}
		bug.Status = AcceptanceBugReadyForRetest
		bug.ReadyForRetestRevision = review.CandidateGitRevision
	} else if payload.Outcome == "reproduced" {
		if !validDeliveryGapOwner(payload.Owner) || payload.DeliveryUnitID == "" {
			return Invalid("acceptance_bug_triage_invalid")
		}
		unit := deliveryUnitByID(run, payload.DeliveryUnitID)
		if unit == nil {
			return Invalid("acceptance_delivery_unit_missing")
		}
		routeDeliveryUnitGap(unit, payload.Owner, []developmentRepairTarget{{
			SourceKind: "acceptance_bug",
			SourceID:   bug.ID,
			Title:      "Repair acceptance bug: " + bug.Title,
			Detail:     payload.Note,
		}})
		run.ActiveDeliveryUnitID = unit.ID
		bug.Status = AcceptanceBugFixing
		bug.Owner = payload.Owner
		bug.DeliveryUnitID = unit.ID
		bug.ReadyForRetestRevision = ""
	} else {
		return Invalid("acceptance_bug_triage_invalid")
	}
	bug.EvidenceRefs = append(bug.EvidenceRefs, payload.EvidenceRefs...)
	bug.UpdatedAt = now
	appendAcceptanceEvent(review, command.Actor, bug.ID, "bug_triaged", payload.Note, review.CandidateGitRevision, payload.EvidenceRefs, now)
	appendActivity(run, command.Actor, "acceptance_bug_triaged", bug.ID+" triaged", payload.Note, now)
	return nil
}

func markAcceptanceBugReady(run *DeliveryRun, command Command, now time.Time) error {
	review, err := openAcceptanceReview(run)
	if err != nil {
		return err
	}
	var payload struct {
		BugID        string   `json:"bug_id"`
		GitRevision  string   `json:"git_revision"`
		EvidenceRefs []string `json:"evidence_refs"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	bug := findAcceptanceBug(review, strings.TrimSpace(payload.BugID))
	if bug == nil {
		return NotFound("AcceptanceBug", payload.BugID)
	}
	payload.GitRevision = strings.TrimSpace(payload.GitRevision)
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	revision, ready := verifiedJourneyRevision(run)
	if bug.Status != AcceptanceBugFixing || !ready || revision != payload.GitRevision || revision == review.CandidateGitRevision || run.ExecutableRevision == nil || run.ExecutableRevision.CodeRevision != revision || !allQualityPass(run, revision) || !evidenceRefsMatchRevision(payload.EvidenceRefs, revision) {
		return Invalid("acceptance_bug_fix_not_ready")
	}
	review.CandidateGitRevision = revision
	review.EnvironmentRevision = ""
	review.EnvironmentRef = ""
	review.RuntimeURL = ""
	for index := range review.Bugs {
		candidate := &review.Bugs[index]
		if candidate.Status == AcceptanceBugFixing || candidate.Status == AcceptanceBugResolved {
			candidate.Status = AcceptanceBugReadyForRetest
			candidate.ReadyForRetestRevision = revision
			candidate.UpdatedAt = now
		}
	}
	bug.EvidenceRefs = append(bug.EvidenceRefs, payload.EvidenceRefs...)
	appendAcceptanceEvent(review, command.Actor, bug.ID, "fix_ready_for_retest", "Verified repair is ready for user retest.", revision, payload.EvidenceRefs, now)
	appendActivity(run, command.Actor, "acceptance_bug_fix_ready", bug.ID+" ready for retest", revision, now)
	return nil
}

func resolveAcceptanceBug(run *DeliveryRun, command Command, now time.Time) error {
	return updateAcceptanceBugByUser(run, command, now, AcceptanceBugResolved)
}

func reopenAcceptanceBug(run *DeliveryRun, command Command, now time.Time) error {
	return updateAcceptanceBugByUser(run, command, now, AcceptanceBugReported)
}

func updateAcceptanceBugByUser(run *DeliveryRun, command Command, now time.Time, next AcceptanceBugStatus) error {
	review, err := readyAcceptanceReview(run)
	if err != nil {
		return err
	}
	var payload struct {
		BugID        string   `json:"bug_id"`
		Note         string   `json:"note"`
		EvidenceRefs []string `json:"evidence_refs"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	bug := findAcceptanceBug(review, strings.TrimSpace(payload.BugID))
	if bug == nil {
		return NotFound("AcceptanceBug", payload.BugID)
	}
	payload.Note = strings.TrimSpace(payload.Note)
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	if !boundedAcceptanceText(payload.Note, maxAcceptanceTextRunes) {
		return Invalid("acceptance_bug_note_required")
	}
	kind := "bug_resolved"
	if next == AcceptanceBugResolved {
		if bug.Status != AcceptanceBugReadyForRetest || bug.ReadyForRetestRevision != review.CandidateGitRevision {
			return Invalid("acceptance_bug_not_retestable")
		}
	} else {
		if bug.Status != AcceptanceBugReadyForRetest && bug.Status != AcceptanceBugResolved {
			return Invalid("acceptance_bug_not_reopenable")
		}
		kind = "bug_reopened"
		bug.ReportedAgainstRevision = review.CandidateGitRevision
		bug.ReadyForRetestRevision = ""
		bug.Owner = ""
		bug.DeliveryUnitID = ""
	}
	bug.Status = next
	bug.EvidenceRefs = append(bug.EvidenceRefs, payload.EvidenceRefs...)
	bug.UpdatedAt = now
	appendAcceptanceEvent(review, command.Actor, bug.ID, kind, payload.Note, review.CandidateGitRevision, payload.EvidenceRefs, now)
	appendActivity(run, command.Actor, "acceptance_"+kind, bug.ID+" "+strings.ReplaceAll(kind, "_", " "), payload.Note, now)
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
	if !ready || revision != payload.GitRevision || run.ExecutableRevision == nil || run.ExecutableRevision.CodeRevision != revision || !allQualityPass(run, revision) {
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
	if review.EnvironmentRevision != review.CandidateGitRevision || review.EnvironmentRef == "" || review.RuntimeURL == "" {
		return nil, Invalid("acceptance_environment_not_ready")
	}
	return review, nil
}

func findAcceptanceBug(review *AcceptanceReview, id string) *AcceptanceBug {
	for index := range review.Bugs {
		if review.Bugs[index].ID == id {
			return &review.Bugs[index]
		}
	}
	return nil
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
	return ready && revision == review.CandidateGitRevision && run.ExecutableRevision != nil && run.ExecutableRevision.CodeRevision == revision && allQualityPass(run, revision)
}

func validAcceptanceSeverity(value string) bool {
	return value == "blocker" || value == "major" || value == "minor"
}

func validAcceptanceScreenshotRefs(references []string) bool {
	if len(references) != 1 {
		return false
	}
	attachmentID, ok := strings.CutPrefix(references[0], "attachment://")
	if !ok || attachmentID == "" || len(attachmentID) > 191 {
		return false
	}
	for _, character := range attachmentID {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func boundedAcceptanceText(value string, limit int) bool {
	return value != "" && utf8.RuneCountInString(value) <= limit
}

func appendAcceptanceEvent(review *AcceptanceReview, actor Actor, bugID, kind, note, revision string, evidenceRefs []string, now time.Time) {
	review.Events = append(review.Events, AcceptanceEvent{
		ID: newID("acceptance_event"), BugID: bugID, Kind: kind, Note: note,
		GitRevision: revision, EvidenceRefs: clone(evidenceRefs), ActorID: actor.ID, OccurredAt: now,
	})
}
