package httpapi_test

import (
	"context"
	"encoding/json"
	"github.com/domainry/domainry-delivery/internal/application"
	delivery "github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/infrastructure/persistence/sqlite"
	httpapi "github.com/domainry/domainry-delivery/internal/transport/http"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
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
}
