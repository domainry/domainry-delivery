package deliveryrun

import "time"

// Summary is the lightweight read model used to build Product workspaces.
// It deliberately excludes evidence, activity, executable Product content and
// other detail-only state so listing DeliveryRuns never reconstructs every
// aggregate in the Product history.
type Summary struct {
	ID                               string          `json:"id"`
	Code                             string          `json:"code"`
	Goal                             string          `json:"goal"`
	Stage                            Stage           `json:"stage"`
	Revision                         uint64          `json:"revision"`
	Feature                          SummaryFeature  `json:"feature"`
	Progress                         SummaryProgress `json:"progress"`
	LatestActionableReleaseCreatedAt *time.Time      `json:"latest_actionable_release_created_at,omitempty"`
	UpdatedAt                        time.Time       `json:"updated_at"`
}

type SummaryFeature struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type SummaryProgress struct {
	DeliveryUnitsCompleted int `json:"delivery_units_completed"`
	DeliveryUnitsTotal     int `json:"delivery_units_total"`
	ReleaseGatesPassed     int `json:"release_gates_passed"`
	ReleaseGatesTotal      int `json:"release_gates_total"`
}

func SummaryFor(run DeliveryRun) Summary {
	completed := 0
	journeysPassed := 0
	for _, unit := range run.DeliveryUnits {
		if unit.Phase == DeliveryUnitComplete {
			completed++
		}
		if unit.JourneyStatus == DeliveryGatePassed {
			journeysPassed++
		}
	}

	passed := 0
	if run.Feature.ID != "" && run.Feature.Source.FeatureRevision > 0 {
		passed++
	}
	if len(run.DeliveryUnits) > 0 && completed == len(run.DeliveryUnits) {
		passed++
	}
	if len(run.DeliveryUnits) > 0 && journeysPassed == len(run.DeliveryUnits) {
		passed++
	}
	if backendGuideEvidenceReady(&run) {
		passed++
	}
	// Entering testing proves the executable revision gate passed. Later stages
	// prove the preceding quality and acceptance gates by domain transition.
	if stageAtLeast(run.Stage, StageTesting) {
		passed++
	}
	if stageAtLeast(run.Stage, StageAcceptance) {
		passed++
	}
	if stageAtLeast(run.Stage, StageRelease) {
		passed++
	}
	if len(run.ReleaseChecks) > 0 {
		allPassed := true
		for _, check := range run.ReleaseChecks {
			if check.Status != ReleaseCheckPassed {
				allPassed = false
				break
			}
		}
		if allPassed {
			passed++
		}
	}

	var latestActionable *time.Time
	for _, release := range run.Releases {
		if !actionableReleaseStatus(release.Status) {
			continue
		}
		createdAt := release.CreatedAt
		if latestActionable == nil || createdAt.After(*latestActionable) {
			latestActionable = &createdAt
		}
	}

	return Summary{
		ID:       run.ID,
		Code:     run.Code,
		Goal:     run.Goal,
		Stage:    run.Stage,
		Revision: run.Revision,
		Feature: SummaryFeature{
			ID:    run.Feature.ID,
			Title: run.Feature.Title,
		},
		Progress: SummaryProgress{
			DeliveryUnitsCompleted: completed,
			DeliveryUnitsTotal:     len(run.DeliveryUnits),
			ReleaseGatesPassed:     passed,
			ReleaseGatesTotal:      8,
		},
		LatestActionableReleaseCreatedAt: latestActionable,
		UpdatedAt:                        run.UpdatedAt,
	}
}

func stageAtLeast(stage, expected Stage) bool {
	order := map[Stage]int{
		StageDevelopment: 0,
		StageTesting:     1,
		StageAcceptance:  2,
		StageRelease:     3,
		StageLive:        4,
	}
	return order[stage] >= order[expected]
}

func actionableReleaseStatus(status ReleaseStatus) bool {
	return status == ReleaseApproved || status == ReleaseCancelled || status == ReleaseFailed ||
		status == ReleaseLive || status == ReleaseUnpublished || status == ReleaseNeedsReconciliation
}
