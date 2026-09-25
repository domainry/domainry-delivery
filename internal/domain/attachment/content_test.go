package attachment

import (
	"testing"
	"time"
)

func TestBuildObjectKeyUsesUTCUploadDay(t *testing.T) {
	local := time.Date(2026, time.September, 25, 7, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	got := BuildObjectKey(local, "attachment-123", "ABCDEF", ".pdf")
	want := "20260924/attachment-123-abcdef.pdf"
	if got != want {
		t.Fatalf("S3 object key = %q, want %q", got, want)
	}
}

func TestClassifyContentAcceptsUTF8TextWithoutCharsetInMediaType(t *testing.T) {
	contentType, extension, supported := ClassifyContent([]byte("Fictional Delivery attachment test.\n"))
	if !supported || contentType != "text/plain" || extension != ".txt" {
		t.Fatalf("ClassifyContent() = (%q, %q, %t), want (%q, %q, true)", contentType, extension, supported, "text/plain", ".txt")
	}
}
