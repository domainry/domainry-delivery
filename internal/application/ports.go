package application

import (
	"context"

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

type Ports struct {
	Products  ProductRepository
	Runs      DeliveryRunRepository
	Lifecycle LifecycleRepository
}
