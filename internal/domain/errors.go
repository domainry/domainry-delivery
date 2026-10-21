package domain

import (
	"strings"

	"github.com/domainry/domainry-foundation/apperror"
)

type Error struct {
	Class   apperror.ErrorKind `json:"-"`
	Code    string             `json:"code"`
	Message string             `json:"message"`
	Details map[string]any     `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func (e *Error) ApplicationErrorKind() string {
	if e == nil {
		return ""
	}
	return string(apperror.NormalizeKind(string(e.Class)))
}

func (e *Error) ErrorCode() string {
	if e == nil {
		return ""
	}
	code := strings.TrimSpace(e.Code)
	if code == "" {
		return ""
	}
	if strings.Contains(code, ".") {
		return code
	}
	return "delivery." + code
}

func (*Error) ErrorParams() map[string]string { return nil }

func Invalid(code string) error {
	return &Error{Class: apperror.KindBadRequest, Code: code}
}

func Forbidden(code string) error {
	return &Error{Class: apperror.KindForbidden, Code: code}
}

func ConflictCode(code string) error {
	return &Error{Class: apperror.KindConflict, Code: code}
}

func Internal(code string, details map[string]any) error {
	return &Error{Class: apperror.KindInternal, Code: code, Details: details}
}

func NotFound(entity, id string) error {
	return &Error{Class: apperror.KindNotFound, Code: "not_found", Details: map[string]any{"entity": entity, "id": id}}
}

func Conflict(actual uint64) error {
	return &Error{Class: apperror.KindConflict, Code: "revision_conflict", Details: map[string]any{"actual_revision": actual}}
}
