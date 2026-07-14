package nativehttp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
)

type Provider struct {
	id            modelinvoker.ProviderID
	baseURL       string
	auth          AuthMode
	apiKey        string
	headerName    string
	headerPrefix  string
	userAgent     string
	staticHeaders http.Header
	specs         map[operation.Kind]Spec
	client        *http.Client
}

func New(config Config) (*Provider, error) {
	redactor := adaptercore.NewRedactor(config.APIKey)
	base, specs, err := config.validate()
	if err != nil {
		return nil, redactor.Error(&modelinvoker.Error{Kind: modelinvoker.ErrorInvalidRequest, Provider: config.Provider, Operation: "configure_operation", Message: err.Error()})
	}
	client := adaptercore.CloneHTTPClientWithoutRedirects(config.HTTPClient)
	return &Provider{id: config.Provider, baseURL: base, auth: config.Auth, apiKey: config.APIKey, headerName: config.HeaderName, headerPrefix: config.HeaderPrefix, userAgent: config.UserAgent, staticHeaders: config.StaticHeaders.Clone(), specs: specs, client: client}, nil
}

func (p *Provider) ID() modelinvoker.ProviderID {
	if p == nil {
		return ""
	}
	return p.id
}

func (p *Provider) Kinds() []operation.Kind {
	if p == nil {
		return nil
	}
	kinds := make([]operation.Kind, 0, len(p.specs))
	for kind := range p.specs {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return kinds
}

func (p *Provider) Capabilities(ctx context.Context, query operation.Query) (operation.CapabilityContract, error) {
	if p == nil {
		return nil, p.err(modelinvoker.ErrorProviderUnavailable, "capabilities", "provider is not initialized", 0, "", nil)
	}
	if ctx == nil {
		return nil, p.err(modelinvoker.ErrorInvalidRequest, "capabilities", "context is nil", 0, "", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	contract := make(operation.CapabilityContract, len(p.specs))
	for kind, spec := range p.specs {
		contract[kind] = operation.Capability{Level: spec.Support, Lifecycle: spec.Lifecycle, Models: append([]string(nil), spec.Models...), Limitations: append([]string(nil), spec.Limitations...)}
	}
	return contract, nil
}

func (p *Provider) Invoke(ctx context.Context, request operation.Request) (operation.Result, error) {
	spec, response, rawRequest, err := p.do(ctx, request, false)
	if err != nil {
		return operation.Result{}, err
	}
	defer response.Body.Close()
	limit := request.Budget.MaxResponseBytes
	if limit <= 0 {
		limit = 64 << 20
	}
	body, err := readBounded(response.Body, limit)
	if err != nil {
		return operation.Result{}, p.err(modelinvoker.ErrorProvider, "read_operation", "operation response could not be read", response.StatusCode, requestID(response.Header), err)
	}
	result := operation.Result{Provider: p.id, Kind: request.Kind, Model: request.Model, Status: operation.StatusSucceeded, RequestID: requestID(response.Header), ProviderMetadata: responseMetadata(response.Header), RawRequest: modelinvoker.NewRawPayload(rawRequest), RawResponse: modelinvoker.NewRawPayload(body)}
	if spec.ResponseMode == ResponseBinary {
		mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if mediaType == "" {
			mediaType = spec.ArtifactMIME
		}
		result.Artifacts = []operation.Artifact{{Kind: spec.ArtifactKind, MIMEType: mediaType, Data: append([]byte(nil), body...), SizeBytes: int64(len(body))}}
		return result, nil
	}
	if len(body) == 0 {
		if spec.Lifecycle == operation.LifecycleJob {
			result.Status = operation.StatusRunning
		}
		return result, nil
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return result, p.err(modelinvoker.ErrorProvider, "decode_operation", "operation returned invalid JSON", response.StatusCode, result.RequestID, err)
	}
	decodeJSONResult(&result, spec, payload)
	return result, nil
}

func (p *Provider) Stream(ctx context.Context, request operation.Request) (operation.Stream, error) {
	spec, response, rawRequest, err := p.do(ctx, request, true)
	if err != nil {
		return nil, err
	}
	return newHTTPStream(p.id, request, spec, response, rawRequest), nil
}

func (p *Provider) do(ctx context.Context, request operation.Request, streaming bool) (Spec, *http.Response, []byte, error) {
	if p == nil {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorProviderUnavailable, "invoke_operation", "provider is not initialized", 0, "", nil)
	}
	if ctx == nil {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorInvalidRequest, "invoke_operation", "context is nil", 0, "", nil)
	}
	if err := request.Validate(); err != nil {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorInvalidRequest, "validate_operation", err.Error(), 0, "", nil)
	}
	if request.Provider != p.id {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorMapping, "validate_operation", "request provider does not match operation binding", 0, "", nil)
	}
	if len(request.ProviderOptions) != 0 {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorMapping, "validate_operation", "native operation body must consume provider options before transport", 0, "", nil)
	}
	spec, ok := p.specs[request.Kind]
	if !ok {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorUnsupportedCapability, "validate_operation", "operation is not configured", 0, "", nil)
	}
	if !streaming && (spec.ResponseMode == ResponseSSE || spec.ResponseMode == ResponseNDJSON) {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorInvalidRequest, "validate_operation", "streaming response mode requires Stream", 0, "", nil)
	}
	if len(spec.Models) > 0 && !slices.Contains(spec.Models, request.Model) {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorMapping, "validate_operation", "model is outside operation allowlist", 0, "", nil)
	}
	if spec.RequiresResourceID && !validPathValue(request.ResourceID) {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorInvalidRequest, "validate_operation", "resource ID is required", 0, "", nil)
	}
	if spec.RequiresParentID && !validPathValue(request.ParentID) {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorInvalidRequest, "validate_operation", "parent ID is required", 0, "", nil)
	}
	if spec.RequiresModel && !validModelPathValue(request.Model) {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorInvalidRequest, "validate_operation", "model is required by operation path", 0, "", nil)
	}
	if spec.BodyModelField != "" {
		actual, err := bodyModel(request.Body.Bytes(), request.ContentType, spec.BodyModelField)
		if err != nil {
			return Spec{}, nil, nil, p.err(modelinvoker.ErrorInvalidRequest, "validate_operation", err.Error(), 0, "", err)
		}
		if request.Model == "" || actual != request.Model {
			return Spec{}, nil, nil, p.err(modelinvoker.ErrorMapping, "validate_operation", "operation body model does not match the selected model", 0, "", nil)
		}
	}
	contentType := request.ContentType
	if request.Body.Len() > 0 && contentType == "" {
		contentType = "application/json"
	}
	if len(spec.ContentTypes) > 0 && contentType != "" && !contentTypeAllowed(spec.ContentTypes, contentType) {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorMapping, "validate_operation", "content type is outside operation allowlist", 0, "", nil)
	}
	path := strings.ReplaceAll(spec.Path, "{id}", url.PathEscape(request.ResourceID))
	path = strings.ReplaceAll(path, "{parent_id}", url.PathEscape(request.ParentID))
	path = strings.ReplaceAll(path, "{model}", url.PathEscape(request.Model))
	target, err := url.Parse(p.baseURL + path)
	if err != nil {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorMapping, "build_operation", "operation endpoint is invalid", 0, "", err)
	}
	allowedQuery := make(map[string]struct{}, len(spec.AllowedQuery))
	for _, key := range spec.AllowedQuery {
		allowedQuery[key] = struct{}{}
	}
	values := target.Query()
	for key, items := range request.Query {
		if _, ok := allowedQuery[key]; !ok {
			return Spec{}, nil, nil, p.err(modelinvoker.ErrorMapping, "validate_operation", "query field is outside operation allowlist", 0, "", nil)
		}
		for _, item := range items {
			values.Add(key, item)
		}
	}
	target.RawQuery = values.Encode()
	body := request.Body.Bytes()
	httpRequest, err := http.NewRequestWithContext(ctx, strings.ToUpper(spec.Method), target.String(), bytes.NewReader(body))
	if err != nil {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorInvalidRequest, "build_operation", "operation request could not be created", 0, "", err)
	}
	if contentType != "" {
		httpRequest.Header.Set("Content-Type", contentType)
	}
	for name, values := range p.staticHeaders {
		for _, value := range values {
			httpRequest.Header.Add(name, value)
		}
	}
	for name, values := range spec.Headers {
		for _, value := range values {
			httpRequest.Header.Add(name, value)
		}
	}
	if streaming {
		httpRequest.Header.Set("Accept", "text/event-stream, application/x-ndjson, application/octet-stream")
	}
	if request.IdempotencyKey != "" {
		httpRequest.Header.Set("Idempotency-Key", request.IdempotencyKey)
	}
	if p.userAgent != "" {
		httpRequest.Header.Set("User-Agent", p.userAgent)
	}
	switch p.auth {
	case AuthBearer:
		httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	case AuthHeader:
		httpRequest.Header.Set(p.headerName, p.headerPrefix+p.apiKey)
	}
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return Spec{}, nil, nil, p.err(modelinvoker.ErrorProviderUnavailable, "send_operation", "operation transport failed", 0, "", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		failure, _ := readBounded(response.Body, 1<<20)
		return Spec{}, nil, nil, p.httpError(response.StatusCode, response.Header, failure)
	}
	return spec, response, body, nil
}

func (p *Provider) httpError(status int, headers http.Header, body []byte) error {
	kind, retryable := modelinvoker.ErrorProvider, false
	switch status {
	case 400, 405, 409, 413, 415, 422:
		kind = modelinvoker.ErrorInvalidRequest
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 402:
		kind = modelinvoker.ErrorBilling
	case 403, 404:
		kind = modelinvoker.ErrorPermission
	case 408:
		kind, retryable = modelinvoker.ErrorTimeout, true
	case 429:
		kind, retryable = modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		kind, retryable = modelinvoker.ErrorProviderUnavailable, true
	}
	code := ""
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		code = firstString(payload, []string{"code", "type", "error_code"})
	}
	err := p.err(kind, "invoke_operation", "upstream operation failed", status, requestID(headers), nil)
	err.Code, err.Retryable = code, retryable
	if seconds, parseErr := strconv.Atoi(headers.Get("Retry-After")); parseErr == nil && seconds >= 0 {
		err.RetryAfter = time.Duration(seconds) * time.Second
	}
	return err
}

func (p *Provider) err(kind modelinvoker.ErrorKind, operationName, message string, status int, requestID string, err error) *modelinvoker.Error {
	provider := modelinvoker.ProviderID("")
	if p != nil {
		provider = p.id
	}
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operationName, Message: message, HTTPStatus: status, RequestID: requestID, Err: err}
}

func validPathValue(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 512 && !strings.ContainsAny(value, "/\\\r\n\x00") && value != "." && value != ".."
}
func validModelPathValue(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 512 && !strings.ContainsAny(value, "\\\r\n\x00") && value != "." && value != ".."
}
func contentTypeAllowed(allowed []string, actual string) bool {
	mediaType, _, err := mime.ParseMediaType(actual)
	if err != nil {
		return false
	}
	for _, candidate := range allowed {
		if mediaType == candidate {
			return true
		}
	}
	return false
}

func bodyModel(body []byte, contentType, field string) (string, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", fmt.Errorf("operation model content type is invalid")
	}
	switch mediaType {
	case "application/json":
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", fmt.Errorf("operation model JSON is invalid")
		}
		value, ok := payload[field].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("operation model field is required")
		}
		return value, nil
	case "multipart/form-data":
		boundary := params["boundary"]
		if boundary == "" {
			return "", fmt.Errorf("operation multipart boundary is required")
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		for {
			part, partErr := reader.NextPart()
			if errors.Is(partErr, io.EOF) {
				break
			}
			if partErr != nil {
				return "", fmt.Errorf("operation multipart body is invalid")
			}
			if part.FormName() == field {
				value, readErr := readBounded(part, 513)
				if readErr != nil || len(value) == 0 {
					return "", fmt.Errorf("operation multipart model field is invalid")
				}
				return string(value), nil
			}
		}
		return "", fmt.Errorf("operation model field is required")
	default:
		return "", fmt.Errorf("operation model binding does not support content type %q", mediaType)
	}
}
func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("response limit must be positive")
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds configured byte limit")
	}
	return data, nil
}
func requestID(h http.Header) string {
	for _, key := range []string{"x-request-id", "request-id", "x-goog-request-id", "cf-ray"} {
		if v := h.Get(key); v != "" {
			return v
		}
	}
	return ""
}
func responseMetadata(h http.Header) modelinvoker.ProviderMetadata {
	out := modelinvoker.ProviderMetadata{}
	for name, values := range h {
		lower := strings.ToLower(name)
		if lower == "x-request-id" || lower == "request-id" || lower == "x-goog-request-id" || lower == "cf-ray" || lower == "retry-after" || strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "x-goog-quota-") {
			out[lower] = strings.Join(values, ",")
		}
	}
	return out
}

func decodeJSONResult(result *operation.Result, spec Spec, payload any) {
	if result == nil {
		return
	}
	root, _ := payload.(map[string]any)
	id := firstString(root, spec.IDKeys)
	if spec.IDPrefix != "" {
		id = strings.TrimPrefix(id, spec.IDPrefix)
	}
	nativeStatus := firstString(root, spec.StatusKeys)
	normalizedStatus := normalizeStatus(nativeStatus)
	if firstPresent(root, spec.FailureKeys) {
		normalizedStatus = operation.StatusFailed
	} else if firstBool(root, spec.DoneKeys) {
		normalizedStatus = operation.StatusSucceeded
	}
	if spec.Lifecycle == operation.LifecycleResource {
		if id != "" || nativeStatus != "" {
			result.Resource = &operation.ResourceRef{ID: id, NativeStatus: nativeStatus, Status: normalizedStatus}
			if result.Resource.Status == operation.StatusUnknown {
				result.Resource.Status = operation.StatusSucceeded
			}
			result.Status = result.Resource.Status
		} else {
			result.Status = operation.StatusSucceeded
		}
	} else if spec.Lifecycle == operation.LifecycleJob {
		result.Job = &operation.JobRef{ID: id, NativeStatus: nativeStatus, Status: normalizedStatus}
		result.Status = result.Job.Status
		if result.Status == operation.StatusUnknown && spec.Lifecycle == operation.LifecycleJob {
			result.Status = operation.StatusRunning
			result.Job.Status = result.Status
		}
	}
	for _, key := range spec.URLKeys {
		for _, value := range collectStrings(payload, key) {
			result.Artifacts = append(result.Artifacts, operation.Artifact{Kind: spec.ArtifactKind, MIMEType: spec.ArtifactMIME, URL: value, ExpiryUnknown: true})
		}
	}
	for _, key := range spec.Base64Keys {
		for _, value := range collectStrings(payload, key) {
			if data, err := base64.StdEncoding.DecodeString(value); err == nil {
				result.Artifacts = append(result.Artifacts, operation.Artifact{Kind: spec.ArtifactKind, MIMEType: spec.ArtifactMIME, Data: data, SizeBytes: int64(len(data))})
			}
		}
	}
	result.Transcript = firstString(root, spec.TranscriptKeys)
	result.Vectors = collectVectors(payload)
	result.Rankings = collectRankings(payload)
	result.Usage = collectUsage(payload)
}

func firstString(root map[string]any, keys []string) string {
	if root == nil {
		return ""
	}
	for _, key := range keys {
		if value := findFirst(root, key); value != nil {
			if text, ok := value.(string); ok {
				return text
			}
		}
	}
	return ""
}

func firstBool(root map[string]any, keys []string) bool {
	for _, key := range keys {
		if value, ok := findFirst(root, key).(bool); ok && value {
			return true
		}
	}
	return false
}

func firstPresent(root map[string]any, keys []string) bool {
	for _, key := range keys {
		if findFirst(root, key) != nil {
			return true
		}
	}
	return false
}
func findFirst(value any, key string) any {
	switch current := value.(type) {
	case map[string]any:
		if found, ok := current[key]; ok {
			return found
		}
		for _, child := range current {
			if found := findFirst(child, key); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range current {
			if found := findFirst(child, key); found != nil {
				return found
			}
		}
	}
	return nil
}
func collectStrings(value any, key string) []string {
	if strings.Contains(key, ".") {
		return collectPathStrings(value, strings.Split(key, "."))
	}
	out := []string{}
	var walk func(any)
	walk = func(node any) {
		switch current := node.(type) {
		case map[string]any:
			for name, child := range current {
				if name == key {
					if text, ok := child.(string); ok && text != "" {
						out = append(out, text)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func collectPathStrings(value any, path []string) []string {
	out := []string{}
	var exact func(any, int)
	exact = func(node any, index int) {
		if index == len(path) {
			if text, ok := node.(string); ok && text != "" {
				out = append(out, text)
			}
			return
		}
		switch current := node.(type) {
		case map[string]any:
			if child, ok := current[path[index]]; ok {
				exact(child, index+1)
			}
		case []any:
			for _, child := range current {
				exact(child, index)
			}
		}
	}
	var scan func(any)
	scan = func(node any) {
		switch current := node.(type) {
		case map[string]any:
			if child, ok := current[path[0]]; ok {
				exact(child, 1)
			}
			for _, child := range current {
				scan(child)
			}
		case []any:
			for _, child := range current {
				scan(child)
			}
		}
	}
	if len(path) > 0 {
		scan(value)
	}
	return out
}
func collectVectors(value any) []operation.Vector {
	out := []operation.Vector{}
	var walk func(any)
	walk = func(node any) {
		switch current := node.(type) {
		case map[string]any:
			if raw, ok := current["embedding"].([]any); ok {
				values := make([]float32, 0, len(raw))
				for _, item := range raw {
					if number, ok := item.(float64); ok {
						values = append(values, float32(number))
					}
				}
				index := len(out)
				if number, ok := current["index"].(float64); ok {
					index = int(number)
				}
				if len(values) > 0 {
					out = append(out, operation.Vector{Index: index, Values: values})
				}
			}
			for _, child := range current {
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}
func collectRankings(value any) []operation.Ranking {
	out := []operation.Ranking{}
	var walk func(any)
	walk = func(node any) {
		switch current := node.(type) {
		case map[string]any:
			score, scoreOK := current["relevance_score"].(float64)
			if !scoreOK {
				score, scoreOK = current["score"].(float64)
			}
			if scoreOK {
				ranking := operation.Ranking{Index: len(out), Score: score}
				if number, ok := current["index"].(float64); ok {
					ranking.Index = int(number)
				}
				if text, ok := current["text"].(string); ok {
					ranking.Text = text
				}
				out = append(out, ranking)
			}
			for _, child := range current {
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func collectUsage(value any) modelinvoker.Usage {
	root, _ := value.(map[string]any)
	usageNode := findFirst(root, "usage")
	if usageNode == nil {
		usageNode = findFirst(root, "usageMetadata")
	}
	usageMap, _ := usageNode.(map[string]any)
	number := func(keys ...string) int64 {
		for _, key := range keys {
			if value, ok := usageMap[key].(float64); ok && value >= 0 {
				return int64(value)
			}
		}
		return 0
	}
	usage := modelinvoker.Usage{
		InputTokens:     number("prompt_tokens", "input_tokens", "promptTokenCount"),
		OutputTokens:    number("completion_tokens", "output_tokens", "candidatesTokenCount"),
		ReasoningTokens: number("reasoning_tokens", "thoughtsTokenCount"),
		CacheReadTokens: number("cached_tokens", "cache_read_input_tokens", "cachedContentTokenCount"),
		TotalTokens:     number("total_tokens", "totalTokenCount"),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return usage
}
func normalizeStatus(value string) operation.Status {
	normalized := strings.TrimPrefix(strings.ToLower(value), "job_state_")
	switch normalized {
	case "queued", "pending":
		return operation.StatusQueued
	case "validating":
		return operation.StatusValidating
	case "running", "in_progress", "processing":
		return operation.StatusRunning
	case "finalizing":
		return operation.StatusFinalizing
	case "completed", "succeeded", "success", "done":
		return operation.StatusSucceeded
	case "failed", "error", "errored":
		return operation.StatusFailed
	case "cancelling", "canceling":
		return operation.StatusCancelling
	case "cancelled", "canceled":
		return operation.StatusCancelled
	case "expired":
		return operation.StatusExpired
	default:
		return operation.StatusUnknown
	}
}

var _ operation.Provider = (*Provider)(nil)
var _ operation.KindProvider = (*Provider)(nil)

// httpStream preserves byte/event order and never starts a background goroutine.
type httpStream struct {
	provider   modelinvoker.ProviderID
	request    operation.Request
	spec       Spec
	response   *http.Response
	scanner    *bufio.Scanner
	reader     io.Reader
	event      operation.StreamEvent
	sequence   int64
	err        error
	closed     bool
	rawRequest []byte
}

func newHTTPStream(provider modelinvoker.ProviderID, request operation.Request, spec Spec, response *http.Response, rawRequest []byte) *httpStream {
	stream := &httpStream{provider: provider, request: request, spec: spec, response: response, rawRequest: append([]byte(nil), rawRequest...)}
	if spec.ResponseMode == ResponseSSE || spec.ResponseMode == ResponseNDJSON {
		stream.scanner = bufio.NewScanner(response.Body)
		stream.scanner.Buffer(make([]byte, 4096), 4<<20)
	} else {
		stream.reader = response.Body
	}
	return stream
}
func (s *httpStream) Next() bool {
	if s == nil || s.closed || s.err != nil {
		return false
	}
	s.sequence++
	if s.scanner != nil {
		for s.scanner.Scan() {
			line := bytes.TrimSpace(s.scanner.Bytes())
			if len(line) == 0 || bytes.HasPrefix(line, []byte("event:")) || bytes.HasPrefix(line, []byte(":")) {
				continue
			}
			line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if bytes.Equal(line, []byte("[DONE]")) {
				s.event = operation.StreamEvent{Type: operation.StreamCompleted, Sequence: s.sequence}
				return true
			}
			s.event = operation.StreamEvent{Type: operation.StreamNative, Sequence: s.sequence, Raw: modelinvoker.NewRawPayload(line)}
			return true
		}
		if err := s.scanner.Err(); err != nil {
			s.err = err
		}
		return false
	}
	buffer := make([]byte, 32<<10)
	count, err := s.reader.Read(buffer)
	if count > 0 {
		s.event = operation.StreamEvent{Type: operation.StreamArtifactChunk, Sequence: s.sequence, Chunk: append([]byte(nil), buffer[:count]...)}
		return true
	}
	if err != nil && !errors.Is(err, io.EOF) {
		s.err = err
	}
	return false
}
func (s *httpStream) Event() operation.StreamEvent {
	if s == nil {
		return operation.StreamEvent{}
	}
	return s.event
}
func (s *httpStream) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}
func (s *httpStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.response != nil && s.response.Body != nil {
		return s.response.Body.Close()
	}
	return nil
}

var _ operation.Stream = (*httpStream)(nil)
