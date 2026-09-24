package domain

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

func Invalid(code string) error {
	return &Error{Code: code}
}

func NotFound(entity, id string) error {
	return &Error{Code: "not_found", Details: map[string]any{"entity": entity, "id": id}}
}

func Conflict(actual uint64) error {
	return &Error{Code: "revision_conflict", Details: map[string]any{"actual_revision": actual}}
}
