package contracttest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	deliverysdk "github.com/domainry/domainry-delivery-sdk"
	"github.com/domainry/domainry-delivery-sdk/modulehost"
	deliveryremote "github.com/domainry/domainry-delivery-sdk/remote"
	ormdialect "github.com/domainry/domainry-orm/dialect"
	ormmigration "github.com/domainry/domainry-orm/migration"
	"github.com/domainry/domainry-orm/sqlhost"

	"github.com/domainry/domainry-delivery/internal/application"
	delivery "github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/infrastructure/persistence/sqlite"
	httpapi "github.com/domainry/domainry-delivery/internal/transport/http"
	deliverymodule "github.com/domainry/domainry-delivery/module"
	_ "modernc.org/sqlite"
)

const contractWorkspaceID = "workspace-contract"

type databaseHost struct {
	database   *sql.DB
	dialect    modulehost.Dialect
	migrations *migrationRegistrar
}

type migrationRegistrar struct{ database *sql.DB }

func (host databaseHost) RuntimeID() string                         { return "delivery-contract-test" }
func (host databaseHost) Database() sqlhost.Database                { return host.database }
func (host databaseHost) Dialect() modulehost.Dialect               { return host.dialect }
func (host databaseHost) Migrations() modulehost.MigrationRegistrar { return host.migrations }
func (registrar *migrationRegistrar) Driver() string                { return "sqlite" }
func (registrar *migrationRegistrar) Schema() string                { return "" }
func (registrar *migrationRegistrar) ApplyOwnedMigrations(ctx context.Context, _ string, migrations []ormmigration.Migration) error {
	for _, migration := range migrations {
		for _, statement := range migration.Statements {
			if _, err := registrar.database.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestModuleAndSaaSBindingsShareOneContract(t *testing.T) {
	moduleDatabase, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "module.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer moduleDatabase.Close()
	dialect, err := ormdialect.New(ormdialect.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	host := databaseHost{database: moduleDatabase, dialect: dialect.WithSchema(""), migrations: &migrationRegistrar{database: moduleDatabase}}
	moduleBinding, err := deliverymodule.NewFactory().OpenModule(t.Context(), deliverysdk.ApplicationRef{RuntimeID: "delivery-contract-test"}, host)
	if err != nil {
		t.Fatal(err)
	}

	saasStore, err := sqlite.Open(filepath.Join(t.TempDir(), "saas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer saasStore.Close()
	saasHandler := httpapi.New(application.NewService(application.Ports{
		Products: saasStore, Runs: saasStore, Lifecycle: saasStore,
	}), slog.New(slog.NewTextHandler(io.Discard, nil)), "delivery-contract-test")
	server := httptest.NewServer(contractAuthentication(saasHandler))
	defer server.Close()
	remoteBinding, err := deliveryremote.Open(t.Context(), deliveryremote.Config{BaseURL: server.URL, RuntimeID: "delivery-contract-test", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	moduleDescriptor := moduleBinding.Descriptor()
	remoteDescriptor := remoteBinding.Descriptor()
	if err := moduleDescriptor.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := remoteDescriptor.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(moduleDescriptor.Capabilities, remoteDescriptor.Capabilities) {
		t.Fatalf("capabilities differ: module=%v saas=%v", moduleDescriptor.Capabilities, remoteDescriptor.Capabilities)
	}
	if !reflect.DeepEqual(moduleDescriptor.SupportedLocales, remoteDescriptor.SupportedLocales) {
		t.Fatalf("supported locales differ: module=%v saas=%v", moduleDescriptor.SupportedLocales, remoteDescriptor.SupportedLocales)
	}

	moduleContext := deliverysdk.WithLocale(application.WithTrustedPrincipal(
		context.Background(), contractWorkspaceID, delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
		application.PermissionProductRead, application.PermissionProductWrite,
	), "fr")
	remoteContext := deliverysdk.WithLocale(deliverysdk.WithAccessToken(context.Background(), "owner-token"), "fr")
	cases := []struct {
		name    string
		binding deliverysdk.Binding
		ctx     context.Context
	}{
		{name: "module", binding: moduleBinding, ctx: moduleContext},
		{name: "saas", binding: remoteBinding, ctx: remoteContext},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			session, err := testCase.binding.Session(testCase.ctx, contractWorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if session.WorkspaceID != contractWorkspaceID || session.Actor.ID != "owner" || session.Actor.Kind != deliverysdk.ActorHuman {
				t.Fatalf("authenticated session differs: %#v", session)
			}
			created, err := testCase.binding.DispatchProduct(testCase.ctx, contractWorkspaceID, "product-contract", productCreateCommand())
			if err != nil {
				t.Fatal(err)
			}
			if created.Product.Revision != 1 || created.Product.Revisions[0].CreatedBy != "owner" {
				t.Fatalf("authenticated actor did not override caller claims: %#v", created.Product.Revisions[0])
			}
			listed, err := testCase.binding.ListProducts(testCase.ctx, contractWorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if len(listed) != 1 || listed[0].Product.ID != "product-contract" || !reflect.DeepEqual(listed[0].AvailableActions, created.AvailableActions) {
				t.Fatalf("list projection differs from command projection: %#v", listed)
			}
			_, err = testCase.binding.DispatchProduct(testCase.ctx, contractWorkspaceID, "product-contract", deliverysdk.Command{
				ClientID: "unknown-command", ExpectedRevision: 1, Type: "product.unsupported", Payload: json.RawMessage(`{}`),
			})
			var deliveryError *deliverysdk.Error
			if !errors.As(err, &deliveryError) || deliveryError.Code != "command_unknown" || deliveryError.Message != "La demande ne respecte pas les règles Delivery actuelles." {
				t.Fatalf("error semantics differ: %#v", err)
			}

			opened, err := testCase.binding.DispatchProduct(testCase.ctx, contractWorkspaceID, "product-contract", featureOpenCommand())
			if err != nil {
				t.Fatal(err)
			}
			if opened.Product.Revision != 2 || len(opened.Product.Features) != 1 || opened.Product.Features[0].ID != "feature-inquiry" {
				t.Fatalf("Feature discovery differs: %#v", opened.Product.Features)
			}

		})
	}

	if err := moduleBinding.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := moduleDatabase.PingContext(t.Context()); err != nil {
		t.Fatalf("Delivery Module closed the host database: %v", err)
	}
}

func productCreateCommand() deliverysdk.Command {
	payload, _ := json.Marshal(map[string]any{
		"name": "Contract Product", "code": "CONTRACT", "goal": "Verify deployment-neutral behavior", "industry": "Professional services",
		"story": map[string]any{
			"title": "Contract Product", "summary": "One contract", "narrative": "Module and SaaS execute the same Delivery domain behavior.",
		},
		"definition": map[string]any{
			"schema_version": 2, "actors": []any{}, "scenarios": []any{}, "objects": []any{}, "rules": []any{},
			"exceptions": []any{}, "actions": []any{}, "pages": []any{},
		},
		"decisions": []any{},
	})
	return deliverysdk.Command{
		ClientID: "create-product", ExpectedRevision: 0,
		Type: "product.create", Payload: payload,
	}
}

func featureOpenCommand() deliverysdk.Command {
	payload, _ := json.Marshal(map[string]any{"feature_id": "feature-inquiry"})
	return deliverysdk.Command{
		ClientID: "open-feature", ExpectedRevision: 1,
		Type: "feature.discovery.open", Payload: payload,
	}
}

func contractAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/healthz" || request.URL.Path == "/api/v1/delivery/descriptor" {
			next.ServeHTTP(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer owner-token" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		ctx := application.WithTrustedPrincipal(
			request.Context(), contractWorkspaceID, delivery.Actor{ID: "owner", Kind: delivery.ActorHuman},
			application.PermissionProductRead, application.PermissionProductWrite,
			application.PermissionDeliveryRunRead, application.PermissionDeliveryRunWrite,
		)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}
