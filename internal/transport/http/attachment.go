package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/presentation"
)

func (handler *Handler) uploadFeatureAttachment(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 8<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input application.UploadFeatureAttachment
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeJSON(writer, http.StatusBadRequest, presentation.LocalizeError(&domain.Error{Code: "request_invalid"}, requestLocale(request)))
		return
	}
	metadata, err := handler.service.UploadFeatureAttachment(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), request.PathValue("featureID"), input)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, metadata)
}

func (handler *Handler) listFeatureAttachments(writer http.ResponseWriter, request *http.Request) {
	values, err := handler.service.ListFeatureAttachments(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), request.PathValue("featureID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": values})
}

func (handler *Handler) downloadFeatureAttachment(writer http.ResponseWriter, request *http.Request) {
	value, err := handler.service.DownloadFeatureAttachment(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), request.PathValue("featureID"), request.PathValue("attachmentID"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (handler *Handler) removeFeatureAttachment(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input struct {
		ClientID         string `json:"client_id"`
		DeviceID         string `json:"device_id"`
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeJSON(writer, http.StatusBadRequest, presentation.LocalizeError(&domain.Error{Code: "request_invalid"}, requestLocale(request)))
		return
	}
	value, err := handler.service.RemoveFeatureAttachment(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), request.PathValue("featureID"), request.PathValue("attachmentID"), input.ClientID, input.DeviceID, input.ExpectedRevision)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}
