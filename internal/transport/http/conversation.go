package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/domainry/domainry-delivery/internal/application"
	"github.com/domainry/domainry-delivery/internal/domain"
	"github.com/domainry/domainry-delivery/internal/presentation"
)

func (handler *Handler) appendFeatureMessage(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 128<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input application.AppendFeatureMessage
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeJSON(writer, http.StatusBadRequest, presentation.LocalizeError(&domain.Error{Code: "request_invalid"}, requestLocale(request)))
		return
	}
	message, err := handler.service.AppendFeatureMessage(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), request.PathValue("featureID"), input)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, message)
}

func (handler *Handler) listFeatureMessages(writer http.ResponseWriter, request *http.Request) {
	limit := 0
	if raw := request.URL.Query().Get("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, presentation.LocalizeError(&domain.Error{Code: "request_invalid"}, requestLocale(request)))
			return
		}
	}
	messages, err := handler.service.ListFeatureMessages(request.Context(), request.PathValue("workspaceID"), request.PathValue("productID"), request.PathValue("featureID"), request.URL.Query().Get("cursor"), limit)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, messages)
}
