package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	actioncontract "github.com/domainry/domainry-foundation/action"
	"github.com/domainry/domainry-foundation/modulehttp"
	"github.com/domainry/domainry-foundation/processhost"
	bridgemodule "github.com/domainry/domainry-identity-bridge/module"
	identityprincipal "github.com/domainry/domainry-identity-sdk/authorization/principal"
	identityhttpmiddleware "github.com/domainry/domainry-identity-sdk/httpmiddleware"

	deliverysaas "github.com/domainry/domainry-delivery/internal/assembly/saas"
	httpapi "github.com/domainry/domainry-delivery/internal/transport/http"
)

func main() {
	if run() != nil {
		os.Exit(1)
	}
}

func run() (runErr error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	observability, err := processhost.Open(ctx, processhost.Config{ServiceName: "domainry-delivery"})
	if err != nil {
		return err
	}
	defer func() { runErr = observability.Finish(context.Background(), nil, runErr) }()
	databaseDriver := env("DELIVERY_DB_DRIVER", "sqlite")
	applicationRuntime, err := deliverysaas.Open(ctx, deliverysaas.DatabaseConfig{
		Driver: databaseDriver, MySQLDSN: os.Getenv("DELIVERY_MYSQL_DSN"), SQLitePath: env("DELIVERY_DB", "data/domainry-delivery.db"),
	})
	if err != nil {
		return err
	}
	defer func() { _ = applicationRuntime.Close() }()

	service := applicationRuntime.Service
	deliveryHandler := httpapi.New(service, env("DELIVERY_RUNTIME_ID", "domainry-delivery-dev"))
	serverAddr := env("DELIVERY_ADDR", "127.0.0.1:8096")
	authenticatedHandler, identityRoutes, closeIdentity, _, err := authenticatedDeliveryHandler(ctx, serverAddr, applicationRuntime, deliveryHandler)
	if err != nil {
		return err
	}
	defer closeIdentity()
	server := &http.Server{
		Addr:              serverAddr,
		Handler:           observability.Handler("public", publicDeliveryRoutes(deliveryHandler, authenticatedHandler, identityRoutes)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	observability.Started(ctx, map[string]any{"listener_group": "public", "database_driver": databaseDriver})
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		observability.Stopping(context.Background(), map[string]any{"reason": "signal"})
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func authenticatedDeliveryHandler(ctx context.Context, serverAddr string, applicationRuntime *deliverysaas.Application, handler http.Handler) (http.Handler, map[string]http.Handler, func(), bool, error) {
	development, enabled, err := developmentIdentityFromEnvironment(serverAddr)
	if err != nil {
		return nil, nil, func() {}, false, err
	}
	if enabled {
		return development.Authenticate(handler), nil, func() {}, true, nil
	}
	external, err := applicationRuntime.OpenExternalIdentity(ctx, bridgemodule.ConfigPathFromEnvironment())
	if err != nil {
		return nil, nil, func() {}, false, err
	}
	identityBinding := external.Binding
	authenticator, err := identityprincipal.NewAuthenticator(identityBinding, identityprincipal.Options{})
	if err != nil {
		_ = identityBinding.Close(context.Background())
		return nil, nil, func() {}, false, err
	}
	identityMiddleware, err := identityhttpmiddleware.New(
		authenticator,
		identityhttpmiddleware.WithAuthorization(identityBinding.Authorization()),
		identityhttpmiddleware.WithBindingCredential(identityBinding),
	)
	if err != nil {
		_ = identityBinding.Close(context.Background())
		return nil, nil, func() {}, false, err
	}
	provider, ok := identityBinding.(modulehttp.Provider)
	if !ok {
		_ = identityBinding.Close(context.Background())
		return nil, nil, func() {}, false, errors.New("external Identity HTTP adapter is required")
	}
	identityRoutes, err := externalIdentityRoutes(provider, identityMiddleware)
	if err != nil {
		_ = identityBinding.Close(context.Background())
		return nil, nil, func() {}, false, err
	}
	closeIdentity := func() { _ = identityBinding.Close(context.Background()) }
	return identityMiddleware.Authenticate(handler), identityRoutes, closeIdentity, false, nil
}

func externalIdentityRoutes(provider modulehttp.Provider, middleware *identityhttpmiddleware.Middleware) (map[string]http.Handler, error) {
	result := map[string]http.Handler{}
	for _, adapter := range provider.HTTPAdapters() {
		if err := modulehttp.ValidateAdapter(adapter); err != nil {
			return nil, err
		}
		for _, route := range adapter.Routes() {
			pattern := route.Pattern()
			if pattern == "" || result[pattern] != nil {
				return nil, errors.New("external Identity route is invalid or duplicated")
			}
			switch route.Action.Authorization.Strategy {
			case actioncontract.AuthorizationAnonymous:
				result[pattern] = adapter.Handler()
			case actioncontract.AuthorizationAuthenticated:
				result[pattern] = middleware.Authenticate(adapter.Handler())
			default:
				return nil, errors.New("external Identity route has an unsupported authorization strategy")
			}
		}
	}
	return result, nil
}

func publicDeliveryRoutes(public, authenticated http.Handler, identityRoutes map[string]http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/healthz" || request.URL.Path == "/api/v1/delivery/descriptor" {
			public.ServeHTTP(writer, request)
			return
		}
		if identityRoute := identityRoutes[request.Method+" "+request.URL.Path]; identityRoute != nil {
			identityRoute.ServeHTTP(writer, request)
			return
		}
		authenticated.ServeHTTP(writer, request)
	})
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func enabled(key string) bool {
	value := os.Getenv(key)
	return value == "1" || value == "true"
}
