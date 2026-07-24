package reviewhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	reviewhttp "github.com/Proview-China/rax/ExecutionRuntime/review/api/http"
	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	reviewsdk "github.com/Proview-China/rax/ExecutionRuntime/review/sdk/go"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const testToken = "0123456789abcdef0123456789abcdef"

type httpFixture struct {
	now     time.Time
	handler http.Handler
	owner   *service.Service
	store   *memory.Store
	clock   *testkit.ManualClock
	token   string
}

func newHTTPFixture(t testing.TB) httpFixture {
	t.Helper()
	now := time.Unix(1_900_100_000, 0)
	clock := testkit.NewClock(now)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, err := service.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	principal := reviewhttp.PrincipalV1{TenantID: "tenant-a", SubjectID: "reviewer-a", Capabilities: []string{reviewhttp.CapabilityAttestV1, reviewhttp.CapabilityCancelV1, reviewhttp.CapabilityClaimV1, reviewhttp.CapabilityFeedbackV1, reviewhttp.CapabilityEvidenceAttachV1, reviewhttp.CapabilityFindingV1, reviewhttp.CapabilityReadV1, reviewhttp.CapabilitySubmitV1}, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	auth, err := reviewhttp.NewStaticBearerAuthenticatorV1(map[string]reviewhttp.PrincipalV1{testToken: principal})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := reviewhttp.New(reviewhttp.Config{Service: owner, Authenticator: auth, Clock: clock.Now, CursorKey: bytes.Repeat([]byte{0x42}, 32), WatchPoll: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return httpFixture{now: now, handler: handler, owner: owner, store: store, clock: clock, token: testToken}
}

func BenchmarkHTTPInspectV1(b *testing.B) {
	f := newHTTPFixture(b)
	command := submitCommand(f.now, testkit.Target(f.now), "case-benchmark")
	if _, err := f.owner.SubmitV1(context.Background(), command); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := httptest.NewRequest(http.MethodGet, "/v1/reviews/tenant-a/case-benchmark", nil)
		r.Header.Set("Authorization", "Bearer "+f.token)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			b.Fatalf("status=%d", w.Code)
		}
	}
}

func submitCommand(now time.Time, target contract.TargetSnapshotV1, caseID string) service.SubmitCommandV1 {
	request := testkit.Request(now, target, caseID)
	trace := testkit.TraceForTarget(now, caseID, target, contract.TraceRequestedV1, 1, request.ID)
	trace.ID = "trace-requested-" + caseID
	trace.Digest = ""
	trace, _ = contract.SealTraceFactV1(trace)
	return service.SubmitCommandV1{Request: request, Target: target, Trace: trace}
}

func request(t *testing.T, handler http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	r := httptest.NewRequest(method, path, reader)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestHTTPSubmitGetAndStrictJSONV1(t *testing.T) {
	f := newHTTPFixture(t)
	command := submitCommand(f.now, testkit.Target(f.now), "case-http")
	unauthenticated := request(t, f.handler, "", http.MethodPost, "/v1/reviews", command)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", unauthenticated.Code)
	}
	created := request(t, f.handler, f.token, http.MethodPost, "/v1/reviews", command)
	if created.Code != http.StatusCreated {
		t.Fatalf("submit status=%d body=%s", created.Code, created.Body.String())
	}
	got := request(t, f.handler, f.token, http.MethodGet, "/v1/reviews/tenant-a/case-http", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("get status=%d", got.Code)
	}
	duplicate := httptest.NewRequest(http.MethodPost, "/v1/reviews", strings.NewReader(`{"request":null,"request":null,"target":null,"trace":null}`))
	duplicate.Header.Set("Authorization", "Bearer "+f.token)
	duplicate.Header.Set("Content-Type", "application/json")
	duplicateResult := httptest.NewRecorder()
	f.handler.ServeHTTP(duplicateResult, duplicate)
	if duplicateResult.Code != http.StatusBadRequest {
		t.Fatalf("duplicate JSON status=%d body=%s", duplicateResult.Code, duplicateResult.Body.String())
	}
	unknown := request(t, f.handler, f.token, http.MethodPost, "/v1/reviews", map[string]any{"unknown": true})
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown JSON status=%d", unknown.Code)
	}
	wrongTenant := command
	wrongTenant.Request.TenantID = "tenant-b"
	forbidden := request(t, f.handler, f.token, http.MethodPost, "/v1/reviews", wrongTenant)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("tenant drift status=%d", forbidden.Code)
	}
	unsafePath := submitCommand(f.now, testkit.Target(f.now), "case/unsafe")
	badPath := request(t, f.handler, f.token, http.MethodPost, "/v1/reviews", unsafePath)
	if badPath.Code != http.StatusBadRequest {
		t.Fatalf("path-unsafe Case ID status=%d body=%s", badPath.Code, badPath.Body.String())
	}
}

func TestHTTPListCursorTamperV1(t *testing.T) {
	f := newHTTPFixture(t)
	first := testkit.Target(f.now)
	if _, err := f.owner.SubmitV1(context.Background(), submitCommand(f.now, first, "case-a")); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = "target-b"
	second.Digest = ""
	second, _ = contract.SealTargetSnapshotV1(second)
	if _, err := f.owner.SubmitV1(context.Background(), submitCommand(f.now, second, "case-b")); err != nil {
		t.Fatal(err)
	}
	page := request(t, f.handler, f.token, http.MethodGet, "/v1/reviews?tenant=tenant-a&limit=1", nil)
	if page.Code != 200 {
		t.Fatalf("list status=%d %s", page.Code, page.Body.String())
	}
	var decoded struct {
		Cases      []contract.ReviewCaseV1 `json:"cases"`
		NextCursor string                  `json:"next_cursor"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &decoded); err != nil || len(decoded.Cases) != 1 || decoded.NextCursor == "" {
		t.Fatalf("bad page: %v %+v", err, decoded)
	}
	tampered := decoded.NextCursor[:len(decoded.NextCursor)-1] + "A"
	bad := request(t, f.handler, f.token, http.MethodGet, "/v1/reviews?tenant=tenant-a&cursor="+tampered, nil)
	if bad.Code != http.StatusConflict && bad.Code != http.StatusBadRequest {
		t.Fatalf("tampered cursor status=%d", bad.Code)
	}
	next := request(t, f.handler, f.token, http.MethodGet, "/v1/reviews?tenant=tenant-a&cursor="+decoded.NextCursor, nil)
	if next.Code != 200 {
		t.Fatalf("next page status=%d %s", next.Code, next.Body.String())
	}
}

func TestSDKSubmitGetWatchV1(t *testing.T) {
	f := newHTTPFixture(t)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	client, err := reviewsdk.New(reviewsdk.Config{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: reviewsdk.TokenProviderFuncV1(func(context.Context) (string, error) { return f.token, nil })})
	if err != nil {
		t.Fatal(err)
	}
	command := submitCommand(f.now, testkit.Target(f.now), "case-sdk")
	bundle := testkit.ResultBundle(f.now, command.Target.TenantID, "bundle-sdk")
	command.Request.ResultBundle = &contract.ExactResourceRefV1{ID: bundle.ID, Revision: bundle.Revision, Digest: bundle.Digest}
	command.Request.Digest = ""
	command.Request, _ = contract.SealReviewRequestV1(command.Request)
	command.ResultBundle = &bundle
	view, err := client.SubmitV1(context.Background(), command)
	if err != nil || view.Case.ID != "case-sdk" || view.ResultBundle == nil || view.ResultBundle.Digest != bundle.Digest {
		t.Fatalf("SDK submit: %v", err)
	}
	got, err := client.GetV1(context.Background(), "tenant-a", "case-sdk")
	if err != nil || got.Case.Digest != view.Case.Digest {
		t.Fatalf("SDK get: %v", err)
	}
	stop := errors.New("stop after committed event")
	cursor, err := client.WatchV1(context.Background(), "tenant-a", "case-sdk", "", func(trace contract.TraceFactV1) error {
		if trace.Event != contract.TraceRequestedV1 {
			t.Fatalf("unexpected trace %s", trace.Event)
		}
		return stop
	})
	if !errors.Is(err, stop) || cursor != "" {
		t.Fatalf("SDK watch result cursor=%q err=%v", cursor, err)
	}
}

func TestHTTPAndSDKEventsPageV2ShareExactReader(t *testing.T) {
	f := newHTTPFixture(t)
	command := submitCommand(f.now, testkit.Target(f.now), "case-events-v2")
	view, err := f.owner.SubmitV1(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second := testkit.Trace(f.now.Add(time.Second), view.Case, contract.TraceAdmittedV1, 2, command.Request.ID)
	if _, err := f.store.InjectTraceForTestV1(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	firstHTTP := request(t, f.handler, f.token, http.MethodGet, "/v1/reviews/tenant-a/case-events-v2/events?limit=1", nil)
	if firstHTTP.Code != http.StatusOK {
		t.Fatalf("HTTP page status=%d body=%s", firstHTTP.Code, firstHTTP.Body.String())
	}
	var first reviewsdk.EventsPageResultV2
	if err := json.Unmarshal(firstHTTP.Body.Bytes(), &first); err != nil || len(first.Events) != 1 || first.NextCursor == "" {
		t.Fatalf("HTTP page drifted: %+v err=%v", first, err)
	}
	server := httptest.NewServer(f.handler)
	defer server.Close()
	client, err := reviewsdk.New(reviewsdk.Config{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: reviewsdk.TokenProviderFuncV1(func(context.Context) (string, error) { return f.token, nil })})
	if err != nil {
		t.Fatal(err)
	}
	next, err := client.EventsPageV2(context.Background(), reviewsdk.EventsPageRequestV2{TenantID: "tenant-a", CaseID: "case-events-v2", Limit: 1, Cursor: first.NextCursor})
	if err != nil || len(next.Events) != 1 || next.Events[0].Digest != second.Digest {
		t.Fatalf("SDK next page drifted: %+v err=%v", next, err)
	}
	tampered := first.NextCursor[:len(first.NextCursor)-1] + "A"
	bad := request(t, f.handler, f.token, http.MethodGet, "/v1/reviews/tenant-a/case-events-v2/events?limit=1&cursor="+tampered, nil)
	if bad.Code != http.StatusBadRequest && bad.Code != http.StatusConflict {
		t.Fatalf("tampered event cursor status=%d body=%s", bad.Code, bad.Body.String())
	}
	f.clock.Advance(16 * time.Minute)
	expired := request(t, f.handler, f.token, http.MethodGet, "/v1/reviews/tenant-a/case-events-v2/events?limit=1&cursor="+first.NextCursor, nil)
	if expired.Code == http.StatusOK {
		t.Fatalf("expired event cursor was accepted: %s", expired.Body.String())
	}
}

func TestSDKFindingWithTraceV2PublishesOneAtomicEvent(t *testing.T) {
	f := newHTTPFixture(t)
	command := submitCommand(f.now, testkit.Target(f.now), "case-finding-v2")
	view, err := f.owner.SubmitV1(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := caseengine.New(f.store, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	c := view.Case
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		f.clock.Advance(time.Second)
		c, err = engine.TransitionWithTraceV2(context.Background(), caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Next: state}, Trace: testkit.TransitionTrace(f.clock.Now(), c, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.clock.Advance(time.Second)
	round := testkit.Round(f.clock.Now(), c, contract.RouteHumanV1)
	assignment := testkit.Assignment(f.clock.Now(), c, round, contract.RouteHumanV1)
	c, round, assignment, err = f.owner.StartRoundV1(context.Background(), reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(f.clock.Now(), c, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	claimTraceCase := c
	claimTraceCase.Revision++
	claimTraceCase.State = contract.CaseReviewingV1
	claimTraceCase.UpdatedUnixNano = f.clock.Now().UnixNano()
	claimTraceCase.Digest = ""
	claimTraceCase, err = contract.SealReviewCaseV1(claimTraceCase)
	if err != nil {
		t.Fatal(err)
	}
	claimTrace := testkit.Trace(f.clock.Now(), claimTraceCase, contract.TraceStartedV1, 3, assignment.ID)
	c, assignment, err = f.owner.ClaimV1(context.Background(), reviewport.ClaimAssignmentMutationV1{TenantID: c.TenantID, CaseID: c.ID, AssignmentID: assignment.ID, ExpectedCase: reviewport.ExpectedV1(c.Revision, c.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: f.clock.Now().Add(10 * time.Minute).UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano(), Traces: []contract.TraceFactV1{claimTrace}})
	if err != nil {
		t.Fatal(err)
	}
	evidence := []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("http-finding-v2")}
	finding, err := contract.SealFindingV1(contract.FindingV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: "finding-http-v2", Revision: 1, CreatedUnixNano: f.clock.Now().UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano()}, CaseID: c.ID, CaseRevision: c.Revision, RoundID: round.ID, RoundRevision: round.Revision, RoundDigest: round.Digest, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, Category: "review.test/http", Priority: "high", Anchor: "http", Claim: "atomic Finding event", Impact: "audit closure", Evidence: evidence, Status: contract.FindingOpenV1, ExpiresUnixNano: f.clock.Now().Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	trace := testkit.Trace(f.clock.Now(), c, contract.TraceFindingV1, 4, finding.ID)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	client, err := reviewsdk.New(reviewsdk.Config{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: reviewsdk.TokenProviderFuncV1(func(context.Context) (string, error) { return f.token, nil })})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateFindingWithTraceV2(context.Background(), reviewport.CreateFindingWithTraceMutationV2{Finding: finding, Trace: trace})
	if err != nil || created.Digest != finding.Digest {
		t.Fatalf("SDK Finding V2: %+v err=%v", created, err)
	}
	events, err := client.EventsPageV2(context.Background(), reviewsdk.EventsPageRequestV2{TenantID: c.TenantID, CaseID: c.ID, Limit: reviewport.MaxTracePageV2})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events.Events {
		if event.Digest == trace.Digest {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("FindingObserved event count=%d, want 1", count)
	}
}

func TestPublicFindingV1SurfaceIsRemovedAndHTTPFailsClosed(t *testing.T) {
	f := newHTTPFixture(t)
	target := testkit.Target(f.now)
	view, err := f.owner.SubmitV1(context.Background(), submitCommand(f.now, target, "case-finding-sdk"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := caseengine.New(f.store, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	current := view.Case
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		f.clock.Advance(time.Second)
		current, err = engine.TransitionWithTraceV2(context.Background(), caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: current.TenantID, CaseID: current.ID, Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Next: state}, Trace: testkit.TransitionTrace(f.clock.Now(), current, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.clock.Advance(time.Second)
	round := testkit.Round(f.clock.Now(), current, contract.RouteHumanV1)
	assignment := testkit.Assignment(f.clock.Now(), current, round, contract.RouteHumanV1)
	current, _, assignment, err = engine.StartRoundV1(context.Background(), reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(f.clock.Now(), current, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	current, assignment, err = engine.ClaimAssignmentV1(context.Background(), reviewport.ClaimAssignmentMutationV1{TenantID: current.TenantID, ExpectedCase: reviewport.ExpectedV1(current.Revision, current.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: current.ID, AssignmentID: assignment.ID, LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: f.clock.Now().Add(time.Minute).UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(f.clock.Now(), current, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	evidence := []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("finding-sdk")}
	finding, err := contract.SealFindingV1(contract.FindingV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: current.TenantID, ID: "finding-sdk", Revision: 1, CreatedUnixNano: f.clock.Now().UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano()}, CaseID: current.ID, CaseRevision: current.Revision, RoundID: round.ID, RoundRevision: round.Revision, RoundDigest: round.Digest, TargetID: current.TargetID, TargetRevision: current.TargetRevision, TargetDigest: current.TargetDigest, Category: "praxis.review/correctness", Priority: "p1", Anchor: "artifact/root", Claim: "the candidate contradicts the declared acceptance", Impact: "the result cannot be accepted", Evidence: evidence, Status: contract.FindingOpenV1, ExpiresUnixNano: f.clock.Now().Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	for name, typ := range map[string]reflect.Type{
		"caseengine.Engine":  reflect.TypeOf((*caseengine.Engine)(nil)),
		"service.Service":    reflect.TypeOf((*service.Service)(nil)),
		"reviewsdk.Client":   reflect.TypeOf((*reviewsdk.Client)(nil)),
		"reviewhttp.Service": reflect.TypeOf((*reviewhttp.ServiceV1)(nil)).Elem(),
	} {
		if _, ok := typ.MethodByName("CreateFindingV1"); ok {
			t.Fatalf("%s still exposes eventless CreateFindingV1", name)
		}
	}
	legacyHTTP := request(t, f.handler, f.token, http.MethodPost, "/v1/reviews/"+string(finding.TenantID)+"/"+finding.CaseID+"/findings", finding)
	if legacyHTTP.Code != http.StatusNotFound && legacyHTTP.Code != http.StatusMethodNotAllowed {
		t.Fatalf("removed HTTP Finding endpoint status=%d body=%s", legacyHTTP.Code, legacyHTTP.Body.String())
	}
	if _, err := f.store.InspectFindingV1(context.Background(), finding.TenantID, finding.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("legacy HTTP Finding leaked an eventless fact: %v", err)
	}
}

func TestSDKAttachEvidencePersistsNeutralRefsV1(t *testing.T) {
	f := newHTTPFixture(t)
	view, err := f.owner.SubmitV1(context.Background(), submitCommand(f.now, testkit.Target(f.now), "case-evidence-sdk"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := contract.SealEvidenceAttachmentV1(contract.EvidenceAttachmentV1{
		FactIdentityV1:   contract.FactIdentityV1{TenantID: view.Case.TenantID, ID: "attachment-sdk", Revision: 1, CreatedUnixNano: f.clock.Now().UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano()},
		IdempotencyKey:   "attachment-sdk-key",
		Case:             contract.ExactResourceRefV1{ID: view.Case.ID, Revision: view.Case.Revision, Digest: view.Case.Digest},
		Target:           contract.ExactResourceRefV1{ID: view.Target.ID, Revision: view.Target.Revision, Digest: view.Target.Digest},
		SubmitterID:      "reviewer-a",
		Evidence:         []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("attachment-sdk-evidence")},
		ObservedUnixNano: f.clock.Now().UnixNano(), ExpiresUnixNano: f.clock.Now().Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(f.handler)
	defer server.Close()
	client, err := reviewsdk.New(reviewsdk.Config{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: reviewsdk.TokenProviderFuncV1(func(context.Context) (string, error) { return f.token, nil })})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.AttachEvidenceV1(context.Background(), value)
	if err != nil || created.Digest != value.Digest || created.Evidence[0] != value.Evidence[0] {
		t.Fatalf("SDK attach Evidence failed: %+v %v", created, err)
	}
	stored, err := f.store.InspectEvidenceAttachmentByIdempotencyV1(context.Background(), value.TenantID, value.IdempotencyKey)
	if err != nil || stored.Digest != value.Digest {
		t.Fatalf("stored Evidence Attachment drifted: %+v %v", stored, err)
	}
}

func TestHTTPAttachEvidenceRejectsSubmitterAndPathDriftV1(t *testing.T) {
	f := newHTTPFixture(t)
	value := contract.EvidenceAttachmentV1{FactIdentityV1: contract.FactIdentityV1{TenantID: "tenant-a"}, Case: contract.ExactResourceRefV1{ID: "other-case"}, SubmitterID: "another-reviewer"}
	response := request(t, f.handler, f.token, http.MethodPost, "/v1/reviews/tenant-a/case-a/evidence-attachments", value)
	if response.Code != http.StatusConflict {
		t.Fatalf("Evidence Attachment path/submitter drift status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHTTPAttachEvidenceRechecksPrincipalAtActualPointV1(t *testing.T) {
	now := time.Unix(1_900_105_000, 0)
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, _ := service.New(store, func() time.Time { return now })
	view, err := owner.SubmitV1(context.Background(), submitCommand(now, testkit.Target(now), "case-evidence-principal-expiry"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := contract.SealEvidenceAttachmentV1(contract.EvidenceAttachmentV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: view.Case.TenantID, ID: "attachment-principal-expiry", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		IdempotencyKey: "attachment-principal-expiry-key", Case: contract.ExactResourceRefV1{ID: view.Case.ID, Revision: view.Case.Revision, Digest: view.Case.Digest}, Target: contract.ExactResourceRefV1{ID: view.Target.ID, Revision: view.Target.Revision, Digest: view.Target.Digest}, SubmitterID: "reviewer-a", Evidence: []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("attachment-principal-expiry-evidence")}, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := reviewhttp.PrincipalV1{TenantID: "tenant-a", SubjectID: "reviewer-a", Capabilities: []string{reviewhttp.CapabilityEvidenceAttachV1}, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Second).UnixNano()}
	auth, err := reviewhttp.NewStaticBearerAuthenticatorV1(map[string]reviewhttp.PrincipalV1{testToken: principal})
	if err != nil {
		t.Fatal(err)
	}
	clockCalls := 0
	handler, err := reviewhttp.New(reviewhttp.Config{Service: owner, Authenticator: auth, Clock: func() time.Time {
		clockCalls++
		if clockCalls == 1 {
			return now
		}
		return now.Add(time.Second)
	}, CursorKey: bytes.Repeat([]byte{0x43}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	response := request(t, handler, testToken, http.MethodPost, "/v1/reviews/tenant-a/"+view.Case.ID+"/evidence-attachments", value)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("principal actual-point expiry status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err = store.InspectEvidenceAttachmentExactV1(context.Background(), value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("expired principal leaked an attachment: %v", err)
	}
}

func TestHTTPAttachEvidenceCapsAttachmentTTLAtPrincipalV1(t *testing.T) {
	now := time.Unix(1_900_106_000, 0)
	store := storetestkit.NewMemoryStoreV1(func() time.Time { return now })
	testkit.PublishRubric(context.Background(), store, now, "tenant-a")
	owner, _ := service.New(store, func() time.Time { return now })
	view, err := owner.SubmitV1(context.Background(), submitCommand(now, testkit.Target(now), "case-evidence-principal-ttl"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := contract.SealEvidenceAttachmentV1(contract.EvidenceAttachmentV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: view.Case.TenantID, ID: "attachment-principal-ttl", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		IdempotencyKey: "attachment-principal-ttl-key", Case: contract.ExactResourceRefV1{ID: view.Case.ID, Revision: view.Case.Revision, Digest: view.Case.Digest}, Target: contract.ExactResourceRefV1{ID: view.Target.ID, Revision: view.Target.Revision, Digest: view.Target.Digest}, SubmitterID: "reviewer-a", Evidence: []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("attachment-principal-ttl-evidence")}, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := reviewhttp.PrincipalV1{TenantID: "tenant-a", SubjectID: "reviewer-a", Capabilities: []string{reviewhttp.CapabilityEvidenceAttachV1}, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}
	auth, err := reviewhttp.NewStaticBearerAuthenticatorV1(map[string]reviewhttp.PrincipalV1{testToken: principal})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := reviewhttp.New(reviewhttp.Config{Service: owner, Authenticator: auth, Clock: func() time.Time { return now }, CursorKey: bytes.Repeat([]byte{0x44}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	response := request(t, handler, testToken, http.MethodPost, "/v1/reviews/tenant-a/"+view.Case.ID+"/evidence-attachments", value)
	if response.Code != http.StatusPreconditionFailed {
		t.Fatalf("principal TTL cap status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err = store.InspectEvidenceAttachmentExactV1(context.Background(), value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("principal TTL conflict leaked an attachment: %v", err)
	}
}

func TestSDKAttachEvidenceRejectsDriftedSealedResponseV1(t *testing.T) {
	now := time.Unix(1_900_107_000, 0)
	target := testkit.Target(now)
	requestFact := testkit.Request(now, target, "case-evidence-sdk-drift")
	caseFact, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: requestFact.CaseID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseRequestedV1, ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	seal := func(id, key, evidence string) contract.EvidenceAttachmentV1 {
		value, sealErr := contract.SealEvidenceAttachmentV1(contract.EvidenceAttachmentV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: id, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, IdempotencyKey: key, Case: contract.ExactResourceRefV1{ID: caseFact.ID, Revision: caseFact.Revision, Digest: caseFact.Digest}, Target: contract.ExactResourceRefV1{ID: target.ID, Revision: target.Revision, Digest: target.Digest}, SubmitterID: "reviewer-a", Evidence: []runtimeports.ReviewEvidenceRefV2{testkit.Evidence(evidence)}, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		return value
	}
	submitted := seal("attachment-sdk-exact", "attachment-sdk-exact-key", "evidence-sdk-exact")
	drifted := seal("attachment-sdk-drifted", "attachment-sdk-drifted-key", "evidence-sdk-drifted")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(drifted)
	}))
	defer server.Close()
	client, err := reviewsdk.New(reviewsdk.Config{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: reviewsdk.TokenProviderFuncV1(func(context.Context) (string, error) { return testToken, nil })})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.AttachEvidenceV1(context.Background(), submitted); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("SDK accepted a sealed but drifted response: %v", err)
	}
}

func TestHTTPOversizedBodyAndTypedErrorV1(t *testing.T) {
	f := newHTTPFixture(t)
	raw := strings.Repeat("x", reviewhttp.MaxRequestBytesV1+1)
	r := httptest.NewRequest(http.MethodPost, "/v1/reviews", strings.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+f.token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["category"] == nil || body["reason"] == nil || body["request_id"] == nil {
		t.Fatalf("error is not typed: %s", w.Body.String())
	}
}

func TestHTTPPrincipalCannotClaimOrAttestAsAnotherReviewerV1(t *testing.T) {
	f := newHTTPFixture(t)
	claim := reviewport.ClaimAssignmentMutationV1{TenantID: "tenant-a", CaseID: "case-a", LeaseHolder: "another-reviewer"}
	result := request(t, f.handler, f.token, http.MethodPost, "/v1/reviews/tenant-a/case-a/claim", claim)
	if result.Code != http.StatusConflict {
		t.Fatalf("cross-reviewer claim status=%d body=%s", result.Code, result.Body.String())
	}
	body := map[string]any{"expected": map[string]any{}, "attestation": map[string]any{"tenant_id": "tenant-a", "case_id": "case-a", "reviewer_id": "another-reviewer"}, "trace": map[string]any{"tenant_id": "tenant-a"}}
	result = request(t, f.handler, f.token, http.MethodPost, "/v1/reviews/tenant-a/case-a/attestations", body)
	if result.Code != http.StatusConflict {
		t.Fatalf("cross-reviewer attestation status=%d body=%s", result.Code, result.Body.String())
	}
}

var _ = core.ErrorConflict
