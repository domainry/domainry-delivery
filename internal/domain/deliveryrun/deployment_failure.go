package deliveryrun

import (
	"fmt"
	"strings"
	"time"
)

const (
	deploymentFailureSource   = "source"
	deploymentFailurePlatform = "platform"
)

func validateDeploymentFailureRouting(run *DeliveryRun, outcome DeploymentOutcome, failureKind, failureOwner, deliveryUnitID string, diagnostics []GateDiagnostic) error {
	if outcome != DeploymentFailure {
		if failureKind != "" || failureOwner != "" || deliveryUnitID != "" || len(diagnostics) != 0 {
			return Invalid("deployment_failure_routing_unexpected")
		}
		return nil
	}
	if failureKind == deploymentFailurePlatform {
		if failureOwner != "" || deliveryUnitID != "" || len(diagnostics) != 0 {
			return Invalid("deployment_failure_routing_invalid")
		}
		return nil
	}
	if failureKind != deploymentFailureSource || !validDeliveryGapOwner(failureOwner) || deliveryUnitByID(run, deliveryUnitID) == nil || len(diagnostics) == 0 || len(diagnostics) > 100 {
		return Invalid("deployment_failure_routing_invalid")
	}
	for index := range diagnostics {
		diagnostic := &diagnostics[index]
		diagnostic.Code = strings.TrimSpace(diagnostic.Code)
		diagnostic.Severity = strings.TrimSpace(diagnostic.Severity)
		diagnostic.Path = strings.TrimSpace(diagnostic.Path)
		diagnostic.Message = strings.TrimSpace(diagnostic.Message)
		diagnostic.Owner = strings.TrimSpace(diagnostic.Owner)
		diagnostic.Category = strings.TrimSpace(diagnostic.Category)
		diagnostic.Remediation = strings.TrimSpace(diagnostic.Remediation)
		if diagnostic.Code == "" || len(diagnostic.Code) > 128 || diagnostic.Severity != "error" || diagnostic.Path == "" || len(diagnostic.Path) > 1024 || diagnostic.Message == "" || len(diagnostic.Message) > 2000 || diagnostic.Owner != failureOwner || diagnostic.Category == "" || len(diagnostic.Category) > 128 || len(diagnostic.Remediation) > 2000 {
			return Invalid("deployment_failure_diagnostics_invalid")
		}
	}
	return nil
}

func routeDeploymentFailure(run *DeliveryRun, actor Actor, release *Release, attempt *DeploymentAttempt, now time.Time) error {
	review := run.AcceptanceReview
	if review == nil || review.Status != AcceptanceReviewAccepted || review.CandidateGitRevision != release.CodeRevision || len(review.Bugs) >= maxAcceptanceBugs {
		return Invalid("deployment_failure_acceptance_invalid")
	}
	diagnostic := attempt.Diagnostics[0]
	bug := AcceptanceBug{
		ID: newID("bug"), Title: "Cloud deployment build failed",
		Description: diagnostic.Message,
		Expected:    "The accepted Product revision builds and starts in the release environment.",
		Actual:      diagnostic.Message, Severity: "blocker", Status: AcceptanceBugFixing,
		Owner: attempt.FailureOwner, DeliveryUnitID: attempt.DeliveryUnitID,
		ReportedAgainstRevision: release.CodeRevision, EvidenceRefs: []string{attempt.ReceiptRef},
		ReportedBy: actor.ID, ReportedAt: now, UpdatedAt: now,
	}
	review.Status = AcceptanceReviewOpen
	review.EnvironmentRevision = ""
	review.EnvironmentRef = ""
	review.RuntimeURL = ""
	review.AcceptedBy = ""
	review.AcceptedAt = nil
	review.Bugs = append(review.Bugs, bug)
	targets := repairTargetsFromDiagnostics(attempt.Diagnostics, attempt.FailureOwner)
	for index := range targets {
		targets[index].SourceKind = "cloud_deployment"
		targets[index].Title = "Repair cloud deployment: " + strings.TrimPrefix(targets[index].Title, "Repair ")
	}
	if err := routeVerificationFailure(run, attempt.DeliveryUnitID, attempt.FailureOwner, targets); err != nil {
		return err
	}
	run.ReleaseChecks = []ReleaseCheck{}
	appendAcceptanceEvent(review, actor, bug.ID, "deployment_repair_started", diagnostic.Message, release.CodeRevision, bug.EvidenceRefs, now)
	appendActivity(run, actor, "release_repair_routed", fmt.Sprintf("%s routed to %s repair", release.ID, attempt.FailureOwner), diagnostic.Message, now)
	return nil
}
