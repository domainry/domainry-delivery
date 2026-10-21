package database

import (
	"github.com/domainry/domainry-delivery/internal/domain"
)

func storageError(err error) error {
	return domain.Internal("storage_failure", map[string]any{"cause": err.Error()})
}
