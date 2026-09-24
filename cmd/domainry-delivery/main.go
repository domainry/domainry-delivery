package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	actioncontract "github.com/domainry/domainry-foundation/action"
	"github.com/domainry/domainry-foundation/modulehttp"
	bridgemodule "github.com/domainry/domainry-identity-bridge/module"
	identityprincipal "github.com/domainry/domainry-identity-sdk/authorization/principal"
	identityhttpmiddleware "github.com/domainry/domainry-identity-sdk/httpmiddleware"

	deliverysaas "github.com/domainry/domainry-delivery/internal/assembly/saas"
	httpapi "github.com/domainry/domainry-delivery/internal/transport/http"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	databaseDriver := env("DELIVERY_DB_DRIVER", "sqlite")
	applicationRuntime, err := deliverysaas.Open(context.Background(), deliverysaas.DatabaseConfig{
		Driver: databaseDriver, MySQLDSN: os.Getenv("DELIVERY_MYSQL_DSN"), SQLitePath: env("DELIVERY_DB", "data/domainry-delivery.db"),
	})
	if err != nil {
		logger.Error("open delivery store", "error", err)
		os.Exit(1)
	}
	defer func() { _ = applicationRuntime.Close() }()

	service := applicationRuntime.Service
	deliveryHandler := httpapi.New(service, logger, env("DELIVERY_RUNTIME_ID", "domainry-delivery-dev"))
	serverAddr := env("DELIVERY_ADDR", "127.0.0.1:8096")
	authenticatedHandler, identityRoutes, closeIdentity, _, err := authenticatedDeliveryHandler(context.Background(), serverAddr, applicationRuntime, deliveryHandler)
	if err != nil {
		logger.Error("configure Delivery identity", "error", err)
		os.Exit(1)
	}
	defer closeIdentity()
	server := &http.Server{
		Addr:              serverAddr,
		Handler:           publicDeliveryRoutes(deliveryHandler, authenticatedHandler, identityRoutes),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		logger.Info("domainry-delivery listening", "addr", server.Addr, "database_driver", databaseDriver)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("delivery server stopped", "error", err)
			os.Exit(1)
		}
	}()

	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
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
