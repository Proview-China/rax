package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSubmitAndClaimCommandsUseExistingStableRoutesV1(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("unexpected authorization %q", r.Header.Get("Authorization"))
		}
		requests <- r.Method + " " + r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	t.Setenv("PRAXIS_REVIEW_URL", server.URL)
	t.Setenv("PRAXIS_REVIEW_TOKEN", token)

	for _, test := range []struct {
		name string
		args []string
		body any
		want string
	}{
		{name: "submit", args: []string{"submit"}, body: service.SubmitCommandV1{}, want: "POST /v1/reviews"},
		{name: "claim", args: []string{"claim"}, body: reviewport.ClaimAssignmentMutationV1{TenantID: "tenant-a", CaseID: "case-a"}, want: "POST /v1/reviews/tenant-a/case-a/claim"},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, err := json.Marshal(test.body)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err = run(context.Background(), test.args, bytes.NewReader(payload), &output); err != nil {
				t.Fatal(err)
			}
			if got := <-requests; got != test.want {
				t.Fatalf("route = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAttachEvidenceCommandUsesStableRouteV1(t *testing.T) {
	now := time.Unix(1_900_350_000, 0)
	value, err := contract.SealEvidenceAttachmentV1(contract.EvidenceAttachmentV1{
		FactIdentityV1:   contract.FactIdentityV1{TenantID: "tenant-a", ID: "attachment-cli", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		IdempotencyKey:   "attachment-cli-key",
		Case:             contract.ExactResourceRefV1{ID: "case-a", Revision: 1, Digest: core.DigestBytes([]byte("case"))},
		Target:           contract.ExactResourceRefV1{ID: "target-a", Revision: 1, Digest: core.DigestBytes([]byte("target"))},
		SubmitterID:      "reviewer-a",
		Evidence:         []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence-a", Classification: "praxis.evidence/test", Digest: core.DigestBytes([]byte("evidence"))}},
		ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/reviews/tenant-a/case-a/evidence-attachments" || r.Header.Get("Authorization") != "Bearer 0123456789abcdef0123456789abcdef" {
			t.Fatalf("unexpected CLI request %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
	}))
	defer server.Close()
	t.Setenv("PRAXIS_REVIEW_URL", server.URL)
	t.Setenv("PRAXIS_REVIEW_TOKEN", "0123456789abcdef0123456789abcdef")
	payload, _ := json.Marshal(value)
	var output bytes.Buffer
	if err := run(context.Background(), []string{"attach-evidence"}, bytes.NewReader(payload), &output); err != nil {
		t.Fatal(err)
	}
	var got contract.EvidenceAttachmentV1
	if err := json.Unmarshal(output.Bytes(), &got); err != nil || got.Digest != value.Digest {
		t.Fatalf("CLI output drifted: %+v %v", got, err)
	}
}

func TestEventsCommandUsesPagedReaderRouteV2(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/reviews/tenant-a/case-a/events" || r.URL.Query().Get("limit") != "7" || r.URL.Query().Get("cursor") != "sealed-cursor" {
			t.Fatalf("unexpected event request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("unexpected authorization %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"next_cursor":""}`))
	}))
	defer server.Close()
	t.Setenv("PRAXIS_REVIEW_URL", server.URL)
	t.Setenv("PRAXIS_REVIEW_TOKEN", token)
	var output bytes.Buffer
	if err := run(context.Background(), []string{"events", "--tenant", "tenant-a", "--case", "case-a", "--limit", "7", "--cursor", "sealed-cursor"}, bytes.NewReader(nil), &output); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Events []contract.TraceFactV1 `json:"events"`
	}
	if err := json.Unmarshal(output.Bytes(), &got); err != nil || got.Events == nil {
		t.Fatalf("CLI event output drifted: %+v err=%v", got, err)
	}
}
