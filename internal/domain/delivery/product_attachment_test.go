package delivery_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain/delivery"
)

func TestFeatureAttachmentIsFrozenIntoConfirmedRevision(t *testing.T) {
	product := newProduct(t)
	mustApplyProduct(t, &product, agent("pm-agent"), "feature.discovery.replace", featureDraftPayload("feature-cancel", "F-001"))
	attachment, err := delivery.AddFeatureAttachment(&product, "feature-cancel", delivery.FeatureAttachmentInput{
		ID: "attachment-quote", Name: "supplier-quote.pdf", MediaType: "application/pdf", SizeBytes: 128,
		SHA256: strings.Repeat("a", 64),
	}, human("product-owner"), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if attachment.ContentRef != "delivery-attachment://attachment-quote" || len(product.Features[0].Attachments) != 1 {
		t.Fatalf("attachment metadata was not retained: %#v", product.Features[0].Attachments)
	}

	mustApplyProduct(t, &product, human("product-owner"), "feature.confirm", map[string]any{"feature_id": "feature-cancel", "draft_version": 1})
	confirmed := product.Features[0]
	if len(confirmed.Revisions[0].Attachments) != 1 || confirmed.Revisions[0].Attachments[0] != attachment {
		t.Fatalf("confirmed FeatureRevision lost its attachment: %#v", confirmed.Revisions[0].Attachments)
	}
	_, err = delivery.RemoveFeatureAttachment(&product, "feature-cancel", "attachment-quote", time.Now().UTC())
	assertCode(t, err, "feature_locked")
}

func TestFeatureAttachmentLimitIsEnforcedByDelivery(t *testing.T) {
	product := newProduct(t)
	mustApplyProduct(t, &product, human("product-owner"), "feature.discovery.open", map[string]any{"feature_id": "feature-quotes"})
	for index := 0; index < delivery.MaxFeatureAttachments; index++ {
		_, err := delivery.AddFeatureAttachment(&product, "feature-quotes", delivery.FeatureAttachmentInput{
			ID: fmt.Sprintf("attachment-%d", index), Name: fmt.Sprintf("quote-%d.pdf", index), MediaType: "application/pdf", SizeBytes: 128,
			SHA256: strings.Repeat(fmt.Sprintf("%x", index%16), 64),
		}, human("product-owner"), time.Now().UTC())
		if err != nil {
			t.Fatalf("attachment %d failed: %v", index, err)
		}
	}
	_, err := delivery.AddFeatureAttachment(&product, "feature-quotes", delivery.FeatureAttachmentInput{
		ID: "attachment-over-limit", Name: "extra.pdf", MediaType: "application/pdf", SizeBytes: 128,
		SHA256: strings.Repeat("f", 64),
	}, human("product-owner"), time.Now().UTC())
	assertCode(t, err, "attachment_limit_reached")
}

func TestFeatureAttachmentRejectsUnsafeMetadata(t *testing.T) {
	product := newProduct(t)
	mustApplyProduct(t, &product, human("product-owner"), "feature.discovery.open", map[string]any{"feature_id": "feature-quotes"})
	_, err := delivery.AddFeatureAttachment(&product, "feature-quotes", delivery.FeatureAttachmentInput{
		ID: "attachment-injected", Name: "quote\r\nX-Injected: yes.pdf", MediaType: "application/pdf", SizeBytes: 128,
		SHA256: strings.Repeat("a", 64),
	}, human("product-owner"), time.Now().UTC())
	assertCode(t, err, "attachment_name_invalid")

	_, err = delivery.AddFeatureAttachment(&product, "feature-quotes", delivery.FeatureAttachmentInput{
		ID: "attachment-injected", Name: "quote.pdf", MediaType: "application/pdf\r\nX-Injected: yes", SizeBytes: 128,
		SHA256: strings.Repeat("a", 64),
	}, human("product-owner"), time.Now().UTC())
	assertCode(t, err, "attachment_media_type_invalid")
}
