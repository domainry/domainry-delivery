package deliveryrun

import (
	"fmt"
	"strings"
)

// ProjectionFor is the only source of workflow actions consumed by clients.
func ProjectionFor(run DeliveryRun) Projection {
	workflow := Workflow{AvailableActions: []AvailableAction{}, ReleaseGates: deliveryUnitReleaseGates(&run)}
	if runIsLive(&run) {
		return Projection{DeliveryRun: run, Workflow: workflow}
	}
	if activeDeliveryUnit(&run) != nil {
		workflow.AvailableActions, _ = deliveryUnitActions(&run)
		return Projection{DeliveryRun: run, Workflow: workflow}
	}
	if _, ready := verifiedJourneyRevision(&run); ready && run.ExecutableRevision == nil {
		workflow.AvailableActions = []AvailableAction{catalogAction(commandProductRevisionRecord, "")}
		return Projection{DeliveryRun: run, Workflow: workflow}
	}
	workflow.AvailableActions = lifecycleActions(&run)
	return Projection{DeliveryRun: run, Workflow: workflow}
}

func releaseCheckDetail(run *DeliveryRun) string {
	passed := 0
	for _, check := range run.ReleaseChecks {
		if check.Status == ReleaseCheckPassed {
			passed++
		}
	}
	if len(run.ReleaseChecks) == 0 {
		return "0 / 0"
	}
	return fmt.Sprintf("%d / %d · %s", passed, len(run.ReleaseChecks), strings.TrimSpace(run.ReleaseChecks[0].EnvironmentRef))
}

func allReleaseGatesPass(gates []ReleaseGate) bool {
	if len(gates) == 0 {
		return false
	}
	for _, gate := range gates {
		if !gate.OK {
			return false
		}
	}
	return true
}
