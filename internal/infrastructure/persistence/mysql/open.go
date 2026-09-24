package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	deliverydb "github.com/domainry/domainry-delivery/internal/infrastructure/persistence/database"
	_ "github.com/go-sql-driver/mysql"
)

type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func Open(ctx context.Context, config Config) (*deliverydb.Store, error) {
	if ctx == nil || config.DSN == "" {
		return nil, fmt.Errorf("MySQL context and DSN are required")
	}
	database, err := sql.Open("mysql", config.DSN)
	if err != nil {
		return nil, err
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 20
	}
	if config.MaxIdleConns <= 0 {
		config.MaxIdleConns = 5
	}
	if config.ConnMaxLifetime <= 0 {
		config.ConnMaxLifetime = 5 * time.Minute
	}
	database.SetMaxOpenConns(config.MaxOpenConns)
	database.SetMaxIdleConns(config.MaxIdleConns)
	database.SetConnMaxLifetime(config.ConnMaxLifetime)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return deliverydb.New(ctx, database, "mysql", true)
}
