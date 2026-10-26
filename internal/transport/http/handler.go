package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	deliverysdk "github.com/domainry/domainry-delivery-sdk"
	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/domain/deliveryrun"
	"github.com/domainry/domainry-delivery/internal/domain/product"
	"github.com/domainry/domainry-delivery/internal/presentation"
	"github.com/domainry/domainry-delivery/internal/utcjson"
	"github.com/domainry/domainry-foundation/apperror"
	"github.com/domainry/domainry-foundation/httpresponse"
)

type Handler struct {
	service   *application.Service
	runtimeID string
}

func New(service *application.Service, runtimeID string) http.Handler {
	handler := &Handler{service: service, runtimeID: runtimeID}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.health)
	mux.HandleFunc("GET /api/v1/delivery/descriptor", handler.descriptor)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/session", handler.session)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products", handler.listProducts)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/archived-products", handler.listArchivedProducts)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products/{productID}", handler.getProduct)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products/{productID}/agent-context", handler.getProductAgentContext)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/products/{productID}/commands", handler.dispatchProduct)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/products/{productID}/features/{featureID}/messages", handler.appendFeatureMessage)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products/{productID}/features/{featureID}/messages", handler.listFeatureMessages)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/products/{productID}/features/{featureID}/attachments", handler.uploadFeatureAttachment)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products/{productID}/features/{featureID}/attachments", handler.listFeatureAttachments)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products/{productID}/features/{featureID}/attachments/{attachmentID}", handler.downloadFeatureAttachment)
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/products/{productID}/features/{featureID}/attachments/{attachmentID}", handler.removeFeatureAttachment)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/products/{productID}/delivery-runs/{deliveryRunID}", handler.startDelivery)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/delivery-runs", handler.listDeliveryRuns)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/delivery-run-summaries", handler.listDeliveryRunSummaries)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/delivery-runs/{deliveryRunID}", handler.getDeliveryRun)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/delivery-runs/{deliveryRunID}/agent-context", handler.getAgentContext)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/delivery-runs/{deliveryRunID}/acceptance-attachments", handler.uploadAcceptanceAttachment)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/delivery-runs/{deliveryRunID}/acceptance-attachments/{attachmentID}", handler.downloadAcceptanceAttachment)
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/delivery-runs/{deliveryRunID}/acceptance-attachments/{attachmentID}", handler.removeAcceptanceAttachment)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/delivery-runs/{deliveryRunID}/commands", handler.dispatch)
	return handler.securityHeaders(mux)
}

func (handler *Handler) session(writer http.ResponseWriter, request *http.Request) {
	session, err := handler.service.Session(request.Context(), request.PathValue("workspaceID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, session)
}

func (handler *Handler) descriptor(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, deliverysdk.NewDescriptor(deliverysdk.DeploymentModeSaaS, handler.runtimeID, presentation.SupportedLocales))
}

func (handler *Handler) listProducts(writer http.ResponseWriter, request *http.Request) {
	products, err := handler.service.ListProducts(request.Context(), request.PathValue("workspaceID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	projections := make([]product.ProductProjection, 0, len(products))
	for _, productState := range products {
		projections = append(projections, product.ProductProjectionFor(productState))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": projections})
}

func (handler *Handler) listArchivedProducts(writer http.ResponseWriter, request *http.Request) {
	products, err := handler.service.ListArchivedProducts(request.Context(), request.PathValue("workspaceID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	projections := make([]product.ProductProjection, 0, len(products))
	for _, productState := range products {
		projections = append(projections, product.ProductProjectionFor(productState))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": projections})
}

func (handler *Handler) listDeliveryRuns(writer http.ResponseWriter, request *http.Request) {
	runs, err := handler.service.ListDeliveryRuns(request.Context(), request.PathValue("workspaceID"), request.URL.Query().Get("product_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	projections := make([]deliveryrun.Projection, 0, len(runs))
	for _, run := range runs {
		projections = append(projections, presentation.ProjectionForLocale(run, requestLocale(request)))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": projections})
}

func (handler *Handler) listDeliveryRunSummaries(writer http.ResponseWriter, request *http.Request) {
	summaries, err := handler.service.ListDeliveryRunSummaries(request.Context(), request.PathValue("workspaceID"), request.URL.Query().Get("product_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": summaries})
}

func (handler *Handler) getProduct(writer http.ResponseWriter, request *http.Request) {
	productState, err := handler.service.GetProduct(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, product.ProductProjectionFor(productState))
}

func (handler *Handler) getProductAgentContext(writer http.ResponseWriter, request *http.Request) {
	context, err := handler.service.ProductAgentContext(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, context)
}

func (handler *Handler) dispatchProduct(writer http.ResponseWriter, request *http.Request) {
	command, ok := handler.decodeCommand(writer, request)
	if !ok {
		return
	}
	productState, err := handler.service.DispatchProduct(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), domainCommand(command))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	status := http.StatusOK
	if command.Type == "product.create" {
		status = http.StatusCreated
	}
	writeJSON(writer, status, product.ProductProjectionFor(productState))
}

func (handler *Handler) startDelivery(writer http.ResponseWriter, request *http.Request) {
	command, ok := handler.decodeCommand(writer, request)
	if !ok {
		return
	}
	productState, run, err := handler.service.StartDelivery(
		request.Context(),
		request.PathValue("workspaceID"),
		request.PathValue("productID"),
		request.PathValue("deliveryRunID"),
		domainCommand(command),
	)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{
		"product":      product.ProductProjectionFor(productState),
		"delivery_run": presentation.ProjectionForLocale(run, requestLocale(request)),
	})
}

func (handler *Handler) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok", "service": "domainry-delivery"})
}

func (handler *Handler) getDeliveryRun(writer http.ResponseWriter, request *http.Request) {
	run, err := handler.service.Get(request.Context(), request.PathValue("workspaceID"), request.PathValue("deliveryRunID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, presentation.ProjectionForLocale(run, requestLocale(request)))
}

func (handler *Handler) getAgentContext(writer http.ResponseWriter, request *http.Request) {
	context, err := handler.service.AgentContext(request.Context(), request.PathValue("workspaceID"), request.PathValue("deliveryRunID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, context)
}

func (handler *Handler) dispatch(writer http.ResponseWriter, request *http.Request) {
	command, ok := handler.decodeCommand(writer, request)
	if !ok {
		return
	}
	run, err := handler.service.Dispatch(request.Context(), request.PathValue("workspaceID"), request.PathValue("deliveryRunID"), domainCommand(command))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, presentation.ProjectionForLocale(run, requestLocale(request)))
}

func (handler *Handler) decodeCommand(writer http.ResponseWriter, request *http.Request) (deliverysdk.Command, bool) {
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var command deliverysdk.Command
	if err := decoder.Decode(&command); err != nil {
		message := "The request JSON is invalid."
		if errors.Is(err, io.EOF) {
			message = "The request body cannot be empty."
		}
		handler.writeError(writer, request, &domain.Error{Class: apperror.KindBadRequest, Code: "request_invalid", Message: message})
		return deliverysdk.Command{}, false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		handler.writeError(writer, request, &domain.Error{Class: apperror.KindBadRequest, Code: "request_invalid", Message: "The request must contain exactly one JSON object."})
		return deliverysdk.Command{}, false
	}
	if len(command.Payload) == 0 {
		handler.writeError(writer, request, domain.Invalid("payload_invalid"))
		return deliverysdk.Command{}, false
	}
	normalized, err := utcjson.NormalizeInput(command.Payload)
	if err != nil {
		handler.writeError(writer, request, domain.Invalid("payload_invalid"))
		return deliverysdk.Command{}, false
	}
	command.Payload = normalized
	return command, true
}

func domainCommand(command deliverysdk.Command) domain.Command {
	return domain.Command{ClientID: command.ClientID, ExpectedRevision: command.ExpectedRevision, Type: command.Type, Payload: append(json.RawMessage(nil), command.Payload...)}
}

func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var domainError *domain.Error
	if !errors.As(err, &domainError) {
		httpresponse.WriteError(writer, request, httpresponse.Failure{Status: http.StatusInternalServerError, Code: "delivery.internal_error", Cause: err})
		return
	}
	status := deliveryHTTPStatus(apperror.KindOf(domainError))
	localized := presentation.LocalizeError(domainError, requestLocale(request))
	params := make(map[string]string, len(localized.Details))
	for key, value := range localized.Details {
		params[key] = fmt.Sprint(value)
	}
	httpresponse.WriteError(writer, request, httpresponse.Failure{Status: status, Code: domainError.ErrorCode(), Message: localized.Message, Params: params, Cause: err})
}

func deliveryHTTPStatus(kind apperror.ErrorKind) int {
	switch kind {
	case apperror.KindBadRequest:
		return http.StatusBadRequest
	case apperror.KindForbidden:
		return http.StatusForbidden
	case apperror.KindNotFound:
		return http.StatusNotFound
	case apperror.KindConflict:
		return http.StatusConflict
	case apperror.KindRateLimited:
		return http.StatusTooManyRequests
	case apperror.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func requestLocale(request *http.Request) string {
	return presentation.NormalizeLocale(request.Header.Get("Accept-Language"))
}

func (handler *Handler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(writer, request)
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	raw, err := utcjson.Marshal(value)
	if err != nil {
		httpresponse.WriteError(writer, nil, httpresponse.Failure{Status: http.StatusInternalServerError, Code: "backend.response_write_failed", Cause: err})
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write(append(raw, '\n'))
}
