package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	deliverydb "github.com/domainry/domainry-delivery/internal/infrastructure/persistence/database"
	_ "modernc.org/sqlite"
)

func Open(path string) (*deliverydb.Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	for _, statement := range []string{"PRAGMA journal_mode = WAL", "PRAGMA foreign_keys = ON", "PRAGMA busy_timeout = 5000"} {
		if _, err := database.Exec(statement); err != nil {
			_ = database.Close()
			return nil, err
		}
	}
	return deliverydb.New(context.Background(), database, "sqlite", true)
}
