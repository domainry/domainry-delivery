package saas

import (
	"context"
	"fmt"
	"strings"

	agentsdk "github.com/domainry/domainry-agent-sdk"
	"github.com/domainry/domainry-delivery/internal/application"
	deliverydb "github.com/domainry/domainry-delivery/internal/infrastructure/persistence/database"
	deliverymysql "github.com/domainry/domainry-delivery/internal/infrastructure/persistence/mysql"
	deliverysqlite "github.com/domainry/domainry-delivery/internal/infrastructure/persistence/sqlite"
)

type DatabaseConfig struct {
	Driver          string
	MySQLDSN        string
	SQLitePath      string
	SourceRuntimeID string
	Sources         agentsdk.ConversationSourceVerifier
}

type Application struct {
	Service *application.Service
	store   *deliverydb.Store
}

func Open(ctx context.Context, config DatabaseConfig) (*Application, error) {
	driver := strings.ToLower(strings.TrimSpace(config.Driver))
	var store *deliverydb.Store
	var err error
	switch driver {
	case "mysql":
		store, err = deliverymysql.Open(ctx, deliverymysql.Config{DSN: strings.TrimSpace(config.MySQLDSN)})
	case "sqlite":
		store, err = deliverysqlite.Open(strings.TrimSpace(config.SQLitePath))
	default:
		return nil, fmt.Errorf("unsupported DELIVERY_DB_DRIVER %q", config.Driver)
	}
	if err != nil {
		return nil, err
	}
	return &Application{Service: application.NewService(application.Ports{
		Products: store, Runs: store, Lifecycle: store, Sources: config.Sources, SourceRuntimeID: strings.TrimSpace(config.SourceRuntimeID),
	}), store: store}, nil
}

func (application *Application) Close() error {
	if application == nil || application.store == nil {
		return nil
	}
	return application.store.Close()
}
