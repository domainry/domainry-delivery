package delivery

import (
	"fmt"
	"strings"
	"time"
)

func v3ReleaseActions(run *DeliveryRun) []AvailableAction {
	if run.Stage != StageRelease {
		return []AvailableAction{}
	}
	for _, release := range run.Releases {
		switch release.Status {
		case ReleaseDraft:
			return []AvailableAction{{Command: "release.approve", TargetID: release.ID, ActorKind: ActorHuman}}
		case ReleaseApproved:
			return []AvailableAction{{Command: "release.deploy_result", TargetID: release.ID, ActorKind: ActorSystem}}
		case ReleaseNeedsReconciliation:
			return []AvailableAction{{Command: "release.reconcile", TargetID: release.ID, ActorKind: ActorHuman}}
		case ReleaseLive, ReleaseFailed:
		}
	}
	actions := make([]AvailableAction, 0, len(run.ReleaseChecks)+1)
	if len(run.ReleaseChecks) == 0 {
		actions = append(actions, AvailableAction{Command: "release_checks.replace", ActorKind: ActorAgent})
	} else {
		for _, check := range run.ReleaseChecks {
			if check.Status != ReleaseCheckPassed {
				actions = append(actions, AvailableAction{Command: "release_check.record", TargetID: check.ID, ActorKind: ActorAgent})
			}
		}
	}
	if canPrepareV3Release(run) {
		actions = append(actions, AvailableAction{Command: "release.prepare", ActorKind: ActorHuman})
	}
	return actions
}

func activeV3Release(run *DeliveryRun) bool {
	for _, release := range run.Releases {
		if release.Status != ReleaseFailed {
			return true
		}
	}
	return false
}

func prepareV3Release(run *DeliveryRun, command Command, now time.Time) error {
	if run.Stage != StageRelease || activeDeliveryUnit(run) != nil {
		return Invalid("release_stage_invalid", "A V3 release can be prepared only after QA and business acceptance pass.")
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
	revision, ok := v3VerificationRevision(run)
	if !ok || !allQualityPass(run, revision) || !allV3AcceptancePass(run, revision) {
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
		EnvironmentRef: payload.EnvironmentRef, Status: ReleaseDraft, CreatedAt: now,
	})
	appendActivity(run, command.Actor, "release_prepared", releaseID+" prepared", "The accepted Git revision awaits release approval.", now)
	return nil
}

func canPrepareV3Release(run *DeliveryRun) bool {
	revision, ok := v3VerificationRevision(run)
	if !ok || !allQualityPass(run, revision) || !allV3AcceptancePass(run, revision) || !allRequiredReleaseChecksPass(run) {
		return false
	}
	for _, release := range run.Releases {
		if release.CodeRevision == revision && release.Status != ReleaseFailed {
			return false
		}
	}
	return true
}

func ensureV3ReleaseStillValid(run *DeliveryRun, release *Release) error {
	revision, ok := v3VerificationRevision(run)
	if !ok || release.CodeRevision != revision {
		return Invalid("release_stale", "The Release is not bound to the current verified Git revision.")
	}
	if !allQualityPass(run, revision) || !allV3AcceptancePass(run, revision) {
		return Invalid("release_gate_failed", "QA or business acceptance for the Release Git revision is no longer valid.")
	}
	if !allRequiredReleaseChecksPass(run) {
		return Invalid("release_checks_incomplete", "ReleaseChecks for the Release are no longer valid.")
	}
	return nil
}

func v3ReleaseChecksGate(run *DeliveryRun) ReleaseGate {
	return ReleaseGate{
		Code: "release_checks_passed", Label: "Release checks passed",
		Detail: releaseCheckDetail(run), OK: allRequiredReleaseChecksPass(run),
	}
}
