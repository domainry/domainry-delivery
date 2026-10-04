package deliveryrun

import (
	"strings"
	"testing"
)

func TestModelRepairImpactRejectsMissingOrMismatchedReceipts(t *testing.T) {
	unit := DeliveryUnit{ModelGitRevision: strings.Repeat("a", 40), ModelEvidence: &BackendGuideEvidence{ModelSHA256: strings.Repeat("b", 64)}}
	payload := deliveryUnitResult{GitRevision: strings.Repeat("c", 40), BackendGuide: &BackendGuideEvidence{ModelSHA256: strings.Repeat("d", 64)}}
	valid := ModelRepairImpact{BaselineGitRevision: unit.ModelGitRevision, BaselineModelSHA256: unit.ModelEvidence.ModelSHA256, CurrentGitRevision: payload.GitRevision, CurrentModelSHA256: payload.BackendGuide.ModelSHA256, SourceImpact: "permissions_only"}
	for _, field := range []string{"missing", "baseline_revision", "baseline_hash", "current_revision", "current_hash", "scope"} {
		t.Run(field, func(t *testing.T) {
			impact := valid
			candidate := payload
			candidate.ModelRepairImpact = &impact
			switch field {
			case "missing":
				candidate.ModelRepairImpact = nil
			case "baseline_revision":
				impact.BaselineGitRevision = strings.Repeat("e", 40)
			case "baseline_hash":
				impact.BaselineModelSHA256 = strings.Repeat("e", 64)
			case "current_revision":
				impact.CurrentGitRevision = strings.Repeat("e", 40)
			case "current_hash":
				impact.CurrentModelSHA256 = strings.Repeat("e", 64)
			case "scope":
				impact.SourceImpact = "agent_says_safe"
			}
			if err := validateModelRepairImpact(&unit, commandModelVerify, candidate); err == nil {
				t.Fatal("accepted unbound model repair impact")
			}
		})
	}
	payload.ModelRepairImpact = &valid
	if err := validateModelRepairImpact(&unit, commandModelVerify, payload); err != nil {
		t.Fatal(err)
	}
	if err := validateModelRepairImpact(&unit, commandContractVerify, payload); err == nil {
		t.Fatal("accepted model comparison on another gate")
	}
}
