package module

import (
	"context"

	deliverysdk "github.com/domainry/domainry-delivery-sdk"
	"github.com/domainry/domainry-delivery/internal/application"
)

func (binding *Binding) AppendFeatureMessage(ctx context.Context, workspaceID, productID, featureID string, input deliverysdk.FeatureMessageAppend) (deliverysdk.FeatureMessage, error) {
	value, err := binding.service.AppendFeatureMessage(ctx, workspaceID, productID, featureID, application.AppendFeatureMessage(input))
	return convert[deliverysdk.FeatureMessage](ctx, value, err)
}

func (binding *Binding) ListFeatureMessages(ctx context.Context, workspaceID, productID, featureID, cursor string, limit int) (deliverysdk.FeatureMessagesPage, error) {
	value, err := binding.service.ListFeatureMessages(ctx, workspaceID, productID, featureID, cursor, limit)
	return convert[deliverysdk.FeatureMessagesPage](ctx, value, err)
}

func (binding *Binding) UploadFeatureAttachment(ctx context.Context, workspaceID, productID, featureID string, input deliverysdk.FeatureAttachmentUpload) (deliverysdk.FeatureAttachment, error) {
	value, err := binding.service.UploadFeatureAttachment(ctx, workspaceID, productID, featureID, application.UploadFeatureAttachment(input))
	return convert[deliverysdk.FeatureAttachment](ctx, value, err)
}

func (binding *Binding) ListFeatureAttachments(ctx context.Context, workspaceID, productID, featureID string) ([]deliverysdk.FeatureAttachment, error) {
	value, err := binding.service.ListFeatureAttachments(ctx, workspaceID, productID, featureID)
	return convert[[]deliverysdk.FeatureAttachment](ctx, value, err)
}

func (binding *Binding) DownloadFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string) (deliverysdk.FeatureAttachmentDownload, error) {
	value, err := binding.service.DownloadFeatureAttachment(ctx, workspaceID, productID, featureID, attachmentID)
	return convert[deliverysdk.FeatureAttachmentDownload](ctx, value, err)
}

func (binding *Binding) RemoveFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string, input deliverysdk.FeatureAttachmentRemove) (deliverysdk.FeatureAttachment, error) {
	value, err := binding.service.RemoveFeatureAttachment(ctx, workspaceID, productID, featureID, attachmentID, input.ClientID, input.DeviceID, input.ExpectedRevision)
	return convert[deliverysdk.FeatureAttachment](ctx, value, err)
}
