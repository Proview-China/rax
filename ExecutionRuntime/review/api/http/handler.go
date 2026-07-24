// Package reviewhttp provides the authenticated HTTP/JSON and SSE boundary of
// the Review-owned service. It never exposes a direct Verdict write.
package reviewhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MaxRequestBytesV1 = core.MaxCanonicalDocumentBytes

type ServiceV1 interface {
	SubmitV1(context.Context, service.SubmitCommandV1) (service.ReviewViewV1, error)
	InspectV1(context.Context, core.TenantID, string) (service.ReviewViewV1, error)
	ListV1(context.Context, reviewport.ListCasesRequestV1) (reviewport.ListCasesResultV1, error)
	EventsV1(context.Context, core.TenantID, string) ([]contract.TraceFactV1, error)
	EventsPageV2(context.Context, reviewport.ListTracePageRequestV2) (reviewport.ListTracePageResultV2, error)
	ClaimV1(context.Context, reviewport.ClaimAssignmentMutationV1) (contract.ReviewCaseV1, contract.ReviewerAssignmentV1, error)
	AttestV1(context.Context, reviewport.ExpectedFactV1, contract.AttestationV1, contract.TraceFactV1) (contract.ReviewCaseV1, contract.AttestationV1, error)
	AttestWithTraceV2(context.Context, reviewport.ExpectedFactV1, contract.AttestationV1, contract.TraceFactV1, []contract.TraceFactV1) (contract.ReviewCaseV1, contract.AttestationV1, error)
	CancelV1(context.Context, service.CancelCommandV1) (contract.ReviewCaseV1, error)
	CreateFindingForReviewerWithTraceV2(context.Context, string, reviewport.CreateFindingWithTraceMutationV2) (contract.FindingV1, error)
	CreateBehaviorFeedbackCandidateV1(context.Context, contract.BehaviorFeedbackCandidateV1) (contract.BehaviorFeedbackCandidateV1, error)
	AttachEvidenceV1(context.Context, contract.EvidenceAttachmentV1) (contract.EvidenceAttachmentV1, error)
}

type Config struct {
	Service       ServiceV1
	Authenticator AuthenticatorV1
	Clock         func() time.Time
	CursorKey     []byte
	CursorTTL     time.Duration
	WatchPoll     time.Duration
}

type Handler struct {
	service         ServiceV1
	auth            AuthenticatorV1
	clock           func() time.Time
	cursorKey       []byte
	cursorTTL       time.Duration
	watchPoll       time.Duration
	requestSequence atomic.Uint64
}

func New(config Config) (*Handler, error) {
	if nilcheck.IsNil(config.Service) || nilcheck.IsNil(config.Authenticator) || config.Clock == nil || len(config.CursorKey) < 32 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "review HTTP handler requires service, authenticator, clock and cursor key")
	}
	if config.CursorTTL <= 0 {
		config.CursorTTL = 15 * time.Minute
	}
	if config.CursorTTL > 24*time.Hour {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review cursor TTL exceeds its bound")
	}
	if config.WatchPoll <= 0 {
		config.WatchPoll = 250 * time.Millisecond
	}
	if config.WatchPoll < 10*time.Millisecond || config.WatchPoll > 30*time.Second {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review watch poll interval is outside its bound")
	}
	return &Handler{service: config.Service, auth: config.Authenticator, clock: config.Clock, cursorKey: append([]byte(nil), config.CursorKey...), cursorTTL: config.CursorTTL, watchPoll: config.WatchPoll}, nil
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	requestID := h.requestID(request)
	principal, err := h.auth.AuthenticateReviewV1(request.Context(), request)
	if err == nil {
		err = principal.ValidateCurrent(h.clock())
	}
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	if request.URL.Path == "/v1/reviews" || request.URL.Path == "/v1/reviews/" {
		switch request.Method {
		case http.MethodPost:
			h.submit(writer, request, requestID, principal)
		case http.MethodGet:
			h.list(writer, request, requestID, principal)
		default:
			h.writeMethodNotAllowed(writer, requestID)
		}
		return
	}
	segments, pathErr := reviewPathSegments(request.URL.EscapedPath())
	if pathErr != nil || len(segments) < 2 {
		h.writeError(writer, requestID, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "review API route not found"))
		return
	}
	tenant, caseID := core.TenantID(segments[0]), segments[1]
	if tenant != principal.TenantID {
		h.writeError(writer, requestID, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "review API tenant is outside the authenticated principal"))
		return
	}
	if len(segments) == 2 && request.Method == http.MethodGet {
		h.get(writer, request, requestID, principal, tenant, caseID)
		return
	}
	if len(segments) != 3 {
		h.writeError(writer, requestID, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "review API route not found"))
		return
	}
	switch segments[2] {
	case "events":
		if request.Method == http.MethodGet {
			h.events(writer, request, requestID, principal, tenant, caseID)
			return
		}
	case "watch":
		if request.Method == http.MethodGet {
			h.watch(writer, request, requestID, principal, tenant, caseID)
			return
		}
	case "claim":
		if request.Method == http.MethodPost {
			h.claim(writer, request, requestID, principal, tenant, caseID)
			return
		}
	case "attestations":
		if request.Method == http.MethodPost {
			h.attest(writer, request, requestID, principal, tenant, caseID)
			return
		}
	case "cancel":
		if request.Method == http.MethodPost {
			h.cancel(writer, request, requestID, principal, tenant, caseID)
			return
		}
	case "finding-events":
		if request.Method == http.MethodPost {
			h.createFindingWithTraceV2(writer, request, requestID, principal, tenant, caseID)
			return
		}
	case "behavior-feedback-candidates":
		if request.Method == http.MethodPost {
			h.createBehaviorFeedback(writer, request, requestID, principal, tenant, caseID)
			return
		}
	case "evidence-attachments":
		if request.Method == http.MethodPost {
			h.attachEvidence(writer, request, requestID, principal, tenant, caseID)
			return
		}
	}
	h.writeMethodNotAllowed(writer, requestID)
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1) {
	if err := requireCapability(p, CapabilitySubmitV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	var command service.SubmitCommandV1
	if err := decodeBody(w, r, &command); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	if command.Request.TenantID != p.TenantID || command.Target.TenantID != p.TenantID || (command.ResultBundle != nil && command.ResultBundle.TenantID != p.TenantID) {
		h.writeError(w, requestID, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "review submit tenant drifted"))
		return
	}
	if !validPathIdentifierV1(command.Request.CaseID) {
		h.writeError(w, requestID, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review API Case ID is not path-safe"))
		return
	}
	view, err := h.service.SubmitV1(r.Context(), command)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, view)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1, tenant core.TenantID, caseID string) {
	if err := requireCapability(p, CapabilityReadV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	view, err := h.service.InspectV1(r.Context(), tenant, caseID)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusOK, view)
}

type listResponseV1 struct {
	Cases      []contract.ReviewCaseV1 `json:"cases"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1) {
	if err := requireCapability(p, CapabilityReadV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	query := r.URL.Query()
	tenant := core.TenantID(query.Get("tenant"))
	if tenant == "" || tenant != p.TenantID {
		h.writeError(w, requestID, core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "review list tenant is required and must match principal"))
		return
	}
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			h.writeError(w, requestID, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review list limit is invalid"))
			return
		}
		limit = parsed
	}
	request := reviewport.ListCasesRequestV1{TenantID: tenant, Limit: limit}
	for _, state := range query["state"] {
		request.States = append(request.States, contract.CaseStateV1(state))
	}
	if encoded := query.Get("cursor"); encoded != "" {
		cursor, err := decodeCursorV1(encoded, h.cursorKey, h.clock())
		if err != nil {
			h.writeError(w, requestID, err)
			return
		}
		if cursor.TenantID != tenant || len(request.States) != 0 {
			h.writeError(w, requestID, core.NewError(core.ErrorConflict, core.ReasonEvidenceCursorInvalid, "review list cursor request drifted"))
			return
		}
		request.States, request.AfterID = cursor.States, cursor.AfterID
	}
	result, err := h.service.ListV1(r.Context(), request)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	response := listResponseV1{Cases: result.Cases}
	if result.NextAfterID != "" {
		response.NextCursor, err = encodeCursorV1(listCursorV1{TenantID: tenant, States: append([]contract.CaseStateV1(nil), request.States...), AfterID: result.NextAfterID, ExpiresUnixNano: h.clock().Add(h.cursorTTL).UnixNano()}, h.cursorKey)
		if err != nil {
			h.writeError(w, requestID, err)
			return
		}
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1, tenant core.TenantID, caseID string) {
	if err := requireCapability(p, CapabilityReadV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	query := r.URL.Query()
	if query.Get("limit") == "" && query.Get("cursor") == "" {
		values, err := h.service.EventsV1(r.Context(), tenant, caseID)
		if err != nil {
			h.writeError(w, requestID, err)
			return
		}
		h.writeJSON(w, http.StatusOK, struct {
			Events []contract.TraceFactV1 `json:"events"`
		}{values})
		return
	}
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			h.writeError(w, requestID, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review Trace page limit is invalid"))
			return
		}
		limit = parsed
	}
	pageRequest := reviewport.ListTracePageRequestV2{TenantID: tenant, CaseID: caseID, Limit: limit}
	if encoded := query.Get("cursor"); encoded != "" {
		cursor, err := decodeTraceCursorV2(encoded, h.cursorKey, h.clock())
		if err != nil {
			h.writeError(w, requestID, err)
			return
		}
		if cursor.TenantID != tenant || cursor.CaseID != caseID {
			h.writeError(w, requestID, core.NewError(core.ErrorConflict, core.ReasonEvidenceCursorInvalid, "review Trace cursor request drifted"))
			return
		}
		pageRequest.After = &cursor.After
	}
	page, err := h.service.EventsPageV2(r.Context(), pageRequest)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	response := struct {
		Events     []contract.TraceFactV1 `json:"events"`
		NextCursor string                 `json:"next_cursor,omitempty"`
	}{Events: page.Events}
	if page.Next != nil {
		response.NextCursor, err = encodeTraceCursorV2(traceCursorV2{TenantID: tenant, CaseID: caseID, After: *page.Next, ExpiresUnixNano: h.clock().Add(h.cursorTTL).UnixNano()}, h.cursorKey)
		if err != nil {
			h.writeError(w, requestID, err)
			return
		}
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *Handler) watch(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1, tenant core.TenantID, caseID string) {
	if err := requireCapability(p, CapabilityReadV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, requestID, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "review SSE flushing is unavailable"))
		return
	}
	values, err := h.service.EventsV1(r.Context(), tenant, caseID)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	last := r.Header.Get("Last-Event-ID")
	start, err := traceStartAfter(values, last)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	write := func(events []contract.TraceFactV1, begin int) bool {
		for _, event := range events[begin:] {
			payload, _ := json.Marshal(event)
			if _, err := fmt.Fprintf(w, "id: %s\nevent: review.trace\ndata: %s\n\n", event.Digest, payload); err != nil {
				return false
			}
			last = string(event.Digest)
			flusher.Flush()
		}
		return true
	}
	if !write(values, start) {
		return
	}
	ticker := time.NewTicker(h.watchPoll)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			values, err = h.service.EventsV1(r.Context(), tenant, caseID)
			if err != nil {
				return
			}
			start, err = traceStartAfter(values, last)
			if err != nil {
				return
			}
			if !write(values, start) {
				return
			}
		}
	}
}

type attestBodyV1 struct {
	Expected         reviewport.ExpectedFactV1 `json:"expected"`
	Attestation      contract.AttestationV1    `json:"attestation"`
	Trace            contract.TraceFactV1      `json:"trace"`
	AdditionalTraces []contract.TraceFactV1    `json:"additional_traces,omitempty"`
}

func (h *Handler) claim(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1, tenant core.TenantID, caseID string) {
	if err := requireCapability(p, CapabilityClaimV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	var mutation reviewport.ClaimAssignmentMutationV1
	if err := decodeBody(w, r, &mutation); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	if mutation.TenantID != tenant || mutation.CaseID != caseID || mutation.LeaseHolder != p.SubjectID {
		h.writeError(w, requestID, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "review claim path/body drifted"))
		return
	}
	caseFact, assignment, err := h.service.ClaimV1(r.Context(), mutation)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusOK, struct {
		Case       contract.ReviewCaseV1         `json:"case"`
		Assignment contract.ReviewerAssignmentV1 `json:"assignment"`
	}{caseFact, assignment})
}

func (h *Handler) attest(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1, tenant core.TenantID, caseID string) {
	if err := requireCapability(p, CapabilityAttestV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	var body attestBodyV1
	if err := decodeBody(w, r, &body); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	if body.Attestation.TenantID != tenant || body.Attestation.CaseID != caseID || body.Attestation.ReviewerID != p.SubjectID || body.Trace.TenantID != tenant {
		h.writeError(w, requestID, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "review attestation path/body drifted"))
		return
	}
	caseFact, attestation, err := h.service.AttestWithTraceV2(r.Context(), body.Expected, body.Attestation, body.Trace, body.AdditionalTraces)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, struct {
		Case        contract.ReviewCaseV1  `json:"case"`
		Attestation contract.AttestationV1 `json:"attestation"`
	}{caseFact, attestation})
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1, tenant core.TenantID, caseID string) {
	if err := requireCapability(p, CapabilityCancelV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	var command service.CancelCommandV1
	if err := decodeBody(w, r, &command); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	if command.TenantID != tenant || command.CaseID != caseID || command.Trace.TenantID != tenant {
		h.writeError(w, requestID, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "review cancel path/body drifted"))
		return
	}
	caseFact, err := h.service.CancelV1(r.Context(), command)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusOK, caseFact)
}

func (h *Handler) createFindingWithTraceV2(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1, tenant core.TenantID, caseID string) {
	if err := requireCapability(p, CapabilityFindingV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	var mutation reviewport.CreateFindingWithTraceMutationV2
	if err := decodeBody(w, r, &mutation); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	if mutation.Finding.TenantID != tenant || mutation.Finding.CaseID != caseID || mutation.Trace.TenantID != tenant || mutation.Trace.CaseID != caseID {
		h.writeError(w, requestID, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "review Finding V2 path/body/Trace drifted"))
		return
	}
	created, err := h.service.CreateFindingForReviewerWithTraceV2(r.Context(), p.SubjectID, mutation)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) createBehaviorFeedback(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1, tenant core.TenantID, caseID string) {
	if err := requireCapability(p, CapabilityFeedbackV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	var value contract.BehaviorFeedbackCandidateV1
	if err := decodeBody(w, r, &value); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	if value.TenantID != tenant || value.Case.ID != caseID {
		h.writeError(w, requestID, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "review Behavior Feedback path/body drifted"))
		return
	}
	created, err := h.service.CreateBehaviorFeedbackCandidateV1(r.Context(), value)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) attachEvidence(w http.ResponseWriter, r *http.Request, requestID string, p PrincipalV1, tenant core.TenantID, caseID string) {
	if err := requireCapability(p, CapabilityEvidenceAttachV1); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	var value contract.EvidenceAttachmentV1
	if err := decodeBody(w, r, &value); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	if value.TenantID != tenant || value.Case.ID != caseID || value.SubmitterID != p.SubjectID {
		h.writeError(w, requestID, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "review Evidence Attachment path, tenant or submitter drifted"))
		return
	}
	actual := h.clock()
	if err := p.ValidateCurrent(actual); err != nil {
		h.writeError(w, requestID, err)
		return
	}
	if value.ExpiresUnixNano > p.ExpiresUnixNano {
		h.writeError(w, requestID, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "review Evidence Attachment TTL exceeds the authenticated principal"))
		return
	}
	created, err := h.service.AttachEvidenceV1(r.Context(), value)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, created)
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) error {
	if media := r.Header.Get("Content-Type"); media != "" && !strings.HasPrefix(strings.ToLower(media), "application/json") {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review request content type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytesV1)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review request body exceeds its bound")
		}
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "review request body read failed")
	}
	if err := core.DecodeStrictJSON(payload, target); err != nil {
		if core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonDuplicateCanonicalKey, "review request JSON contains a duplicate key")
		}
		return err
	}
	return nil
}

func traceStartAfter(values []contract.TraceFactV1, last string) (int, error) {
	if last == "" {
		return 0, nil
	}
	for index, value := range values {
		if string(value.Digest) == last {
			return index + 1, nil
		}
	}
	return 0, core.NewError(core.ErrorConflict, core.ReasonEvidenceCursorInvalid, "review SSE cursor is not in the current trace history")
}

func reviewPathSegments(escaped string) ([]string, error) {
	prefix := "/v1/reviews/"
	if !strings.HasPrefix(escaped, prefix) {
		return nil, errors.New("not review path")
	}
	raw := strings.Split(strings.TrimPrefix(escaped, prefix), "/")
	values := make([]string, len(raw))
	for i, item := range raw {
		value, err := url.PathUnescape(item)
		if err != nil || value == "" || strings.Contains(value, "/") {
			return nil, errors.New("invalid review path")
		}
		values[i] = value
	}
	return values, nil
}

func requireCapability(principal PrincipalV1, capability string) error {
	if !principal.Allows(capability) {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownCapability, "review API capability is missing")
	}
	return nil
}

func (h *Handler) requestID(request *http.Request) string {
	if value := request.Header.Get("X-Request-ID"); value != "" && len(value) <= 128 && !strings.ContainsAny(value, "\r\n") {
		return value
	}
	sequence := h.requestSequence.Add(1)
	return fmt.Sprintf("review-%d-%d", h.clock().UnixNano(), sequence)
}

type errorResponseV1 struct {
	Category  core.ErrorCategory `json:"category"`
	Reason    core.ReasonCode    `json:"reason"`
	Message   string             `json:"message"`
	RequestID string             `json:"request_id"`
}

func (h *Handler) writeError(w http.ResponseWriter, requestID string, err error) {
	category, reason, message := core.ErrorInternal, core.ReasonInvalidState, "review request failed"
	var domain *core.DomainError
	if errors.As(err, &domain) {
		category, reason, message = domain.Category, domain.Reason, domain.Message
	}
	status := map[core.ErrorCategory]int{core.ErrorInvalidArgument: 400, core.ErrorUnauthenticated: 401, core.ErrorForbidden: 403, core.ErrorNotFound: 404, core.ErrorConflict: 409, core.ErrorPreconditionFailed: 412, core.ErrorCapabilityUnavailable: 501, core.ErrorIndeterminate: 503, core.ErrorRateLimited: 429, core.ErrorUnavailable: 503, core.ErrorInternal: 500}[category]
	if status == 0 {
		status = 500
	}
	if reason == core.ReasonCanonicalLimitExceeded {
		status = http.StatusRequestEntityTooLarge
	}
	h.writeJSON(w, status, errorResponseV1{category, reason, message, requestID})
}
func (h *Handler) writeMethodNotAllowed(w http.ResponseWriter, requestID string) {
	h.writeJSON(w, http.StatusMethodNotAllowed, errorResponseV1{Category: core.ErrorInvalidArgument, Reason: core.ReasonInvalidState, Message: "review API method is not allowed", RequestID: requestID})
}
func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
