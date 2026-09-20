// Package module provides the in-process Delivery Binding.
package module

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	deliverysdk "github.com/domainry/domainry-delivery"
	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain/delivery"
	"github.com/domainry/domainry-delivery/internal/infrastructure/sqlite"
)

type Host interface {
	Database() *sql.DB
}

type Binding struct {
	store   *sqlite.Store
	service *application.Service
}

func Open(ctx context.Context, host Host) (*Binding, error) {
	if host == nil || host.Database() == nil {
		return nil, fmt.Errorf("Delivery Module requires a host-owned database")
	}
	store, err := sqlite.OpenBorrowed(ctx, host.Database())
	if err != nil {
		return nil, err
	}
	return &Binding{store: store, service: application.NewService(store)}, nil
}

func (binding *Binding) Descriptor() deliverysdk.Descriptor { return deliverysdk.ModuleDescriptor() }

func (binding *Binding) Session(ctx context.Context, workspaceID string) (deliverysdk.Session, error) {
	session, err := binding.service.Session(ctx, workspaceID)
	return session, localizedError(ctx, err)
}

func (binding *Binding) ListProducts(ctx context.Context, workspaceID string) ([]deliverysdk.ProductProjection, error) {
	products, err := binding.service.ListProducts(ctx, workspaceID)
	if err != nil {
		return nil, localizedError(ctx, err)
	}
	result := make([]deliverysdk.ProductProjection, 0, len(products))
	for _, product := range products {
		result = append(result, delivery.ProductProjectionFor(product))
	}
	return result, nil
}

func (binding *Binding) GetProduct(ctx context.Context, workspaceID, productID string) (deliverysdk.ProductProjection, error) {
	product, err := binding.service.GetProduct(ctx, workspaceID, productID)
	if err != nil {
		return deliverysdk.ProductProjection{}, localizedError(ctx, err)
	}
	return delivery.ProductProjectionFor(product), nil
}

func (binding *Binding) ProductAgentContext(ctx context.Context, workspaceID, productID string) (deliverysdk.ProductAgentContext, error) {
	agentContext, err := binding.service.ProductAgentContext(ctx, workspaceID, productID)
	return agentContext, localizedError(ctx, err)
}

func (binding *Binding) DispatchProduct(ctx context.Context, workspaceID, productID string, command deliverysdk.Command) (deliverysdk.ProductProjection, error) {
	product, err := binding.service.DispatchProduct(ctx, workspaceID, productID, command)
	if err != nil {
		return deliverysdk.ProductProjection{}, localizedError(ctx, err)
	}
	return delivery.ProductProjectionFor(product), nil
}

func (binding *Binding) UploadFeatureAttachment(ctx context.Context, workspaceID, productID, featureID string, upload deliverysdk.FeatureAttachmentUpload) (deliverysdk.FeatureAttachmentUploadResult, error) {
	product, attachment, err := binding.service.UploadFeatureAttachment(
		ctx, workspaceID, productID, featureID, upload.AttachmentID, upload.FileName, upload.MediaType, upload.ExpectedRevision, upload.Content,
	)
	if err != nil {
		return deliverysdk.FeatureAttachmentUploadResult{}, localizedError(ctx, err)
	}
	return deliverysdk.FeatureAttachmentUploadResult{Product: delivery.ProductProjectionFor(product), Attachment: attachment}, nil
}

func (binding *Binding) DownloadFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string) (deliverysdk.FeatureAttachmentContent, error) {
	result, err := binding.service.DownloadFeatureAttachment(ctx, workspaceID, productID, featureID, attachmentID)
	return result, localizedError(ctx, err)
}

func (binding *Binding) RemoveFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string, expectedRevision uint64) (deliverysdk.ProductProjection, error) {
	product, err := binding.service.RemoveFeatureAttachment(ctx, workspaceID, productID, featureID, attachmentID, expectedRevision)
	if err != nil {
		return deliverysdk.ProductProjection{}, localizedError(ctx, err)
	}
	return delivery.ProductProjectionFor(product), nil
}

func (binding *Binding) StartDelivery(ctx context.Context, workspaceID, productID, deliveryRunID string, command deliverysdk.Command) (deliverysdk.DeliveryStartResult, error) {
	product, run, err := binding.service.StartDelivery(ctx, workspaceID, productID, deliveryRunID, command)
	if err != nil {
		return deliverysdk.DeliveryStartResult{}, localizedError(ctx, err)
	}
	return deliverysdk.DeliveryStartResult{Product: delivery.ProductProjectionFor(product), DeliveryRun: delivery.ProjectionForLocale(run, deliverysdk.LocaleFromContext(ctx))}, nil
}

func (binding *Binding) ListDeliveryRuns(ctx context.Context, workspaceID, productID string) ([]deliverysdk.DeliveryRunProjection, error) {
	runs, err := binding.service.ListDeliveryRuns(ctx, workspaceID, productID)
	if err != nil {
		return nil, localizedError(ctx, err)
	}
	result := make([]deliverysdk.DeliveryRunProjection, 0, len(runs))
	for _, run := range runs {
		result = append(result, delivery.ProjectionForLocale(run, deliverysdk.LocaleFromContext(ctx)))
	}
	return result, nil
}

func (binding *Binding) GetDeliveryRun(ctx context.Context, workspaceID, deliveryRunID string) (deliverysdk.DeliveryRunProjection, error) {
	run, err := binding.service.Get(ctx, workspaceID, deliveryRunID)
	if err != nil {
		return deliverysdk.DeliveryRunProjection{}, localizedError(ctx, err)
	}
	return delivery.ProjectionForLocale(run, deliverysdk.LocaleFromContext(ctx)), nil
}

func (binding *Binding) DeliveryRunAgentContext(ctx context.Context, workspaceID, deliveryRunID string) (deliverysdk.DeliveryRunAgentContext, error) {
	agentContext, err := binding.service.AgentContext(ctx, workspaceID, deliveryRunID)
	return agentContext, localizedError(ctx, err)
}

func (binding *Binding) DispatchDeliveryRun(ctx context.Context, workspaceID, deliveryRunID string, command deliverysdk.Command) (deliverysdk.DeliveryRunProjection, error) {
	run, err := binding.service.Dispatch(ctx, workspaceID, deliveryRunID, command)
	if err != nil {
		return deliverysdk.DeliveryRunProjection{}, localizedError(ctx, err)
	}
	return delivery.ProjectionForLocale(run, deliverysdk.LocaleFromContext(ctx)), nil
}

func (binding *Binding) Close(context.Context) error { return binding.store.Close() }

func localizedError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var domainError *delivery.Error
	if !errors.As(err, &domainError) {
		return err
	}
	return delivery.LocalizeError(domainError, deliverysdk.LocaleFromContext(ctx))
}

var _ deliverysdk.Binding = (*Binding)(nil)
