package delivery

import "fmt"

// ProjectionFor produces the server-owned UI contract for a DeliveryRun.
// Keeping availability here prevents desktop and web clients from reimplementing
// lifecycle gates differently.
func ProjectionFor(run DeliveryRun) Projection {
	workflow := Workflow{
		AvailableActions: make([]AvailableAction, 0),
		ReleaseGates:     releaseGatesFor(&run),
	}
	add := func(command, targetID string, actorKind ActorKind) {
		workflow.AvailableActions = append(workflow.AvailableActions, AvailableAction{
			Command: command, TargetID: targetID, ActorKind: actorKind,
		})
	}
	if runIsLive(&run) {
		return Projection{DeliveryRun: run, Workflow: workflow}
	}
	if actions, v3 := deliveryUnitActions(&run); v3 {
		workflow.AvailableActions = actions
		workflow.ReleaseGates = deliveryUnitReleaseGates(&run)
		return Projection{DeliveryRun: run, Workflow: workflow}
	}

	if workPlanCanBeReplaced(&run) {
		add("work.plan.replace", "", ActorAgent)
	}

	for _, item := range run.WorkItems {
		switch item.Status {
		case WorkTodo:
			add("work.start", item.ID, ActorAgent)
		case WorkInProgress:
			if workDependenciesMerged(&run, item) {
				add("work.complete", item.ID, ActorAgent)
			}
		}
	}
	if canRecordExecutableRevision(&run) {
		add("product_revision.record", "", ActorAgent)
	}

	if canCreateBuild(&run) {
		add("build.create", "", ActorAgent)
	}
	if len(run.Builds) > 0 {
		build := &run.Builds[len(run.Builds)-1]
		if build.Status == BuildGenerated {
			add("build.deploy_to_test", build.ID, ActorAgent)
		}
		if build.Status == BuildTestDeployed && !verificationLockedByRelease(&run, build.ID) {
			for _, testCase := range run.TestCases {
				add("test.record", testCase.ID, ActorAgent)
			}
			if canRecordAcceptance(&run, build.ID) {
				for _, acceptanceCase := range run.AcceptanceCases {
					add("acceptance.record", acceptanceCase.ID, ActorHuman)
				}
			}
		}
	}

	for _, issue := range run.Issues {
		switch issue.Status {
		case IssueOpen:
			add("issue.start_fix", issue.ID, ActorAgent)
		case IssueFixing:
			add("issue.complete_fix", issue.ID, ActorAgent)
		}
	}
	if len(run.Releases) == 0 {
		add("release_checks.replace", "", ActorAgent)
		for _, check := range run.ReleaseChecks {
			add("release_check.record", check.ID, ActorAgent)
		}
	}

	if allReleaseGatesPass(workflow.ReleaseGates) && canPrepareRelease(&run) {
		add("release.prepare", "", ActorHuman)
	}
	for _, release := range run.Releases {
		switch release.Status {
		case ReleaseDraft:
			add("release.approve", release.ID, ActorHuman)
		case ReleaseApproved:
			add("release.deploy_result", release.ID, ActorSystem)
		case ReleaseNeedsReconciliation:
			add("release.reconcile", release.ID, ActorHuman)
		}
	}

	return Projection{DeliveryRun: run, Workflow: workflow}
}

func workPlanCanBeReplaced(run *DeliveryRun) bool {
	if len(run.Builds) > 0 {
		return false
	}
	for _, item := range run.WorkItems {
		if item.Status != WorkTodo {
			return false
		}
	}
	return true
}

func workDependenciesMerged(run *DeliveryRun, item WorkItem) bool {
	for _, dependencyID := range item.DependsOn {
		dependency := findWork(run, dependencyID)
		if dependency == nil || dependency.Status != WorkMerged {
			return false
		}
	}
	return true
}

func canCreateBuild(run *DeliveryRun) bool {
	if run.WorkPlanRevision == 0 || run.ExecutableRevision == nil {
		return false
	}
	for _, item := range run.WorkItems {
		if item.Status != WorkMerged {
			return false
		}
	}
	readyRetest := false
	for _, issue := range run.Issues {
		switch issue.Status {
		case IssueOpen, IssueFixing:
			return false
		case IssueReadyRetest:
			if issue.IncludedInBuildID == "" {
				readyRetest = true
			}
		}
	}
	if len(run.Builds) == 0 {
		return true
	}
	latestBuild := run.Builds[len(run.Builds)-1]
	return readyRetest && latestBuild.CodeRevision != run.ExecutableRevision.CodeRevision
}

func canRecordExecutableRevision(run *DeliveryRun) bool {
	if run.WorkPlanRevision == 0 || len(run.Releases) > 0 {
		return false
	}
	for _, item := range run.WorkItems {
		if item.Status != WorkMerged {
			return false
		}
	}
	if len(run.Builds) == 0 {
		return true
	}
	for _, issue := range run.Issues {
		if issue.Status == IssueReadyRetest && issue.IncludedInBuildID == "" {
			return true
		}
	}
	return false
}

func canRecordAcceptance(run *DeliveryRun, buildID string) bool {
	if !allTechnicalTestsPass(run, buildID) {
		return false
	}
	for _, issue := range run.Issues {
		allowedRetest := issue.Status == IssueReadyRetest &&
			issue.IncludedInBuildID == buildID &&
			issue.Origin.Kind == "acceptance"
		if issue.Status != IssueClosed && !allowedRetest {
			return false
		}
	}
	return true
}

func canPrepareRelease(run *DeliveryRun) bool {
	if len(run.Builds) == 0 || run.ExecutableRevision == nil {
		return false
	}
	build := run.Builds[len(run.Builds)-1]
	if build.ProductRevision != run.ExecutableRevision.TargetRevision || build.ProductRevisionRef != run.ExecutableRevision.EvidenceRef || build.CodeRevision != run.ExecutableRevision.CodeRevision {
		return false
	}
	buildID := build.ID
	for _, release := range run.Releases {
		if release.BuildID == buildID && release.Status != ReleaseFailed {
			return false
		}
	}
	return true
}

func releaseGatesFor(run *DeliveryRun) []ReleaseGate {
	latestBuildID := ""
	if len(run.Builds) > 0 {
		latestBuildID = run.Builds[len(run.Builds)-1].ID
	}
	merged := 0
	for _, item := range run.WorkItems {
		if item.Status == WorkMerged {
			merged++
		}
	}
	openIssues := 0
	for _, issue := range run.Issues {
		if issue.Status != IssueClosed {
			openIssues++
		}
	}
	accepted := 0
	if latestBuildID != "" {
		for _, acceptanceCase := range run.AcceptanceCases {
			if latestAcceptanceResult(run, latestBuildID, acceptanceCase.ID) == ResultPass {
				accepted++
			}
		}
	}
	buildDetail := "No build"
	if latestBuildID != "" {
		buildDetail = latestBuildID
	}
	acceptanceDetail := "No build"
	if latestBuildID != "" {
		acceptanceDetail = fmt.Sprintf("%d / %d", accepted, len(run.AcceptanceCases))
	}
	executableDetail := "No executable ProductRevision"
	executableReady := false
	if run.ExecutableRevision != nil {
		executableDetail = fmt.Sprintf("R%d · %s", run.ExecutableRevision.TargetRevision, run.ExecutableRevision.EvidenceRef)
		if latestBuildID != "" {
			build := run.Builds[len(run.Builds)-1]
			executableReady = build.ProductRevision == run.ExecutableRevision.TargetRevision && build.ProductRevisionRef == run.ExecutableRevision.EvidenceRef && build.CodeRevision == run.ExecutableRevision.CodeRevision
		}
	}
	return []ReleaseGate{
		{Code: "feature_frozen", Label: "FeatureRevision frozen", Detail: fmt.Sprintf("%s / R%d", run.Feature.ID, run.Feature.Source.FeatureRevision), OK: run.Feature.ID != "" && run.Feature.Source.FeatureRevision > 0},
		{Code: "development_completed", Label: "Development completed", Detail: fmt.Sprintf("%d / %d", merged, len(run.WorkItems)), OK: run.WorkPlanRevision > 0 && merged == len(run.WorkItems)},
		{Code: "product_revision_bound", Label: "Executable ProductRevision bound", Detail: executableDetail, OK: executableReady},
		{Code: "technical_tests_passed", Label: "Technical tests passed", Detail: buildDetail, OK: latestBuildID != "" && allTechnicalTestsPass(run, latestBuildID)},
		{Code: "issues_closed", Label: "All Issues closed", Detail: fmt.Sprintf("%d open", openIssues), OK: openIssues == 0},
		{Code: "acceptance_passed", Label: "Business acceptance passed", Detail: acceptanceDetail, OK: latestBuildID != "" && allAcceptancePass(run, latestBuildID)},
		{Code: "release_checks_passed", Label: "Release checks passed", Detail: releaseCheckDetail(run), OK: allRequiredReleaseChecksPass(run)},
	}
}

func releaseCheckDetail(run *DeliveryRun) string {
	passed := 0
	required := 0
	for _, check := range run.ReleaseChecks {
		if !check.Required {
			continue
		}
		required++
		if check.Status == ReleaseCheckPassed {
			passed++
		}
	}
	return fmt.Sprintf("%d / %d", passed, required)
}

func allReleaseGatesPass(gates []ReleaseGate) bool {
	for _, gate := range gates {
		if !gate.OK {
			return false
		}
	}
	return true
}
