package deliveryrun

func acceptanceReviewRequiresRenewal(run *DeliveryRun) bool {
	review := run.AcceptanceReview
	if review == nil || review.Status != AcceptanceReviewAccepted || activeDeliveryUnit(run) != nil {
		return false
	}
	revision, ready := verifiedJourneyRevision(run)
	return ready && run.ExecutableRevision != nil && run.ExecutableRevision.CodeRevision == revision && qualityReviewComplete(run, revision) && !acceptanceReviewCurrent(run)
}
