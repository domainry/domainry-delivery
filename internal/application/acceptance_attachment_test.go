package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/attachment"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/domain/product"
)

type acceptanceRunStore struct {
	run deliveryrun.DeliveryRun
}

func (store *acceptanceRunStore) Get(_ context.Context, workspaceID, deliveryRunID string) (deliveryrun.DeliveryRun, error) {
	if store.run.WorkspaceID != workspaceID || store.run.ID != deliveryRunID {
		return deliveryrun.DeliveryRun{}, domain.NotFound("delivery_run", deliveryRunID)
	}
	return store.run, nil
}

func (*acceptanceRunStore) ListDeliveryRuns(context.Context, string, string) ([]deliveryrun.DeliveryRun, error) {
	panic("not used")
}

func (*acceptanceRunStore) ListDeliveryRunSummaries(context.Context, string, string) ([]deliveryrun.Summary, error) {
	panic("not used")
}

func (*acceptanceRunStore) Transact(context.Context, string, string, Mutation, uint64, func(*deliveryrun.DeliveryRun) error, func(*product.Product, *deliveryrun.DeliveryRun) error) (deliveryrun.DeliveryRun, error) {
	panic("not used")
}

type acceptanceAttachmentStore struct {
	metadata map[string]attachment.Metadata
	objects  map[string][]byte
}

func (store *acceptanceAttachmentStore) Put(_ context.Context, request attachment.PutRequest) (attachment.PutResult, error) {
	store.objects[request.ObjectKey] = bytes.Clone(request.Content)
	return attachment.PutResult{ContentRef: request.ObjectKey}, nil
}

func (store *acceptanceAttachmentStore) Get(_ context.Context, _ string, contentRef string) ([]byte, error) {
	content, ok := store.objects[contentRef]
	if !ok {
		return nil, attachment.ErrContentNotFound
	}
	return bytes.Clone(content), nil
}

func (store *acceptanceAttachmentStore) PutAttachment(_ context.Context, _, productID, scopeID string, metadata attachment.Metadata) (attachment.Metadata, error) {
	store.metadata[productID+"/"+scopeID+"/"+metadata.ID] = metadata
	return metadata, nil
}

func (store *acceptanceAttachmentStore) GetAttachment(_ context.Context, _, productID, scopeID, attachmentID string) (attachment.Metadata, error) {
	metadata, ok := store.metadata[productID+"/"+scopeID+"/"+attachmentID]
	if !ok {
		return attachment.Metadata{}, domain.NotFound("attachment", attachmentID)
	}
	return metadata, nil
}

func (store *acceptanceAttachmentStore) ListAttachments(_ context.Context, _, productID, scopeID string) ([]attachment.Metadata, error) {
	result := make([]attachment.Metadata, 0)
	prefix := productID + "/" + scopeID + "/"
	for key, metadata := range store.metadata {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix && metadata.Active {
			result = append(result, metadata)
		}
	}
	return result, nil
}

func (store *acceptanceAttachmentStore) RemoveAttachment(ctx context.Context, workspaceID, productID, scopeID, attachmentID, clientID, deviceID string, expectedRevision uint64) (attachment.Metadata, error) {
	metadata, err := store.GetAttachment(ctx, workspaceID, productID, scopeID, attachmentID)
	if err != nil {
		return attachment.Metadata{}, err
	}
	if metadata.Revision != expectedRevision {
		return attachment.Metadata{}, domain.Conflict(metadata.Revision)
	}
	metadata.Active = false
	metadata.Revision++
	metadata.RemoveDeviceID = deviceID
	store.metadata[productID+"/"+scopeID+"/"+attachmentID] = metadata
	return metadata, nil
}

func TestAcceptanceScreenshotIsStoredAndReadThroughTheDeliveryRunScope(t *testing.T) {
	content, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/leUAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	runs := &acceptanceRunStore{run: deliveryrun.DeliveryRun{
		ID: "run-1", WorkspaceID: "workspace-1", Product: deliveryrun.ProductSnapshot{ID: "product-1"}, Stage: deliveryrun.StageAcceptance,
		AcceptanceReview: &deliveryrun.AcceptanceReview{ID: "acceptance-1", Status: deliveryrun.AcceptanceReviewOpen},
	}}
	attachments := &acceptanceAttachmentStore{metadata: map[string]attachment.Metadata{}, objects: map[string][]byte{}}
	service := NewService(Ports{Runs: runs, Attachments: attachments, AttachmentRecords: attachments})
	ctx := WithTrustedPrincipal(context.Background(), "workspace-1", domain.Actor{ID: "owner", Kind: domain.ActorHuman}, PermissionDeliveryRunRead, PermissionDeliveryRunWrite)

	uploaded, err := service.UploadAcceptanceAttachment(ctx, "workspace-1", "run-1", UploadFeatureAttachment{
		ClientID: "upload-1", DeviceID: "device-1", Filename: "acceptance.png", Data: content,
	})
	if err != nil || uploaded.ContentType != "image/png" || !uploaded.Active {
		t.Fatalf("upload: value=%#v error=%v", uploaded, err)
	}
	downloaded, err := service.DownloadAcceptanceAttachment(ctx, "workspace-1", "run-1", uploaded.ID)
	if err != nil || downloaded.Data != base64.StdEncoding.EncodeToString(content) {
		t.Fatalf("download: value=%#v error=%v", downloaded, err)
	}
	payload, err := json.Marshal(map[string]any{
		"evidence_refs": []string{"attachment://" + uploaded.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.validateAcceptanceBugAttachment(ctx, "workspace-1", "run-1", payload); err != nil {
		t.Fatalf("validate bug screenshot: %v", err)
	}
	removed, err := service.RemoveAcceptanceAttachment(ctx, "workspace-1", "run-1", uploaded.ID, "remove-1", "device-1", uploaded.Revision)
	if err != nil || removed.Active {
		t.Fatalf("remove: value=%#v error=%v", removed, err)
	}
}
