package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/domainry/domainry-delivery/internal/application"
	delivery "github.com/domainry/domainry-delivery/internal/domain"
)

const developmentIdentityEnv = "DELIVERY_DEV_IDENTITY"

type developmentPrincipal struct {
	token       string
	actor       delivery.Actor
	permissions []string
}

type developmentIdentity struct {
	workspaceID string
	principals  []developmentPrincipal
}

func developmentIdentityFromEnvironment(serverAddr string) (developmentIdentity, bool, error) {
	if !enabled(developmentIdentityEnv) {
		return developmentIdentity{}, false, nil
	}
	host, _, err := net.SplitHostPort(serverAddr)
	if err != nil {
		return developmentIdentity{}, false, errors.New("DELIVERY_ADDR must contain a host and port")
	}
	address := net.ParseIP(strings.Trim(host, "[]"))
	if address == nil || !address.IsLoopback() {
		return developmentIdentity{}, false, errors.New("DELIVERY_DEV_IDENTITY requires a loopback DELIVERY_ADDR")
	}
	workspaceID := strings.TrimSpace(os.Getenv("DELIVERY_DEV_WORKSPACE_ID"))
	if workspaceID == "" {
		return developmentIdentity{}, false, errors.New("DELIVERY_DEV_WORKSPACE_ID is required")
	}
	readWrite := []string{
		application.PermissionProductRead,
		application.PermissionProductWrite,
		application.PermissionDeliveryRunRead,
		application.PermissionDeliveryRunWrite,
	}
	inputs := []struct {
		env   string
		id    string
		kind  delivery.ActorKind
		perms []string
	}{
		{"DELIVERY_DEV_USER_TOKEN", "local-owner", delivery.ActorHuman, readWrite},
		{"DELIVERY_DEV_PM_TOKEN", "local-pm", delivery.ActorAgent, readWrite},
		{"DELIVERY_DEV_RD_TOKEN", "local-rd", delivery.ActorAgent, readWrite},
		{"DELIVERY_DEV_QA_TOKEN", "local-qa", delivery.ActorAgent, readWrite},
		{"DELIVERY_DEV_OP_TOKEN", "local-op", delivery.ActorAgent, readWrite},
		{"DELIVERY_DEV_SYSTEM_TOKEN", "local-deployment-adapter", delivery.ActorSystem, []string{
			application.PermissionProductRead,
			application.PermissionDeliveryRunRead,
			application.PermissionDeploymentRecord,
		}},
	}
	principals := make([]developmentPrincipal, 0, len(inputs))
	seen := map[string]bool{}
	for _, input := range inputs {
		token := strings.TrimSpace(os.Getenv(input.env))
		if token == "" {
			return developmentIdentity{}, false, errors.New(input.env + " is required")
		}
		if seen[token] {
			return developmentIdentity{}, false, errors.New("Delivery development identity tokens must be unique")
		}
		seen[token] = true
		principals = append(principals, developmentPrincipal{
			token: token, actor: delivery.Actor{ID: input.id, Kind: input.kind}, permissions: input.perms,
		})
	}
	return developmentIdentity{workspaceID: workspaceID, principals: principals}, true, nil
}

func (identity developmentIdentity) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
		principal, found := identity.principal(token)
		if !found {
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			writer.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"code": "authentication_required", "message": "A valid local development credential is required.",
			})
			return
		}
		context := application.WithTrustedPrincipal(
			request.Context(), identity.workspaceID, principal.actor, principal.permissions...,
		)
		next.ServeHTTP(writer, request.WithContext(context))
	})
}

func (identity developmentIdentity) principal(token string) (developmentPrincipal, bool) {
	for _, principal := range identity.principals {
		if len(token) == len(principal.token) && subtle.ConstantTimeCompare([]byte(token), []byte(principal.token)) == 1 {
			return principal, true
		}
	}
	return developmentPrincipal{}, false
}
