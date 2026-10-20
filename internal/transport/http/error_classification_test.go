package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/domainry/domainry-delivery/internal/domain"
)

type classificationCapture struct {
	*httptest.ResponseRecorder
	kind, code string
}

func (capture *classificationCapture) RecordError(kind, code string) {
	capture.kind, capture.code = kind, code
}

func TestDeliveryErrorClassificationMatrix(t *testing.T) {
	tests := []struct{ source, kind string }{
		{"storage_failure", "internal"},
		{"revision_conflict", "conflict"},
		{"request_invalid", "bad_request"},
		{"permission_denied", "forbidden"},
	}
	for _, test := range tests {
		capture := &classificationCapture{ResponseRecorder: httptest.NewRecorder()}
		new(Handler).writeError(capture, httptest.NewRequest("GET", "/", nil), &domain.Error{Code: test.source})
		if capture.kind != test.kind || capture.code != "delivery."+test.source {
			t.Fatalf("source=%q kind=%q code=%q", test.source, capture.kind, capture.code)
		}
	}
}
