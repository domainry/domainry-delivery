package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain/delivery"
)

type Repository interface {
	Ensure(context.Context, delivery.DeliveryRun) error
	Get(context.Context, string, string) (delivery.DeliveryRun, error)
	ListDeliveryRuns(context.Context, string, string) ([]delivery.DeliveryRun, error)
	Transact(context.Context, string, string, string, string, uint64, func(*delivery.DeliveryRun) error, func(*delivery.Product, *delivery.DeliveryRun) error) (delivery.DeliveryRun, error)
	EnsureProduct(context.Context, delivery.Product) error
	GetProduct(context.Context, string, string) (delivery.Product, error)
	ListProducts(context.Context, string) ([]delivery.Product, error)
	CreateProduct(context.Context, string, string, string, string, uint64, func() (delivery.Product, error)) (delivery.Product, error)
	TransactProduct(context.Context, string, string, string, string, uint64, func(*delivery.Product) error) (delivery.Product, error)
	StoreFeatureAttachment(context.Context, string, string, string, string, uint64, string, []byte, func(*delivery.Product) (delivery.FeatureAttachment, error)) (delivery.Product, delivery.FeatureAttachment, error)
	GetFeatureAttachment(context.Context, string, string, string, string) (delivery.FeatureAttachmentContent, error)
	DeleteFeatureAttachment(context.Context, string, string, string, string, uint64, func(*delivery.Product) (delivery.FeatureAttachment, error)) (delivery.Product, error)
	StartDelivery(context.Context, string, string, string, string, string, uint64, func(*delivery.Product) (delivery.DeliveryRun, error)) (delivery.Product, delivery.DeliveryRun, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (service *Service) Session(ctx context.Context, workspaceID string) (delivery.Session, error) {
	return authenticatedSession(ctx, workspaceID)
}

func (service *Service) Ensure(ctx context.Context, run delivery.DeliveryRun) error {
	return service.repository.Ensure(ctx, run)
}

func (service *Service) Get(ctx context.Context, workspaceID, deliveryRunID string) (delivery.DeliveryRun, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionDeliveryRunRead, nil); err != nil {
		return delivery.DeliveryRun{}, err
	}
	return service.repository.Get(ctx, workspaceID, deliveryRunID)
}

func (service *Service) ListDeliveryRuns(ctx context.Context, workspaceID, productID string) ([]delivery.DeliveryRun, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionDeliveryRunRead, nil); err != nil {
		return nil, err
	}
	return service.repository.ListDeliveryRuns(ctx, workspaceID, productID)
}

func (service *Service) AgentContext(ctx context.Context, workspaceID, deliveryRunID string) (delivery.AgentContext, error) {
	run, err := service.Get(ctx, workspaceID, deliveryRunID)
	if err != nil {
		return delivery.AgentContext{}, err
	}
	return delivery.AgentContextFor(run), nil
}

func (service *Service) Dispatch(ctx context.Context, workspaceID, deliveryRunID string, command delivery.Command) (delivery.DeliveryRun, error) {
	if command.ClientID == "" {
		return delivery.DeliveryRun{}, delivery.Invalid("client_id_required", "Every write requires client_id.")
	}
	permission := PermissionDeliveryRunWrite
	var forcedKind *delivery.ActorKind
	if command.Type == "delivery_unit.compile.complete" ||
		command.Type == "delivery_unit.contract.freeze" ||
		command.Type == "delivery_unit.backend.reopen" ||
		command.Type == "delivery_unit.contract.reopen" ||
		command.Type == "delivery_unit.journey.complete" ||
		command.Type == "release.deploy_result" {
		permission = PermissionDeploymentRecord
		systemKind := delivery.ActorSystem
		forcedKind = &systemKind
	}
	actor, err := authenticatedActor(ctx, workspaceID, permission, forcedKind)
	if err != nil {
		return delivery.DeliveryRun{}, err
	}
	command.Actor = actor
	fingerprint, err := commandFingerprint(command)
	if err != nil {
		return delivery.DeliveryRun{}, delivery.Invalid("command_invalid", "The command could not be canonicalized.")
	}
	now := service.now()
	return service.repository.Transact(
		ctx, workspaceID, deliveryRunID, command.ClientID, fingerprint, command.ExpectedRevision,
		func(run *delivery.DeliveryRun) error { return delivery.Apply(run, command, now) },
		func(product *delivery.Product, run *delivery.DeliveryRun) error {
			if run.Stage != delivery.StageLive {
				return nil
			}
			return delivery.InstallDeliveryRun(product, run, command.Actor, now)
		},
	)
}

func (service *Service) EnsureProduct(ctx context.Context, product delivery.Product) error {
	return service.repository.EnsureProduct(ctx, product)
}

func (service *Service) GetProduct(ctx context.Context, workspaceID, productID string) (delivery.Product, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductRead, nil); err != nil {
		return delivery.Product{}, err
	}
	product, err := service.repository.GetProduct(ctx, workspaceID, productID)
	if err != nil {
		return delivery.Product{}, err
	}
	if product.Status == delivery.ProductArchived {
		return delivery.Product{}, delivery.NotFound("product", productID)
	}
	return product, nil
}

func (service *Service) ListProducts(ctx context.Context, workspaceID string) ([]delivery.Product, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductRead, nil); err != nil {
		return nil, err
	}
	products, err := service.repository.ListProducts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	visible := make([]delivery.Product, 0, len(products))
	for _, product := range products {
		if product.Status != delivery.ProductArchived {
			visible = append(visible, product)
		}
	}
	return visible, nil
}

func (service *Service) ProductAgentContext(ctx context.Context, workspaceID, productID string) (delivery.ProductAgentContext, error) {
	product, err := service.GetProduct(ctx, workspaceID, productID)
	if err != nil {
		return delivery.ProductAgentContext{}, err
	}
	return delivery.ProductAgentContextFor(product), nil
}

func (service *Service) DispatchProduct(ctx context.Context, workspaceID, productID string, command delivery.Command) (delivery.Product, error) {
	if command.ClientID == "" {
		return delivery.Product{}, delivery.Invalid("client_id_required", "Every write requires client_id.")
	}
	actor, err := authenticatedActor(ctx, workspaceID, PermissionProductWrite, nil)
	if err != nil {
		return delivery.Product{}, err
	}
	command.Actor = actor
	fingerprint, err := commandFingerprint(command)
	if err != nil {
		return delivery.Product{}, delivery.Invalid("command_invalid", "The command could not be canonicalized.")
	}
	if command.Type == "product.create" {
		return service.repository.CreateProduct(ctx, workspaceID, productID, command.ClientID, fingerprint, command.ExpectedRevision, func() (delivery.Product, error) {
			return delivery.NewProduct(workspaceID, productID, command, service.now())
		})
	}
	if command.Type == "feature.delivery.start" || command.Type == "feature.install" {
		return delivery.Product{}, delivery.Invalid("command_endpoint_invalid", "Use the DeliveryRun lifecycle endpoint for delivery start and installation.")
	}
	return service.repository.TransactProduct(ctx, workspaceID, productID, command.ClientID, fingerprint, command.ExpectedRevision, func(product *delivery.Product) error {
		return delivery.ApplyProduct(product, command, service.now())
	})
}

func (service *Service) UploadFeatureAttachment(
	ctx context.Context,
	workspaceID string,
	productID string,
	featureID string,
	attachmentID string,
	fileName string,
	mediaType string,
	expectedRevision uint64,
	content []byte,
) (delivery.Product, delivery.FeatureAttachment, error) {
	actor, err := authenticatedActor(ctx, workspaceID, PermissionProductWrite, nil)
	if err != nil {
		return delivery.Product{}, delivery.FeatureAttachment{}, err
	}
	if len(content) == 0 || int64(len(content)) > delivery.MaxFeatureAttachmentBytes {
		return delivery.Product{}, delivery.FeatureAttachment{}, delivery.Invalid("attachment_size_invalid", "The attachment size is outside the supported range.")
	}
	digest := sha256.Sum256(content)
	hash := hex.EncodeToString(digest[:])
	return service.repository.StoreFeatureAttachment(
		ctx, workspaceID, productID, featureID, attachmentID, expectedRevision, hash, content,
		func(product *delivery.Product) (delivery.FeatureAttachment, error) {
			return delivery.AddFeatureAttachment(product, featureID, delivery.FeatureAttachmentInput{
				ID: attachmentID, Name: fileName, MediaType: mediaType, SizeBytes: int64(len(content)), SHA256: hash,
			}, actor, service.now())
		},
	)
}

func (service *Service) DownloadFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string) (delivery.FeatureAttachmentContent, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductRead, nil); err != nil {
		return delivery.FeatureAttachmentContent{}, err
	}
	return service.repository.GetFeatureAttachment(ctx, workspaceID, productID, featureID, attachmentID)
}

func (service *Service) RemoveFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string, expectedRevision uint64) (delivery.Product, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductWrite, nil); err != nil {
		return delivery.Product{}, err
	}
	return service.repository.DeleteFeatureAttachment(
		ctx, workspaceID, productID, featureID, attachmentID, expectedRevision,
		func(product *delivery.Product) (delivery.FeatureAttachment, error) {
			return delivery.RemoveFeatureAttachment(product, featureID, attachmentID, service.now())
		},
	)
}

func (service *Service) StartDelivery(ctx context.Context, workspaceID, productID, deliveryRunID string, command delivery.Command) (delivery.Product, delivery.DeliveryRun, error) {
	if command.ClientID == "" {
		return delivery.Product{}, delivery.DeliveryRun{}, delivery.Invalid("client_id_required", "Every write requires client_id.")
	}
	actor, err := authenticatedActor(ctx, workspaceID, PermissionProductWrite, nil)
	if err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, err
	}
	command.Actor = actor
	fingerprint, err := commandFingerprint(command)
	if err != nil {
		return delivery.Product{}, delivery.DeliveryRun{}, delivery.Invalid("command_invalid", "The command could not be canonicalized.")
	}
	now := service.now()
	return service.repository.StartDelivery(
		ctx, workspaceID, productID, deliveryRunID, command.ClientID, fingerprint, command.ExpectedRevision,
		func(product *delivery.Product) (delivery.DeliveryRun, error) {
			return delivery.StartDelivery(product, deliveryRunID, command, now)
		},
	)
}

func commandFingerprint(command delivery.Command) (string, error) {
	var payload any = map[string]any{}
	if len(command.Payload) > 0 {
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return "", err
		}
	}
	canonical, err := json.Marshal(struct {
		Actor   delivery.Actor `json:"actor"`
		Type    string         `json:"type"`
		Payload any            `json:"payload"`
	}{Actor: command.Actor, Type: command.Type, Payload: payload})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
