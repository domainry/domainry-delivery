package delivery

import "fmt"

type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func Invalid(code, message string) error {
	return &Error{Code: code, Message: message}
}

func NotFound(entity, id string) error {
	return &Error{Code: "not_found", Message: fmt.Sprintf("%s was not found: %s", entity, id)}
}

func Conflict(actual uint64) error {
	return &Error{Code: "revision_conflict", Message: "The revision changed; read the resource again before submitting.", Details: map[string]any{"actual_revision": actual}}
}
