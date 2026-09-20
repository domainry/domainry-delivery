package contracttest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	deliverysdk "github.com/domainry/domainry-delivery"
	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain/delivery"
	"github.com/domainry/domainry-delivery/internal/infrastructure/sqlite"
	"github.com/domainry/domainry-delivery/internal/transport/httpapi"
	deliverymodule "github.com/domainry/domainry-delivery/module"
	deliveryremote "github.com/domainry/domainry-delivery/remote"
	_ "modernc.org/sqlite"
)

const contractWorkspaceID = "workspace-contract"

type databaseHost struct{ database *sql.DB }

func (host databaseHost) Database() *sql.DB { return host.database }

func TestModuleAndSaaSBindingsShareOneContract(t *testing.T) {
	moduleDatabase, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "module.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer moduleDatabase.Close()
	moduleBinding, err := deliverymodule.Open(t.Context(), databaseHost{database: moduleDatabase})
	if err != nil {
		t.Fatal(err)
	}

	saasStore, err := sqlite.Open(filepath.Join(t.TempDir(), "saas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer saasStore.Close()
	saasHandler := httpapi.New(application.NewService(saasStore), slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(contractAuthentication(saasHandler))
	defer server.Close()
	remoteBinding, err := deliveryremote.Open(t.Context(), deliveryremote.Config{BaseURL: server.URL, Client: server.Client()})
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
			if session.WorkspaceID != contractWorkspaceID || session.Actor.ID != "owner" || session.Actor.Kind != delivery.ActorHuman {
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

			content := []byte("supplier,item,price\nAcme,Widget,120\n")
			uploaded, err := testCase.binding.UploadFeatureAttachment(testCase.ctx, contractWorkspaceID, "product-contract", "feature-inquiry", deliverysdk.FeatureAttachmentUpload{
				AttachmentID: "attachment-quote", FileName: "supplier-quote.csv", MediaType: "text/csv", ExpectedRevision: 2, Content: content,
			})
			if err != nil {
				t.Fatal(err)
			}
			expectedDigest := fmt.Sprintf("%x", sha256.Sum256(content))
			if uploaded.Product.Product.Revision != 3 || uploaded.Attachment.Name != "supplier-quote.csv" || uploaded.Attachment.SHA256 != expectedDigest || uploaded.Attachment.ContentRef != "delivery-attachment://attachment-quote" || uploaded.Attachment.UploadedBy != "owner" {
				t.Fatalf("attachment upload differs: %#v", uploaded)
			}

			downloaded, err := testCase.binding.DownloadFeatureAttachment(testCase.ctx, contractWorkspaceID, "product-contract", "feature-inquiry", "attachment-quote")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(downloaded.Attachment, uploaded.Attachment) || !reflect.DeepEqual(downloaded.Content, content) {
				t.Fatalf("attachment download differs: %#v", downloaded)
			}

			removed, err := testCase.binding.RemoveFeatureAttachment(testCase.ctx, contractWorkspaceID, "product-contract", "feature-inquiry", "attachment-quote", 3)
			if err != nil {
				t.Fatal(err)
			}
			if removed.Product.Revision != 4 || len(removed.Product.Features[0].Attachments) != 0 {
				t.Fatalf("attachment removal differs: %#v", removed.Product.Features[0].Attachments)
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
		Actor: deliverysdk.Actor{ID: "spoofed-client", Kind: delivery.ActorSystem},
		Type:  "product.create", Payload: payload,
	}
}

func featureOpenCommand() deliverysdk.Command {
	payload, _ := json.Marshal(map[string]any{"feature_id": "feature-inquiry"})
	return deliverysdk.Command{
		ClientID: "open-feature", ExpectedRevision: 1,
		Actor: deliverysdk.Actor{ID: "spoofed-client", Kind: delivery.ActorSystem},
		Type:  "feature.discovery.open", Payload: payload,
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
