package deliveryrun

import (
	"strings"
	"time"
	"unicode/utf8"
)

func reportAcceptanceBug(run *DeliveryRun, command Command, now time.Time) error {
	if run.Mode == DeliveryModeExistingSourceVerification {
		return Invalid("historical_source_acceptance_read_only")
	}
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
	if run.Mode == DeliveryModeExistingSourceVerification {
		return Invalid("historical_source_acceptance_read_only")
	}
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
		bug.RetestCaseID = "T-" + bug.ID
		run.TestCases = append(run.TestCases, TestCase{
			ID: bug.RetestCaseID, FeatureID: unit.ID, Title: "Retest repaired bug: " + bug.Title,
			Expected: "Reported behavior: " + bug.Description + " | Expected: " + bug.Expected + " | Previous actual: " + bug.Actual,
			Revision: 1,
		})
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
	if run.Mode == DeliveryModeExistingSourceVerification {
		return Invalid("historical_source_acceptance_read_only")
	}
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
	if run.Mode == DeliveryModeExistingSourceVerification {
		return Invalid("historical_source_acceptance_read_only")
	}
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

func findAcceptanceBug(review *AcceptanceReview, id string) *AcceptanceBug {
	for index := range review.Bugs {
		if review.Bugs[index].ID == id {
			return &review.Bugs[index]
		}
	}
	return nil
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
