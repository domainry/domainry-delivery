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
	for _, release := range run.Releases {
		switch release.Status {
		case ReleaseDraft:
			return []AvailableAction{catalogAction(commandReleaseApprove, release.ID)}
		case ReleaseApproved:
			return []AvailableAction{catalogAction(commandReleaseDeployResult, release.ID)}
		case ReleaseNeedsReconciliation:
			return []AvailableAction{catalogAction(commandReleaseReconcile, release.ID)}
		case ReleaseLive, ReleaseFailed:
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
		return Invalid("release_stage_invalid", "A release can be prepared only after QA and business acceptance pass.")
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
		return Invalid("release_target_required", "A release requires a version and target environment.")
	}
	revision, ok := verifiedJourneyRevision(run)
	if !ok || !backendGuideEvidenceReady(run) || !executableRevisionReady(run) || !allQualityPass(run, revision) || !allAcceptancePass(run, revision) {
		return Invalid("release_gate_failed", "Journey, independent QA, and business acceptance must pass for the current Git revision.")
	}
	if !allRequiredReleaseChecksPass(run) {
		return Invalid("release_checks_incomplete", "Every required ReleaseCheck must pass before preparing a release.")
	}
	for _, check := range run.ReleaseChecks {
		if check.EnvironmentRef != payload.EnvironmentRef {
			return Invalid("release_check_environment_mismatch", "Release checks must match the release target environment.")
		}
	}
	for _, release := range run.Releases {
		if release.CodeRevision == revision && release.Status != ReleaseFailed {
			return Invalid("release_exists", "The current Git revision already has an active Release.")
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

func canPrepareRelease(run *DeliveryRun) bool {
	revision, ok := verifiedJourneyRevision(run)
	if !ok || !backendGuideEvidenceReady(run) || !executableRevisionReady(run) || !allQualityPass(run, revision) || !allAcceptancePass(run, revision) || !allRequiredReleaseChecksPass(run) {
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
		return Invalid("release_stale", "The Release is not bound to the current verified Git revision.")
	}
	if !backendGuideEvidenceReady(run) {
		return Invalid("backend_guide_evidence_stale", "The Release no longer has complete Plane backend guide evidence.")
	}
	if !executableRevisionReady(run) || release.ProductRevision != run.ExecutableRevision.TargetRevision || release.ProductRevisionRef != run.ExecutableRevision.EvidenceRef || release.ModelSHA256 != run.ExecutableRevision.ModelSHA256 {
		return Invalid("product_revision_release_mismatch", "The Release is not bound to the executable ProductRevision and model hash.")
	}
	if !allQualityPass(run, revision) || !allAcceptancePass(run, revision) {
		return Invalid("release_gate_failed", "QA or business acceptance for the Release Git revision is no longer valid.")
	}
	if !allRequiredReleaseChecksPass(run) {
		return Invalid("release_checks_incomplete", "ReleaseChecks for the Release are no longer valid.")
	}
	return nil
}

func releaseChecksGate(run *DeliveryRun) ReleaseGate {
	return ReleaseGate{
		Code: "release_checks_passed", Label: "Release checks passed",
		Detail: releaseCheckDetail(run), OK: allRequiredReleaseChecksPass(run),
	}
}
