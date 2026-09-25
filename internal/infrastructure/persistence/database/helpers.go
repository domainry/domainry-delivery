package database

import (
	"github.com/domainry/domainry-delivery/internal/domain"
)

func storageError(err error) error {
	return &domain.Error{Code: "storage_failure", Message: "Delivery data storage failed.", Details: map[string]any{"cause": err.Error()}}
}
