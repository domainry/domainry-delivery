package application

import (
	"context"
	"sort"
	"strings"

	identitysdk "github.com/domainry/domainry-identity-sdk"

	"github.com/domainry/domainry-delivery/internal/domain/delivery"
)

func authenticatedSession(ctx context.Context, workspaceID string) (delivery.Session, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if ctx != nil {
		if principal, ok := ctx.Value(trustedPrincipalContextKey{}).(trustedPrincipal); ok {
			if principal.WorkspaceID != workspaceID {
				return delivery.Session{}, delivery.Invalid("identity_workspace_mismatch", "The authenticated identity does not belong to the requested Workspace.")
			}
			permissions := make([]string, 0, len(principal.Permissions))
			for permission, granted := range principal.Permissions {
				if granted {
					permissions = append(permissions, permission)
				}
			}
			sort.Strings(permissions)
			return delivery.Session{WorkspaceID: workspaceID, Actor: principal.Actor, Permissions: permissions}, nil
		}
	}

	principal, ok := identitysdk.PrincipalFromContext(ctx)
	if !ok || !principal.Known || strings.TrimSpace(principal.UserID) == "" {
		return delivery.Session{}, delivery.Invalid("authentication_required", "An authenticated Identity principal is required.")
	}
	if strings.TrimSpace(principal.WorkspaceID) != workspaceID {
		return delivery.Session{}, delivery.Invalid("identity_workspace_mismatch", "The authenticated identity does not belong to the requested Workspace.")
	}
	kind := delivery.ActorHuman
	if principal.Workload != nil {
		kind = delivery.ActorAgent
	}
	permissions := append([]string(nil), principal.Permissions...)
	sort.Strings(permissions)
	return delivery.Session{
		WorkspaceID: workspaceID,
		Actor:       delivery.Actor{ID: principal.UserID, Kind: kind},
		Permissions: permissions,
	}, nil
}

const (
	PermissionProductRead      = "delivery_product.read"
	PermissionProductWrite     = "delivery_product.write"
	PermissionDeliveryRunRead  = "delivery_run.read"
	PermissionDeliveryRunWrite = "delivery_run.write"
	PermissionDeploymentRecord = "delivery_deployment.record"
)

type trustedPrincipal struct {
	WorkspaceID string
	Actor       delivery.Actor
	Permissions map[string]bool
}

type trustedPrincipalContextKey struct{}

// WithTrustedPrincipal is an in-process boundary for an embedding host or a
// verified infrastructure adapter. HTTP request data must never call it.
func WithTrustedPrincipal(ctx context.Context, workspaceID string, actor delivery.Actor, permissions ...string) context.Context {
	grants := make(map[string]bool, len(permissions))
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if permission != "" {
			grants[permission] = true
		}
	}
	return context.WithValue(ctx, trustedPrincipalContextKey{}, trustedPrincipal{
		WorkspaceID: strings.TrimSpace(workspaceID), Actor: actor, Permissions: grants,
	})
}

func authenticatedActor(ctx context.Context, workspaceID, permission string, forcedKind *delivery.ActorKind) (delivery.Actor, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	permission = strings.TrimSpace(permission)
	if ctx != nil {
		if principal, ok := ctx.Value(trustedPrincipalContextKey{}).(trustedPrincipal); ok {
			if principal.WorkspaceID != workspaceID {
				return delivery.Actor{}, delivery.Invalid("identity_workspace_mismatch", "The authenticated identity does not belong to the requested Workspace.")
			}
			if !principal.Permissions[permission] {
				return delivery.Actor{}, delivery.Invalid("permission_denied", "The authenticated identity is not allowed to perform this operation.")
			}
			actor := principal.Actor
			if forcedKind != nil {
				actor.Kind = *forcedKind
			}
			if err := validateAuthenticatedActor(actor); err != nil {
				return delivery.Actor{}, err
			}
			return actor, nil
		}
	}

	principal, ok := identitysdk.PrincipalFromContext(ctx)
	if !ok || !principal.Known || strings.TrimSpace(principal.UserID) == "" {
		return delivery.Actor{}, delivery.Invalid("authentication_required", "An authenticated Identity principal is required.")
	}
	if strings.TrimSpace(principal.WorkspaceID) != workspaceID {
		return delivery.Actor{}, delivery.Invalid("identity_workspace_mismatch", "The authenticated identity does not belong to the requested Workspace.")
	}
	if !principal.HasPermission(permission) {
		return delivery.Actor{}, delivery.Invalid("permission_denied", "The authenticated identity is not allowed to perform this operation.")
	}
	kind := delivery.ActorHuman
	if principal.Workload != nil {
		kind = delivery.ActorAgent
	}
	if forcedKind != nil {
		kind = *forcedKind
	}
	return delivery.Actor{ID: principal.UserID, Kind: kind}, nil
}

func validateAuthenticatedActor(actor delivery.Actor) error {
	if strings.TrimSpace(actor.ID) == "" {
		return delivery.Invalid("authentication_required", "An authenticated Identity principal is required.")
	}
	if actor.Kind != delivery.ActorHuman && actor.Kind != delivery.ActorAgent && actor.Kind != delivery.ActorSystem {
		return delivery.Invalid("identity_actor_invalid", "The authenticated Identity principal has an invalid actor kind.")
	}
	return nil
}
