package deliveryrun

import (
	"fmt"
	"strings"
	"time"
)

func releaseActions(run *DeliveryRun) []AvailableAction {
	if run.Stage != StageRelease {
		return []AvailableAction{}
	}
	if canPrepareRelease(run) {
		if release := releaseForReissue(run); release != nil {
			return []AvailableAction{catalogAction(commandReleaseReissue, release.ID)}
		}
	}
	for _, release := range run.Releases {
		switch release.Status {
		case ReleaseDraft:
			return []AvailableAction{catalogAction(commandReleaseApprove, release.ID)}
		case ReleaseApproved:
			return []AvailableAction{catalogAction(commandReleaseDeployResult, release.ID)}
		case ReleaseNeedsReconciliation:
			return []AvailableAction{catalogAction(commandReleaseReconcile, release.ID)}
		case ReleaseCancelled, ReleaseLive, ReleaseFailed, ReleaseUnpublished:
		}
	}
	actions := make([]AvailableAction, 0, len(run.ReleaseChecks)+1)
	if len(run.ReleaseChecks) == 0 {
		actions = append(actions, catalogAction(commandReleaseChecksReplace, ""))
	} else {
		for _, check := range run.ReleaseChecks {
			if check.Status != ReleaseCheckPassed {
				actions = append(actions, catalogAction(commandReleaseCheckRecord, check.ID))
			}
		}
	}
	if canPrepareRelease(run) {
		actions = append(actions, catalogAction(commandReleasePrepare, ""))
	}
	return actions
}

func activeRelease(run *DeliveryRun) bool {
	for _, release := range run.Releases {
		if release.Status != ReleaseFailed {
			return true
		}
	}
	return false
}

func prepareRelease(run *DeliveryRun, command Command, now time.Time) error {
	if run.Stage != StageRelease || activeDeliveryUnit(run) != nil {
		return Invalid("release_stage_invalid")
	}
	var payload struct {
		Version        string `json:"version"`
		EnvironmentRef string `json:"environment_ref"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.Version = strings.TrimSpace(payload.Version)
	payload.EnvironmentRef = strings.TrimSpace(payload.EnvironmentRef)
	if payload.Version == "" || payload.EnvironmentRef == "" {
		return Invalid("release_target_required")
	}
	revision, ok := verifiedJourneyRevision(run)
	if !ok || !backendGuideEvidenceReady(run) || !executableRevisionReady(run) || !allQualityPass(run, revision) || !acceptanceReviewCurrent(run) {
		return Invalid("release_gate_failed")
	}
	if !allRequiredReleaseChecksPass(run) {
		return Invalid("release_checks_incomplete")
	}
	for _, check := range run.ReleaseChecks {
		if check.EnvironmentRef != payload.EnvironmentRef {
			return Invalid("release_check_environment_mismatch")
		}
	}
	for _, release := range run.Releases {
		if release.CodeRevision == revision && release.Status != ReleaseFailed {
			return Invalid("release_exists")
		}
	}
	releaseID := fmt.Sprintf("REL-%03d", len(run.Releases)+1)
	run.Releases = append(run.Releases, Release{
		ID: releaseID, Version: payload.Version, CodeRevision: revision,
		ProductRevision: run.ExecutableRevision.TargetRevision, ProductRevisionRef: run.ExecutableRevision.EvidenceRef, ModelSHA256: run.ExecutableRevision.ModelSHA256,
		EnvironmentRef: payload.EnvironmentRef, Status: ReleaseDraft, CreatedAt: now,
	})
	appendActivity(run, command.Actor, "release_prepared", releaseID+" prepared", "The accepted Git revision awaits release approval.", now)
	return nil
}

func reissueRelease(run *DeliveryRun, command Command, now time.Time) error {
	if run.Stage != StageRelease || activeDeliveryUnit(run) != nil || !canPrepareRelease(run) {
		return Invalid("release_reissue_not_ready")
	}
	var payload struct {
		ReleaseID string `json:"release_id"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	source := releaseForReissue(run)
	if source == nil || source.ID != strings.TrimSpace(payload.ReleaseID) {
		return Invalid("release_reissue_source_invalid")
	}
	for _, check := range run.ReleaseChecks {
		if check.EnvironmentRef != source.EnvironmentRef {
			return Invalid("release_check_environment_mismatch")
		}
	}
	revision, _ := verifiedJourneyRevision(run)
	releaseID := fmt.Sprintf("REL-%03d", len(run.Releases)+1)
	approvedAt := now
	if source.Status == ReleaseApproved {
		source.Status = ReleaseCancelled
	}
	run.Releases = append(run.Releases, Release{
		ID: releaseID, Version: source.Version, CodeRevision: revision,
		ProductRevision: run.ExecutableRevision.TargetRevision, ProductRevisionRef: run.ExecutableRevision.EvidenceRef, ModelSHA256: run.ExecutableRevision.ModelSHA256,
		EnvironmentRef: source.EnvironmentRef, Status: ReleaseApproved, ApprovedBy: source.ApprovedBy, ApprovedAt: &approvedAt, CreatedAt: now,
	})
	appendActivity(run, command.Actor, "release_reissued", releaseID+" reissued", "The latest verified source retained the approved release target and is ready to publish.", now)
	return nil
}

func releaseForReissue(run *DeliveryRun) *Release {
	revision, ready := verifiedJourneyRevision(run)
	if !ready {
		return nil
	}
	for index := len(run.Releases) - 1; index >= 0; index-- {
		release := &run.Releases[index]
		if release.ApprovedBy == "" {
			continue
		}
		if release.Status == ReleaseApproved && release.CodeRevision != revision {
			return release
		}
		if release.Status != ReleaseFailed || len(release.DeploymentAttempts) == 0 {
			continue
		}
		attempt := release.DeploymentAttempts[len(release.DeploymentAttempts)-1]
		if attempt.Outcome == DeploymentFailure && attempt.FailureKind == deploymentFailureSource {
			return release
		}
	}
	return nil
}

func canPrepareRelease(run *DeliveryRun) bool {
	revision, ok := verifiedJourneyRevision(run)
	if !ok || !backendGuideEvidenceReady(run) || !executableRevisionReady(run) || !allQualityPass(run, revision) || !acceptanceReviewCurrent(run) || !allRequiredReleaseChecksPass(run) {
		return false
	}
	for _, release := range run.Releases {
		if release.CodeRevision == revision && release.Status != ReleaseFailed {
			return false
		}
	}
	return true
}

func ensureReleaseStillValid(run *DeliveryRun, release *Release) error {
	revision, ok := verifiedJourneyRevision(run)
	if !ok || release.CodeRevision != revision {
		return Invalid("release_stale")
	}
	if !backendGuideEvidenceReady(run) {
		return Invalid("backend_guide_evidence_stale")
	}
	if !executableRevisionReady(run) || release.ProductRevision != run.ExecutableRevision.TargetRevision || release.ProductRevisionRef != run.ExecutableRevision.EvidenceRef || release.ModelSHA256 != run.ExecutableRevision.ModelSHA256 {
		return Invalid("product_revision_release_mismatch")
	}
	if !allQualityPass(run, revision) || !acceptanceReviewCurrent(run) {
		return Invalid("release_gate_failed")
	}
	if !allRequiredReleaseChecksPass(run) {
		return Invalid("release_checks_incomplete")
	}
	return nil
}

func releaseChecksGate(run *DeliveryRun) ReleaseGate {
	return ReleaseGate{
		Code: "release_checks_passed", Label: "Release checks passed",
		Detail: releaseCheckDetail(run), OK: allRequiredReleaseChecksPass(run),
	}
}
