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
