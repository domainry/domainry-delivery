package delivery

import (
	"fmt"
	"mime"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const MaxFeatureAttachmentBytes int64 = 20 << 20
const MaxFeatureAttachments = 10

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type FeatureAttachmentInput struct {
	ID        string
	Name      string
	MediaType string
	SizeBytes int64
	SHA256    string
}

func AddFeatureAttachment(product *Product, featureID string, input FeatureAttachmentInput, actor Actor, now time.Time) (FeatureAttachment, error) {
	feature := findFeature(product, strings.TrimSpace(featureID))
	if feature == nil {
		return FeatureAttachment{}, NotFound("Feature", featureID)
	}
	if feature.Status != FeatureDraft || feature.Draft == nil {
		return FeatureAttachment{}, Invalid("feature_locked", "Attachments can only be changed while the Feature is being discovered.")
	}
	attachment, err := newFeatureAttachment(input, actor, now)
	if err != nil {
		return FeatureAttachment{}, err
	}
	for _, existing := range feature.Attachments {
		if existing.ID == attachment.ID {
			if existing.SHA256 == attachment.SHA256 {
				return existing, nil
			}
			return FeatureAttachment{}, Invalid("attachment_id_reused", "An attachment ID cannot identify different content.")
		}
	}
	if len(feature.Attachments) >= MaxFeatureAttachments {
		return FeatureAttachment{}, Invalid("attachment_limit_reached", fmt.Sprintf("A Feature supports no more than %d attachments.", MaxFeatureAttachments))
	}
	feature.Attachments = append(feature.Attachments, attachment)
	feature.UpdatedAt = now
	product.UpdatedAt = now
	return attachment, nil
}

func RemoveFeatureAttachment(product *Product, featureID, attachmentID string, now time.Time) (FeatureAttachment, error) {
	feature := findFeature(product, strings.TrimSpace(featureID))
	if feature == nil {
		return FeatureAttachment{}, NotFound("Feature", featureID)
	}
	if feature.Status != FeatureDraft || feature.Draft == nil {
		return FeatureAttachment{}, Invalid("feature_locked", "Attachments can only be changed while the Feature is being discovered.")
	}
	attachmentID = strings.TrimSpace(attachmentID)
	for index, attachment := range feature.Attachments {
		if attachment.ID == attachmentID {
			feature.Attachments = append(feature.Attachments[:index], feature.Attachments[index+1:]...)
			feature.UpdatedAt = now
			product.UpdatedAt = now
			return attachment, nil
		}
	}
	return FeatureAttachment{}, NotFound("Feature attachment", attachmentID)
}

func newFeatureAttachment(input FeatureAttachmentInput, actor Actor, now time.Time) (FeatureAttachment, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = strings.TrimSpace(input.Name)
	input.MediaType = strings.TrimSpace(input.MediaType)
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	if input.ID == "" || input.Name == "" || input.MediaType == "" || strings.TrimSpace(actor.ID) == "" {
		return FeatureAttachment{}, Invalid("attachment_incomplete", "An attachment requires an ID, file name, media type, and authenticated owner.")
	}
	if filepath.Base(input.Name) != input.Name || strings.ContainsAny(input.Name, "/\\") || strings.IndexFunc(input.Name, unicode.IsControl) >= 0 {
		return FeatureAttachment{}, Invalid("attachment_name_invalid", "The attachment file name is invalid.")
	}
	parsedMediaType, _, err := mime.ParseMediaType(input.MediaType)
	if err != nil || !strings.Contains(parsedMediaType, "/") {
		return FeatureAttachment{}, Invalid("attachment_media_type_invalid", "The attachment media type is invalid.")
	}
	if input.SizeBytes <= 0 || input.SizeBytes > MaxFeatureAttachmentBytes {
		return FeatureAttachment{}, Invalid("attachment_size_invalid", fmt.Sprintf("An attachment must be between 1 byte and %d bytes.", MaxFeatureAttachmentBytes))
	}
	if !sha256Pattern.MatchString(input.SHA256) {
		return FeatureAttachment{}, Invalid("attachment_hash_invalid", "The attachment SHA-256 digest is invalid.")
	}
	return FeatureAttachment{
		ID: input.ID, Name: input.Name, MediaType: input.MediaType, SizeBytes: input.SizeBytes,
		SHA256: input.SHA256, ContentRef: "delivery-attachment://" + input.ID,
		UploadedBy: actor.ID, CreatedAt: now,
	}, nil
}
