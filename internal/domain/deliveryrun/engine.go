package deliveryrun

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
)

const (
	commandProductRevisionRecord = commanddomain.ProductRevisionRecord
	commandReleaseChecksReplace  = commanddomain.ReleaseChecksReplace
	commandReleaseCheckRecord    = commanddomain.ReleaseCheckRecord
	commandReleasePrepare        = commanddomain.ReleasePrepare
	commandReleaseApprove        = commanddomain.ReleaseApprove
	commandReleaseDeployResult   = commanddomain.ReleaseDeployResult
	commandReleaseReconcile      = commanddomain.ReleaseReconcile
)

var sha256ValuePattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func Apply(run *DeliveryRun, command Command, now time.Time) error {
	if runIsLive(run) {
		return Invalid("delivery_run_live")
	}
	if err := validateCommandActor(command, CommandTargetDeliveryRun); err != nil {
		return err
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
		run.Stage = DeriveStage(run)
		run.UpdatedAt = now
		return nil
	}

	var err error
	switch command.Type {
	case commandProductRevisionRecord:
		err = recordExecutableRevision(run, command, now)
	case commandQualityRecord:
		err = recordQuality(run, command, now)
	case commandAcceptanceConfirm:
		err = confirmAcceptance(run, command, now)
	case commandReleaseChecksReplace:
		err = replaceReleaseChecks(run, command, now)
	case commandReleaseCheckRecord:
		err = recordReleaseCheck(run, command, now)
	case commandReleasePrepare:
		err = prepareRelease(run, command, now)
	case commandReleaseApprove:
		err = approveRelease(run, command, now)
	case commandReleaseDeployResult:
		err = recordDeployment(run, command, now)
	case commandReleaseReconcile:
		err = reconcileDeployment(run, command, now)
	default:
		err = Invalid("command_unknown")
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
		return Invalid("product_revision_record_not_allowed")
	}
	var payload struct {
		Content      ProductRevisionContent `json:"content"`
		EvidenceRef  string                 `json:"evidence_ref"`
		CodeRevision string                 `json:"code_revision"`
		ModelSHA256  string                 `json:"model_sha256"`
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
	payload.ModelSHA256 = strings.ToLower(strings.TrimSpace(payload.ModelSHA256))
	verifiedRevision, ready := verifiedJourneyRevision(run)
	if !ready || payload.CodeRevision != verifiedRevision || !evidenceRefsMatchRevision([]string{payload.EvidenceRef}, verifiedRevision) || !sha256ValuePattern.MatchString(payload.ModelSHA256) || !deliveryUnitsMatchModelHash(run, payload.ModelSHA256) {
		return Invalid("product_revision_evidence_invalid")
	}
	if err := validateProductRevisionContent(payload.Content, true); err != nil {
		return err
	}
	baseRevision := run.Feature.BaselineProductRevision
	if baseRevision == 0 || baseRevision != run.Product.ProductRevision || run.Feature.Source.ProductRevision != baseRevision {
		return Invalid("product_revision_baseline_invalid")
	}
	run.ExecutableRevision = &ExecutableProductRevision{
		BaseRevision: baseRevision, TargetRevision: baseRevision + 1,
		Content: clone(payload.Content), EvidenceRef: payload.EvidenceRef,
		CodeRevision: payload.CodeRevision, ModelSHA256: payload.ModelSHA256,
		RecordedBy: command.Actor.ID, RecordedAt: now,
	}
	appendActivity(run, command.Actor, "product_revision_recorded", fmt.Sprintf("ProductRevision R%d recorded", baseRevision+1), payload.EvidenceRef, now)
	return nil
}

func deliveryUnitsMatchModelHash(run *DeliveryRun, modelSHA256 string) bool {
	for _, unit := range run.DeliveryUnits {
		if unit.ModelEvidence == nil || strings.ToLower(strings.TrimSpace(unit.ModelEvidence.ModelSHA256)) != modelSHA256 {
			return false
		}
	}
	return len(run.DeliveryUnits) > 0
}

func canRecordExecutableRevision(run *DeliveryRun) bool {
	if run.ExecutableRevision != nil || len(run.Releases) > 0 {
		return false
	}
	_, ready := verifiedJourneyRevision(run)
	return ready
}

func replaceReleaseChecks(run *DeliveryRun, command Command, now time.Time) error {
	if run.Stage != StageRelease {
		return Invalid("release_checks_stage_invalid")
	}
	if activeRelease(run) {
		return Invalid("release_checks_locked")
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
		return Invalid("environment_ref_required")
	}
	if len(payload.Checks) == 0 {
		return Invalid("release_checks_empty")
	}
	seen := map[string]bool{}
	checks := make([]ReleaseCheck, 0, len(payload.Checks))
	for _, input := range payload.Checks {
		input.ID = strings.TrimSpace(input.ID)
		input.Title = strings.TrimSpace(input.Title)
		if input.ID == "" || input.Title == "" {
			return Invalid("release_check_incomplete")
		}
		if seen[input.ID] {
			return Invalid("release_check_duplicate")
		}
		seen[input.ID] = true
		checks = append(checks, ReleaseCheck{ID: input.ID, Title: input.Title, EnvironmentRef: payload.EnvironmentRef, Required: input.Required, Status: ReleaseCheckPending, EvidenceRefs: []string{}})
	}
	run.ReleaseChecks = checks
	appendActivity(run, command.Actor, "release_checks_replaced", "Release checks recorded", fmt.Sprintf("%d ReleaseChecks are awaiting execution.", len(checks)), now)
	return nil
}

func recordReleaseCheck(run *DeliveryRun, command Command, now time.Time) error {
	if run.Stage != StageRelease {
		return Invalid("release_checks_stage_invalid")
	}
	if activeRelease(run) {
		return Invalid("release_checks_locked")
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
		return Invalid("release_check_status_invalid")
	}
	revision, ready := verifiedJourneyRevision(run)
	if payload.Note == "" || !ready || !evidenceRefsMatchRevision(payload.EvidenceRefs, revision) {
		return Invalid("release_check_evidence_missing")
	}
	check.Status, check.Note, check.EvidenceRefs = payload.Status, payload.Note, payload.EvidenceRefs
	check.UpdatedBy, check.UpdatedAt = command.Actor.ID, &now
	appendActivity(run, command.Actor, "release_check_recorded", check.ID+" release check recorded", payload.Note, now)
	return nil
}

func approveRelease(run *DeliveryRun, command Command, now time.Time) error {
	var payload struct {
		ReleaseID string `json:"release_id"`
	}
	if err := decode(command.Payload, &payload); err != nil {
		return err
	}
	release := findRelease(run, payload.ReleaseID)
	if release == nil {
		return NotFound("Release", payload.ReleaseID)
	}
	if release.Status != ReleaseDraft {
		return Invalid("release_not_draft")
	}
	if err := ensureReleaseStillValid(run, release); err != nil {
		return err
	}
	release.Status, release.ApprovedBy, release.ApprovedAt = ReleaseApproved, command.Actor.ID, &now
	appendActivity(run, command.Actor, "release_approved", release.ID+" approved", "The release owner confirmed the exact ProductRevision and evidence.", now)
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
	release := findRelease(run, strings.TrimSpace(payload.ReleaseID))
	if release == nil {
		return NotFound("Release", payload.ReleaseID)
	}
	if release.Status != ReleaseApproved || release.DeploymentAttempt != nil {
		return Invalid("release_not_deployable")
	}
	if err := ensureReleaseStillValid(run, release); err != nil {
		return err
	}
	payload.EnvironmentRef = strings.TrimSpace(payload.EnvironmentRef)
	if !validOutcome(payload.Outcome) || payload.EnvironmentRef == "" || payload.EnvironmentRef != release.EnvironmentRef || strings.TrimSpace(payload.ReceiptRef) == "" {
		return Invalid("deployment_receipt_missing")
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
		return Invalid("reconciliation_unresolved")
	}
	release := findRelease(run, strings.TrimSpace(payload.ReleaseID))
	if release == nil {
		return NotFound("Release", payload.ReleaseID)
	}
	if release.Status != ReleaseNeedsReconciliation || release.DeploymentAttempt == nil {
		return Invalid("release_not_reconcilable")
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
			return "", Invalid("deployment_launch_url_required")
		}
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", Invalid("deployment_launch_url_invalid")
	}
	return parsed.String(), nil
}

func DeriveStage(run *DeliveryRun) Stage {
	for _, release := range run.Releases {
		if release.Status == ReleaseLive {
			return StageLive
		}
	}
	return deriveStage(run)
}

func runIsLive(run *DeliveryRun) bool {
	return DeriveStage(run) == StageLive
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
	case commandReleasePrepare:
		requiredRole = RoleProductOwner
	case commandAcceptanceConfirm:
		requiredRole = RoleBusinessAcceptor
	case commandReleaseApprove, commandReleaseReconcile:
		requiredRole = RoleReleaseApprover
	default:
		return nil
	}
	for _, member := range run.Members {
		if member.ID == command.Actor.ID && contains(member.Roles, requiredRole) {
			return nil
		}
	}
	return Invalid("actor_unauthorized")
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
		DeliveryRun: run, AllowedAgentCommands: commands, AvailableActions: available,
		HumanOnlyCommands:  commanddomain.KeysFor(commanddomain.TargetDeliveryRun, ActorHuman),
		SystemOnlyCommands: commanddomain.KeysFor(commanddomain.TargetDeliveryRun, ActorSystem),
		SourcePolicy:       "DeliveryUnits preserve the confirmed Feature source, validate backend/model.json through Runtime, record typed Plane guide evidence, and bind ProductRevision, QA, acceptance, release, and deployment to one exact Git revision.",
	}
}

func appendActivity(run *DeliveryRun, actor Actor, kind, title, detail string, now time.Time) {
	run.Activity = append(run.Activity, ActivityEvent{ID: newID("event"), Kind: kind, Title: title, Detail: detail, ActorID: actor.ID, OccurredAt: now})
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
