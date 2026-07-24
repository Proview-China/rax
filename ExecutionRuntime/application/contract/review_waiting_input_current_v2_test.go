package contract_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestReviewWaitingInputCurrentV2CanonicalGolden(t *testing.T) {
	now, request, projection := reviewWaitingInputFixtureV2(t)
	const requestGolden = "sha256:49c18d1b87971622cebaf2519ba19b6b38ddad564e8eb40598ec47fa5a7150fd"
	const projectionGolden = "sha256:b20281f69988212d6b5aa5c6543bb4f2e7e031b46fdd41edda1beee56b0160a6"
	if string(request.Digest) != requestGolden {
		t.Fatalf("request canonical golden drifted: got %q", request.Digest)
	}
	if string(projection.Digest) != projectionGolden {
		t.Fatalf("projection canonical golden drifted: got %q", projection.Digest)
	}
	if err := request.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateCurrentFor(request, now); err != nil {
		t.Fatal(err)
	}
}

func TestReviewWaitingInputCurrentV2RejectsMissingAndTypePun(t *testing.T) {
	_, valid, _ := reviewWaitingInputFixtureV2(t)
	for name, testCase := range map[string]struct {
		mutate   func(*contract.ReviewWaitingInputCurrentRequestV2)
		category core.ErrorCategory
	}{
		"missing source id":      {func(r *contract.ReviewWaitingInputCurrentRequestV2) { r.Source.ID = "" }, core.ErrorInvalidArgument},
		"missing source closure": {func(r *contract.ReviewWaitingInputCurrentRequestV2) { r.ExpectedSourceClosureDigest = "" }, core.ErrorInvalidArgument},
		"missing source version": {func(r *contract.ReviewWaitingInputCurrentRequestV2) { r.Source.ContractVersion = "" }, core.ErrorInvalidArgument},
		"unknown source kind":    {func(r *contract.ReviewWaitingInputCurrentRequestV2) { r.Source.Kind = "praxis.unknown/review-source" }, core.ErrorInvalidArgument},
		"source phase type pun":  {func(r *contract.ReviewWaitingInputCurrentRequestV2) { r.Source.Kind = contract.ReviewPhaseRunV1 }, core.ErrorConflict},
		"subject phase type pun": {func(r *contract.ReviewWaitingInputCurrentRequestV2) { r.Subject.PhaseKind = contract.ReviewPhaseRunV1 }, core.ErrorConflict},
		"target tenant drift":    {func(r *contract.ReviewWaitingInputCurrentRequestV2) { r.Subject.TenantID = "other-tenant" }, core.ErrorConflict},
	} {
		t.Run(name, func(t *testing.T) {
			changed := valid.Clone()
			testCase.mutate(&changed)
			_, err := contract.SealReviewWaitingInputCurrentRequestV2(changed)
			if !core.HasCategory(err, testCase.category) {
				t.Fatalf("unsafe request returned %v, want %s", err, testCase.category)
			}
		})
	}

	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	delete(object, "expected_source_closure_digest")
	missing, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	var decoded contract.ReviewWaitingInputCurrentRequestV2
	if err := core.DecodeStrictJSON(missing, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("missing literal field was accepted: %v", err)
	}
}

func TestReviewWaitingInputCurrentV2ExactMinimumTTLAndClock(t *testing.T) {
	now, base, _ := reviewWaitingInputFixtureV2(t)
	for name, testCase := range map[string]struct {
		expiries [3]time.Duration
		expected time.Duration
	}{
		"source":   {[3]time.Duration{5 * time.Second, 12 * time.Second, 13 * time.Second}, 5 * time.Second},
		"phase":    {[3]time.Duration{12 * time.Second, 6 * time.Second, 13 * time.Second}, 6 * time.Second},
		"target":   {[3]time.Duration{12 * time.Second, 13 * time.Second, 7 * time.Second}, 7 * time.Second},
		"hard cap": {[3]time.Duration{10 * time.Minute, 20 * time.Minute, 30 * time.Minute}, contract.MaxReviewWaitingInputCurrentProjectionTTLV2},
	} {
		t.Run(name, func(t *testing.T) {
			request := base.Clone()
			request.Source.NotAfterUnixNano = now.Add(testCase.expiries[0]).UnixNano()
			request.ExpectedPhase.ExpiresUnixNano = now.Add(testCase.expiries[1]).UnixNano()
			request.ExpectedTarget.ExpiresUnixNano = now.Add(testCase.expiries[2]).UnixNano()
			request, err := contract.SealReviewWaitingInputCurrentRequestV2(request)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := contract.SealReviewWaitingInputCurrentProjectionV2(contract.ReviewWaitingInputCurrentProjectionV2{CheckedUnixNano: now.UnixNano()}, request, now)
			if err != nil {
				t.Fatal(err)
			}
			expected := now.Add(testCase.expected).UnixNano()
			if projection.ExpiresUnixNano != expected {
				t.Fatalf("%s TTL was not exact minimum: got=%d want=%d", name, projection.ExpiresUnixNano, expected)
			}
			if err := projection.ValidateCurrentFor(request, time.Unix(0, expected)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
				t.Fatalf("%s TTL boundary was accepted: %v", name, err)
			}
			if err := projection.ValidateCurrentFor(request, now.Add(-time.Second)); !core.HasReason(err, core.ReasonClockRegression) {
				t.Fatalf("%s clock rollback was accepted: %v", name, err)
			}
		})
	}
}

func TestReviewWaitingInputCurrentV2DriftAndClone(t *testing.T) {
	now, request, projection := reviewWaitingInputFixtureV2(t)
	clone := projection.Clone()
	clone.Source.ID = "caller-mutated-source"
	if projection.Source.ID == clone.Source.ID || !reflect.DeepEqual(projection, projection.Clone()) {
		t.Fatal("projection Clone retained caller mutation")
	}

	drifted := projection.Clone()
	drifted.Source.ID = "other-source"
	if err := drifted.ValidateCurrentFor(request, now); err == nil {
		t.Fatal("projection source drift was accepted")
	}
	drifted = projection.Clone()
	drifted.SourceClosureDigest = core.DigestBytes([]byte("other-source-closure"))
	digest, err := drifted.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	drifted.Digest = digest
	if err := drifted.ValidateCurrentFor(request, now); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("source closure drift was accepted: %v", err)
	}
	drifted = projection.Clone()
	drifted.ExpiresUnixNano--
	digest, _ = drifted.DigestV2()
	drifted.Digest = digest
	if err := drifted.ValidateCurrentFor(request, now); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("non-minimum TTL was accepted: %v", err)
	}
	forged := projection.Clone()
	forged.ExpiresUnixNano = forged.CheckedUnixNano + int64(contract.MaxReviewWaitingInputCurrentProjectionTTLV2) + 1
	digest, _ = forged.DigestV2()
	forged.Digest = digest
	if err := forged.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("re-digested projection beyond the Application hard cap was accepted: %v", err)
	}
}

func reviewWaitingInputFixtureV2(t testing.TB) (time.Time, contract.ReviewWaitingInputCurrentRequestV2, contract.ReviewWaitingInputCurrentProjectionV2) {
	t.Helper()
	now := time.Unix(2_100_000_000, 123_000_000)
	tenant := core.TenantID("tenant-review-input-v2")
	run := core.AgentRunID("run-review-input-v2")
	phase := contract.ReviewPhasePointCoordinateV1{Kind: contract.ReviewPhaseActionV1, ID: "phase-review-input-v2", Revision: 7, Digest: core.DigestBytes([]byte("phase-review-input-v2")), CheckedUnixNano: now.Add(-3 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(40 * time.Minute).UnixNano()}
	target := contract.ReviewWaitingTargetCoordinateV1{TenantID: tenant, ID: "target-review-input-v2", Revision: 11, Digest: core.DigestBytes([]byte("target-review-input-v2")), RunID: run, CheckedUnixNano: now.Add(-2 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(35 * time.Minute).UnixNano()}
	request, err := contract.SealReviewWaitingInputCurrentRequestV2(contract.ReviewWaitingInputCurrentRequestV2{
		Subject: contract.ReviewWaitingInputSubjectV1{TenantID: tenant, RunID: run, PhaseKind: phase.Kind, PhaseID: phase.ID},
		Source: contract.ReviewWaitingInputSourceRefV2{
			ContractVersion: contract.ReviewWaitingInputSourceContractVersionV1,
			Kind:            phase.Kind, ID: "review-phase-source-v2", Revision: 13,
			Digest: core.DigestBytes([]byte("review-phase-source-v2")), NotAfterUnixNano: now.Add(30 * time.Minute).UnixNano(),
		},
		ExpectedSourceClosureDigest: core.DigestBytes([]byte("review-phase-source-closure-v2")),
		ExpectedPhase:               phase, ExpectedTarget: target,
		ExecutionScopeDigest: core.DigestBytes([]byte("review-input-execution-scope-v2")),
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := contract.SealReviewWaitingInputCurrentProjectionV2(contract.ReviewWaitingInputCurrentProjectionV2{CheckedUnixNano: now.UnixNano()}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	return now, request, projection
}
