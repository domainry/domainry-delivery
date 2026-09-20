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

	identitysdk "github.com/domainry/domainry-identity-sdk"
	identityprincipal "github.com/domainry/domainry-identity-sdk/authorization/principal"
	identityhttpmiddleware "github.com/domainry/domainry-identity-sdk/httpmiddleware"
	identityremote "github.com/domainry/domainry-identity-sdk/remote"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/infrastructure/sqlite"
	"github.com/domainry/domainry-delivery/internal/transport/httpapi"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	databasePath := env("DELIVERY_DB", "data/domainry-delivery.db")
	store, err := sqlite.Open(databasePath)
	if err != nil {
		logger.Error("open delivery store", "error", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	service := application.NewService(store)
	deliveryHandler := httpapi.New(service, logger)
	serverAddr := env("DELIVERY_ADDR", "127.0.0.1:8096")
	authenticatedHandler, closeIdentity, localDevelopment, err := authenticatedDeliveryHandler(serverAddr, deliveryHandler)
	if err != nil {
		logger.Error("configure Delivery identity", "error", err)
		os.Exit(1)
	}
	defer closeIdentity()
	if localDevelopment && enabled("DELIVERY_DEV_SEED") {
		if err := service.EnsureProduct(context.Background(), application.DemoProduct(time.Now().UTC())); err != nil {
			logger.Error("seed local development Product", "error", err)
			os.Exit(1)
		}
	}

	server := &http.Server{
		Addr:              serverAddr,
		Handler:           publicDeliveryRoutes(deliveryHandler, authenticatedHandler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		logger.Info("domainry-delivery listening", "addr", server.Addr, "database", databasePath)
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

func authenticatedDeliveryHandler(serverAddr string, handler http.Handler) (http.Handler, func(), bool, error) {
	development, enabled, err := developmentIdentityFromEnvironment(serverAddr)
	if err != nil {
		return nil, func() {}, false, err
	}
	if enabled {
		return development.Authenticate(handler), func() {}, true, nil
	}
	identityConfig := identityremote.ConfigFromEnvironment()
	identityBinding, err := identityremote.NewFactory(identityConfig).Open(context.Background(), identitysdk.ApplicationRef{
		WorkspaceID:    identitysdk.WorkspaceID(identityConfig.WorkspaceID),
		ApplicationKey: identitysdk.ApplicationKey(identityConfig.Audience),
	})
	if err != nil {
		return nil, func() {}, false, err
	}
	authenticator, err := identityprincipal.NewAuthenticator(identityBinding, identityprincipal.Options{})
	if err != nil {
		_ = identityBinding.Close(context.Background())
		return nil, func() {}, false, err
	}
	identityMiddleware, err := identityhttpmiddleware.New(
		authenticator,
		identityhttpmiddleware.WithAuthorization(identityBinding.Authorization()),
		identityhttpmiddleware.WithBindingCredential(identityBinding),
	)
	if err != nil {
		_ = identityBinding.Close(context.Background())
		return nil, func() {}, false, err
	}
	closeIdentity := func() { _ = identityBinding.Close(context.Background()) }
	return identityMiddleware.Authenticate(identityMiddleware.RequirePasswordChanged(handler)), closeIdentity, false, nil
}

func publicDeliveryRoutes(public, authenticated http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/healthz" || request.URL.Path == "/api/v1/delivery/descriptor" {
			public.ServeHTTP(writer, request)
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
