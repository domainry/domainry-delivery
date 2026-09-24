package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/domainry/domainry-delivery/internal/application"
	delivery "github.com/domainry/domainry-delivery/internal/domain"
)

func TestDevelopmentIdentityResolvesServerOwnedPrincipal(t *testing.T) {
	t.Setenv(developmentIdentityEnv, "true")
	t.Setenv("DELIVERY_DEV_WORKSPACE_ID", "workspace-local")
	t.Setenv("DELIVERY_DEV_USER_TOKEN", "user-token")
	t.Setenv("DELIVERY_DEV_PM_TOKEN", "pm-token")
	t.Setenv("DELIVERY_DEV_RD_TOKEN", "rd-token")
	t.Setenv("DELIVERY_DEV_QA_TOKEN", "qa-token")
	t.Setenv("DELIVERY_DEV_OP_TOKEN", "op-token")
	t.Setenv("DELIVERY_DEV_SYSTEM_TOKEN", "system-token")

	identity, configured, err := developmentIdentityFromEnvironment("127.0.0.1:18096")
	if err != nil || !configured {
		t.Fatalf("development identity configuration: configured=%t error=%v", configured, err)
	}
	service := application.NewService(application.Ports{})
	handler := identity.Authenticate(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		session, err := service.Session(request.Context(), "workspace-local")
		if err != nil {
			t.Fatalf("resolve session: %v", err)
		}
		_ = json.NewEncoder(writer).Encode(session)
	}))
	request := httptest.NewRequest(http.MethodGet, "/session", nil)
	request.Header.Set("Authorization", "Bearer pm-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var session delivery.Session
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	want := delivery.Session{
		WorkspaceID: "workspace-local",
		Actor:       delivery.Actor{ID: "local-pm", Kind: delivery.ActorAgent},
		Permissions: []string{
			application.PermissionProductRead,
			application.PermissionProductWrite,
			application.PermissionDeliveryRunRead,
			application.PermissionDeliveryRunWrite,
		},
	}
	if !reflect.DeepEqual(session, want) {
		t.Fatalf("session=%+v want=%+v", session, want)
	}
}

func TestDevelopmentIdentityRejectsNonLoopbackListener(t *testing.T) {
	t.Setenv(developmentIdentityEnv, "true")
	if _, _, err := developmentIdentityFromEnvironment("0.0.0.0:18096"); err == nil {
		t.Fatal("development identity accepted a non-loopback listener")
	}
}
