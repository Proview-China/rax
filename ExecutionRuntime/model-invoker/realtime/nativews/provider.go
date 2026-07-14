// Package nativews implements an explicit, pinned WebSocket session boundary
// for OpenAI Realtime, Gemini Live, xAI Voice, and provider-native equivalents.
package nativews

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/realtime"
	"github.com/gorilla/websocket"
)

type TrustMode string

const (
	TrustOfficial TrustMode = "official"
	TrustRelay    TrustMode = "relay"
	TrustLocal    TrustMode = "local"
)

type AuthMode string

const (
	AuthAnonymous AuthMode = "anonymous"
	AuthBearer    AuthMode = "bearer"
	AuthHeader    AuthMode = "header"
	AuthQuery     AuthMode = "query"
)

type Config struct {
	Provider      modelinvoker.ProviderID
	URL           string
	Trust         TrustMode
	OfficialHosts []string
	Auth          AuthMode
	APIKey        string
	HeaderName    string
	HeaderPrefix  string
	QueryName     string
	StaticHeaders http.Header
	ModelQueryKey string
	// ConfigurationModelPath binds Request.Model to a string field in the
	// provider-native first frame (for example "setup.model").
	ConfigurationModelPath   string
	ConfigurationModelPrefix string
	AllowedModels            []string
	Dialer                   *websocket.Dialer
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "nativews.Config([REDACTED])")
}
func (Config) GoString() string { return "nativews.Config([REDACTED])" }

type Provider struct {
	config   Config
	endpoint *url.URL
	dialer   *websocket.Dialer
}

func New(config Config) (*Provider, error) {
	endpoint, err := validate(config)
	if err != nil {
		return nil, adaptercore.NewRedactor(config.APIKey).Error(&modelinvoker.Error{Kind: modelinvoker.ErrorInvalidRequest, Provider: config.Provider, Operation: "configure_realtime", Message: err.Error()})
	}
	dialer := config.Dialer
	if dialer == nil {
		clone := *websocket.DefaultDialer
		dialer = &clone
	}
	copy := config
	copy.OfficialHosts = append([]string(nil), config.OfficialHosts...)
	copy.AllowedModels = append([]string(nil), config.AllowedModels...)
	copy.StaticHeaders = config.StaticHeaders.Clone()
	return &Provider{config: copy, endpoint: endpoint, dialer: dialer}, nil
}

func (p *Provider) ID() modelinvoker.ProviderID {
	if p == nil {
		return ""
	}
	return p.config.Provider
}

func (p *Provider) Open(ctx context.Context, request realtime.Request) (realtime.Session, error) {
	if p == nil {
		return nil, p.err(modelinvoker.ErrorProviderUnavailable, "open_realtime", "provider is not initialized", nil)
	}
	if ctx == nil {
		return nil, p.err(modelinvoker.ErrorInvalidRequest, "open_realtime", "context is nil", nil)
	}
	if request.Provider != p.config.Provider || strings.TrimSpace(request.Model) == "" {
		return nil, p.err(modelinvoker.ErrorMapping, "open_realtime", "request does not match realtime provider", nil)
	}
	if len(p.config.AllowedModels) > 0 && !slices.Contains(p.config.AllowedModels, request.Model) {
		return nil, p.err(modelinvoker.ErrorMapping, "open_realtime", "model is outside realtime allowlist", nil)
	}
	if len(request.ProviderOptions) != 0 {
		return nil, p.err(modelinvoker.ErrorMapping, "open_realtime", "provider options must be compiled into session configuration", nil)
	}
	if request.Timeout < 0 {
		return nil, p.err(modelinvoker.ErrorInvalidRequest, "open_realtime", "timeout must not be negative", nil)
	}
	configuration := request.Configuration.Bytes()
	if len(configuration) > 0 && !json.Valid(configuration) {
		return nil, p.err(modelinvoker.ErrorInvalidRequest, "open_realtime", "session configuration must be valid JSON", nil)
	}
	if p.config.ConfigurationModelPath != "" {
		configuredModel, ok := jsonStringPath(configuration, p.config.ConfigurationModelPath)
		if !ok || configuredModel != p.config.ConfigurationModelPrefix+request.Model {
			return nil, p.err(modelinvoker.ErrorMapping, "open_realtime", "session configuration model does not match request model", nil)
		}
	}
	for _, modality := range request.Modalities {
		if modality != realtime.Text && modality != realtime.Audio && modality != realtime.Video {
			return nil, p.err(modelinvoker.ErrorInvalidRequest, "open_realtime", "session modality is invalid", nil)
		}
	}
	endpoint := *p.endpoint
	query := endpoint.Query()
	if p.config.Auth == AuthQuery {
		query.Set(p.config.QueryName, p.config.APIKey)
	}
	if p.config.ModelQueryKey != "" {
		query.Set(p.config.ModelQueryKey, request.Model)
	}
	endpoint.RawQuery = query.Encode()
	headers := p.config.StaticHeaders.Clone()
	switch p.config.Auth {
	case AuthBearer:
		headers.Set("Authorization", "Bearer "+p.config.APIKey)
	case AuthHeader:
		headers.Set(p.config.HeaderName, p.config.HeaderPrefix+p.config.APIKey)
	}
	call := ctx
	cancel := func() {}
	if request.Timeout > 0 {
		call, cancel = context.WithTimeout(ctx, request.Timeout)
	}
	connection, response, err := p.dialer.DialContext(call, endpoint.String(), headers)
	cancel()
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		return nil, adaptercore.NewRedactor(p.config.APIKey).Error(&modelinvoker.Error{Kind: modelinvoker.ErrorProviderUnavailable, Provider: p.config.Provider, Operation: "open_realtime", Message: "realtime WebSocket handshake failed", HTTPStatus: status, Err: err})
	}
	session := &session{provider: p.config.Provider, connection: connection}
	if len(configuration) > 0 {
		if err := session.Send(ctx, realtime.ClientEvent{Raw: request.Configuration}); err != nil {
			_ = session.Close()
			return nil, err
		}
	}
	return session, nil
}

func (p *Provider) err(kind modelinvoker.ErrorKind, operationName, message string, err error) *modelinvoker.Error {
	provider := modelinvoker.ProviderID("")
	if p != nil {
		provider = p.config.Provider
	}
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operationName, Message: message, Err: err}
}

func validate(config Config) (*url.URL, error) {
	if strings.TrimSpace(string(config.Provider)) == "" {
		return nil, fmt.Errorf("realtime provider ID is required")
	}
	u, err := url.Parse(config.URL)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return nil, fmt.Errorf("realtime URL must be absolute and credential-free")
	}
	loopback := adaptercore.IsLoopbackHost(u.Hostname())
	switch config.Trust {
	case TrustOfficial:
		if u.Scheme != "wss" || !slices.Contains(config.OfficialHosts, strings.ToLower(u.Hostname())) {
			return nil, fmt.Errorf("realtime official URL is outside host allowlist")
		}
	case TrustRelay:
		if u.Scheme != "wss" && !(u.Scheme == "ws" && loopback) {
			return nil, fmt.Errorf("realtime relay requires WSS or loopback WS")
		}
	case TrustLocal:
		if u.Scheme != "ws" || !loopback {
			return nil, fmt.Errorf("local realtime URL must use loopback WS")
		}
	default:
		return nil, fmt.Errorf("realtime trust mode is invalid")
	}
	if strings.ContainsAny(u.Path, "\\\r\n\x00") || strings.Contains(u.Path, "/../") {
		return nil, fmt.Errorf("realtime path is invalid")
	}
	switch config.Auth {
	case AuthAnonymous:
		if config.APIKey != "" {
			return nil, fmt.Errorf("anonymous realtime auth must not include key")
		}
	case AuthBearer:
		if config.APIKey == "" || strings.ContainsAny(config.APIKey, "\r\n") {
			return nil, fmt.Errorf("realtime bearer credential is invalid")
		}
	case AuthHeader:
		if config.APIKey == "" || config.HeaderName == "" || strings.ContainsAny(config.APIKey+config.HeaderName+config.HeaderPrefix, "\r\n") {
			return nil, fmt.Errorf("realtime header credential is invalid")
		}
	case AuthQuery:
		if config.APIKey == "" || strings.ContainsAny(config.APIKey, "\r\n\x00") || !validHeaderToken(config.QueryName) || config.QueryName == config.ModelQueryKey {
			return nil, fmt.Errorf("realtime query credential is invalid")
		}
	default:
		return nil, fmt.Errorf("realtime auth mode is invalid")
	}
	if len(config.AllowedModels) == 0 {
		return nil, fmt.Errorf("realtime provider requires exact model allowlist")
	}
	seenModels := make(map[string]struct{}, len(config.AllowedModels))
	for _, model := range config.AllowedModels {
		if model == "" || model != strings.TrimSpace(model) || len(model) > 512 || strings.ContainsAny(model, "\r\n\x00") {
			return nil, fmt.Errorf("realtime model allowlist contains an invalid model")
		}
		if _, exists := seenModels[model]; exists {
			return nil, fmt.Errorf("realtime model allowlist contains a duplicate")
		}
		seenModels[model] = struct{}{}
	}
	if config.ModelQueryKey != "" && !validHeaderToken(config.ModelQueryKey) {
		return nil, fmt.Errorf("realtime model query key is invalid")
	}
	if config.ModelQueryKey == "" && config.ConfigurationModelPath == "" {
		return nil, fmt.Errorf("realtime model must be bound to query or configuration")
	}
	if config.ModelQueryKey != "" && config.ConfigurationModelPath != "" {
		return nil, fmt.Errorf("realtime model binding must have one owner")
	}
	if config.ConfigurationModelPath != "" && !validJSONPath(config.ConfigurationModelPath) {
		return nil, fmt.Errorf("realtime configuration model path is invalid")
	}
	for name, values := range config.StaticHeaders {
		if !validHeaderToken(name) || forbiddenStaticHeader(name) {
			return nil, fmt.Errorf("realtime static header is invalid or security-sensitive")
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n\x00") {
				return nil, fmt.Errorf("realtime static header value is invalid")
			}
		}
	}
	return u, nil
}

func validJSONPath(path string) bool {
	parts := strings.Split(path, ".")
	for _, part := range parts {
		if !validHeaderToken(part) {
			return false
		}
	}
	return len(parts) > 0
}

func jsonStringPath(raw []byte, path string) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	current, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	parts := strings.Split(path, ".")
	for index, part := range parts {
		next, exists := current[part]
		if !exists {
			return "", false
		}
		if index == len(parts)-1 {
			result, ok := next.(string)
			return result, ok
		}
		current, ok = next.(map[string]any)
		if !ok {
			return "", false
		}
	}
	return "", false
}

func validHeaderToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

func forbiddenStaticHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "host", "origin", "sec-websocket-key", "sec-websocket-protocol", "sec-websocket-version":
		return true
	default:
		return false
	}
}

type session struct {
	provider   modelinvoker.ProviderID
	connection *websocket.Conn
	writeMu    sync.Mutex
	event      realtime.ServerEvent
	sequence   int64
	err        error
	closed     bool
}

func (s *session) Send(ctx context.Context, event realtime.ClientEvent) error {
	if s == nil || s.connection == nil {
		provider := modelinvoker.ProviderID("")
		if s != nil {
			provider = s.provider
		}
		return &modelinvoker.Error{Kind: modelinvoker.ErrorProviderUnavailable, Provider: provider, Operation: "realtime_send", Message: "session is not initialized"}
	}
	if ctx == nil {
		return &modelinvoker.Error{Kind: modelinvoker.ErrorInvalidRequest, Provider: s.provider, Operation: "realtime_send", Message: "context is nil"}
	}
	if len(event.Binary) > 0 && (event.Raw.Len() > 0 || event.Type != "" || event.Text != "") {
		return &modelinvoker.Error{Kind: modelinvoker.ErrorInvalidRequest, Provider: s.provider, Operation: "realtime_send", Message: "binary realtime events cannot include text or JSON payloads"}
	}
	if event.Raw.Len() > 0 && !json.Valid(event.Raw.Bytes()) {
		return &modelinvoker.Error{Kind: modelinvoker.ErrorInvalidRequest, Provider: s.provider, Operation: "realtime_send", Message: "realtime raw event must be valid JSON"}
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if deadline, ok := ctx.Deadline(); ok {
		_ = s.connection.SetWriteDeadline(deadline)
	} else {
		_ = s.connection.SetWriteDeadline(time.Time{})
	}
	if len(event.Binary) > 0 {
		return s.connection.WriteMessage(websocket.BinaryMessage, event.Binary)
	}
	payload := event.Raw.Bytes()
	if len(payload) == 0 {
		encoded, err := json.Marshal(map[string]string{"type": event.Type, "text": event.Text})
		if err != nil {
			return err
		}
		payload = encoded
	}
	return s.connection.WriteMessage(websocket.TextMessage, payload)
}
func (s *session) Next() bool {
	if s == nil || s.closed || s.err != nil {
		return false
	}
	messageType, payload, err := s.connection.ReadMessage()
	if err != nil {
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			s.err = err
		}
		return false
	}
	s.sequence++
	s.event = realtime.ServerEvent{Type: "native", Sequence: s.sequence, Raw: modelinvoker.NewRawPayload(payload)}
	if messageType == websocket.BinaryMessage {
		s.event.Type = "binary"
		s.event.Binary = append([]byte(nil), payload...)
	} else {
		var envelope struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(payload, &envelope) == nil {
			if envelope.Type != "" {
				s.event.Type = envelope.Type
			}
			s.event.Text = envelope.Text
		}
	}
	return true
}
func (s *session) Event() realtime.ServerEvent {
	if s == nil {
		return realtime.ServerEvent{}
	}
	return s.event
}
func (s *session) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}
func (s *session) CloseWrite() error {
	if s == nil || s.connection == nil {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
}
func (s *session) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.connection != nil {
		return s.connection.Close()
	}
	return nil
}

var _ realtime.Provider = (*Provider)(nil)
var _ realtime.Session = (*session)(nil)
