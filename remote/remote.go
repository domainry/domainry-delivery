// Package remote provides the HTTP-backed Delivery SaaS Binding.
package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	deliverysdk "github.com/domainry/domainry-delivery"
)

type Config struct {
	BaseURL string
	Client  *http.Client
}

type Binding struct {
	baseURL    string
	client     *http.Client
	descriptor deliverysdk.Descriptor
}

func Open(ctx context.Context, config Config) (*Binding, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("a valid Delivery SaaS base URL is required")
	}
	client := config.Client
	if client == nil {
		client = http.DefaultClient
	}
	binding := &Binding{baseURL: baseURL, client: client}
	if err := binding.doJSON(ctx, http.MethodGet, "/api/v1/delivery/descriptor", nil, &binding.descriptor); err != nil {
		return nil, err
	}
	if err := binding.descriptor.Validate(); err != nil {
		return nil, err
	}
	if binding.descriptor.Mode != deliverysdk.DeploymentModeSaaS {
		return nil, fmt.Errorf("remote Delivery endpoint did not advertise SaaS mode")
	}
	return binding, nil
}

func (binding *Binding) Descriptor() deliverysdk.Descriptor { return binding.descriptor }

func (binding *Binding) Session(ctx context.Context, workspaceID string) (deliverysdk.Session, error) {
	var response deliverysdk.Session
	err := binding.doJSON(ctx, http.MethodGet, workspacePath(workspaceID)+"/session", nil, &response)
	return response, err
}

func (binding *Binding) ListProducts(ctx context.Context, workspaceID string) ([]deliverysdk.ProductProjection, error) {
	var response struct {
		Data []deliverysdk.ProductProjection `json:"data"`
	}
	err := binding.doJSON(ctx, http.MethodGet, workspacePath(workspaceID)+"/products", nil, &response)
	return response.Data, err
}

func (binding *Binding) GetProduct(ctx context.Context, workspaceID, productID string) (deliverysdk.ProductProjection, error) {
	var response deliverysdk.ProductProjection
	err := binding.doJSON(ctx, http.MethodGet, productPath(workspaceID, productID), nil, &response)
	return response, err
}

func (binding *Binding) ProductAgentContext(ctx context.Context, workspaceID, productID string) (deliverysdk.ProductAgentContext, error) {
	var response deliverysdk.ProductAgentContext
	err := binding.doJSON(ctx, http.MethodGet, productPath(workspaceID, productID)+"/agent-context", nil, &response)
	return response, err
}

func (binding *Binding) DispatchProduct(ctx context.Context, workspaceID, productID string, command deliverysdk.Command) (deliverysdk.ProductProjection, error) {
	var response deliverysdk.ProductProjection
	err := binding.doJSON(ctx, http.MethodPost, productPath(workspaceID, productID)+"/commands", command, &response)
	return response, err
}

func (binding *Binding) UploadFeatureAttachment(ctx context.Context, workspaceID, productID, featureID string, upload deliverysdk.FeatureAttachmentUpload) (deliverysdk.FeatureAttachmentUploadResult, error) {
	path := featureAttachmentPath(workspaceID, productID, featureID, upload.AttachmentID) +
		"?file_name=" + url.QueryEscape(upload.FileName) + "&expected_revision=" + fmt.Sprint(upload.ExpectedRevision)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, binding.baseURL+path, bytes.NewReader(upload.Content))
	if err != nil {
		return deliverysdk.FeatureAttachmentUploadResult{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Language", deliverysdk.LocaleFromContext(ctx))
	request.Header.Set("Content-Type", upload.MediaType)
	binding.authorize(ctx, request)
	var result deliverysdk.FeatureAttachmentUploadResult
	if err := binding.do(request, &result); err != nil {
		return deliverysdk.FeatureAttachmentUploadResult{}, err
	}
	return result, nil
}

func (binding *Binding) DownloadFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string) (deliverysdk.FeatureAttachmentContent, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, binding.baseURL+featureAttachmentPath(workspaceID, productID, featureID, attachmentID), nil)
	if err != nil {
		return deliverysdk.FeatureAttachmentContent{}, err
	}
	request.Header.Set("Accept-Language", deliverysdk.LocaleFromContext(ctx))
	binding.authorize(ctx, request)
	response, err := binding.client.Do(request)
	if err != nil {
		return deliverysdk.FeatureAttachmentContent{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return deliverysdk.FeatureAttachmentContent{}, decodeError(response)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, deliverysdk.MaxFeatureAttachmentBytes+1))
	if err != nil || int64(len(content)) > deliverysdk.MaxFeatureAttachmentBytes {
		return deliverysdk.FeatureAttachmentContent{}, &deliverysdk.Error{Code: "remote_response_invalid", Message: "Delivery SaaS returned an invalid attachment."}
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	if expected := response.Header.Get("X-Attachment-SHA256"); expected == "" || expected != digest {
		return deliverysdk.FeatureAttachmentContent{}, &deliverysdk.Error{Code: "attachment_hash_mismatch", Message: "The downloaded attachment failed integrity verification."}
	}
	metadata, err := decodeAttachmentMetadata(response.Header.Get("X-Attachment-Metadata"))
	if err != nil || metadata.ID != attachmentID || metadata.SHA256 != digest || metadata.SizeBytes != int64(len(content)) {
		return deliverysdk.FeatureAttachmentContent{}, &deliverysdk.Error{Code: "remote_response_invalid", Message: "Delivery SaaS returned invalid attachment metadata."}
	}
	return deliverysdk.FeatureAttachmentContent{Attachment: metadata, Content: content}, nil
}

func (binding *Binding) RemoveFeatureAttachment(ctx context.Context, workspaceID, productID, featureID, attachmentID string, expectedRevision uint64) (deliverysdk.ProductProjection, error) {
	path := featureAttachmentPath(workspaceID, productID, featureID, attachmentID) + "?expected_revision=" + fmt.Sprint(expectedRevision)
	var response deliverysdk.ProductProjection
	err := binding.doJSON(ctx, http.MethodDelete, path, nil, &response)
	return response, err
}

func (binding *Binding) StartDelivery(ctx context.Context, workspaceID, productID, deliveryRunID string, command deliverysdk.Command) (deliverysdk.DeliveryStartResult, error) {
	var response deliverysdk.DeliveryStartResult
	path := productPath(workspaceID, productID) + "/delivery-runs/" + url.PathEscape(deliveryRunID)
	err := binding.doJSON(ctx, http.MethodPost, path, command, &response)
	return response, err
}

func (binding *Binding) ListDeliveryRuns(ctx context.Context, workspaceID, productID string) ([]deliverysdk.DeliveryRunProjection, error) {
	path := workspacePath(workspaceID) + "/delivery-runs"
	if strings.TrimSpace(productID) != "" {
		path += "?product_id=" + url.QueryEscape(productID)
	}
	var response struct {
		Data []deliverysdk.DeliveryRunProjection `json:"data"`
	}
	err := binding.doJSON(ctx, http.MethodGet, path, nil, &response)
	return response.Data, err
}

func (binding *Binding) GetDeliveryRun(ctx context.Context, workspaceID, deliveryRunID string) (deliverysdk.DeliveryRunProjection, error) {
	var response deliverysdk.DeliveryRunProjection
	err := binding.doJSON(ctx, http.MethodGet, deliveryRunPath(workspaceID, deliveryRunID), nil, &response)
	return response, err
}

func (binding *Binding) DeliveryRunAgentContext(ctx context.Context, workspaceID, deliveryRunID string) (deliverysdk.DeliveryRunAgentContext, error) {
	var response deliverysdk.DeliveryRunAgentContext
	err := binding.doJSON(ctx, http.MethodGet, deliveryRunPath(workspaceID, deliveryRunID)+"/agent-context", nil, &response)
	return response, err
}

func (binding *Binding) DispatchDeliveryRun(ctx context.Context, workspaceID, deliveryRunID string, command deliverysdk.Command) (deliverysdk.DeliveryRunProjection, error) {
	var response deliverysdk.DeliveryRunProjection
	err := binding.doJSON(ctx, http.MethodPost, deliveryRunPath(workspaceID, deliveryRunID)+"/commands", command, &response)
	return response, err
}

func (binding *Binding) Close(context.Context) error { return nil }

func (binding *Binding) doJSON(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, binding.baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Language", deliverysdk.LocaleFromContext(ctx))
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token, ok := deliverysdk.AccessTokenFromContext(ctx); ok {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return binding.do(request, output)
}

func (binding *Binding) authorize(ctx context.Context, request *http.Request) {
	if token, ok := deliverysdk.AccessTokenFromContext(ctx); ok {
		request.Header.Set("Authorization", "Bearer "+token)
	}
}

func (binding *Binding) do(request *http.Request, output any) error {
	response, err := binding.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeErrorWithDecoder(response.StatusCode, decoder)
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := decoder.Decode(output); err != nil {
		return &deliverysdk.Error{Code: "remote_response_invalid", Message: "Delivery SaaS returned an invalid response."}
	}
	return nil
}

func decodeError(response *http.Response) error {
	return decodeErrorWithDecoder(response.StatusCode, json.NewDecoder(io.LimitReader(response.Body, 4<<20)))
}

func decodeErrorWithDecoder(statusCode int, decoder *json.Decoder) error {
	var deliveryError deliverysdk.Error
	if err := decoder.Decode(&deliveryError); err != nil || strings.TrimSpace(deliveryError.Code) == "" {
		return &deliverysdk.Error{Code: "remote_failure", Message: fmt.Sprintf("Delivery SaaS returned HTTP %d.", statusCode)}
	}
	return &deliveryError
}

func decodeAttachmentMetadata(encoded string) (deliverysdk.FeatureAttachment, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return deliverysdk.FeatureAttachment{}, err
	}
	var attachment deliverysdk.FeatureAttachment
	if err := json.Unmarshal(data, &attachment); err != nil {
		return deliverysdk.FeatureAttachment{}, err
	}
	return attachment, nil
}

func workspacePath(workspaceID string) string {
	return "/api/v1/workspaces/" + url.PathEscape(workspaceID)
}

func productPath(workspaceID, productID string) string {
	return workspacePath(workspaceID) + "/products/" + url.PathEscape(productID)
}

func featureAttachmentPath(workspaceID, productID, featureID, attachmentID string) string {
	return productPath(workspaceID, productID) + "/features/" + url.PathEscape(featureID) + "/attachments/" + url.PathEscape(attachmentID)
}

func deliveryRunPath(workspaceID, deliveryRunID string) string {
	return workspacePath(workspaceID) + "/delivery-runs/" + url.PathEscape(deliveryRunID)
}

var _ deliverysdk.Binding = (*Binding)(nil)
