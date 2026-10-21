package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-foundation/apperror"
)

type classificationCapture struct {
	*httptest.ResponseRecorder
	kind, code string
}

func (capture *classificationCapture) RecordError(kind, code string) {
	capture.kind, capture.code = kind, code
}

func TestDeliveryErrorClassificationMatrix(t *testing.T) {
	tests := []struct {
		source string
		class  apperror.ErrorKind
		status int
	}{
		{"storage_failure", apperror.KindInternal, http.StatusInternalServerError},
		{"revision_conflict", apperror.KindConflict, http.StatusConflict},
		{"request_invalid", apperror.KindBadRequest, http.StatusBadRequest},
		{"permission_denied", apperror.KindForbidden, http.StatusForbidden},
	}
	for _, test := range tests {
		capture := &classificationCapture{ResponseRecorder: httptest.NewRecorder()}
		new(Handler).writeError(capture, httptest.NewRequest("GET", "/", nil), &domain.Error{Class: test.class, Code: test.source})
		if capture.Code != test.status || capture.kind != string(test.class) || capture.code != "delivery."+test.source {
			t.Fatalf("source=%q kind=%q code=%q", test.source, capture.kind, capture.code)
		}
	}
}
