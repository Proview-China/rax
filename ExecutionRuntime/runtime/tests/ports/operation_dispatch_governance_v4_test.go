package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationAuthorizedAdmissionV4CanonicalTamperAndRequestValidation(t *testing.T) {
	now := time.Unix(900_000, 0)
	admission := ports.OperationAuthorizedAdmissionV4{
		Admission: ports.OperationEffectAdmissionReceiptV3{
			OperationDigest: dispatchV4Digest("operation"), EffectID: "effect-v4", IntentRevision: 1,
			IntentDigest: dispatchV4Digest("intent"), FactRevision: 2, State: "accepted",
		},
		Authorization: ports.OperationReviewAuthorizationRefV4{ID: "authorization-v4", Revision: 1, Digest: dispatchV4Digest("authorization")},
		PayloadSchema: ports.SchemaRefV2{
			Namespace: "custom", Name: "dispatch", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: dispatchV4Digest("schema"),
		},
		PayloadDigest: dispatchV4Digest("payload"), PayloadRevision: 1,
		ReviewProjectionDigest: dispatchV4Digest("review"), ReviewCurrentnessDigest: dispatchV4Digest("currentness"),
		LegacyReviewProjectionDigest: dispatchV4Digest("legacy"), GovernanceSnapshotDigest: dispatchV4Digest("governance"),
		AuthorizationFenceDigest: dispatchV4Digest("fence"), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	sealed, err := ports.SealOperationAuthorizedAdmissionV4(admission)
	if err != nil {
		t.Fatal(err)
	}
	if err := sealed.Validate(); err != nil {
		t.Fatal(err)
	}
	tampered := sealed
	tampered.Authorization.Digest = dispatchV4Digest("other-authorization")
	if err := tampered.Validate(); err == nil {
		t.Fatal("changed Authorization ref retained the old dispatch admission digest")
	}
	if err := (ports.IssueGovernedOperationDispatchRequestV4{}).Validate(); err == nil {
		t.Fatal("empty V4 Issue request was accepted")
	}
	if err := (ports.BeginGovernedOperationDispatchRequestV4{}).Validate(); err == nil {
		t.Fatal("empty V4 Begin request was accepted")
	}
}

func TestOperationDispatchEnforcementRefV4ReservesExactAuthorizationBinding(t *testing.T) {
	ref := ports.OperationDispatchEnforcementRefV4{
		PermitID: "permit-v4", PermitRevision: 1, PermitDigest: dispatchV4Digest("permit"), AttemptID: "attempt-v4",
		ReviewAuthorization: ports.OperationReviewAuthorizationRefV4{ID: "authorization-v4", Revision: 1, Digest: dispatchV4Digest("authorization")},
		ReceiptDigest:       dispatchV4Digest("receipt"), RecordedRevision: 3,
	}
	if err := ref.Validate(); err != nil {
		t.Fatal(err)
	}
	ref.ReviewAuthorization = ports.OperationReviewAuthorizationRefV4{}
	if err := ref.Validate(); err == nil {
		t.Fatal("future Enforcement ref accepted a missing Review Authorization")
	}
}

func TestOperationReviewAuthorizationV3CoversOnlyAllowsAggregateTTLNarrowing(t *testing.T) {
	authorized := ports.OperationReviewAuthorizationV3{
		Case:              ports.OperationGovernanceFactRefV3{Ref: "case", Revision: 1, Digest: dispatchV4Digest("case"), ExpiresUnixNano: 100},
		CandidateDigest:   dispatchV4Digest("candidate"),
		CandidateRevision: 1,
		Verdict:           ports.OperationGovernanceFactRefV3{Ref: "verdict", Revision: 1, Digest: dispatchV4Digest("verdict"), ExpiresUnixNano: 100},
		ReviewerAuthority: ports.OperationGovernanceFactRefV3{Ref: "reviewer", Revision: 1, Digest: dispatchV4Digest("reviewer"), ExpiresUnixNano: 100},
		PolicyDigest:      dispatchV4Digest("policy"),
		ExpiresUnixNano:   80,
	}
	current := authorized
	current.ExpiresUnixNano = 90
	if !ports.OperationReviewAuthorizationV3Covers(current, authorized) {
		t.Fatal("later aggregate current TTL did not cover the narrowed V4 authorization TTL")
	}
	current = authorized
	current.ExpiresUnixNano = 79
	if ports.OperationReviewAuthorizationV3Covers(current, authorized) {
		t.Fatal("shorter current TTL covered a later V4 authorization TTL")
	}
	current = authorized
	current.Verdict.Digest = dispatchV4Digest("other-verdict")
	if ports.OperationReviewAuthorizationV3Covers(current, authorized) {
		t.Fatal("source Fact drift was hidden as aggregate TTL compatibility")
	}
}

func dispatchV4Digest(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}
