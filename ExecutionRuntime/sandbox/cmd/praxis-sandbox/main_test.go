package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/api"
)

func TestCLI_SubmitsGovernedCheckpointAndExplicitlyExecutes(t *testing.T) {
	var mu sync.Mutex
	var submitted api.OperationRequestV1
	var executeCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("Authorization") != "Bearer token" {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if request.URL.Path == "/v1/operations" {
			if err := json.NewDecoder(request.Body).Decode(&submitted); err != nil {
				t.Errorf("decode request: %v", err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			fact, err := api.SealOperationFactV1(api.OperationFactV1{ID: submitted.RequestID, Revision: 1, Request: submitted, State: api.OperationQueuedV1, CreatedUnixNano: submitted.RequestedUnixNano, UpdatedUnixNano: submitted.RequestedUnixNano, ExpiresUnixNano: submitted.RequestedNotAfterUnixNano})
			if err != nil {
				t.Errorf("seal queued: %v", err)
			}
			writer.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(writer).Encode(fact)
			return
		}
		if strings.HasSuffix(request.URL.Path, "/execute") {
			executeCalls++
			result, _ := api.SealResultV1(api.ResultV1{Schema: "praxis.sandbox.api/checkpoint-result/v1", Revision: 1, Payload: []byte(`{"accepted":true}`)})
			fact, err := api.SealOperationFactV1(api.OperationFactV1{ID: submitted.RequestID, Revision: 3, Request: submitted, State: api.OperationSucceededV1, Result: &result, CreatedUnixNano: submitted.RequestedUnixNano, UpdatedUnixNano: submitted.RequestedUnixNano + 1, ExpiresUnixNano: submitted.RequestedNotAfterUnixNano})
			if err != nil {
				t.Errorf("seal completed: %v", err)
			}
			_ = json.NewEncoder(writer).Encode(fact)
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"--endpoint", server.URL, "--tenant", "tenant-1", "--token", "token", "--execute", "--payload", `{"attempt_id":"checkpoint-1"}`, "checkpoint"}, &stdout, &stderr, func(string) string { return "" })
	if err != nil {
		t.Fatalf("run: %v stderr=%s", err, stderr.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if submitted.Action != api.ActionCheckpointV1 || submitted.TenantID != "tenant-1" || executeCalls != 1 || !strings.Contains(stdout.String(), `"state": "succeeded"`) {
		t.Fatalf("submitted=%#v execute=%d stdout=%s", submitted, executeCalls, stdout.String())
	}
}

func TestCLI_RejectsMissingTokenAndInvalidEndpoint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--tenant", "tenant-1", "backends"}, &stdout, &stderr, func(string) string { return "" }); err == nil {
		t.Fatal("missing transport token was accepted")
	}
	if _, _, err := newAPIClient("file:///tmp/sandbox", time.Second); err == nil {
		t.Fatal("non HTTP/Unix endpoint was accepted")
	}
}
