// Package reviewsdk is the transport-only Go client for the Review HTTP API.
// It has no Store, Verdict Owner, dispatch or commit capability.
package reviewsdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type TokenProviderV1 interface {
	ReviewTokenV1(context.Context) (string, error)
}

type TokenProviderFuncV1 func(context.Context) (string, error)

func (f TokenProviderFuncV1) ReviewTokenV1(ctx context.Context) (string, error) { return f(ctx) }

type Config struct {
	BaseURL       string
	HTTPClient    *http.Client
	TokenProvider TokenProviderV1
}

type Client struct {
	base   *url.URL
	http   *http.Client
	tokens TokenProviderV1
}

func New(config Config) (*Client, error) {
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review SDK base URL is invalid")
	}
	if config.HTTPClient == nil || config.TokenProvider == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "review SDK requires HTTP client and token provider")
	}
	copyURL := *parsed
	copyURL.RawQuery, copyURL.Fragment = "", ""
	return &Client{base: &copyURL, http: config.HTTPClient, tokens: config.TokenProvider}, nil
}

func (c *Client) SubmitV1(ctx context.Context, command service.SubmitCommandV1) (service.ReviewViewV1, error) {
	var result service.ReviewViewV1
	err := c.doJSON(ctx, http.MethodPost, "/v1/reviews", command, &result)
	return result, err
}

func (c *Client) GetV1(ctx context.Context, tenant core.TenantID, caseID string) (service.ReviewViewV1, error) {
	var result service.ReviewViewV1
	err := c.doJSON(ctx, http.MethodGet, reviewPath(tenant, caseID), nil, &result)
	return result, err
}

type ListRequestV1 struct {
	TenantID core.TenantID
	States   []contract.CaseStateV1
	Limit    int
	Cursor   string
}
type ListResultV1 struct {
	Cases      []contract.ReviewCaseV1 `json:"cases"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

func (c *Client) ListV1(ctx context.Context, request ListRequestV1) (ListResultV1, error) {
	values := url.Values{"tenant": []string{string(request.TenantID)}}
	if request.Limit > 0 {
		values.Set("limit", strconv.Itoa(request.Limit))
	}
	if request.Cursor != "" {
		values.Set("cursor", request.Cursor)
	} else {
		for _, state := range request.States {
			values.Add("state", string(state))
		}
	}
	var result ListResultV1
	err := c.doJSON(ctx, http.MethodGet, "/v1/reviews?"+values.Encode(), nil, &result)
	return result, err
}

// ListPendingV1 is a typed convenience over the same canonical List endpoint;
// it creates no second pending-state authority.
func (c *Client) ListPendingV1(ctx context.Context, tenant core.TenantID, limit int, cursor string) (ListResultV1, error) {
	states := []contract.CaseStateV1{
		contract.CaseRequestedV1, contract.CaseAdmittedV1, contract.CaseRoutedV1,
		contract.CaseWaitingReviewerV1, contract.CaseReviewingV1, contract.CaseAttestedV1,
		contract.CaseDecidingV1, contract.CaseWaitingRevisionV1, contract.CaseWaitingHumanV1,
		contract.CaseWaitingEvidenceV1, contract.CaseIndeterminateV1,
	}
	return c.ListV1(ctx, ListRequestV1{TenantID: tenant, States: states, Limit: limit, Cursor: cursor})
}

func (c *Client) EventsV1(ctx context.Context, tenant core.TenantID, caseID string) ([]contract.TraceFactV1, error) {
	var response struct {
		Events []contract.TraceFactV1 `json:"events"`
	}
	err := c.doJSON(ctx, http.MethodGet, reviewPath(tenant, caseID)+"/events", nil, &response)
	return response.Events, err
}

type EventsPageRequestV2 struct {
	TenantID core.TenantID
	CaseID   string
	Limit    int
	Cursor   string
}

type EventsPageResultV2 struct {
	Events     []contract.TraceFactV1 `json:"events"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

func (c *Client) EventsPageV2(ctx context.Context, request EventsPageRequestV2) (EventsPageResultV2, error) {
	values := url.Values{}
	if request.Limit > 0 {
		values.Set("limit", strconv.Itoa(request.Limit))
	}
	if request.Cursor != "" {
		values.Set("cursor", request.Cursor)
	}
	var result EventsPageResultV2
	err := c.doJSON(ctx, http.MethodGet, reviewPath(request.TenantID, request.CaseID)+"/events?"+values.Encode(), nil, &result)
	return result, err
}

func (c *Client) ClaimV1(ctx context.Context, mutation reviewport.ClaimAssignmentMutationV1) (contract.ReviewCaseV1, contract.ReviewerAssignmentV1, error) {
	var response struct {
		Case       contract.ReviewCaseV1         `json:"case"`
		Assignment contract.ReviewerAssignmentV1 `json:"assignment"`
	}
	err := c.doJSON(ctx, http.MethodPost, reviewPath(mutation.TenantID, mutation.CaseID)+"/claim", mutation, &response)
	return response.Case, response.Assignment, err
}

type AttestCommandV1 struct {
	Expected         reviewport.ExpectedFactV1 `json:"expected"`
	Attestation      contract.AttestationV1    `json:"attestation"`
	Trace            contract.TraceFactV1      `json:"trace"`
	AdditionalTraces []contract.TraceFactV1    `json:"additional_traces,omitempty"`
}

func (c *Client) AttestV1(ctx context.Context, command AttestCommandV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	var response struct {
		Case        contract.ReviewCaseV1  `json:"case"`
		Attestation contract.AttestationV1 `json:"attestation"`
	}
	err := c.doJSON(ctx, http.MethodPost, reviewPath(command.Attestation.TenantID, command.Attestation.CaseID)+"/attestations", command, &response)
	return response.Case, response.Attestation, err
}

// ResolveV1 uses the single Attestation admission path. The response remains
// an Attestation; only the Review Verdict Owner may form a Verdict afterward.
func (c *Client) ResolveV1(ctx context.Context, command AttestCommandV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	if command.Attestation.Resolution == contract.ResolutionRequestChangesV1 {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "use RequestChangesV1 for a request-changes Attestation")
	}
	return c.AttestV1(ctx, command)
}

func (c *Client) RequestChangesV1(ctx context.Context, command AttestCommandV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	if command.Attestation.Resolution != contract.ResolutionRequestChangesV1 {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "RequestChangesV1 requires a request-changes Attestation")
	}
	return c.AttestV1(ctx, command)
}

func (c *Client) CancelV1(ctx context.Context, command service.CancelCommandV1) (contract.ReviewCaseV1, error) {
	var result contract.ReviewCaseV1
	err := c.doJSON(ctx, http.MethodPost, reviewPath(command.TenantID, command.CaseID)+"/cancel", command, &result)
	return result, err
}

// CreateFindingWithTraceV2 publishes the Finding and its FindingObserved event
// at one Review Owner linearization point.
func (c *Client) CreateFindingWithTraceV2(ctx context.Context, mutation reviewport.CreateFindingWithTraceMutationV2) (contract.FindingV1, error) {
	var result contract.FindingV1
	err := c.doJSON(ctx, http.MethodPost, reviewPath(mutation.Finding.TenantID, mutation.Finding.CaseID)+"/finding-events", mutation, &result)
	return result, err
}

func (c *Client) CreateBehaviorFeedbackCandidateV1(ctx context.Context, value contract.BehaviorFeedbackCandidateV1) (contract.BehaviorFeedbackCandidateV1, error) {
	var result contract.BehaviorFeedbackCandidateV1
	err := c.doJSON(ctx, http.MethodPost, reviewPath(value.TenantID, value.Case.ID)+"/behavior-feedback-candidates", value, &result)
	return result, err
}

func (c *Client) AttachEvidenceV1(ctx context.Context, value contract.EvidenceAttachmentV1) (contract.EvidenceAttachmentV1, error) {
	var result contract.EvidenceAttachmentV1
	err := c.doJSON(ctx, http.MethodPost, reviewPath(value.TenantID, value.Case.ID)+"/evidence-attachments", value, &result)
	if err == nil {
		if validateErr := result.Validate(); validateErr != nil {
			return contract.EvidenceAttachmentV1{}, validateErr
		}
		if !reflect.DeepEqual(result, value) {
			return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review SDK Evidence Attachment response drifted from the exact submitted fact")
		}
	}
	return result, err
}

// WatchV1 reads one SSE connection. The callback is invoked in committed Trace
// order and receives only strict, sealed Trace facts. Callers may reconnect with
// the returned digest cursor; transport exactly-once is not promised.
func (c *Client) WatchV1(ctx context.Context, tenant core.TenantID, caseID, after string, receive func(contract.TraceFactV1) error) (string, error) {
	if receive == nil {
		return after, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review SDK watch callback is required")
	}
	request, err := c.newRequest(ctx, http.MethodGet, reviewPath(tenant, caseID)+"/watch", nil)
	if err != nil {
		return after, err
	}
	if after != "" {
		request.Header.Set("Last-Event-ID", after)
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return after, ctx.Err()
		}
		return after, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "review SDK watch transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return after, decodeErrorResponse(response)
	}
	if !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return after, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "review SDK watch response is not SSE")
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), core.MaxCanonicalDocumentBytes+4096)
	var eventID, data string
	flush := func() error {
		if eventID == "" && data == "" {
			return nil
		}
		if eventID == "" || data == "" {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "review SDK SSE event is incomplete")
		}
		var trace contract.TraceFactV1
		if err := core.DecodeStrictJSON([]byte(data), &trace); err != nil {
			return err
		}
		if err := trace.Validate(); err != nil {
			return err
		}
		if eventID != string(trace.Digest) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceCursorInvalid, "review SDK SSE cursor does not bind the trace")
		}
		if err := receive(trace); err != nil {
			return err
		}
		after, eventID, data = eventID, "", ""
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return after, err
			}
			continue
		}
		if strings.HasPrefix(line, "id: ") {
			if eventID != "" {
				return after, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "review SDK SSE event has duplicate id")
			}
			eventID = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "data: ") {
			if data != "" {
				return after, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "review SDK SSE event has duplicate data")
			}
			data = strings.TrimPrefix(line, "data: ")
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return after, ctx.Err()
		}
		return after, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "review SDK SSE stream failed")
	}
	if err := flush(); err != nil {
		return after, err
	}
	return after, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil || len(payload) > core.MaxCanonicalDocumentBytes {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review SDK request is not serializable or exceeds its bound")
		}
		body = bytes.NewReader(payload)
	}
	request, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "review SDK transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeErrorResponse(response)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, core.MaxCanonicalDocumentBytes+1))
	if err != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "review SDK response read failed")
	}
	if len(payload) > core.MaxCanonicalDocumentBytes {
		return core.NewError(core.ErrorConflict, core.ReasonCanonicalLimitExceeded, "review SDK response exceeds its bound")
	}
	if output != nil {
		return core.DecodeStrictJSON(payload, output)
	}
	return nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	relative, err := url.Parse(path)
	if err != nil || relative.IsAbs() {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review SDK request path is invalid")
	}
	target := c.base.ResolveReference(relative)
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review SDK request could not be created")
	}
	token, err := c.tokens.ReviewTokenV1(ctx)
	if err != nil {
		return nil, err
	}
	if len(token) < 32 || strings.ContainsAny(token, "\r\n") {
		return nil, core.NewError(core.ErrorUnauthenticated, core.ReasonInvalidReference, "review SDK token is unavailable")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	return request, nil
}

func decodeErrorResponse(response *http.Response) error {
	payload, err := io.ReadAll(io.LimitReader(response.Body, core.MaxCanonicalDocumentBytes+1))
	if err != nil || len(payload) > core.MaxCanonicalDocumentBytes {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "review SDK error response is unreadable")
	}
	var value struct {
		Category  core.ErrorCategory `json:"category"`
		Reason    core.ReasonCode    `json:"reason"`
		Message   string             `json:"message"`
		RequestID string             `json:"request_id"`
	}
	if err := core.DecodeStrictJSON(payload, &value); err != nil || value.Category == "" || value.Reason == "" {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidCanonicalForm, fmt.Sprintf("review SDK received HTTP %d without a typed error", response.StatusCode))
	}
	return core.NewError(value.Category, value.Reason, value.Message)
}

func reviewPath(tenant core.TenantID, caseID string) string {
	return "/v1/reviews/" + url.PathEscape(string(tenant)) + "/" + url.PathEscape(caseID)
}
