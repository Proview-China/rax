package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/api"
)

type apiTransportAuth struct {
	token string
}

func (a *apiTransportAuth) AuthorizeSandboxAPI(request *http.Request, tenant string, _ api.ActionV1) error {
	if tenant != "tenant-1" || request.Header.Get("Authorization") != "Bearer "+a.token {
		return errors.New("denied")
	}
	return nil
}

func TestHTTPServerV1AsyncSubmitExecuteInspectAndWatch(t *testing.T) {
	fixture := newAPIFixture(t)
	server, err := api.NewHTTPServerV1(fixture.service, &apiTransportAuth{token: "secret"}, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	request := apiRequest(t, fixture.now, "http-operation", "http-key", `{"backend":"container"}`)
	body, _ := json.Marshal(request)
	response := serveAPIRequestV1(t, server, http.MethodPost, "/v1/operations", body, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAPIRequestV1(t, server, http.MethodPost, "/v1/operations/http-operation/execute", nil, true)
	if response.Code != http.StatusOK || fixture.handler.executeCalls.Load() != 1 {
		t.Fatalf("execute status=%d body=%s calls=%d", response.Code, response.Body.String(), fixture.handler.executeCalls.Load())
	}
	var completed api.OperationFactV1
	if err := json.Unmarshal(response.Body.Bytes(), &completed); err != nil || completed.State != api.OperationSucceededV1 {
		t.Fatalf("decode completed=%#v err=%v", completed, err)
	}
	response = serveAPIRequestV1(t, server, http.MethodGet, "/v1/operations/http-operation", nil, true)
	if response.Code != http.StatusOK {
		t.Fatalf("inspect status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAPIRequestV1(t, server, http.MethodGet, "/v1/watch?after=0&limit=16", nil, true)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"cursor":3`)) {
		t.Fatalf("watch status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHTTPServerV1AuthorizationAndStrictBodyFailBeforeMutation(t *testing.T) {
	fixture := newAPIFixture(t)
	server, err := api.NewHTTPServerV1(fixture.service, &apiTransportAuth{token: "secret"}, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	request := apiRequest(t, fixture.now, "http-denied", "http-denied", `{"backend":"wasm"}`)
	body, _ := json.Marshal(request)
	denied := serveAPIRequestV1(t, server, http.MethodPost, "/v1/operations", body, false)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", denied.Code, denied.Body.String())
	}
	if _, err := fixture.service.Inspect(context.Background(), request.RequestID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("unauthorized submit changed state: %v", err)
	}
	trailing := append(append([]byte(nil), body...), []byte(` {}`)...)
	bad := serveAPIRequestV1(t, server, http.MethodPost, "/v1/operations", trailing, true)
	if bad.Code != http.StatusConflict {
		t.Fatalf("trailing JSON status=%d body=%s", bad.Code, bad.Body.String())
	}
	unknown := append(body[:len(body)-1], []byte(`,"unknown":true}`)...)
	bad = serveAPIRequestV1(t, server, http.MethodPost, "/v1/operations", unknown, true)
	if bad.Code != http.StatusConflict {
		t.Fatalf("unknown field status=%d body=%s", bad.Code, bad.Body.String())
	}
}

func serveAPIRequestV1(t *testing.T, server http.Handler, method, target string, body []byte, authorized bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if authorized {
		request.Header.Set("Authorization", "Bearer secret")
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}
