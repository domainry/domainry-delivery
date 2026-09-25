package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/domainry/domainry-delivery/internal/application"
	delivery "github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/attachment"
	"github.com/domainry/domainry-delivery/internal/infrastructure/persistence/sqlite"
	httpapi "github.com/domainry/domainry-delivery/internal/transport/http"
)

type archiveObjectStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (store *archiveObjectStore) Put(_ context.Context, request attachment.PutRequest) (attachment.PutResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.objects == nil {
		store.objects = make(map[string][]byte)
	}
	store.objects[request.ObjectKey] = bytes.Clone(request.Content)
	return attachment.PutResult{ContentRef: request.ObjectKey}, nil
}

func (store *archiveObjectStore) Get(_ context.Context, _ string, ref string) ([]byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	value, ok := store.objects[ref]
	if !ok {
		return nil, attachment.ErrContentNotFound
	}
	return bytes.Clone(value), nil
}

func TestFeatureConversationAndAttachmentArchive(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	objects := &archiveObjectStore{}
	handler := httpapi.New(application.NewService(application.Ports{
		Products: store, Runs: store, Lifecycle: store, Conversations: store,
		Attachments: objects, AttachmentRecords: store,
	}), slog.New(slog.NewTextHandler(io.Discard, nil)), "delivery-test")
	requestContext := application.WithTrustedPrincipal(context.Background(), "workspace-1", delivery.Actor{ID: "owner", Kind: delivery.ActorHuman}, application.PermissionProductRead, application.PermissionProductWrite)
	call := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(raw)).WithContext(requestContext)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode %s: %v", response.Body.String(), err)
		}
		return response.Code, result
	}
	productPath := "/api/v1/workspaces/workspace-1/products/product-1"
	status, value := call(http.MethodPost, productPath+"/commands", map[string]any{
		"client_id": "product-create-1", "expected_revision": 0, "type": "product.create",
		"payload": map[string]any{
			"name": "Booking Product", "code": "BOOKING", "goal": "Make booking executable", "industry": "Fitness",
			"story":      map[string]any{"title": "Booking Product", "summary": "Booking", "narrative": "A member books a session and the location fulfills it."},
			"definition": map[string]any{"schema_version": 2, "actors": []any{}, "scenarios": []any{}, "objects": []any{}, "rules": []any{}, "exceptions": []any{}, "actions": []any{}, "pages": []any{}, "access": map[string]any{}, "integrations": []any{}, "automations": []any{}, "configuration": []any{}, "quality_constraints": []any{}},
			"decisions":  []any{},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create product: status=%d body=%v", status, value)
	}
	if _, ok := value["product"].(map[string]any)["created_at"].(float64); !ok {
		t.Fatalf("Product created_at is not Unix milliseconds: %v", value)
	}
	featurePath := productPath + "/features/feature-1"
	first := map[string]any{"client_id": "message-1", "device_id": "device-1", "expected_revision": 0, "turn_id": "turn-1", "role": "user", "text": "原始用户内容", "attachment_ids": []string{}}
	status, value = call(http.MethodPost, featurePath+"/messages", first)
	if status != http.StatusOK || value["text"] != first["text"] || value["source_id"] == "" || value["device_id"] != "device-1" {
		t.Fatalf("archive user: status=%d body=%v", status, value)
	}
	status, replay := call(http.MethodPost, featurePath+"/messages", first)
	if status != http.StatusOK || replay["id"] != value["id"] {
		t.Fatalf("idempotent replay: status=%d body=%v", status, replay)
	}
	first["text"] = "changed"
	status, _ = call(http.MethodPost, featurePath+"/messages", first)
	if status != http.StatusConflict {
		t.Fatalf("different replay status=%d", status)
	}
	status, _ = call(http.MethodPost, featurePath+"/messages", map[string]any{"client_id": "message-2", "device_id": "device-1", "expected_revision": 0, "turn_id": "turn-1", "role": "assistant", "text": "PM 原始回复"})
	if status != http.StatusOK {
		t.Fatalf("archive assistant status=%d", status)
	}
	status, page := call(http.MethodGet, featurePath+"/messages?limit=1", nil)
	if status != http.StatusOK || page["next_cursor"] != "message-1" || len(page["data"].([]any)) != 1 {
		t.Fatalf("first page: status=%d body=%v", status, page)
	}
	status, page = call(http.MethodGet, featurePath+"/messages?cursor=message-1&limit=1", nil)
	if status != http.StatusOK || len(page["data"].([]any)) != 1 || page["data"].([]any)[0].(map[string]any)["text"] != "PM 原始回复" {
		t.Fatalf("second page: status=%d body=%v", status, page)
	}
	content, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/leUAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	status, uploaded := call(http.MethodPost, featurePath+"/attachments", map[string]any{"client_id": "upload-1", "device_id": "device-1", "expected_revision": 0, "filename": "evidence.png", "data": base64.StdEncoding.EncodeToString(content)})
	if status != http.StatusOK || uploaded["sha256"] != hex.EncodeToString(digest[:]) || uploaded["device_id"] != "device-1" {
		t.Fatalf("upload: status=%d body=%v", status, uploaded)
	}
	id := uploaded["id"].(string)
	featureSource := map[string]any{
		"conversation_id": value["conversation_id"], "run_id": "turn-1",
		"source_ids": []string{value["source_id"].(string), "attachment://" + id},
	}
	status, invalidSource := call(http.MethodPost, productPath+"/commands", map[string]any{
		"client_id": "feature-source-invalid", "expected_revision": 1, "type": "feature.discovery.replace",
		"payload": map[string]any{"feature_id": "feature-1", "code": "F-001", "baseline_product_revision": 1, "source": map[string]any{
			"conversation_id": value["conversation_id"], "run_id": "turn-1", "source_ids": []string{"conversation://another/message/message-1"},
		}},
	})
	if status != http.StatusUnprocessableEntity || invalidSource["code"] != "feature_source_reference_unverified" {
		t.Fatalf("unverified source was accepted: status=%d body=%v", status, invalidSource)
	}
	status, verifiedSource := call(http.MethodPost, productPath+"/commands", map[string]any{
		"client_id": "feature-source-valid", "expected_revision": 1, "type": "feature.discovery.replace",
		"payload": map[string]any{"feature_id": "feature-1", "code": "F-001", "baseline_product_revision": 1, "source": featureSource},
	})
	if status != http.StatusUnprocessableEntity || verifiedSource["code"] == "feature_source_reference_unverified" {
		t.Fatalf("archived message and attachment were not verified: status=%d body=%v", status, verifiedSource)
	}
	status, downloaded := call(http.MethodGet, featurePath+"/attachments/"+id, nil)
	if status != http.StatusOK || downloaded["data"] != base64.StdEncoding.EncodeToString(content) {
		t.Fatalf("download: status=%d body=%v", status, downloaded)
	}
	status, removed := call(http.MethodDelete, featurePath+"/attachments/"+id, map[string]any{"client_id": "remove-1", "device_id": "device-1", "expected_revision": 1})
	if status != http.StatusOK || removed["active"] != false || removed["remove_device_id"] != "device-1" {
		t.Fatalf("remove: status=%d body=%v", status, removed)
	}
	status, listed := call(http.MethodGet, featurePath+"/attachments", nil)
	if status != http.StatusOK || len(listed["data"].([]any)) != 0 {
		t.Fatalf("list removed: status=%d body=%v", status, listed)
	}
	status, downloaded = call(http.MethodGet, featurePath+"/attachments/"+id, nil)
	if status != http.StatusOK || downloaded["data"] != base64.StdEncoding.EncodeToString(content) {
		t.Fatalf("historical download: status=%d body=%v", status, downloaded)
	}
}
