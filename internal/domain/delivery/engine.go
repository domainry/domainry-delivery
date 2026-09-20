package delivery

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var agentCommands = map[string]bool{
	"work.plan.replace":        true,
	"work.start":               true,
	"work.complete":            true,
	"product_revision.record":  true,
	"build.create":             true,
	"build.deploy_to_test":     true,
	"test.record":              true,
	"quality.record":           true,
	"issue.start_fix":          true,
	"issue.complete_fix":       true,
	"release_checks.replace":   true,
	"release_check.record":     true,
	commandInteractionComplete: true,
	commandModelComplete:       true,
	commandFrontendComplete:    true,
	commandContractGapReport:   true,
	commandBackendComplete:     true,
}

var humanOnlyCommands = map[string]bool{
	"acceptance.record":      true,
	commandAcceptanceConfirm: true,
	"release.prepare":        true,
	"release.approve":        true,
	"release.reconcile":      true,
}

var systemOnlyCommands = map[string]bool{
	"release.deploy_result": true,
	commandCompileComplete:  true,
	commandContractFreeze:   true,
	commandBackendReopen:    true,
	commandContractReopen:   true,
	commandJourneyComplete:  true,
}

func Apply(run *DeliveryRun, command Command, now time.Time) error {
	if runIsLive(run) {
		return Invalid("delivery_run_live", "A live DeliveryRun is immutable; create a new Feature for a new requirement or fix.")
	}
	if strings.TrimSpace(command.Actor.ID) == "" {
		return Invalid("actor_required", "An authenticated actor is required.")
	}
	if command.Actor.Kind != ActorHuman && command.Actor.Kind != ActorAgent && command.Actor.Kind != ActorSystem {
		return Invalid("actor_kind_invalid", "The actor kind is invalid.")
	}
	if command.Actor.Kind == ActorAgent && !agentCommands[command.Type] {
		return Invalid("human_confirmation_required", "An authenticated owner must confirm this action; an Agent may only propose or prepare it.")
	}
	if humanOnlyCommands[command.Type] && command.Actor.Kind != ActorHuman {
		return Invalid("human_confirmation_required", "An authenticated owner must confirm this action.")
	}
	if systemOnlyCommands[command.Type] && command.Actor.Kind != ActorSystem {
		return Invalid("system_execution_required", "Only a trusted deployment adapter can record this action.")
	}
	if command.Actor.Kind == ActorHuman {
		if err := authorizeHumanCommand(run, command); err != nil {
			return err
		}
	}
	if handled, err := applyDeliveryUnitCommand(run, command, now); handled {
		if err != nil {
			return err
		}
		run.UpdatedAt = now
		return nil
	}

	var err error
	switch command.Type {
	case "work.plan.replace":
		err = replaceWorkPlan(run, command, now)
	case "work.start":
		err = transitionWork(run, command, WorkTodo, WorkInProgress, now)
	case "work.complete":
		err = completeWork(run, command, now)
	case "product_revision.record":
		err = recordExecutableRevision(run, command, now)
	case "build.create":
		err = createBuild(run, command, now)
	case "build.deploy_to_test":
		err = deployBuild(run, command, now)
	case "test.record":
		err = recordTest(run, command, now)
	case "quality.record":
		err = recordQuality(run, command, now)
	case "issue.start_fix":
		err = transitionIssue(run, command, IssueOpen, IssueFixing, now)
	case "issue.complete_fix":
		err = completeIssueFix(run, command, now)
	case "acceptance.record":
		err = recordAcceptance(run, command, now)
	case commandAcceptanceConfirm:
		err = confirmAcceptance(run, command, now)
	case "release_checks.replace":
		err = replaceReleaseChecks(run, command, now)
	case "release_check.record":
		err = recordReleaseCheck(run, command, now)
	case "release.prepare":
		if len(run.DeliveryUnits) > 0 {
			err = prepareV3Release(run, command, now)
		} else {
			err = prepareRelease(run, command, now)
		}
	case "release.approve":
		err = approveRelease(run, command, now)
	case "release.deploy_result":
		err = recordDeployment(run, command, now)
	case "release.reconcile":
		err = reconcileDeployment(run, command, now)
	default:
		err = Invalid("command_unknown", "The DeliveryRun command is not supported.")
	}
	if err != nil {
		return err
	}
	run.Stage = DeriveStage(run)
	run.UpdatedAt = now
	return nil
}

func recordExecutableRevision(run *DeliveryRun, command Command, now time.Time) error {
	if !canRecordExecutableRevision(run) {
		return Invalid("product_revision_record_not_allowed", "The executable ProductRevision can be recorded only after development completes and before its build is created.")
	}
	var payload struct {
		Content      ProductRevisionContent `json:"content"`
		EvidenceRef  string                 `json:"evidence_ref"`
		CodeRevision string                 `json:"code_revision"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.Content.Story.Title = strings.TrimSpace(payload.Content.Story.Title)
	payload.Content.Story.Summary = strings.TrimSpace(payload.Content.Story.Summary)
	payload.Content.Story.Narrative = strings.TrimSpace(payload.Content.Story.Narrative)
	payload.Content.Definition = normalizeProductDefinition(payload.Content.Definition)
	payload.EvidenceRef = strings.TrimSpace(payload.EvidenceRef)
	payload.CodeRevision = strings.TrimSpace(payload.CodeRevision)
	if !strings.HasPrefix(payload.EvidenceRef, "deck-product-revision://sha256/") || !strings.HasPrefix(payload.CodeRevision, "git:") {
		return Invalid("product_revision_evidence_invalid", "An executable ProductRevision requires Deck-captured definition and Git revision evidence.")
	}
	if err := validateProductRevisionContent(payload.Content, true); err != nil {
		return err
	}
	baseRevision := run.Feature.BaselineProductRevision
	if baseRevision == 0 {
		return Invalid("product_revision_baseline_invalid", "The executable ProductRevision must extend the DeliveryRun Product baseline.")
	}
	run.ExecutableRevision = &ExecutableProductRevision{
		BaseRevision: baseRevision, TargetRevision: baseRevision + 1,
		Content: clone(payload.Content), EvidenceRef: payload.EvidenceRef,
		CodeRevision: payload.CodeRevision, RecordedBy: command.Actor.ID, RecordedAt: now,
	}
	appendActivity(run, command.Actor, "product_revision_recorded", fmt.Sprintf("ProductRevision R%d recorded", baseRevision+1), payload.EvidenceRef, now)
	return nil
}

func replaceWorkPlan(run *DeliveryRun, command Command, now time.Time) error {
	if len(run.Builds) > 0 {
		return Invalid("work_plan_locked", "The work plan cannot change after a build is created.")
	}
	for _, item := range run.WorkItems {
		if item.Status != WorkTodo {
			return Invalid("work_plan_locked", "The work plan cannot be replaced after development starts.")
		}
	}
	var payload struct {
		Note  string `json:"note"`
		Items []struct {
			ID        string   `json:"id"`
			Title     string   `json:"title"`
			OwnerID   string   `json:"owner_id"`
			DependsOn []string `json:"depends_on"`
		} `json:"items"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.Note = strings.TrimSpace(payload.Note)
	if payload.Note == "" {
		return Invalid("work_plan_note_required", "The work plan must explain its decomposition; a simple Feature must explain why no WorkItems are needed.")
	}
	seen := map[string]bool{}
	items := make([]WorkItem, 0, len(payload.Items))
	for _, input := range payload.Items {
		input.ID = strings.TrimSpace(input.ID)
		input.Title = strings.TrimSpace(input.Title)
		input.OwnerID = strings.TrimSpace(input.OwnerID)
		if input.ID == "" || input.Title == "" || input.OwnerID == "" {
			return Invalid("work_item_incomplete", "A WorkItem requires an ID, title, and owner.")
		}
		if seen[input.ID] {
			return Invalid("work_item_duplicate", "WorkItem IDs must be unique within a work plan.")
		}
		seen[input.ID] = true
		items = append(items, WorkItem{
			ID: input.ID, FeatureID: run.Feature.ID, Title: input.Title, OwnerID: input.OwnerID,
			Status: WorkTodo, DependsOn: cleanStrings(input.DependsOn), UpdatedAt: now,
		})
	}
	for _, item := range items {
		for _, dependencyID := range item.DependsOn {
			if dependencyID == item.ID || !seen[dependencyID] {
				return Invalid("work_dependency_invalid", "A WorkItem dependency must reference another WorkItem in the plan.")
			}
		}
	}
	if workPlanHasCycle(items) {
		return Invalid("work_dependency_cycle", "Work item dependencies cannot contain a cycle.")
	}
	run.WorkItems = items
	run.WorkPlanRevision++
	run.WorkPlanNote = payload.Note
	appendActivity(run, command.Actor, "work_plan_replaced", "Work plan recorded", fmt.Sprintf("%d WorkItems; %s", len(items), payload.Note), now)
	return nil
}

func workPlanHasCycle(items []WorkItem) bool {
	dependencies := make(map[string][]string, len(items))
	for _, item := range items {
		dependencies[item.ID] = item.DependsOn
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dependencyID := range dependencies[id] {
			if visit(dependencyID) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range dependencies {
		if visit(id) {
			return true
		}
	}
	return false
}

type entityPayload struct {
	WorkItemID   string `json:"work_item_id"`
	IssueID      string `json:"issue_id"`
	ReleaseID    string `json:"release_id"`
	DeliveryNote string `json:"delivery_note"`
	FixNote      string `json:"fix_note"`
}

func transitionWork(run *DeliveryRun, command Command, expected, next WorkStatus, now time.Time) error {
	var payload entityPayload
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	item := findWork(run, payload.WorkItemID)
	if item == nil {
		return NotFound("WorkItem", payload.WorkItemID)
	}
	if item.Status != expected {
		return Invalid("work_transition_invalid", "The current WorkItem state does not allow this action.")
	}
	item.Status = next
	item.UpdatedAt = now
	if payload.DeliveryNote != "" {
		item.DeliveryNote = strings.TrimSpace(payload.DeliveryNote)
	}
	appendActivity(run, command.Actor, "work_transitioned", item.ID+" advanced", item.Title, now)
	return nil
}

func completeWork(run *DeliveryRun, command Command, now time.Time) error {
	var payload entityPayload
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	item := findWork(run, payload.WorkItemID)
	if item == nil {
		return NotFound("WorkItem", payload.WorkItemID)
	}
	if item.Status != WorkInProgress {
		return Invalid("work_transition_invalid", "Only an in-progress WorkItem can be completed.")
	}
	for _, dependencyID := range item.DependsOn {
		dependency := findWork(run, dependencyID)
		if dependency == nil || dependency.Status != WorkMerged {
			return Invalid("work_dependency_open", "A prerequisite WorkItem is not complete.")
		}
	}
	item.Status = WorkMerged
	item.UpdatedAt = now
	if strings.TrimSpace(payload.DeliveryNote) != "" {
		item.DeliveryNote = strings.TrimSpace(payload.DeliveryNote)
	}
	appendActivity(run, command.Actor, "work_completed", item.ID+" completed", item.Title, now)
	return nil
}

func createBuild(run *DeliveryRun, command Command, now time.Time) error {
	if run.WorkPlanRevision == 0 {
		return Invalid("work_plan_missing", "The RD Agent must record a work plan first; a simple Feature may use an empty WorkItem list.")
	}
	for _, item := range run.WorkItems {
		if item.Status != WorkMerged {
			return Invalid("development_incomplete", "Every baseline WorkItem must be complete before creating a build.")
		}
	}
	for _, issue := range run.Issues {
		if issue.Status == IssueOpen || issue.Status == IssueFixing {
			return Invalid("issue_fix_incomplete", "An Issue still has an incomplete fix.")
		}
	}
	var payload struct {
		CodeRevision        string `json:"code_revision"`
		ArtifactRef         string `json:"artifact_ref"`
		FrontendArtifactRef string `json:"frontend_artifact_ref"`
		BackendArtifactRef  string `json:"backend_artifact_ref"`
		APIContractRef      string `json:"api_contract_ref"`
		TestEvidenceRef     string `json:"test_evidence_ref"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.CodeRevision = strings.TrimSpace(payload.CodeRevision)
	payload.ArtifactRef = strings.TrimSpace(payload.ArtifactRef)
	payload.FrontendArtifactRef = strings.TrimSpace(payload.FrontendArtifactRef)
	payload.BackendArtifactRef = strings.TrimSpace(payload.BackendArtifactRef)
	payload.APIContractRef = strings.TrimSpace(payload.APIContractRef)
	payload.TestEvidenceRef = strings.TrimSpace(payload.TestEvidenceRef)
	if payload.CodeRevision == "" || payload.ArtifactRef == "" || payload.FrontendArtifactRef == "" || payload.BackendArtifactRef == "" || payload.APIContractRef == "" || payload.TestEvidenceRef == "" {
		return Invalid("build_evidence_missing", "A Feature build must bind one code revision, the complete artifact, separate frontend and backend artifacts, the implemented API contract, and real test evidence.")
	}
	if run.ExecutableRevision == nil {
		return Invalid("product_revision_missing", "RD must record the executable ProductRevision before creating a build.")
	}
	if payload.CodeRevision != run.ExecutableRevision.CodeRevision {
		return Invalid("product_revision_code_mismatch", "The build and executable ProductRevision must come from the same repository revision.")
	}
	included := []string{}
	for _, issue := range run.Issues {
		if issue.Status == IssueReadyRetest {
			included = append(included, issue.ID)
		}
	}
	if len(run.Builds) > 0 && len(included) == 0 {
		return Invalid("build_has_no_changes", "A duplicate build cannot be created without a fix awaiting retest.")
	}
	number := len(run.Builds) + 1
	buildID := fmt.Sprintf("BLD-%03d", number)
	for index := range run.Issues {
		if contains(included, run.Issues[index].ID) {
			run.Issues[index].IncludedInBuildID = buildID
		}
	}
	run.Builds = append(run.Builds, Build{
		ID:                  buildID,
		Number:              uint32(number),
		CodeRevision:        payload.CodeRevision,
		ArtifactRef:         payload.ArtifactRef,
		FrontendArtifactRef: payload.FrontendArtifactRef,
		BackendArtifactRef:  payload.BackendArtifactRef,
		APIContractRef:      payload.APIContractRef,
		TestEvidenceRef:     payload.TestEvidenceRef,
		ProductRevision:     run.ExecutableRevision.TargetRevision,
		ProductRevisionRef:  run.ExecutableRevision.EvidenceRef,
		Status:              BuildGenerated,
		IncludedIssueIDs:    included,
		CreatedAt:           now,
	})
	appendActivity(run, command.Actor, "build_created", buildID+" generated", "The build is bound to its code revision, artifact, and included fixes; it is not deployed yet.", now)
	return nil
}

func deployBuild(run *DeliveryRun, command Command, now time.Time) error {
	var payload struct {
		BuildID        string `json:"build_id"`
		EnvironmentRef string `json:"environment_ref"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	if len(run.Builds) == 0 || run.Builds[len(run.Builds)-1].ID != payload.BuildID {
		return Invalid("build_stale", "Only the latest build can be deployed.")
	}
	build := &run.Builds[len(run.Builds)-1]
	if build.Status != BuildGenerated {
		return Invalid("build_already_deployed", "The build is already deployed to a test environment.")
	}
	if strings.TrimSpace(payload.EnvironmentRef) == "" {
		return Invalid("environment_ref_required", "A deployment must bind a test environment receipt.")
	}
	build.Status = BuildTestDeployed
	build.TestEnvironmentRef = strings.TrimSpace(payload.EnvironmentRef)
	build.DeployedAt = &now
	appendActivity(run, command.Actor, "build_deployed", build.ID+" deployed to the test environment", build.TestEnvironmentRef, now)
	return nil
}

func recordTest(run *DeliveryRun, command Command, now time.Time) error {
	var payload struct {
		TestCaseID   string      `json:"test_case_id"`
		Result       CheckResult `json:"result"`
		Note         string      `json:"note"`
		EvidenceRefs []string    `json:"evidence_refs"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	build, err := currentBuild(run)
	if err != nil {
		return err
	}
	testCase := findTestCase(run, payload.TestCaseID)
	if testCase == nil {
		return NotFound("TestCase", payload.TestCaseID)
	}
	if !validResult(payload.Result, true) || strings.TrimSpace(payload.Note) == "" || len(cleanStrings(payload.EvidenceRefs)) == 0 {
		return Invalid("test_evidence_missing", "A test result requires an outcome, actual observation, and at least one evidence reference.")
	}
	if verificationLockedByRelease(run, build.ID) {
		return Invalid("release_locked", "Test results cannot change after the build enters the release process.")
	}
	run.TestRuns = append(run.TestRuns, TestRun{
		ID:           newID("testrun"),
		BuildID:      build.ID,
		TestCaseID:   testCase.ID,
		Result:       payload.Result,
		Note:         strings.TrimSpace(payload.Note),
		EvidenceRefs: cleanStrings(payload.EvidenceRefs),
		ExecutedBy:   command.Actor.ID,
		ExecutedAt:   now,
	})
	origin := IssueOrigin{Kind: "technical_test", TestCaseID: testCase.ID}
	if payload.Result == ResultPass {
		closeRetestedIssues(run, build.ID, origin, now)
	} else if payload.Result == ResultFail {
		openIssue(run, origin, build.ID, "Test failed: "+testCase.Title, now)
	}
	appendActivity(run, command.Actor, "test_recorded", testCase.ID+" result recorded", strings.TrimSpace(payload.Note), now)
	return nil
}

func completeIssueFix(run *DeliveryRun, command Command, now time.Time) error {
	var payload entityPayload
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	issue := findIssue(run, payload.IssueID)
	if issue == nil {
		return NotFound("Issue", payload.IssueID)
	}
	if issue.Status != IssueFixing || strings.TrimSpace(payload.FixNote) == "" {
		return Invalid("fix_evidence_missing", "Completing an Issue fix requires the change, impact, and self-test result.")
	}
	issue.Status = IssueReadyRetest
	issue.FixNote = strings.TrimSpace(payload.FixNote)
	issue.UpdatedAt = now
	appendActivity(run, command.Actor, "issue_fix_completed", issue.ID+" fix completed; awaiting retest", issue.FixNote, now)
	return nil
}

func transitionIssue(run *DeliveryRun, command Command, expected, next IssueStatus, now time.Time) error {
	var payload entityPayload
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	issue := findIssue(run, payload.IssueID)
	if issue == nil {
		return NotFound("Issue", payload.IssueID)
	}
	if issue.Status != expected {
		return Invalid("issue_transition_invalid", "The current Issue state does not allow this action.")
	}
	issue.Status = next
	issue.UpdatedAt = now
	appendActivity(run, command.Actor, "issue_transitioned", issue.ID+" advanced", issue.Title, now)
	return nil
}

func recordAcceptance(run *DeliveryRun, command Command, now time.Time) error {
	var payload struct {
		AcceptanceCaseID string      `json:"acceptance_case_id"`
		Result           CheckResult `json:"result"`
		Note             string      `json:"note"`
		EvidenceRefs     []string    `json:"evidence_refs"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	build, err := currentBuild(run)
	if err != nil {
		return err
	}
	acceptanceCase := findAcceptanceCase(run, payload.AcceptanceCaseID)
	if acceptanceCase == nil {
		return NotFound("AcceptanceCase", payload.AcceptanceCaseID)
	}
	if payload.Result != ResultPass && payload.Result != ResultFail {
		return Invalid("acceptance_result_invalid", "Business acceptance accepts only pass or fail.")
	}
	if strings.TrimSpace(payload.Note) == "" || len(cleanStrings(payload.EvidenceRefs)) == 0 {
		return Invalid("acceptance_evidence_missing", "Business acceptance requires an actual conclusion and evidence.")
	}
	if verificationLockedByRelease(run, build.ID) {
		return Invalid("release_locked", "Business acceptance cannot change after the build enters the release process.")
	}
	if !allTechnicalTestsPass(run, build.ID) {
		return Invalid("technical_tests_incomplete", "Every technical test for the current build must pass before business acceptance.")
	}
	for _, issue := range run.Issues {
		allowedRetest := issue.Status == IssueReadyRetest && issue.IncludedInBuildID == build.ID && issue.Origin.Kind == "acceptance"
		if issue.Status != IssueClosed && !allowedRetest {
			return Invalid("issues_unresolved", "Business acceptance cannot proceed while Issues remain unresolved.")
		}
	}
	run.AcceptanceResults = append(run.AcceptanceResults, AcceptanceResult{
		ID:               newID("acceptance"),
		BuildID:          build.ID,
		AcceptanceCaseID: acceptanceCase.ID,
		Result:           payload.Result,
		Note:             strings.TrimSpace(payload.Note),
		EvidenceRefs:     cleanStrings(payload.EvidenceRefs),
		AcceptedBy:       command.Actor.ID,
		AcceptedAt:       now,
	})
	origin := IssueOrigin{Kind: "acceptance", AcceptanceCaseID: acceptanceCase.ID}
	if payload.Result == ResultPass {
		closeRetestedIssues(run, build.ID, origin, now)
	} else {
		openIssue(run, origin, build.ID, "Acceptance failed: "+acceptanceCase.Title, now)
	}
	appendActivity(run, command.Actor, "acceptance_recorded", acceptanceCase.ID+" acceptance result recorded", strings.TrimSpace(payload.Note), now)
	return nil
}

func replaceReleaseChecks(run *DeliveryRun, command Command, now time.Time) error {
	if len(run.DeliveryUnits) > 0 && run.Stage != StageRelease {
		return Invalid("release_checks_stage_invalid", "V3 ReleaseChecks can be configured only after business acceptance passes.")
	}
	if (len(run.DeliveryUnits) > 0 && activeV3Release(run)) || (len(run.DeliveryUnits) == 0 && len(run.Releases) > 0) {
		return Invalid("release_checks_locked", "ReleaseChecks cannot be replaced after the release process starts.")
	}
	var payload struct {
		EnvironmentRef string `json:"environment_ref"`
		Checks         []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Required bool   `json:"required"`
		} `json:"checks"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	payload.EnvironmentRef = strings.TrimSpace(payload.EnvironmentRef)
	if payload.EnvironmentRef == "" {
		return Invalid("environment_ref_required", "Release checks must be bound to a target environment.")
	}
	if len(payload.Checks) == 0 {
		return Invalid("release_checks_empty", "At least one ReleaseCheck is required before release.")
	}
	seen := map[string]bool{}
	checks := make([]ReleaseCheck, 0, len(payload.Checks))
	for _, input := range payload.Checks {
		input.ID = strings.TrimSpace(input.ID)
		input.Title = strings.TrimSpace(input.Title)
		if input.ID == "" || input.Title == "" {
			return Invalid("release_check_incomplete", "A ReleaseCheck requires an ID and title.")
		}
		if seen[input.ID] {
			return Invalid("release_check_duplicate", "ReleaseCheck IDs must be unique.")
		}
		seen[input.ID] = true
		checks = append(checks, ReleaseCheck{ID: input.ID, Title: input.Title, EnvironmentRef: payload.EnvironmentRef, Required: input.Required, Status: ReleaseCheckPending, EvidenceRefs: []string{}})
	}
	run.ReleaseChecks = checks
	appendActivity(run, command.Actor, "release_checks_replaced", "Release checks recorded", fmt.Sprintf("%d ReleaseChecks are awaiting execution.", len(checks)), now)
	return nil
}

func recordReleaseCheck(run *DeliveryRun, command Command, now time.Time) error {
	if len(run.DeliveryUnits) > 0 && run.Stage != StageRelease {
		return Invalid("release_checks_stage_invalid", "V3 ReleaseChecks can be recorded only after business acceptance passes.")
	}
	if (len(run.DeliveryUnits) > 0 && activeV3Release(run)) || (len(run.DeliveryUnits) == 0 && len(run.Releases) > 0) {
		return Invalid("release_checks_locked", "ReleaseCheck results cannot change after the release process starts.")
	}
	var payload struct {
		CheckID      string             `json:"check_id"`
		Status       ReleaseCheckStatus `json:"status"`
		Note         string             `json:"note"`
		EvidenceRefs []string           `json:"evidence_refs"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	check := findReleaseCheck(run, strings.TrimSpace(payload.CheckID))
	if check == nil {
		return NotFound("ReleaseCheck", payload.CheckID)
	}
	payload.Note = strings.TrimSpace(payload.Note)
	payload.EvidenceRefs = cleanStrings(payload.EvidenceRefs)
	if payload.Status != ReleaseCheckPassed && payload.Status != ReleaseCheckFailed {
		return Invalid("release_check_status_invalid", "A ReleaseCheck result must be passed or failed.")
	}
	if payload.Note == "" || len(payload.EvidenceRefs) == 0 {
		return Invalid("release_check_evidence_missing", "A ReleaseCheck requires an actual conclusion and evidence.")
	}
	if len(run.DeliveryUnits) > 0 {
		revision, ready := v3VerificationRevision(run)
		if !ready || !evidenceRefsMatchRevision(payload.EvidenceRefs, revision) {
			return Invalid("release_check_evidence_revision_invalid", "A V3 ReleaseCheck requires evidence from the verified Git revision.")
		}
	}
	check.Status = payload.Status
	check.Note = payload.Note
	check.EvidenceRefs = payload.EvidenceRefs
	check.UpdatedBy = command.Actor.ID
	check.UpdatedAt = &now
	appendActivity(run, command.Actor, "release_check_recorded", check.ID+" release check recorded", payload.Note, now)
	return nil
}

func prepareRelease(run *DeliveryRun, command Command, now time.Time) error {
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
	build, err := currentBuild(run)
	if err != nil {
		return err
	}
	if !allTechnicalTestsPass(run, build.ID) || !allAcceptancePass(run, build.ID) {
		return Invalid("release_gate_failed", "All technical tests and business acceptance cases must pass.")
	}
	if run.ExecutableRevision == nil || build.ProductRevision != run.ExecutableRevision.TargetRevision || build.ProductRevisionRef != run.ExecutableRevision.EvidenceRef || build.CodeRevision != run.ExecutableRevision.CodeRevision {
		return Invalid("product_revision_build_mismatch", "The Release build is not bound to the current executable ProductRevision.")
	}
	for _, issue := range run.Issues {
		if issue.Status != IssueClosed {
			return Invalid("release_gate_failed", "Every Issue must be closed before preparing a release.")
		}
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
		if release.BuildID == build.ID && release.Status != ReleaseFailed {
			return Invalid("release_exists", "The current build already has an active Release.")
		}
	}
	releaseID := fmt.Sprintf("REL-%03d", len(run.Releases)+1)
	run.Releases = append(run.Releases, Release{
		ID: releaseID, Version: payload.Version, BuildID: build.ID, CodeRevision: build.CodeRevision,
		ArtifactRef: build.ArtifactRef, ProductRevision: build.ProductRevision, ProductRevisionRef: build.ProductRevisionRef,
		EnvironmentRef: payload.EnvironmentRef, Status: ReleaseDraft, CreatedAt: now,
	})
	appendActivity(run, command.Actor, "release_prepared", releaseID+" prepared", "Every release gate passed; the Release awaits approval.", now)
	return nil
}

func approveRelease(run *DeliveryRun, command Command, now time.Time) error {
	var payload entityPayload
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	release := findRelease(run, payload.ReleaseID)
	if release == nil {
		return NotFound("Release", payload.ReleaseID)
	}
	if release.Status != ReleaseDraft {
		return Invalid("release_not_draft", "Only a draft Release can be approved.")
	}
	if err := ensureReleaseStillValid(run, release); err != nil {
		return err
	}
	release.Status = ReleaseApproved
	release.ApprovedBy = command.Actor.ID
	release.ApprovedAt = &now
	appendActivity(run, command.Actor, "release_approved", release.ID+" approved", "The release owner confirmed the bound build and evidence.", now)
	return nil
}

func recordDeployment(run *DeliveryRun, command Command, now time.Time) error {
	var payload struct {
		ReleaseID      string            `json:"release_id"`
		Outcome        DeploymentOutcome `json:"outcome"`
		EnvironmentRef string            `json:"environment_ref"`
		LaunchURL      string            `json:"launch_url"`
		ReceiptRef     string            `json:"receipt_ref"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	release := findRelease(run, payload.ReleaseID)
	if release == nil {
		return NotFound("Release", payload.ReleaseID)
	}
	if release.Status != ReleaseApproved || release.DeploymentAttempt != nil {
		return Invalid("release_not_deployable", "The Release is not approved or already has a deployment attempt.")
	}
	if err := ensureReleaseStillValid(run, release); err != nil {
		return err
	}
	payload.EnvironmentRef = strings.TrimSpace(payload.EnvironmentRef)
	if !validOutcome(payload.Outcome) || payload.EnvironmentRef == "" || payload.EnvironmentRef != release.EnvironmentRef || strings.TrimSpace(payload.ReceiptRef) == "" {
		return Invalid("deployment_receipt_missing", "A deployment result must match the target environment and include a valid outcome and execution receipt.")
	}
	launchURL, err := normalizeLaunchURL(payload.LaunchURL, payload.Outcome == DeploymentSuccess)
	if err != nil {
		return err
	}
	attempt := &DeploymentAttempt{ID: newID("deploy"), Outcome: payload.Outcome, EnvironmentRef: payload.EnvironmentRef, LaunchURL: launchURL, ReceiptRef: strings.TrimSpace(payload.ReceiptRef), StartedAt: now}
	if payload.Outcome != DeploymentUnknown {
		attempt.ResolvedAt = &now
	}
	release.DeploymentAttempt = attempt
	switch payload.Outcome {
	case DeploymentSuccess:
		release.Status = ReleaseLive
	case DeploymentFailure:
		release.Status = ReleaseFailed
	case DeploymentUnknown:
		release.Status = ReleaseNeedsReconciliation
	}
	appendActivity(run, command.Actor, "release_deployed", release.ID+" deployment outcome: "+string(payload.Outcome), "The deployment receipt is bound to the original attempt.", now)
	return nil
}

func reconcileDeployment(run *DeliveryRun, command Command, now time.Time) error {
	var payload struct {
		ReleaseID  string            `json:"release_id"`
		Outcome    DeploymentOutcome `json:"outcome"`
		LaunchURL  string            `json:"launch_url"`
		ReceiptRef string            `json:"receipt_ref"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	if payload.Outcome != DeploymentSuccess && payload.Outcome != DeploymentFailure {
		return Invalid("reconciliation_unresolved", "Reconciliation requires a definitive success or failure outcome.")
	}
	release := findRelease(run, payload.ReleaseID)
	if release == nil {
		return NotFound("Release", payload.ReleaseID)
	}
	if release.Status != ReleaseNeedsReconciliation || release.DeploymentAttempt == nil {
		return Invalid("release_not_reconcilable", "Only an original deployment attempt with an unknown outcome can be reconciled.")
	}
	launchURL := strings.TrimSpace(payload.LaunchURL)
	if launchURL == "" {
		launchURL = release.DeploymentAttempt.LaunchURL
	}
	normalizedLaunchURL, err := normalizeLaunchURL(launchURL, payload.Outcome == DeploymentSuccess)
	if err != nil {
		return err
	}
	release.DeploymentAttempt.Outcome = payload.Outcome
	release.DeploymentAttempt.LaunchURL = normalizedLaunchURL
	release.DeploymentAttempt.ResolvedAt = &now
	if strings.TrimSpace(payload.ReceiptRef) != "" {
		release.DeploymentAttempt.ReceiptRef = strings.TrimSpace(payload.ReceiptRef)
	}
	if payload.Outcome == DeploymentSuccess {
		release.Status = ReleaseLive
	} else {
		release.Status = ReleaseFailed
	}
	appendActivity(run, command.Actor, "release_reconciled", release.ID+" original deployment attempt reconciled", string(payload.Outcome), now)
	return nil
}

func normalizeLaunchURL(value string, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return "", Invalid("deployment_launch_url_required", "A successful SaaS deployment must include its public launch URL.")
		}
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", Invalid("deployment_launch_url_invalid", "A deployment launch URL must be an absolute HTTP or HTTPS URL.")
	}
	return parsed.String(), nil
}

func DeriveStage(run *DeliveryRun) Stage {
	for _, release := range run.Releases {
		if release.Status == ReleaseLive {
			return StageLive
		}
	}
	if len(run.DeliveryUnits) > 0 {
		return deriveV3Stage(run)
	}
	for _, release := range run.Releases {
		if release.Status == ReleaseDraft || release.Status == ReleaseApproved || release.Status == ReleaseNeedsReconciliation {
			return StageRelease
		}
	}
	if run.WorkPlanRevision == 0 {
		return StageDevelopment
	}
	for _, item := range run.WorkItems {
		if item.Status != WorkMerged {
			return StageDevelopment
		}
	}
	var latestBuildID string
	if len(run.Builds) > 0 {
		latestBuildID = run.Builds[len(run.Builds)-1].ID
	}
	for _, issue := range run.Issues {
		if issue.Status == IssueOpen || issue.Status == IssueFixing || (issue.Status == IssueReadyRetest && issue.IncludedInBuildID != latestBuildID) {
			return StageBugs
		}
	}
	if latestBuildID == "" {
		return StageDevelopment
	}
	if run.Builds[len(run.Builds)-1].Status != BuildTestDeployed || !allTechnicalTestsPass(run, latestBuildID) {
		return StageTesting
	}
	if !allAcceptancePass(run, latestBuildID) {
		return StageAcceptance
	}
	return StageRelease
}

func runIsLive(run *DeliveryRun) bool {
	if run.Stage == StageLive {
		return true
	}
	for _, release := range run.Releases {
		if release.Status == ReleaseLive {
			return true
		}
	}
	return false
}

func verificationLockedByRelease(run *DeliveryRun, buildID string) bool {
	for _, release := range run.Releases {
		if release.BuildID != buildID {
			continue
		}
		if release.Status == ReleaseDraft || release.Status == ReleaseApproved || release.Status == ReleaseNeedsReconciliation || release.Status == ReleaseLive {
			return true
		}
	}
	return false
}

func ensureReleaseStillValid(run *DeliveryRun, release *Release) error {
	if len(run.DeliveryUnits) > 0 {
		return ensureV3ReleaseStillValid(run, release)
	}
	if len(run.Builds) == 0 || run.Builds[len(run.Builds)-1].ID != release.BuildID {
		return Invalid("release_stale", "The Release is bound to a build that is no longer current.")
	}
	build := run.Builds[len(run.Builds)-1]
	if run.ExecutableRevision == nil || release.ProductRevision != run.ExecutableRevision.TargetRevision || release.ProductRevisionRef != run.ExecutableRevision.EvidenceRef || release.CodeRevision != run.ExecutableRevision.CodeRevision || build.ProductRevisionRef != release.ProductRevisionRef {
		return Invalid("product_revision_release_mismatch", "The Release is not bound to the current executable ProductRevision.")
	}
	if !allTechnicalTestsPass(run, release.BuildID) || !allAcceptancePass(run, release.BuildID) {
		return Invalid("release_gate_failed", "Test or business acceptance evidence for the Release build is no longer valid.")
	}
	for _, issue := range run.Issues {
		if issue.Status != IssueClosed {
			return Invalid("release_gate_failed", "The Release build still has an open Issue.")
		}
	}
	if !allRequiredReleaseChecksPass(run) {
		return Invalid("release_checks_incomplete", "ReleaseChecks for the Release are no longer valid.")
	}
	return nil
}

func allRequiredReleaseChecksPass(run *DeliveryRun) bool {
	if len(run.ReleaseChecks) == 0 {
		return false
	}
	for _, check := range run.ReleaseChecks {
		if check.Required && check.Status != ReleaseCheckPassed {
			return false
		}
	}
	return true
}

func authorizeHumanCommand(run *DeliveryRun, command Command) error {
	requiredRole := ""
	switch command.Type {
	case "release.prepare":
		requiredRole = RoleProductOwner
	case "acceptance.record", commandAcceptanceConfirm:
		requiredRole = RoleBusinessAcceptor
	case "release.approve", "release.reconcile":
		requiredRole = RoleReleaseApprover
	default:
		return nil
	}
	for _, member := range run.Members {
		if member.ID == command.Actor.ID {
			for _, role := range member.Roles {
				if role == requiredRole {
					return nil
				}
			}
		}
	}
	return Invalid("actor_unauthorized", "The authenticated actor is not assigned to the required confirmation role.")
}

func AgentContextFor(run DeliveryRun) AgentContext {
	projection := ProjectionFor(run)
	available := make([]AvailableAction, 0)
	commands := make([]string, 0)
	seen := map[string]bool{}
	for _, action := range projection.Workflow.AvailableActions {
		if action.ActorKind != ActorAgent {
			continue
		}
		available = append(available, action)
		if !seen[action.Command] {
			commands = append(commands, action.Command)
			seen[action.Command] = true
		}
	}
	return AgentContext{
		DeliveryRun:          run,
		AllowedAgentCommands: commands,
		AvailableActions:     available,
		HumanOnlyCommands:    []string{"acceptance.record", commandAcceptanceConfirm, "release.prepare", "release.approve", "release.reconcile"},
		SystemOnlyCommands:   []string{"delivery_unit.compile.complete", "delivery_unit.contract.freeze", "delivery_unit.backend.reopen", "delivery_unit.contract.reopen", "delivery_unit.journey.complete", "release.deploy_result"},
		SourcePolicy:         "DeliveryUnits preserve the confirmed Feature discovery and specification, advance only through server-owned phase actions, bind phase results to Git revisions without content hashes, and never fabricate a confirmer.",
	}
}

func currentBuild(run *DeliveryRun) (*Build, error) {
	if len(run.Builds) == 0 {
		return nil, Invalid("build_missing", "There is no verifiable build.")
	}
	build := &run.Builds[len(run.Builds)-1]
	if build.Status != BuildTestDeployed {
		return nil, Invalid("build_not_deployed", "The latest build is not deployed to a test environment.")
	}
	return build, nil
}

func allTechnicalTestsPass(run *DeliveryRun, buildID string) bool {
	if len(run.TestCases) == 0 {
		return false
	}
	for _, testCase := range run.TestCases {
		if latestTestResult(run, buildID, testCase.ID) != ResultPass {
			return false
		}
	}
	return true
}

func allAcceptancePass(run *DeliveryRun, buildID string) bool {
	if len(run.AcceptanceCases) == 0 {
		return false
	}
	for _, acceptanceCase := range run.AcceptanceCases {
		if latestAcceptanceResult(run, buildID, acceptanceCase.ID) != ResultPass {
			return false
		}
	}
	return true
}

func latestTestResult(run *DeliveryRun, buildID, testCaseID string) CheckResult {
	for index := len(run.TestRuns) - 1; index >= 0; index-- {
		run := run.TestRuns[index]
		if run.BuildID == buildID && run.TestCaseID == testCaseID {
			return run.Result
		}
	}
	return ""
}

func latestAcceptanceResult(run *DeliveryRun, buildID, caseID string) CheckResult {
	for index := len(run.AcceptanceResults) - 1; index >= 0; index-- {
		result := run.AcceptanceResults[index]
		if result.BuildID == buildID && result.AcceptanceCaseID == caseID {
			return result.Result
		}
	}
	return ""
}

func openIssue(run *DeliveryRun, origin IssueOrigin, buildID, title string, now time.Time) {
	for index := range run.Issues {
		issue := &run.Issues[index]
		if issue.Status != IssueClosed && sameOrigin(issue.Origin, origin) {
			issue.Status = IssueOpen
			issue.FoundInBuildID = buildID
			issue.IncludedInBuildID = ""
			issue.UpdatedAt = now
			return
		}
	}
	run.Issues = append(run.Issues, Issue{
		ID:             fmt.Sprintf("BUG-%03d", len(run.Issues)+1),
		Title:          title,
		Severity:       "major",
		Status:         IssueOpen,
		OwnerID:        "m-dev",
		Origin:         origin,
		FoundInBuildID: buildID,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
}

func closeRetestedIssues(run *DeliveryRun, buildID string, origin IssueOrigin, now time.Time) {
	for index := range run.Issues {
		issue := &run.Issues[index]
		if issue.Status == IssueReadyRetest && issue.IncludedInBuildID == buildID && sameOrigin(issue.Origin, origin) {
			issue.Status = IssueClosed
			issue.UpdatedAt = now
		}
	}
}

func sameOrigin(left, right IssueOrigin) bool {
	return left.Kind == right.Kind && left.TestCaseID == right.TestCaseID && left.AcceptanceCaseID == right.AcceptanceCaseID
}

func appendActivity(run *DeliveryRun, actor Actor, kind, title, detail string, now time.Time) {
	run.Activity = append(run.Activity, ActivityEvent{ID: newID("event"), Kind: kind, Title: title, Detail: detail, ActorID: actor.ID, OccurredAt: now})
}

func findWork(run *DeliveryRun, id string) *WorkItem {
	for index := range run.WorkItems {
		if run.WorkItems[index].ID == id {
			return &run.WorkItems[index]
		}
	}
	return nil
}

func findIssue(run *DeliveryRun, id string) *Issue {
	for index := range run.Issues {
		if run.Issues[index].ID == id {
			return &run.Issues[index]
		}
	}
	return nil
}

func findRelease(run *DeliveryRun, id string) *Release {
	for index := range run.Releases {
		if run.Releases[index].ID == id {
			return &run.Releases[index]
		}
	}
	return nil
}

func findTestCase(run *DeliveryRun, id string) *TestCase {
	for index := range run.TestCases {
		if run.TestCases[index].ID == id {
			return &run.TestCases[index]
		}
	}
	return nil
}

func findAcceptanceCase(run *DeliveryRun, id string) *AcceptanceCase {
	for index := range run.AcceptanceCases {
		if run.AcceptanceCases[index].ID == id {
			return &run.AcceptanceCases[index]
		}
	}
	return nil
}

func findReleaseCheck(run *DeliveryRun, id string) *ReleaseCheck {
	for index := range run.ReleaseChecks {
		if run.ReleaseChecks[index].ID == id {
			return &run.ReleaseChecks[index]
		}
	}
	return nil
}

func validResult(result CheckResult, blocked bool) bool {
	return result == ResultPass || result == ResultFail || (blocked && result == ResultBlocked)
}

func validOutcome(outcome DeploymentOutcome) bool {
	return outcome == DeploymentSuccess || outcome == DeploymentFailure || outcome == DeploymentUnknown
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func decode(payload json.RawMessage, target any) error {
	if len(payload) == 0 {
		payload = json.RawMessage([]byte("{}"))
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return Invalid("payload_invalid", "The command payload is invalid: "+err.Error())
	}
	return nil
}

func clone[T any](value T) T {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var copied T
	if err := json.Unmarshal(data, &copied); err != nil {
		panic(err)
	}
	return copied
}

func newID(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}
