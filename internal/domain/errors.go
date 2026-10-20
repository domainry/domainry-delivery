package domain

import "strings"

type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
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
	switch e.Code {
	case "authentication_required", "permission_denied", "identity_workspace_mismatch", "actor_unauthorized":
		return "forbidden"
	case "not_found":
		return "not_found"
	case "revision_conflict", "idempotency_key_reused":
		return "conflict"
	case "storage_failure":
		return "internal"
	default:
		return "bad_request"
	}
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
	return &Error{Code: code}
}

func NotFound(entity, id string) error {
	return &Error{Code: "not_found", Details: map[string]any{"entity": entity, "id": id}}
}

func Conflict(actual uint64) error {
	return &Error{Code: "revision_conflict", Details: map[string]any{"actual_revision": actual}}
}
