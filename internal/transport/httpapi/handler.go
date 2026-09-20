package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	deliverysdk "github.com/domainry/domainry-delivery"
	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain/delivery"
)

type Handler struct {
	service *application.Service
	logger  *slog.Logger
}

func New(service *application.Service, logger *slog.Logger) http.Handler {
	handler := &Handler{service: service, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.health)
	mux.HandleFunc("GET /api/v1/delivery/descriptor", handler.descriptor)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/session", handler.session)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products", handler.listProducts)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products/{productID}", handler.getProduct)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products/{productID}/agent-context", handler.getProductAgentContext)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/products/{productID}/commands", handler.dispatchProduct)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/products/{productID}/features/{featureID}/attachments/{attachmentID}", handler.uploadFeatureAttachment)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/products/{productID}/features/{featureID}/attachments/{attachmentID}", handler.downloadFeatureAttachment)
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/products/{productID}/features/{featureID}/attachments/{attachmentID}", handler.removeFeatureAttachment)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/products/{productID}/delivery-runs/{deliveryRunID}", handler.startDelivery)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/delivery-runs", handler.listDeliveryRuns)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/delivery-runs/{deliveryRunID}", handler.getDeliveryRun)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/delivery-runs/{deliveryRunID}/agent-context", handler.getAgentContext)
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
	writeJSON(writer, http.StatusOK, deliverysdk.SaaSDescriptor())
}

func (handler *Handler) listProducts(writer http.ResponseWriter, request *http.Request) {
	products, err := handler.service.ListProducts(request.Context(), request.PathValue("workspaceID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	projections := make([]delivery.ProductProjection, 0, len(products))
	for _, product := range products {
		projections = append(projections, delivery.ProductProjectionFor(product))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": projections})
}

func (handler *Handler) listDeliveryRuns(writer http.ResponseWriter, request *http.Request) {
	runs, err := handler.service.ListDeliveryRuns(request.Context(), request.PathValue("workspaceID"), request.URL.Query().Get("product_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	projections := make([]delivery.Projection, 0, len(runs))
	for _, run := range runs {
		projections = append(projections, delivery.ProjectionForLocale(run, requestLocale(request)))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": projections})
}

func (handler *Handler) getProduct(writer http.ResponseWriter, request *http.Request) {
	product, err := handler.service.GetProduct(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, delivery.ProductProjectionFor(product))
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
	product, err := handler.service.DispatchProduct(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), command)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	status := http.StatusOK
	if command.Type == "product.create" {
		status = http.StatusCreated
	}
	writeJSON(writer, status, delivery.ProductProjectionFor(product))
}

func (handler *Handler) uploadFeatureAttachment(writer http.ResponseWriter, request *http.Request) {
	expectedRevision, ok := parseExpectedRevision(writer, request)
	if !ok {
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, delivery.MaxFeatureAttachmentBytes)
	content, err := io.ReadAll(request.Body)
	if err != nil {
		writeJSON(writer, http.StatusRequestEntityTooLarge, delivery.LocalizeError(&delivery.Error{Code: "attachment_size_invalid", Message: "The attachment is too large."}, requestLocale(request)))
		return
	}
	product, attachment, err := handler.service.UploadFeatureAttachment(
		request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), request.PathValue("featureID"),
		request.PathValue("attachmentID"), request.URL.Query().Get("file_name"), request.Header.Get("Content-Type"), expectedRevision, content,
	)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, deliverysdk.FeatureAttachmentUploadResult{
		Product: delivery.ProductProjectionFor(product), Attachment: attachment,
	})
}

func (handler *Handler) downloadFeatureAttachment(writer http.ResponseWriter, request *http.Request) {
	result, err := handler.service.DownloadFeatureAttachment(
		request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), request.PathValue("featureID"), request.PathValue("attachmentID"),
	)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writer.Header().Set("Content-Type", result.Attachment.MediaType)
	writer.Header().Set("Content-Length", strconv.FormatInt(result.Attachment.SizeBytes, 10))
	writer.Header().Set("X-Attachment-SHA256", result.Attachment.SHA256)
	metadataJSON, _ := json.Marshal(result.Attachment)
	writer.Header().Set("X-Attachment-Metadata", base64.RawURLEncoding.EncodeToString(metadataJSON))
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(result.Attachment.Name, `"`, "")))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(result.Content)
}

func (handler *Handler) removeFeatureAttachment(writer http.ResponseWriter, request *http.Request) {
	expectedRevision, ok := parseExpectedRevision(writer, request)
	if !ok {
		return
	}
	product, err := handler.service.RemoveFeatureAttachment(
		request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), request.PathValue("featureID"), request.PathValue("attachmentID"), expectedRevision,
	)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, delivery.ProductProjectionFor(product))
}

func parseExpectedRevision(writer http.ResponseWriter, request *http.Request) (uint64, bool) {
	revision, err := strconv.ParseUint(request.URL.Query().Get("expected_revision"), 10, 64)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, delivery.LocalizeError(&delivery.Error{Code: "request_invalid", Message: "expected_revision must be a positive integer."}, requestLocale(request)))
		return 0, false
	}
	return revision, true
}

func (handler *Handler) startDelivery(writer http.ResponseWriter, request *http.Request) {
	command, ok := handler.decodeCommand(writer, request)
	if !ok {
		return
	}
	product, run, err := handler.service.StartDelivery(
		request.Context(),
		request.PathValue("workspaceID"),
		request.PathValue("productID"),
		request.PathValue("deliveryRunID"),
		command,
	)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{
		"product":      delivery.ProductProjectionFor(product),
		"delivery_run": delivery.ProjectionForLocale(run, requestLocale(request)),
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
	writeJSON(writer, http.StatusOK, delivery.ProjectionForLocale(run, requestLocale(request)))
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
	run, err := handler.service.Dispatch(request.Context(), request.PathValue("workspaceID"), request.PathValue("deliveryRunID"), command)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, delivery.ProjectionForLocale(run, requestLocale(request)))
}

func (handler *Handler) decodeCommand(writer http.ResponseWriter, request *http.Request) (delivery.Command, bool) {
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var command delivery.Command
	if err := decoder.Decode(&command); err != nil {
		message := "The request JSON is invalid."
		if errors.Is(err, io.EOF) {
			message = "The request body cannot be empty."
		}
		writeJSON(writer, http.StatusBadRequest, delivery.LocalizeError(&delivery.Error{Code: "request_invalid", Message: message}, requestLocale(request)))
		return delivery.Command{}, false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeJSON(writer, http.StatusBadRequest, delivery.LocalizeError(&delivery.Error{Code: "request_invalid", Message: "The request must contain exactly one JSON object."}, requestLocale(request)))
		return delivery.Command{}, false
	}
	return command, true
}

func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var domainError *delivery.Error
	if !errors.As(err, &domainError) {
		handler.logger.Error("delivery request failed", "method", request.Method, "path", request.URL.Path, "error", err)
		writeJSON(writer, http.StatusInternalServerError, delivery.LocalizeError(&delivery.Error{Code: "internal_error", Message: "The server could not process the request."}, requestLocale(request)))
		return
	}
	status := http.StatusUnprocessableEntity
	switch domainError.Code {
	case "authentication_required":
		status = http.StatusUnauthorized
	case "permission_denied", "identity_workspace_mismatch", "actor_unauthorized":
		status = http.StatusForbidden
	case "not_found":
		status = http.StatusNotFound
	case "revision_conflict", "idempotency_key_reused":
		status = http.StatusConflict
	case "storage_failure":
		status = http.StatusInternalServerError
	case "request_invalid", "client_id_required", "command_invalid", "payload_invalid":
		status = http.StatusBadRequest
	}
	writeJSON(writer, status, delivery.LocalizeError(domainError, requestLocale(request)))
}

func requestLocale(request *http.Request) string {
	return delivery.NormalizeLocale(request.Header.Get("Accept-Language"))
}

func (handler *Handler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(writer, request)
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
