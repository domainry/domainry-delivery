// Package deliverysdk defines the deployment-neutral Delivery contract.
// In-process Module and remote SaaS implementations expose the same domain
// commands, projections, identity semantics, and errors.
package deliverysdk

import (
	"context"
	"fmt"
	"strings"

	"github.com/domainry/domainry-delivery/internal/domain/delivery"
)

const ProtocolVersionV1 = "domainry-delivery-protocol-v1"
const MaxFeatureAttachmentBytes = delivery.MaxFeatureAttachmentBytes
const MaxFeatureAttachments = delivery.MaxFeatureAttachments

const (
	CapabilityProductRead        = "product.read"
	CapabilityProductCommand     = "product.command"
	CapabilityFeatureAttachment  = "feature_attachment"
	CapabilityDeliveryRunRead    = "delivery_run.read"
	CapabilityDeliveryRunCommand = "delivery_run.command"
	CapabilityAgentContext       = "agent_context.read"
)

const (
	PermissionProductRead      = "delivery_product.read"
	PermissionProductWrite     = "delivery_product.write"
	PermissionDeliveryRunRead  = "delivery_run.read"
	PermissionDeliveryRunWrite = "delivery_run.write"
	PermissionDeploymentRecord = "delivery_deployment.record"
)

type DeploymentMode string

const (
	DeploymentModeModule DeploymentMode = "module"
	DeploymentModeSaaS   DeploymentMode = "saas"
)

type Descriptor struct {
	ProtocolVersion  string         `json:"protocol_version"`
	Mode             DeploymentMode `json:"mode"`
	Capabilities     []string       `json:"capabilities"`
	SupportedLocales []string       `json:"supported_locales"`
}

func (descriptor Descriptor) Validate() error {
	if descriptor.ProtocolVersion != ProtocolVersionV1 {
		return fmt.Errorf("unsupported Delivery protocol %q", descriptor.ProtocolVersion)
	}
	if descriptor.Mode != DeploymentModeModule && descriptor.Mode != DeploymentModeSaaS {
		return fmt.Errorf("invalid Delivery deployment mode %q", descriptor.Mode)
	}
	required := map[string]bool{
		CapabilityProductRead: false, CapabilityProductCommand: false,
		CapabilityFeatureAttachment: false,
		CapabilityDeliveryRunRead:   false, CapabilityDeliveryRunCommand: false,
		CapabilityAgentContext: false,
	}
	for _, capability := range descriptor.Capabilities {
		if _, ok := required[capability]; ok {
			required[capability] = true
		}
	}
	for capability, available := range required {
		if !available {
			return fmt.Errorf("Delivery capability %q is required", capability)
		}
	}
	if len(descriptor.SupportedLocales) == 0 || descriptor.SupportedLocales[0] != "en" {
		return fmt.Errorf("Delivery descriptor must advertise English as its first supported locale")
	}
	return nil
}

func ModuleDescriptor() Descriptor { return descriptor(DeploymentModeModule) }
func SaaSDescriptor() Descriptor   { return descriptor(DeploymentModeSaaS) }

func descriptor(mode DeploymentMode) Descriptor {
	return Descriptor{
		ProtocolVersion: ProtocolVersionV1,
		Mode:            mode,
		Capabilities: []string{
			CapabilityProductRead,
			CapabilityProductCommand,
			CapabilityFeatureAttachment,
			CapabilityDeliveryRunRead,
			CapabilityDeliveryRunCommand,
			CapabilityAgentContext,
		},
		SupportedLocales: append([]string(nil), delivery.SupportedLocales...),
	}
}

type Error = delivery.Error
type Command = delivery.Command
type Actor = delivery.Actor
type ActorKind = delivery.ActorKind
type Session = delivery.Session
type Product = delivery.Product
type ProductProjection = delivery.ProductProjection
type ProductAgentContext = delivery.ProductAgentContext
type FeatureAttachment = delivery.FeatureAttachment
type FeatureAttachmentContent = delivery.FeatureAttachmentContent
type DeliveryRun = delivery.DeliveryRun
type DeliveryRunProjection = delivery.Projection
type DeliveryRunAgentContext = delivery.AgentContext

type DeliveryStartResult struct {
	Product     ProductProjection     `json:"product"`
	DeliveryRun DeliveryRunProjection `json:"delivery_run"`
}

type FeatureAttachmentUpload struct {
	AttachmentID     string
	FileName         string
	MediaType        string
	ExpectedRevision uint64
	Content          []byte
}

type FeatureAttachmentUploadResult struct {
	Product    ProductProjection `json:"product"`
	Attachment FeatureAttachment `json:"attachment"`
}

type Binding interface {
	Descriptor() Descriptor
	Session(context.Context, string) (Session, error)
	ListProducts(context.Context, string) ([]ProductProjection, error)
	GetProduct(context.Context, string, string) (ProductProjection, error)
	ProductAgentContext(context.Context, string, string) (ProductAgentContext, error)
	DispatchProduct(context.Context, string, string, Command) (ProductProjection, error)
	UploadFeatureAttachment(context.Context, string, string, string, FeatureAttachmentUpload) (FeatureAttachmentUploadResult, error)
	DownloadFeatureAttachment(context.Context, string, string, string, string) (FeatureAttachmentContent, error)
	RemoveFeatureAttachment(context.Context, string, string, string, string, uint64) (ProductProjection, error)
	StartDelivery(context.Context, string, string, string, Command) (DeliveryStartResult, error)
	ListDeliveryRuns(context.Context, string, string) ([]DeliveryRunProjection, error)
	GetDeliveryRun(context.Context, string, string) (DeliveryRunProjection, error)
	DeliveryRunAgentContext(context.Context, string, string) (DeliveryRunAgentContext, error)
	DispatchDeliveryRun(context.Context, string, string, Command) (DeliveryRunProjection, error)
	Close(context.Context) error
}

type accessTokenContextKey struct{}
type localeContextKey struct{}

// WithAccessToken supplies a bearer credential to a remote Delivery Binding.
// The credential is never serialized into domain state or command payloads.
func WithAccessToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, accessTokenContextKey{}, strings.TrimSpace(token))
}

func AccessTokenFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	token, ok := ctx.Value(accessTokenContextKey{}).(string)
	return token, ok && strings.TrimSpace(token) != ""
}

// WithLocale selects localized presentation strings without changing stable
// protocol keys or domain values. Unsupported locales fall back to English.
func WithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeContextKey{}, delivery.NormalizeLocale(locale))
}

func LocaleFromContext(ctx context.Context) string {
	if ctx == nil {
		return "en"
	}
	locale, _ := ctx.Value(localeContextKey{}).(string)
	return delivery.NormalizeLocale(locale)
}
