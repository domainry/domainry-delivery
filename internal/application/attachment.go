package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/attachment"
	"github.com/domainry/domainry-delivery/internal/domain/conversation"
)

type UploadFeatureAttachment struct {
	ClientID         string `json:"client_id"`
	DeviceID         string `json:"device_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Filename         string `json:"filename"`
	Data             []byte `json:"data"`
}

func (service *Service) UploadFeatureAttachment(ctx context.Context, workspaceID, productID, featureID string, input UploadFeatureAttachment) (attachment.Metadata, error) {
	actor, err := authenticatedActor(ctx, workspaceID, PermissionProductWrite, nil)
	if err != nil {
		return attachment.Metadata{}, err
	}
	if !conversation.ValidID(productID) || !conversation.ValidID(featureID) || !conversation.ValidID(input.ClientID) || !conversation.ValidID(input.DeviceID) || input.ExpectedRevision != 0 || invalidFilename(input.Filename) {
		return attachment.Metadata{}, domain.Invalid("request_invalid")
	}
	if len(input.Data) == 0 || int64(len(input.Data)) > attachment.MaxUploadBytes {
		return attachment.Metadata{}, domain.Invalid("request_invalid")
	}
	contentType, extension, supported := attachment.ClassifyContent(input.Data)
	if !supported {
		return attachment.Metadata{}, domain.Invalid("attachment_type_unsupported")
	}
	if _, err := service.ports.Products.GetProduct(ctx, workspaceID, productID); err != nil {
		return attachment.Metadata{}, err
	}
	digest := sha256.Sum256([]byte(input.ClientID))
	id := "attachment-" + hex.EncodeToString(digest[:16])
	contentDigest := sha256.Sum256(input.Data)
	sha := hex.EncodeToString(contentDigest[:])
	if previous, err := service.ports.AttachmentRecords.GetAttachment(ctx, workspaceID, productID, featureID, id); err == nil {
		if previous.Filename == input.Filename && previous.SHA256 == sha && previous.CreatedBy == actor.ID && previous.DeviceID == input.DeviceID {
			return previous, nil
		}
		return attachment.Metadata{}, domain.Invalid("idempotency_key_reused")
	} else if !isNotFound(err) {
		return attachment.Metadata{}, err
	}
	current, err := service.ports.AttachmentRecords.ListAttachments(ctx, workspaceID, productID, featureID)
	if err != nil {
		return attachment.Metadata{}, err
	}
	if len(current) >= attachment.MaxPerFeature {
		return attachment.Metadata{}, domain.Invalid("attachment_limit_exceeded")
	}
	objectKey := attachment.BuildObjectKey(workspaceID, productID, featureID, sha, extension)
	stored, err := service.ports.Attachments.Put(ctx, attachment.PutRequest{
		WorkspaceID: workspaceID, ObjectKey: objectKey, ContentType: contentType,
		Content: input.Data, ContentSHA256: sha, IdempotencyKey: input.ClientID,
	})
	if err != nil {
		return attachment.Metadata{}, err
	}
	metadata := attachment.Metadata{
		ID: id, Filename: input.Filename, ContentType: contentType, Bytes: int64(len(input.Data)),
		SHA256: sha, ContentRef: stored.ContentRef, Active: true, Revision: 1,
		CreatedBy: actor.ID, DeviceID: input.DeviceID, CreatedAt: service.now().UnixMilli(),
	}
	return service.ports.AttachmentRecords.PutAttachment(ctx, workspaceID, productID, featureID, metadata)
}

func (service *Service) ListFeatureAttachments(ctx context.Context, workspaceID, productID, featureID string) ([]attachment.Metadata, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductRead, nil); err != nil {
		return nil, err
	}
	if !conversation.ValidID(productID) || !conversation.ValidID(featureID) {
		return nil, domain.Invalid("request_invalid")
	}
	if _, err := service.ports.Products.GetProduct(ctx, workspaceID, productID); err != nil {
		return nil, err
	}
	return service.ports.AttachmentRecords.ListAttachments(ctx, workspaceID, productID, featureID)
}

func (service *Service) DownloadFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string) (attachment.Download, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductRead, nil); err != nil {
		return attachment.Download{}, err
	}
	if !conversation.ValidID(productID) || !conversation.ValidID(featureID) || !conversation.ValidID(attachmentID) {
		return attachment.Download{}, domain.Invalid("request_invalid")
	}
	metadata, err := service.ports.AttachmentRecords.GetAttachment(ctx, workspaceID, productID, featureID, attachmentID)
	if err != nil {
		return attachment.Download{}, err
	}
	content, err := service.ports.Attachments.Get(ctx, workspaceID, metadata.ContentRef)
	if err != nil {
		return attachment.Download{}, err
	}
	digest := sha256.Sum256(content)
	if int64(len(content)) != metadata.Bytes || hex.EncodeToString(digest[:]) != metadata.SHA256 {
		return attachment.Download{}, domain.Invalid("attachment_content_corrupt")
	}
	return attachment.Download{Attachment: metadata, Data: base64.StdEncoding.EncodeToString(content)}, nil
}

func (service *Service) RemoveFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID, clientID, deviceID string, expectedRevision uint64) (attachment.Metadata, error) {
	if _, err := authenticatedActor(ctx, workspaceID, PermissionProductWrite, nil); err != nil {
		return attachment.Metadata{}, err
	}
	if !conversation.ValidID(productID) || !conversation.ValidID(featureID) || !conversation.ValidID(attachmentID) || !conversation.ValidID(clientID) || !conversation.ValidID(deviceID) || expectedRevision == 0 {
		return attachment.Metadata{}, domain.Invalid("request_invalid")
	}
	return service.ports.AttachmentRecords.RemoveAttachment(ctx, workspaceID, productID, featureID, attachmentID, clientID, deviceID, expectedRevision)
}

func invalidFilename(name string) bool {
	return name == "" || len(name) > 255 || strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\\x00\r\n") || name == "." || name == ".."
}

func isNotFound(err error) bool {
	var domainError *domain.Error
	return errors.As(err, &domainError) && domainError.Code == "not_found"
}
