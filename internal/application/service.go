package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain"
	commanddomain "github.com/domainry/domainry-delivery/internal/domain/command"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/domain/lifecycle"
	"github.com/domainry/domainry-delivery/internal/domain/product"
)

type Service struct {
	ports Ports
	now   func() time.Time
}

func NewService(ports Ports) *Service {
	return &Service{ports: ports, now: func() time.Time { return time.Now().UTC() }}
}

func (service *Service) Session(ctx context.Context, workspaceID string) (domain.Session, error) {
	return authenticatedSession(ctx, workspaceID)
}

func (service *Service) Get(ctx context.Context, workspaceID, deliveryRunID string) (deliveryrun.DeliveryRun, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionDeliveryRunRead, nil); err != nil {
		return deliveryrun.DeliveryRun{}, err
	}
	return service.ports.Runs.Get(ctx, workspaceID, deliveryRunID)
}

func (service *Service) ListDeliveryRuns(ctx context.Context, workspaceID, productID string) ([]deliveryrun.DeliveryRun, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionDeliveryRunRead, nil); err != nil {
		return nil, err
	}
	return service.ports.Runs.ListDeliveryRuns(ctx, workspaceID, productID)
}

func (service *Service) ListDeliveryRunSummaries(ctx context.Context, workspaceID, productID string) ([]deliveryrun.Summary, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionDeliveryRunRead, nil); err != nil {
		return nil, err
	}
	return service.ports.Runs.ListDeliveryRunSummaries(ctx, workspaceID, productID)
}

func (service *Service) AgentContext(ctx context.Context, workspaceID, deliveryRunID string) (deliveryrun.AgentContext, error) {
	run, err := service.Get(ctx, workspaceID, deliveryRunID)
	if err != nil {
		return deliveryrun.AgentContext{}, err
	}
	return deliveryrun.AgentContextFor(run), nil
}

func (service *Service) Dispatch(ctx context.Context, workspaceID, deliveryRunID string, command domain.Command) (deliveryrun.DeliveryRun, error) {
	return executeMutation(ctx, service, commandScope{
		Target: commanddomain.TargetDeliveryRun, WorkspaceID: workspaceID,
		FingerprintIdentity: []string{workspaceID, "delivery_run", deliveryRunID},
	}, command, nil, func(command domain.Command, mutation Mutation, now time.Time) (deliveryrun.DeliveryRun, error) {
		if command.Type == commanddomain.AcceptanceBugReport {
			if err := service.validateAcceptanceBugAttachment(ctx, workspaceID, deliveryRunID, command.Payload); err != nil {
				return deliveryrun.DeliveryRun{}, err
			}
		}
		return service.ports.Runs.Transact(
			ctx, workspaceID, deliveryRunID, mutation, command.ExpectedRevision,
			func(run *deliveryrun.DeliveryRun) error { return deliveryrun.Apply(run, command, now) },
			func(productState *product.Product, run *deliveryrun.DeliveryRun) error {
				if command.Type == commanddomain.ReleaseDeployResult {
					if removed, err := lifecycle.RecordDeploymentRemoval(productState, run, command, now); err != nil || removed {
						return err
					}
				}
				if run.Stage != deliveryrun.StageLive {
					return nil
				}
				return lifecycle.InstallDeliveryRun(productState, run, command.Actor, now)
			},
		)
	})
}

func (service *Service) validateAcceptanceBugAttachment(ctx context.Context, workspaceID, deliveryRunID string, payload json.RawMessage) error {
	var input struct {
		EvidenceRefs []string `json:"evidence_refs"`
	}
	if json.Unmarshal(payload, &input) != nil || len(input.EvidenceRefs) != 1 {
		return domain.Invalid("acceptance_bug_attachment_invalid")
	}
	attachmentID, ok := strings.CutPrefix(strings.TrimSpace(input.EvidenceRefs[0]), "attachment://")
	if !ok || attachmentID == "" {
		return domain.Invalid("acceptance_bug_attachment_invalid")
	}
	run, err := service.ports.Runs.Get(ctx, workspaceID, deliveryRunID)
	if err != nil {
		return err
	}
	metadata, err := service.ports.AttachmentRecords.GetAttachment(ctx, workspaceID, run.Product.ID, acceptanceAttachmentScope(deliveryRunID), attachmentID)
	if err != nil {
		return err
	}
	if !metadata.Active || metadata.ContentType != "image/png" || metadata.Bytes <= 0 || metadata.Bytes > 5<<20 {
		return domain.Invalid("acceptance_bug_attachment_invalid")
	}
	return nil
}

func (service *Service) GetProduct(ctx context.Context, workspaceID, productID string) (product.Product, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductRead, nil); err != nil {
		return product.Product{}, err
	}
	productState, err := service.ports.Products.GetProduct(ctx, workspaceID, productID)
	if err != nil {
		return product.Product{}, err
	}
	if productState.Status == product.ProductArchived {
		return product.Product{}, domain.NotFound("product", productID)
	}
	return productState, nil
}

func (service *Service) ListProducts(ctx context.Context, workspaceID string) ([]product.Product, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductRead, nil); err != nil {
		return nil, err
	}
	products, err := service.ports.Products.ListProducts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	visible := make([]product.Product, 0, len(products))
	for _, candidate := range products {
		if candidate.Status != product.ProductArchived {
			visible = append(visible, candidate)
		}
	}
	return visible, nil
}

func (service *Service) ListArchivedProducts(ctx context.Context, workspaceID string) ([]product.Product, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductRead, nil); err != nil {
		return nil, err
	}
	products, err := service.ports.Products.ListProducts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	archived := make([]product.Product, 0)
	for _, candidate := range products {
		if candidate.Status == product.ProductArchived {
			archived = append(archived, candidate)
		}
	}
	return archived, nil
}

func (service *Service) ProductAgentContext(ctx context.Context, workspaceID, productID string) (product.ProductAgentContext, error) {
	productState, err := service.GetProduct(ctx, workspaceID, productID)
	if err != nil {
		return product.ProductAgentContext{}, err
	}
	return product.ProductAgentContextFor(productState), nil
}

func (service *Service) DispatchProduct(ctx context.Context, workspaceID, productID string, command domain.Command) (product.Product, error) {
	return executeMutation(ctx, service, commandScope{
		Target: commanddomain.TargetProduct, WorkspaceID: workspaceID,
		FingerprintIdentity: []string{workspaceID, "product", productID},
	}, command, func(command domain.Command) error {
		return service.verifyFeatureSources(ctx, workspaceID, productID, command)
	}, func(command domain.Command, mutation Mutation, now time.Time) (product.Product, error) {
		if command.Type == commanddomain.ProductCreate {
			return service.ports.Products.CreateProduct(ctx, workspaceID, productID, mutation, command.ExpectedRevision, func() (product.Product, error) {
				return product.NewProduct(workspaceID, productID, command, now)
			})
		}
		if command.Type == commanddomain.FeatureDeliveryStart {
			return product.Product{}, domain.Invalid("command_endpoint_invalid")
		}
		return service.ports.Products.TransactProduct(ctx, workspaceID, productID, mutation, command.ExpectedRevision, func(productState *product.Product) error {
			return product.ApplyProduct(productState, command, now)
		})
	})
}

type deliveryStartResult struct {
	Product product.Product
	Run     deliveryrun.DeliveryRun
}

func (service *Service) StartDelivery(ctx context.Context, workspaceID, productID, deliveryRunID string, command domain.Command) (product.Product, deliveryrun.DeliveryRun, error) {
	result, err := executeMutation(ctx, service, commandScope{
		Target: commanddomain.TargetProduct, WorkspaceID: workspaceID,
		FingerprintIdentity: []string{workspaceID, "product", productID, "delivery_run", deliveryRunID},
	}, command, nil, func(command domain.Command, mutation Mutation, now time.Time) (deliveryStartResult, error) {
		productState, run, err := service.ports.Lifecycle.StartDelivery(
			ctx, workspaceID, productID, deliveryRunID, mutation, command.ExpectedRevision,
			func(productState *product.Product) (deliveryrun.DeliveryRun, error) {
				return lifecycle.StartDelivery(productState, deliveryRunID, command, now)
			},
		)
		return deliveryStartResult{Product: productState, Run: run}, err
	})
	return result.Product, result.Run, err
}
