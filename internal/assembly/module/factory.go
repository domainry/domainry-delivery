package module

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	deliverysdk "github.com/domainry/domainry-delivery-sdk"
	"github.com/domainry/domainry-delivery-sdk/modulehost"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/domain/product"
	deliverydb "github.com/domainry/domainry-delivery/internal/infrastructure/persistence/database"
	"github.com/domainry/domainry-delivery/internal/presentation"
)

type Factory struct{}

func NewFactory() *Factory { return &Factory{} }

func (factory *Factory) OpenModule(ctx context.Context, reference deliverysdk.ApplicationRef, host modulehost.Host) (deliverysdk.Binding, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	if !modulehost.Validate(host) {
		return nil, fmt.Errorf("Delivery Module requires runtime identity, database, dialect, and migration registrar")
	}
	if strings.TrimSpace(host.RuntimeID()) != strings.TrimSpace(reference.RuntimeID) {
		return nil, fmt.Errorf("Delivery ApplicationRef does not match the host runtime audience")
	}
	driver := strings.ToLower(strings.TrimSpace(host.Migrations().Driver()))
	if driver != "sqlite" && driver != "mysql" {
		return nil, fmt.Errorf("unsupported Delivery database driver %q", driver)
	}
	store, err := deliverydb.NewBorrowed(ctx, host.Database(), driver, host.Dialect(), host.Migrations())
	if err != nil {
		return nil, err
	}
	return &Binding{runtimeID: reference.RuntimeID, store: store, service: application.NewService(application.Ports{
		Products: store, Runs: store, Lifecycle: store,
	})}, nil
}

type Binding struct {
	runtimeID string
	store     *deliverydb.Store
	service   *application.Service
}

func (binding *Binding) Descriptor() deliverysdk.Descriptor {
	return deliverysdk.NewDescriptor(deliverysdk.DeploymentModeModule, binding.runtimeID, presentation.SupportedLocales)
}

func (binding *Binding) Session(ctx context.Context, workspaceID string) (deliverysdk.Session, error) {
	value, err := binding.service.Session(ctx, workspaceID)
	return convert[deliverysdk.Session](ctx, value, err)
}

func (binding *Binding) ListProducts(ctx context.Context, workspaceID string) ([]deliverysdk.ProductProjection, error) {
	values, err := binding.service.ListProducts(ctx, workspaceID)
	if err != nil {
		return nil, toSDKError(ctx, err)
	}
	projections := make([]product.ProductProjection, 0, len(values))
	for _, value := range values {
		projections = append(projections, product.ProductProjectionFor(value))
	}
	return convert[[]deliverysdk.ProductProjection](ctx, projections, nil)
}

func (binding *Binding) GetProduct(ctx context.Context, workspaceID, productID string) (deliverysdk.ProductProjection, error) {
	value, err := binding.service.GetProduct(ctx, workspaceID, productID)
	if err != nil {
		return deliverysdk.ProductProjection{}, toSDKError(ctx, err)
	}
	return convert[deliverysdk.ProductProjection](ctx, product.ProductProjectionFor(value), nil)
}

func (binding *Binding) ProductAgentContext(ctx context.Context, workspaceID, productID string) (deliverysdk.ProductAgentContext, error) {
	value, err := binding.service.ProductAgentContext(ctx, workspaceID, productID)
	return convert[deliverysdk.ProductAgentContext](ctx, value, err)
}

func (binding *Binding) DispatchProduct(ctx context.Context, workspaceID, productID string, command deliverysdk.Command) (deliverysdk.ProductProjection, error) {
	value, err := binding.service.DispatchProduct(ctx, workspaceID, productID, domainCommand(command))
	if err != nil {
		return deliverysdk.ProductProjection{}, toSDKError(ctx, err)
	}
	return convert[deliverysdk.ProductProjection](ctx, product.ProductProjectionFor(value), nil)
}

func (binding *Binding) StartDelivery(ctx context.Context, workspaceID, productID, deliveryRunID string, command deliverysdk.Command) (deliverysdk.DeliveryStartResult, error) {
	productState, run, err := binding.service.StartDelivery(ctx, workspaceID, productID, deliveryRunID, domainCommand(command))
	if err != nil {
		return deliverysdk.DeliveryStartResult{}, toSDKError(ctx, err)
	}
	return convert[deliverysdk.DeliveryStartResult](ctx, struct {
		Product     product.ProductProjection `json:"product"`
		DeliveryRun deliveryrun.Projection    `json:"delivery_run"`
	}{Product: product.ProductProjectionFor(productState), DeliveryRun: presentation.ProjectionForLocale(run, deliverysdk.LocaleFromContext(ctx))}, nil)
}

func (binding *Binding) ListDeliveryRuns(ctx context.Context, workspaceID, productID string) ([]deliverysdk.DeliveryRunProjection, error) {
	values, err := binding.service.ListDeliveryRuns(ctx, workspaceID, productID)
	if err != nil {
		return nil, toSDKError(ctx, err)
	}
	projections := make([]deliveryrun.Projection, 0, len(values))
	for _, value := range values {
		projections = append(projections, presentation.ProjectionForLocale(value, deliverysdk.LocaleFromContext(ctx)))
	}
	return convert[[]deliverysdk.DeliveryRunProjection](ctx, projections, nil)
}

func (binding *Binding) GetDeliveryRun(ctx context.Context, workspaceID, deliveryRunID string) (deliverysdk.DeliveryRunProjection, error) {
	value, err := binding.service.Get(ctx, workspaceID, deliveryRunID)
	if err != nil {
		return deliverysdk.DeliveryRunProjection{}, toSDKError(ctx, err)
	}
	return convert[deliverysdk.DeliveryRunProjection](ctx, presentation.ProjectionForLocale(value, deliverysdk.LocaleFromContext(ctx)), nil)
}

func (binding *Binding) DeliveryRunAgentContext(ctx context.Context, workspaceID, deliveryRunID string) (deliverysdk.DeliveryRunAgentContext, error) {
	value, err := binding.service.AgentContext(ctx, workspaceID, deliveryRunID)
	return convert[deliverysdk.DeliveryRunAgentContext](ctx, value, err)
}

func (binding *Binding) DispatchDeliveryRun(ctx context.Context, workspaceID, deliveryRunID string, command deliverysdk.Command) (deliverysdk.DeliveryRunProjection, error) {
	value, err := binding.service.Dispatch(ctx, workspaceID, deliveryRunID, domainCommand(command))
	if err != nil {
		return deliverysdk.DeliveryRunProjection{}, toSDKError(ctx, err)
	}
	return convert[deliverysdk.DeliveryRunProjection](ctx, presentation.ProjectionForLocale(value, deliverysdk.LocaleFromContext(ctx)), nil)
}

func (binding *Binding) Close(context.Context) error { return nil }

func domainCommand(value deliverysdk.Command) domain.Command {
	return domain.Command{ClientID: value.ClientID, ExpectedRevision: value.ExpectedRevision, Type: value.Type, Payload: append(json.RawMessage(nil), value.Payload...)}
}

func convert[T any](ctx context.Context, value any, sourceError error) (T, error) {
	var zero T
	if sourceError != nil {
		return zero, toSDKError(ctx, sourceError)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return zero, err
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return zero, err
	}
	return result, nil
}

func toSDKError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var domainError *domain.Error
	if !errors.As(err, &domainError) {
		return err
	}
	localized := presentation.LocalizeError(domainError, deliverysdk.LocaleFromContext(ctx))
	return &deliverysdk.Error{Code: localized.Code, Message: localized.Message, Details: localized.Details}
}

var _ deliverysdk.Factory = (*Factory)(nil)
var _ deliverysdk.Binding = (*Binding)(nil)
