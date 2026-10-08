package deliveryrun_test

import (
	"testing"
	"time"

	delivery "github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
)

func TestSummaryForProjectsWorkspaceProgressAndLatestActionableRelease(t *testing.T) {
	run := completedRun(t)
	older := time.Date(2026, time.October, 7, 9, 0, 0, 0, time.UTC)
	newerDraft := older.Add(time.Hour)
	run.Stage = delivery.StageLive
	run.ReleaseChecks = []delivery.ReleaseCheck{{ID: "RC-01", Status: delivery.ReleaseCheckPassed}}
	run.Releases = []delivery.Release{
		{ID: "REL-01", Status: delivery.ReleaseApproved, CreatedAt: older},
		{ID: "REL-02", Status: delivery.ReleaseDraft, CreatedAt: newerDraft},
	}

	summary := delivery.SummaryFor(run)
	if summary.ID != run.ID || summary.Code != run.Code || summary.Goal != run.Goal || summary.Revision != run.Revision {
		t.Fatalf("summary lost DeliveryRun identity: %#v", summary)
	}
	if summary.Feature.ID != run.Feature.ID || summary.Feature.Title != run.Feature.Title {
		t.Fatalf("summary lost Feature identity: %#v", summary.Feature)
	}
	if summary.Progress.DeliveryUnitsCompleted != len(run.DeliveryUnits) || summary.Progress.DeliveryUnitsTotal != len(run.DeliveryUnits) {
		t.Fatalf("summary projected incorrect DeliveryUnit progress: %#v", summary.Progress)
	}
	if summary.Progress.ReleaseGatesPassed != 8 || summary.Progress.ReleaseGatesTotal != 8 {
		t.Fatalf("summary projected incorrect release gate progress: %#v", summary.Progress)
	}
	if summary.LatestActionableReleaseCreatedAt == nil || !summary.LatestActionableReleaseCreatedAt.Equal(older) {
		t.Fatalf("summary selected a draft instead of the latest actionable Release: %#v", summary.LatestActionableReleaseCreatedAt)
	}
}
