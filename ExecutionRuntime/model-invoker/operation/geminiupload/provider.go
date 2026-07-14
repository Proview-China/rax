// Package geminiupload implements the Gemini Files API resumable upload
// handshake. It is specialized because the provider returns a one-time upload
// URL; treating that URL as an ordinary caller-controlled endpoint would break
// Praxis endpoint trust guarantees.
package geminiupload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
)

const ProviderID modelinvoker.ProviderID = "gemini"
const defaultRoot = "https://generativelanguage.googleapis.com"

type Config struct {
	APIKey string
	// BaseURL is reserved for loopback protocol tests. Production upload always
	// uses the official Gemini host.
	BaseURL        string
	MaxUploadBytes int64
	HTTPClient     *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "geminiupload.Config([REDACTED])")
}
func (Config) GoString() string { return "geminiupload.Config([REDACTED])" }

type Provider struct {
	root           *url.URL
	apiKey         string
	maxUploadBytes int64
	client         *http.Client
}

func New(config Config) (*Provider, error) {
	redactor := adaptercore.NewRedactor(config.APIKey)
	if config.APIKey == "" || config.APIKey != strings.TrimSpace(config.APIKey) || strings.ContainsAny(config.APIKey, "\r\n\x00") {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure_upload", "Gemini upload API key is invalid", 0, nil))
	}
	raw := config.BaseURL
	policy := adaptercore.EndpointPolicy{OfficialHosts: []string{"generativelanguage.googleapis.com"}, AllowLoopback: true}
	if raw == "" {
		raw = defaultRoot
	} else {
		policy = adaptercore.EndpointPolicy{AllowLoopback: true, LoopbackOnly: true}
	}
	trusted, err := adaptercore.ValidateEndpoint(raw, policy)
	if err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure_upload", "Gemini upload base URL is invalid", 0, err))
	}
	root, _ := url.Parse(trusted)
	maxUploadBytes := config.MaxUploadBytes
	if maxUploadBytes == 0 {
		maxUploadBytes = 64 << 20
	}
	if maxUploadBytes < 0 || maxUploadBytes > 2<<30 {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure_upload", "Gemini upload byte limit is invalid", 0, nil))
	}
	return &Provider{root: root, apiKey: config.APIKey, maxUploadBytes: maxUploadBytes, client: adaptercore.CloneHTTPClientWithoutRedirects(config.HTTPClient)}, nil
}

func (p *Provider) ID() modelinvoker.ProviderID { return ProviderID }
func (*Provider) Kinds() []operation.Kind       { return []operation.Kind{operation.FileCreate} }

func (p *Provider) Capabilities(ctx context.Context, _ operation.Query) (operation.CapabilityContract, error) {
	if p == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "capabilities", "Gemini upload provider is not initialized", 0, nil)
	}
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "context is nil", 0, nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return operation.CapabilityContract{operation.FileCreate: {Level: operation.SupportNative, Lifecycle: operation.LifecycleResource}}, nil
}

func (p *Provider) Invoke(ctx context.Context, request operation.Request) (operation.Result, error) {
	if p == nil {
		return operation.Result{}, providerError(modelinvoker.ErrorProviderUnavailable, "upload_file", "Gemini upload provider is not initialized", 0, nil)
	}
	if ctx == nil {
		return operation.Result{}, providerError(modelinvoker.ErrorInvalidRequest, "upload_file", "context is nil", 0, nil)
	}
	if err := request.Validate(); err != nil {
		return operation.Result{}, providerError(modelinvoker.ErrorInvalidRequest, "upload_file", err.Error(), 0, nil)
	}
	if request.Provider != ProviderID || request.Kind != operation.FileCreate || len(request.ProviderOptions) != 0 {
		return operation.Result{}, providerError(modelinvoker.ErrorMapping, "upload_file", "request does not match Gemini resumable upload binding", 0, nil)
	}
	file := request.Body.Bytes()
	if len(file) == 0 {
		return operation.Result{}, providerError(modelinvoker.ErrorInvalidRequest, "upload_file", "file body is required", 0, nil)
	}
	if int64(len(file)) > p.maxUploadBytes {
		return operation.Result{}, providerError(modelinvoker.ErrorInvalidRequest, "upload_file", "file body exceeds the configured upload limit", 0, nil)
	}
	mediaType, _, err := mime.ParseMediaType(request.ContentType)
	if err != nil || mediaType == "" {
		return operation.Result{}, providerError(modelinvoker.ErrorInvalidRequest, "upload_file", "file content type is invalid", 0, err)
	}
	displayName := request.Metadata["display_name"]
	if displayName == "" || displayName != strings.TrimSpace(displayName) || len(displayName) > 512 || strings.ContainsAny(displayName, "\r\n\x00") {
		return operation.Result{}, providerError(modelinvoker.ErrorInvalidRequest, "upload_file", "display_name metadata is required and must be single-line", 0, nil)
	}
	metadata, _ := json.Marshal(map[string]any{"file": map[string]string{"display_name": displayName}})
	uploadURL, requestID, err := p.start(ctx, metadata, mediaType, len(file))
	if err != nil {
		return operation.Result{}, err
	}
	response, responseID, err := p.finalize(ctx, uploadURL, file, mediaType, request.Budget.MaxResponseBytes)
	if err != nil {
		return operation.Result{}, err
	}
	if responseID != "" {
		requestID = responseID
	}
	var envelope struct {
		File struct {
			Name      string `json:"name"`
			URI       string `json:"uri"`
			State     string `json:"state"`
			MIMEType  string `json:"mimeType"`
			SizeBytes string `json:"sizeBytes"`
		} `json:"file"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil || !strings.HasPrefix(envelope.File.Name, "files/") {
		return operation.Result{}, providerError(modelinvoker.ErrorProvider, "decode_upload", "Gemini upload returned an invalid file resource", 0, err)
	}
	id := strings.TrimPrefix(envelope.File.Name, "files/")
	status := operation.StatusUnknown
	switch strings.ToUpper(envelope.File.State) {
	case "ACTIVE":
		status = operation.StatusSucceeded
	case "PROCESSING":
		status = operation.StatusRunning
	case "FAILED":
		status = operation.StatusFailed
	}
	size, _ := strconv.ParseInt(envelope.File.SizeBytes, 10, 64)
	if size <= 0 {
		size = int64(len(file))
	}
	artifactMIME := envelope.File.MIMEType
	if artifactMIME == "" {
		artifactMIME = mediaType
	}
	result := operation.Result{
		Provider: ProviderID, Kind: operation.FileCreate, Status: status, RequestID: requestID,
		Resource:   &operation.ResourceRef{ID: id, Status: status, NativeStatus: envelope.File.State},
		Artifacts:  []operation.Artifact{{Kind: operation.ArtifactFile, MIMEType: artifactMIME, ResourceID: id, SizeBytes: size}},
		RawRequest: modelinvoker.NewRawPayload(metadata), RawResponse: modelinvoker.NewRawPayload(response),
	}
	return result, nil
}

func (p *Provider) Stream(context.Context, operation.Request) (operation.Stream, error) {
	return nil, providerError(modelinvoker.ErrorUnsupportedCapability, "stream_upload", "Gemini resumable upload is not a streaming response operation", 0, nil)
}

func (p *Provider) start(ctx context.Context, metadata []byte, mediaType string, size int) (*url.URL, string, error) {
	target := *p.root
	target.Path = strings.TrimRight(target.Path, "/") + "/upload/v1beta/files"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(metadata))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", p.apiKey)
	req.Header.Set("X-Goog-Upload-Protocol", "resumable")
	req.Header.Set("X-Goog-Upload-Command", "start")
	req.Header.Set("X-Goog-Upload-Header-Content-Length", strconv.Itoa(size))
	req.Header.Set("X-Goog-Upload-Header-Content-Type", mediaType)
	response, err := p.client.Do(req)
	if err != nil {
		return nil, "", p.redact(providerError(modelinvoker.ErrorProviderUnavailable, "start_upload", "Gemini upload handshake failed", 0, err))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, requestID(response.Header), p.httpError("start_upload", response)
	}
	raw := response.Header.Get("X-Goog-Upload-URL")
	uploadURL, err := p.validateUploadURL(raw)
	if err != nil {
		return nil, requestID(response.Header), providerError(modelinvoker.ErrorProvider, "start_upload", "Gemini returned an untrusted upload URL", response.StatusCode, err)
	}
	return uploadURL, requestID(response.Header), nil
}

func (p *Provider) finalize(ctx context.Context, target *url.URL, file []byte, mediaType string, limit int64) ([]byte, string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(file))
	req.Header.Set("Content-Type", mediaType)
	req.Header.Set("Content-Length", strconv.Itoa(len(file)))
	req.Header.Set("X-Goog-Upload-Offset", "0")
	req.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	response, err := p.client.Do(req)
	if err != nil {
		// The one-time URL can carry an opaque upload credential in its query.
		// Never retain the transport cause because url.Error may include it.
		return nil, "", providerError(modelinvoker.ErrorProviderUnavailable, "finalize_upload", "Gemini file upload failed", 0, nil)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, requestID(response.Header), p.httpError("finalize_upload", response)
	}
	if limit <= 0 {
		limit = 4 << 20
	}
	body, err := readBounded(response.Body, limit)
	if err != nil {
		return nil, requestID(response.Header), providerError(modelinvoker.ErrorProvider, "finalize_upload", "Gemini upload response exceeded the configured limit", response.StatusCode, err)
	}
	return body, requestID(response.Header), nil
}

func (p *Provider) validateUploadURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.Scheme != p.root.Scheme || !strings.EqualFold(u.Host, p.root.Host) {
		return nil, fmt.Errorf("upload URL origin mismatch")
	}
	prefix := strings.TrimRight(p.root.Path, "/") + "/upload/v1beta/"
	if !strings.HasPrefix(u.Path, prefix) || strings.Contains(u.Path, "/../") {
		return nil, fmt.Errorf("upload URL path mismatch")
	}
	return u, nil
}

func (p *Provider) httpError(operationName string, response *http.Response) error {
	kind := modelinvoker.ErrorProvider
	switch response.StatusCode {
	case 400, 405, 409, 413, 415, 422:
		kind = modelinvoker.ErrorInvalidRequest
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 403, 404:
		kind = modelinvoker.ErrorPermission
	case 429:
		kind = modelinvoker.ErrorRateLimit
	case 500, 502, 503, 504:
		kind = modelinvoker.ErrorProviderUnavailable
	}
	return providerError(kind, operationName, "Gemini upload operation failed", response.StatusCode, nil)
}
func (p *Provider) redact(err error) error { return adaptercore.NewRedactor(p.apiKey).Error(err) }
func providerError(kind modelinvoker.ErrorKind, operationName, message string, status int, err error) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: operationName, Message: message, HTTPStatus: status, Err: err}
}
func requestID(header http.Header) string {
	for _, name := range []string{"x-request-id", "x-goog-request-id"} {
		if value := header.Get(name); value != "" {
			return value
		}
	}
	return ""
}
func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response too large")
	}
	return body, nil
}

var _ operation.KindProvider = (*Provider)(nil)
