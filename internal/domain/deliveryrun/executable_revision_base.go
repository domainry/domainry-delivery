package deliveryrun

// executableRevisionBase preserves the frozen Feature baseline while allocating
// source repairs after an installed release a fresh immutable ProductRevision.
// Successful deployment attempts are committed atomically with installation.
func executableRevisionBase(run *DeliveryRun, codeRevision string) uint64 {
	var installed *Release
	for index := range run.Releases {
		release := &run.Releases[index]
		for _, attempt := range release.DeploymentAttempts {
			if attempt.Outcome == DeploymentSuccess && attempt.ResolvedAt != nil && attempt.ReceiptRef != "" && attempt.LaunchURL != "" &&
				(installed == nil || release.ProductRevision >= installed.ProductRevision) {
				installed = release
			}
		}
	}
	if installed == nil || installed.ProductRevision <= run.Feature.BaselineProductRevision {
		return run.Feature.BaselineProductRevision
	}
	if installed.CodeRevision == codeRevision {
		return installed.ProductRevision - 1
	}
	return installed.ProductRevision
}
