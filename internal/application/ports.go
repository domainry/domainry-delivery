package application

import (
	"context"

	attachmentdomain "github.com/domainry/domainry-delivery/internal/domain/attachment"
	"github.com/domainry/domainry-delivery/internal/domain/conversation"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/domain/product"
)

// ProductRepository owns Product aggregate reads and Product-only commits.
type ProductRepository interface {
	GetProduct(context.Context, string, string) (product.Product, error)
	ListProducts(context.Context, string) ([]product.Product, error)
	CreateProduct(context.Context, string, string, Mutation, uint64, func() (product.Product, error)) (product.Product, error)
	TransactProduct(context.Context, string, string, Mutation, uint64, func(*product.Product) error) (product.Product, error)
}

// DeliveryRunRepository owns DeliveryRun aggregate reads and commits. The
// Product callback exists only for the atomic installation transition.
type DeliveryRunRepository interface {
	Get(context.Context, string, string) (deliveryrun.DeliveryRun, error)
	ListDeliveryRuns(context.Context, string, string) ([]deliveryrun.DeliveryRun, error)
	Transact(context.Context, string, string, Mutation, uint64, func(*deliveryrun.DeliveryRun) error, func(*product.Product, *deliveryrun.DeliveryRun) error) (deliveryrun.DeliveryRun, error)
}

// LifecycleRepository commits transitions that create or mutate both Product
// and DeliveryRun. No aggregate Store is exposed outside persistence assembly.
type LifecycleRepository interface {
	StartDelivery(context.Context, string, string, string, Mutation, uint64, func(*product.Product) (deliveryrun.DeliveryRun, error)) (product.Product, deliveryrun.DeliveryRun, error)
}

// ConversationRepository stores exact, immutable user and assistant messages.
// A Feature revision cites these records rather than embedding conversation text.
type ConversationRepository interface {
	PutMessage(context.Context, string, string, string, conversation.Message) (conversation.Message, error)
	ListMessages(context.Context, string, string, string, string, int) ([]conversation.Message, error)
	GetMessage(context.Context, string, string, string, string) (conversation.Message, error)
}

type AttachmentRepository interface {
	PutAttachment(context.Context, string, string, string, attachmentdomain.Metadata) (attachmentdomain.Metadata, error)
	GetAttachment(context.Context, string, string, string, string) (attachmentdomain.Metadata, error)
	ListAttachments(context.Context, string, string, string) ([]attachmentdomain.Metadata, error)
	RemoveAttachment(context.Context, string, string, string, string, string, string, uint64) (attachmentdomain.Metadata, error)
}

type Ports struct {
	Products          ProductRepository
	Runs              DeliveryRunRepository
	Lifecycle         LifecycleRepository
	Attachments       attachmentdomain.Store
	AttachmentRecords AttachmentRepository
	Conversations     ConversationRepository
}
