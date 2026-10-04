package deliveryrun

// ModelRepairImpact binds a System Runner source comparison to both immutable
// model receipts. It does not replace the final contract and journey gates.
type ModelRepairImpact struct {
	BaselineGitRevision string `json:"baseline_git_revision"`
	BaselineModelSHA256 string `json:"baseline_model_sha256"`
	CurrentGitRevision  string `json:"current_git_revision"`
	CurrentModelSHA256  string `json:"current_model_sha256"`
	SourceImpact        string `json:"source_impact"`
}

func validateModelRepairImpact(unit *DeliveryUnit, command string, payload deliveryUnitResult) error {
	impact := payload.ModelRepairImpact
	if command != commandModelVerify || unit.ModelEvidence == nil {
		if impact != nil {
			return Invalid("model_repair_impact_unexpected")
		}
		return nil
	}
	if impact == nil || payload.BackendGuide == nil {
		return Invalid("model_repair_impact_required")
	}
	if impact.BaselineGitRevision != unit.ModelGitRevision ||
		impact.BaselineModelSHA256 != unit.ModelEvidence.ModelSHA256 ||
		impact.CurrentGitRevision != payload.GitRevision ||
		impact.CurrentModelSHA256 != payload.BackendGuide.ModelSHA256 {
		return Invalid("model_repair_impact_revision_conflict")
	}
	switch impact.SourceImpact {
	case "unchanged", "permissions_only", "implementation_contract":
		return nil
	default:
		return Invalid("model_repair_impact_invalid")
	}
}
