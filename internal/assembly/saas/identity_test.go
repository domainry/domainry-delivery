package saas

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	bridgemodule "github.com/domainry/domainry-identity-bridge/module"
	identity "github.com/domainry/domainry-identity-sdk"
)

func TestExternalIdentityCreatesOnePersistentWorkspacePerVerdentUser(t *testing.T) {
	provider := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input map[string]string
		if request.Method != http.MethodPost || request.URL.Path != "/passport/token/validate" || json.NewDecoder(request.Body).Decode(&input) != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		subject := "1001"
		if input["token"] == "user-b" {
			subject = "1002"
		} else if input["token"] != "user-a" {
			_ = json.NewEncoder(writer).Encode(map[string]any{"errCode": 100003, "data": nil})
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"errCode": 0, "data": map[string]any{
			"valid": true, "user_id": subject, "email": subject + "@example.test", "token_type": "access", "expires_at": time.Now().Add(time.Hour).Unix(),
		}})
	}))
	defer provider.Close()

	directory := t.TempDir()
	t.Setenv("DELIVERY_ATTACHMENT_STORAGE_PATH", filepath.Join(directory, "attachments"))
	databasePath := filepath.Join(directory, "delivery.db")
	configPath := filepath.Join(directory, "identity.json")
	configuration := map[string]any{
		"version": "domainry-identity-bridge-config-v1", "installation_id": "delivery-test", "application_key": "delivery-test",
		"provider": map[string]any{"key": "verdent-account", "verification": map[string]any{
			"kind": "http_introspection", "endpoint": provider.URL + "/passport/token/validate", "method": "POST", "timeout": "1s", "max_response_bytes": 65536,
			"credential": map[string]any{"location": "json", "name": "token"},
			"response": map[string]any{"subject_id": "/data/user_id", "display_name": "/data/email", "email": "/data/email", "expires_at": "/data/expires_at", "expiry_format": "unix_seconds", "checks": []map[string]any{
				{"path": "/errCode", "equals": 0}, {"path": "/data/valid", "equals": true}, {"path": "/data/token_type", "equals": "access"},
			}},
		}},
		"personal_workspace": map[string]any{"mode": "per_user", "create_on_first_access": true, "name_template": "Delivery workspace", "initial_role_keys": []string{"delivery_owner"}},
	}
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	var applicationRuntime *Application
	var external ExternalIdentity
	open := func() {
		var openErr error
		applicationRuntime, openErr = Open(t.Context(), DatabaseConfig{Driver: "sqlite", SQLitePath: databasePath})
		if openErr != nil {
			t.Fatal(openErr)
		}
		external, openErr = applicationRuntime.openExternalIdentity(t.Context(), configPath, bridgemodule.Options{Transport: provider.Client().Transport})
		if openErr != nil {
			t.Fatal(openErr)
		}
	}
	authenticate := func(token string) identity.Principal {
		t.Helper()
		source, ok := external.Binding.(identity.PrincipalAuthenticationBinding)
		if !ok {
			t.Fatal("external binding has no principal authenticator")
		}
		principal, authErr := source.PrincipalAuthenticator().Authenticate(t.Context(), token)
		if authErr != nil {
			t.Fatal(authErr)
		}
		return principal
	}

	open()
	firstA, firstB := authenticate("user-a"), authenticate("user-b")
	if firstA.WorkspaceID == firstB.WorkspaceID || firstA.UserID == firstB.UserID || firstA.WorkspaceID == "delivery-test" {
		t.Fatalf("external users share identity scope: a=%+v b=%+v", firstA, firstB)
	}
	if !firstA.HasPermission("delivery_product.write") || !firstA.HasPermission("delivery_deployment.record") {
		t.Fatalf("Delivery role was not provisioned: %v", firstA.Permissions)
	}
	ctx := identity.WithRequestIdentity(t.Context(), identity.RequestIdentity{Principal: firstA, AccessToken: "user-a"})
	session, err := applicationRuntime.Service.Session(ctx, firstA.WorkspaceID)
	if err != nil || session.WorkspaceID != firstA.WorkspaceID || session.Actor.ID != firstA.UserID {
		t.Fatalf("Delivery session does not use bridge principal: %+v %v", session, err)
	}
	if err = external.Binding.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = applicationRuntime.Close(); err != nil {
		t.Fatal(err)
	}
	open()
	defer applicationRuntime.Close()
	defer external.Binding.Close(t.Context())
	secondA := authenticate("user-a")
	if secondA.WorkspaceID != firstA.WorkspaceID || secondA.UserID != firstA.UserID {
		t.Fatalf("external ownership changed after restart: first=%+v second=%+v", firstA, secondA)
	}
}
