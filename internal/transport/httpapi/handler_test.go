package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	deliverysdk "github.com/domainry/domainry-delivery"
	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain/delivery"
	"github.com/domainry/domainry-delivery/internal/infrastructure/sqlite"
	"github.com/domainry/domainry-delivery/internal/transport/httpapi"
)

func TestProductCreateReadAndAgentContext(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := httpapi.New(application.NewService(store), slog.New(slog.NewTextHandler(io.Discard, nil)))

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

func TestFeatureAttachmentUploadDownloadAndRemoval(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	product := delivery.Product{
		ID: "product-1", WorkspaceID: "workspace-1", Name: "Quote workflow", Code: "QUOTE", Goal: "Compare quotes",
		Status: delivery.ProductShaping, Revision: 1, CurrentDefinitionRevision: 1, CurrentReleaseRevision: 1,
		Revisions: []delivery.ProductRevision{}, Features: []delivery.Feature{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.EnsureProduct(t.Context(), product); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(application.NewService(store), slog.New(slog.NewTextHandler(io.Discard, nil)))
	requestContext := application.WithTrustedPrincipal(
		context.Background(), "workspace-1", delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		application.PermissionProductRead, application.PermissionProductWrite,
	)
	openCommand, _ := json.Marshal(map[string]any{
		"client_id": "open-feature-1", "expected_revision": 1, "type": "feature.discovery.open",
		"payload": map[string]any{"feature_id": "feature-1"},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/products/product-1/commands", bytes.NewReader(openCommand)).WithContext(requestContext)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":"F-001"`) {
		t.Fatalf("open Feature status=%d body=%s", response.Code, response.Body.String())
	}

	content := []byte("supplier,item,price\nAcme,Widget,120\n")
	path := "/api/v1/workspaces/workspace-1/products/product-1/features/feature-1/attachments/attachment-1?file_name=quote.csv&expected_revision=2"
	request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(content)).WithContext(requestContext)
	request.Header.Set("Content-Type", "text/csv")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", response.Code, response.Body.String())
	}
	var uploaded deliverysdk.FeatureAttachmentUploadResult
	if err := json.Unmarshal(response.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.Product.Product.Revision != 3 || uploaded.Attachment.Name != "quote.csv" || len(uploaded.Product.Product.Features[0].Attachments) != 1 {
		t.Fatalf("upload result differs: %#v", uploaded)
	}

	request = httptest.NewRequest(http.MethodGet, strings.Split(path, "?")[0], nil).WithContext(requestContext)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), content) || response.Header().Get("X-Attachment-SHA256") != uploaded.Attachment.SHA256 {
		t.Fatalf("download status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.Bytes())
	}

	request = httptest.NewRequest(http.MethodDelete, strings.Split(path, "?")[0]+"?expected_revision=3", nil).WithContext(requestContext)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"revision":4`) || !strings.Contains(response.Body.String(), `"attachments":[]`) {
		t.Fatalf("remove status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAcceptLanguageLocalizesErrorsWithoutChangingCodes(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := httpapi.New(application.NewService(store), slog.New(slog.NewTextHandler(io.Discard, nil)))
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
