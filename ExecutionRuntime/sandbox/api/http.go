package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxHTTPBodyBytesV1 = 8 << 20

type TransportAuthorizationV1 interface {
	AuthorizeSandboxAPI(*http.Request, string, ActionV1) error
}

type HTTPServerV1 struct {
	service *ServiceV1
	auth    TransportAuthorizationV1
	clock   func() time.Time
	mux     *http.ServeMux
}

func NewHTTPServerV1(service *ServiceV1, auth TransportAuthorizationV1, clock func() time.Time) (*HTTPServerV1, error) {
	if service == nil || nilLike(auth) || clock == nil {
		return nil, errors.New("sandbox HTTP API requires service, transport authorization, and clock")
	}
	server := &HTTPServerV1{service: service, auth: auth, clock: clock, mux: http.NewServeMux()}
	server.mux.HandleFunc("POST /v1/operations", server.submit)
	server.mux.HandleFunc("GET /v1/operations/by-idempotency", server.inspectByIdempotency)
	server.mux.HandleFunc("GET /v1/operations/{id}", server.inspect)
	server.mux.HandleFunc("POST /v1/operations/{id}/execute", server.execute)
	server.mux.HandleFunc("POST /v1/operations/{id}/reconcile", server.reconcile)
	server.mux.HandleFunc("POST /v1/operations/{id}/cancel", server.cancel)
	server.mux.HandleFunc("GET /v1/watch", server.watch)
	return server, nil
}

func (s *HTTPServerV1) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	s.mux.ServeHTTP(writer, request)
}

func (s *HTTPServerV1) submit(writer http.ResponseWriter, request *http.Request) {
	var value OperationRequestV1
	if err := decodeHTTPJSONV1(writer, request, &value); err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	if err := s.authorize(request, value.TenantID, value.Action); err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	fact, err := s.service.Submit(request.Context(), value)
	if err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	writeHTTPJSONV1(writer, http.StatusAccepted, fact)
}

func (s *HTTPServerV1) inspect(writer http.ResponseWriter, request *http.Request) {
	fact, err := s.service.Inspect(request.Context(), request.PathValue("id"))
	if err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	if err := s.authorize(request, fact.Request.TenantID, fact.Request.Action); err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	writeHTTPJSONV1(writer, http.StatusOK, fact)
}

func (s *HTTPServerV1) inspectByIdempotency(writer http.ResponseWriter, request *http.Request) {
	tenant := request.URL.Query().Get("tenant_id")
	key := request.URL.Query().Get("key")
	if strings.TrimSpace(tenant) == "" || strings.TrimSpace(key) == "" {
		writeHTTPErrorV1(writer, fmt.Errorf("%w: tenant_id and key are required", ErrConflict))
		return
	}
	fact, err := s.service.InspectByIdempotency(request.Context(), tenant, key)
	if err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	if err := s.authorize(request, fact.Request.TenantID, fact.Request.Action); err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	writeHTTPJSONV1(writer, http.StatusOK, fact)
}

func (s *HTTPServerV1) execute(writer http.ResponseWriter, request *http.Request) {
	s.transition(writer, request, s.service.Execute)
}

func (s *HTTPServerV1) reconcile(writer http.ResponseWriter, request *http.Request) {
	s.transition(writer, request, s.service.Reconcile)
}

func (s *HTTPServerV1) cancel(writer http.ResponseWriter, request *http.Request) {
	s.transition(writer, request, s.service.Cancel)
}

func (s *HTTPServerV1) transition(writer http.ResponseWriter, request *http.Request, transition func(context.Context, string) (OperationFactV1, error)) {
	id := request.PathValue("id")
	current, err := s.service.Inspect(request.Context(), id)
	if err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	if err := s.authorize(request, current.Request.TenantID, current.Request.Action); err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	fact, err := transition(request.Context(), id)
	if err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	writeHTTPJSONV1(writer, http.StatusOK, fact)
}

func (s *HTTPServerV1) watch(writer http.ResponseWriter, request *http.Request) {
	after, err := parseUnsignedV1(request.URL.Query().Get("after"), 0)
	if err != nil {
		writeHTTPErrorV1(writer, err)
		return
	}
	limit64, err := parseUnsignedV1(request.URL.Query().Get("limit"), 100)
	if err != nil || limit64 == 0 || limit64 > 1000 {
		writeHTTPErrorV1(writer, fmt.Errorf("%w: watch limit must be 1..1000", ErrConflict))
		return
	}
	waitMillis, err := parseUnsignedV1(request.URL.Query().Get("wait_ms"), 0)
	if err != nil || waitMillis > 30_000 {
		writeHTTPErrorV1(writer, fmt.Errorf("%w: watch wait_ms must be 0..30000", ErrConflict))
		return
	}
	deadline := s.clock().Add(time.Duration(waitMillis) * time.Millisecond)
	for {
		items, cursor, err := s.service.Watch(request.Context(), after, int(limit64))
		if err != nil {
			writeHTTPErrorV1(writer, err)
			return
		}
		visible := make([]OperationFactV1, 0, len(items))
		for _, item := range items {
			if s.authorize(request, item.Request.TenantID, item.Request.Action) == nil {
				visible = append(visible, item)
			}
		}
		if len(visible) > 0 || waitMillis == 0 || !s.clock().Before(deadline) {
			writeHTTPJSONV1(writer, http.StatusOK, struct {
				Items  []OperationFactV1 `json:"items"`
				Cursor uint64            `json:"cursor"`
			}{visible, cursor})
			return
		}
		select {
		case <-request.Context().Done():
			writeHTTPErrorV1(writer, request.Context().Err())
			return
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (s *HTTPServerV1) authorize(request *http.Request, tenant string, action ActionV1) error {
	if err := s.auth.AuthorizeSandboxAPI(request, tenant, action); err != nil {
		return fmt.Errorf("authorization denied: %w", err)
	}
	return nil
}

func decodeHTTPJSONV1(writer http.ResponseWriter, request *http.Request, target any) error {
	if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		return fmt.Errorf("%w: Content-Type must be application/json", ErrConflict)
	}
	reader := http.MaxBytesReader(writer, request.Body, maxHTTPBodyBytesV1)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: invalid JSON body: %v", ErrConflict, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing JSON body", ErrConflict)
	}
	return nil
}

func parseUnsignedV1(value string, fallback uint64) (uint64, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid unsigned query value", ErrConflict)
	}
	return parsed, nil
}

func writeHTTPJSONV1(writer http.ResponseWriter, status int, value any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeHTTPErrorV1(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	category := "internal"
	reason := "sandbox_api_internal"
	switch {
	case errors.Is(err, ErrNotFound):
		status, category, reason = http.StatusNotFound, "not_found", "operation_not_found"
	case errors.Is(err, ErrConflict):
		status, category, reason = http.StatusConflict, "conflict", "operation_conflict"
	case errors.Is(err, ErrStale):
		status, category, reason = http.StatusPreconditionFailed, "precondition_failed", "operation_stale"
	case errors.Is(err, context.Canceled):
		status, category, reason = 499, "cancelled", "request_cancelled"
	case strings.Contains(err.Error(), "authorization denied"):
		status, category, reason = http.StatusForbidden, "forbidden", "transport_authorization_denied"
	}
	writeHTTPJSONV1(writer, status, ClosedErrorV1{Category: category, Reason: reason, Message: err.Error()})
}
