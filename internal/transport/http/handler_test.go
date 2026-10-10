package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/domainry/domainry-delivery/internal/application"
	delivery "github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/infrastructure/persistence/sqlite"
	httpapi "github.com/domainry/domainry-delivery/internal/transport/http"
)

func TestProductCreateReadAndAgentContext(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := httpapi.New(application.NewService(application.Ports{Products: store, Runs: store, Lifecycle: store}), slog.New(slog.NewTextHandler(io.Discard, nil)), "delivery-test")

	command := map[string]any{
		"client_id": "create-product-1", "expected_revision": 0,
		"type": "product.create",
		"payload": map[string]any{
			"name": "Booking Product", "code": "BOOKING", "goal": "Make booking operations executable", "industry": "Fitness and wellness",
			"story":      map[string]any{"title": "Booking Product", "summary": "Unified booking", "narrative": "A member books a session, and the location confirms and fulfills it."},
			"definition": map[string]any{"schema_version": 2, "actors": []any{}, "scenarios": []any{}, "objects": []any{}, "rules": []any{}, "exceptions": []any{}, "actions": []any{}, "pages": []any{}, "access": map[string]any{}, "integrations": []any{}, "automations": []any{}, "configuration": []any{}, "quality_constraints": []any{}},
			"decisions":  []any{},
		},
	}
	body, _ := json.Marshal(command)
	requestContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		application.PermissionProductRead, application.PermissionProductWrite,
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/products/product-1/commands", strings.NewReader(string(body))).WithContext(requestContext)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/products/product-1", nil).WithContext(requestContext)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"current_definition_revision":1`) {
		t.Fatalf("get status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/products/product-1/agent-context", nil).WithContext(requestContext)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"feature.discovery.replace"`) {
		t.Fatalf("agent context status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestArchivedProductsCanBeListedAndRestored(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := httpapi.New(application.NewService(application.Ports{Products: store, Runs: store, Lifecycle: store}), slog.New(slog.NewTextHandler(io.Discard, nil)), "delivery-test")
	requestContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		application.PermissionProductRead, application.PermissionProductWrite,
	)
	dispatch := func(clientID string, expectedRevision uint64, commandType string, payload map[string]any, expectedStatus int) []byte {
		t.Helper()
		body, marshalErr := json.Marshal(map[string]any{
			"client_id": clientID, "expected_revision": expectedRevision, "type": commandType, "payload": payload,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/products/product-1/commands", strings.NewReader(string(body))).WithContext(requestContext)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != expectedStatus {
			t.Fatalf("%s status=%d body=%s", commandType, response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}
	dispatch("create-product", 0, "product.create", map[string]any{
		"name": "Booking Product", "code": "BOOKING", "goal": "Make booking operations executable", "industry": "Fitness and wellness",
		"story":      map[string]any{"title": "Booking Product", "summary": "Unified booking", "narrative": "A member books a session."},
		"definition": map[string]any{"schema_version": 2, "actors": []any{}, "scenarios": []any{}, "objects": []any{}, "rules": []any{}, "exceptions": []any{}, "actions": []any{}, "pages": []any{}, "access": map[string]any{}, "integrations": []any{}, "automations": []any{}, "configuration": []any{}, "quality_constraints": []any{}},
		"decisions":  []any{},
	}, http.StatusCreated)
	dispatch("delete-product", 1, "product.delete", map[string]any{}, http.StatusOK)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/archived-products", nil).WithContext(requestContext)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"product.restore"`) {
		t.Fatalf("archived list status=%d body=%s", response.Code, response.Body.String())
	}

	dispatch("restore-product", 2, "product.restore", map[string]any{}, http.StatusOK)
	request = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/products", nil).WithContext(requestContext)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"product-1"`) {
		t.Fatalf("restored list status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestVerifiedFrontendCompletionProjectsFoundationInstallation(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := httpapi.New(application.NewService(application.Ports{Products: store, Runs: store, Lifecycle: store}), slog.New(slog.NewTextHandler(io.Discard, nil)), "delivery-test")
	humanContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		application.PermissionProductRead, application.PermissionProductWrite,
	)
	agentContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "rd-agent", Kind: delivery.ActorAgent},
		application.PermissionProductWrite,
	)
	dispatch := func(requestContext context.Context, clientID string, expectedRevision uint64, commandType string, payload map[string]any, expectedStatus int) {
		t.Helper()
		body, marshalErr := json.Marshal(map[string]any{
			"client_id": clientID, "expected_revision": expectedRevision, "type": commandType, "payload": payload,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/products/product-1/commands", strings.NewReader(string(body))).WithContext(requestContext)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != expectedStatus {
			t.Fatalf("%s status=%d body=%s", commandType, response.Code, response.Body.String())
		}
	}

	dispatch(humanContext, "create-product", 0, "product.create", map[string]any{
		"name": "Booking Product", "code": "BOOKING", "goal": "Make booking operations executable", "industry": "Fitness and wellness",
		"story":      map[string]any{"title": "Booking Product", "summary": "Unified booking", "narrative": "A member books a session, and the location confirms and fulfills it."},
		"definition": map[string]any{"schema_version": 2, "actors": []any{}, "scenarios": []any{}, "objects": []any{}, "rules": []any{}, "exceptions": []any{}, "actions": []any{}, "pages": []any{}, "access": map[string]any{}, "integrations": []any{}, "automations": []any{}, "configuration": []any{}, "quality_constraints": []any{}},
		"decisions":  []any{},
	}, http.StatusCreated)
	dispatch(agentContext, "frontend-start", 1, "product.engineering.frontend.start", map[string]any{}, http.StatusOK)
	dispatch(agentContext, "frontend-complete", 2, "product.engineering.frontend.complete", map[string]any{
		"code_revision": "workspace:frontend", "artifact_ref": "artifact://frontend", "design_contract_ref": "evidence://design",
		"login_entry": "frontend/login.tsx", "shell_entry": "frontend/shell.tsx", "preview_entry": "frontend/dist/index.html",
	}, http.StatusOK)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/products/product-1", nil).WithContext(humanContext)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", response.Code, response.Body.String())
	}
	var projection struct {
		AvailableActions []delivery.AvailableAction `json:"available_actions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(projection.AvailableActions, func(action delivery.AvailableAction) bool {
		return action.Command == "product.engineering.foundation.started" && action.ActorKind == delivery.ActorSystem
	}) {
		t.Fatalf("foundation installation was not projected: %#v", projection.AvailableActions)
	}
}

func TestAcceptLanguageLocalizesErrorsWithoutChangingCodes(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := httpapi.New(application.NewService(application.Ports{Products: store, Runs: store, Lifecycle: store}), slog.New(slog.NewTextHandler(io.Discard, nil)), "delivery-test")
	requestContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		application.PermissionProductRead,
	)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/workspace-1/products/missing", nil).WithContext(requestContext)
	request.Header.Set("Accept-Language", "es-MX,es;q=0.9")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body delivery.Error
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "not_found" || body.Message != "No se encontró el recurso solicitado." {
		t.Fatalf("localized error differs: %#v", body)
	}
}

func TestCommandEnvelopeIsValidatedBeforeDispatch(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := httpapi.New(application.NewService(application.Ports{Products: store, Runs: store, Lifecycle: store}), slog.New(slog.NewTextHandler(io.Discard, nil)), "delivery-test")
	requestContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		application.PermissionProductWrite,
	)
	for _, test := range []struct {
		name string
		body string
		code string
	}{
		{name: "missing client id", body: `{"expected_revision":0,"type":"product.create","payload":{}}`, code: "client_id_required"},
		{name: "unknown command", body: `{"client_id":"request-1","expected_revision":0,"type":"product.unknown","payload":{}}`, code: "command_unknown"},
		{name: "missing payload", body: `{"client_id":"request-1","expected_revision":0,"type":"product.create"}`, code: "payload_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/products/product-1/commands", strings.NewReader(test.body)).WithContext(requestContext)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/workspace-1/products/missing/commands",
		strings.NewReader(`{"client_id":"rebase-1","expected_revision":1,"type":"feature.rebase","payload":{"feature_id":"feature-1","feature_revision":1}}`),
	).WithContext(requestContext)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":"not_found"`) {
		t.Fatalf("feature.rebase did not cross the public SDK boundary: status=%d body=%s", response.Code, response.Body.String())
	}
}
