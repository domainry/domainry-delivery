package database

import (
	"fmt"
	"time"

	"github.com/domainry/domainry-delivery/internal/domain"
)

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func storageError(err error) error {
	return &domain.Error{Code: "storage_failure", Message: "Delivery data storage failed.", Details: map[string]any{"cause": err.Error()}}
}

func parseStoredTime(value string) (time.Time, error) {
	result, err := time.Parse(timeFormat, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid stored time: %w", err)
	}
	return result, nil
}
