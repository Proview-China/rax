package applicationadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

type checkpointDataPlaneProbeV2 struct {
	response      dataplaneadapter.DispatchResponseV1
	dispatchError error
	dispatchCalls int
	inspectCalls  int
}

func (p *checkpointDataPlaneProbeV2) Dispatch(context.Context, dataplaneadapter.DispatchRequestV1) (dataplaneadapter.DispatchResponseV1, error) {
	p.dispatchCalls++
	return p.response, p.dispatchError
}

func (p *checkpointDataPlaneProbeV2) Inspect(context.Context, dataplaneadapter.DispatchRequestV1) (dataplaneadapter.DispatchResponseV1, error) {
	p.inspectCalls++
	return p.response, nil
}

func TestCheckpointResultProjectionUsesProviderInspectAndEvidenceExactRefsV2(t *testing.T) {
	work, _, closure, _ := governedCheckpointFixtureV1(t, "result-current")
	now := time.Unix(1_900_300_000, 0).UTC()
	binding := CheckpointProviderResultBindingV2{
		Reservation: testkit.Ref("checkpoint-result-reservation"), Phase: contract.CheckpointPhasePrepare,
		Execute:  dataplaneadapter.DispatchRequestV1{ContractVersion: dataplaneadapter.ContractVersionV1, RequestID: "checkpoint-result-request", Phase: dataplaneadapter.PhaseExecute, Digest: string(checkpointDigestV1("checkpoint-result-request"))},
		Evidence: closure.Prepare.Evidence,
	}
	attemptDigest := string(checkpointDigestV1("provider-attempt"))
	observationDigest := string(checkpointDigestV1("provider-observation"))
	receiptDigest := string(checkpointDigestV1("provider-receipt"))
	expires := now.Add(20 * time.Second).UnixNano()
	response := dataplaneadapter.DispatchResponseV1{
		ProviderAttempt:     &dataplaneadapter.ExactRefV1{ID: "provider/attempt", Revision: 2, Digest: attemptDigest, ExpiresUnixNano: expires},
		ProviderObservation: &dataplaneadapter.ProviderObservationV1{ObservedUnixNano: now.UnixNano(), CheckpointArtifact: &dataplaneadapter.CheckpointArtifactObservationV1{ArtifactID: "artifact", State: "prepared", CheckpointPhase: "checkpoint_prepare", ExpiresUnixNano: expires}},
		ProviderReceipt:     &dataplaneadapter.ProviderReceiptV1{}, ObservationDigest: &observationDigest, ReceiptDigest: &receiptDigest, ExpiresUnixNano: expires,
	}
	projection, err := checkpointResultProjectionFromInspectionV2(binding, response, now.Add(10*time.Second).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if projection.State != contract.CheckpointPhasePrepared || projection.ExpiresUnixNano != now.Add(10*time.Second).UnixNano() || projection.EvidenceConsumption.ID != closure.Prepare.Evidence.ID || projection.ReservationRef != binding.Reservation || projection.ValidateCurrent(now) != nil || work.Attempt.ID != closure.Prepare.Evidence.Attempt.ID {
		t.Fatalf("checkpoint result projection drifted: %+v", projection)
	}
}

func TestCheckpointResultProjectionRejectsCrossPhaseProviderStateV2(t *testing.T) {
	_, _, closure, _ := governedCheckpointFixtureV1(t, "result-cross-phase")
	now := time.Unix(1_900_300_000, 0).UTC()
	binding := CheckpointProviderResultBindingV2{Reservation: testkit.Ref("checkpoint-result-cross"), Phase: contract.CheckpointPhasePrepare, Execute: dataplaneadapter.DispatchRequestV1{ContractVersion: dataplaneadapter.ContractVersionV1, RequestID: "checkpoint-result-cross", Phase: dataplaneadapter.PhaseExecute, Digest: string(checkpointDigestV1("checkpoint-result-cross"))}, Evidence: closure.Prepare.Evidence}
	digest := string(checkpointDigestV1("provider-cross"))
	response := dataplaneadapter.DispatchResponseV1{ProviderAttempt: &dataplaneadapter.ExactRefV1{ID: "provider/cross", Revision: 2, Digest: digest, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, ProviderObservation: &dataplaneadapter.ProviderObservationV1{ObservedUnixNano: now.UnixNano(), CheckpointArtifact: &dataplaneadapter.CheckpointArtifactObservationV1{ArtifactID: "artifact", State: "committed", CheckpointPhase: "checkpoint_commit", ExpiresUnixNano: now.Add(time.Minute).UnixNano()}}, ProviderReceipt: &dataplaneadapter.ProviderReceiptV1{}, ObservationDigest: &digest, ReceiptDigest: &digest, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	if _, err := checkpointResultProjectionFromInspectionV2(binding, response, now.Add(time.Minute).UnixNano()); err == nil {
		t.Fatal("checkpoint Provider result crossed semantic phase")
	}
}

func TestCheckpointProviderSuccessStillRequiresIndependentExactInspectV2(t *testing.T) {
	request, response := checkpointDispatchResponseFixtureV2(t, "success-inspect")
	probe := &checkpointDataPlaneProbeV2{response: response}
	boundary := &CheckpointProviderBoundaryV1{dataplane: probe, now: func() time.Time { return time.Unix(0, response.CheckedUnixNano) }}
	got, err := boundary.dispatchOrInspectCheckpointV1(context.Background(), request)
	if err != nil || got.Digest != response.Digest {
		t.Fatalf("checkpoint independently inspected response=%+v err=%v", got, err)
	}
	if probe.dispatchCalls != 1 || probe.inspectCalls != 1 {
		t.Fatalf("checkpoint success calls dispatch=%d inspect=%d", probe.dispatchCalls, probe.inspectCalls)
	}
}

func TestCheckpointProviderLostReplyInspectsOriginalWithoutSecondDispatchV2(t *testing.T) {
	request, response := checkpointDispatchResponseFixtureV2(t, "lost-reply")
	probe := &checkpointDataPlaneProbeV2{response: response, dispatchError: errors.New("reply lost")}
	boundary := &CheckpointProviderBoundaryV1{dataplane: probe, now: func() time.Time { return time.Unix(0, response.CheckedUnixNano) }}
	if _, err := boundary.dispatchOrInspectCheckpointV1(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if probe.dispatchCalls != 1 || probe.inspectCalls != 1 {
		t.Fatalf("lost reply replayed Provider: dispatch=%d inspect=%d", probe.dispatchCalls, probe.inspectCalls)
	}
}

func checkpointDispatchResponseFixtureV2(t *testing.T, suffix string) (dataplaneadapter.DispatchRequestV1, dataplaneadapter.DispatchResponseV1) {
	t.Helper()
	now := time.Unix(1_900_400_000, 0).UTC()
	expires := now.Add(time.Minute).UnixNano()
	digest := func(value string) string {
		sum := sha256.Sum256([]byte(value))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	request := dataplaneadapter.DispatchRequestV1{ContractVersion: dataplaneadapter.ContractVersionV1, RequestID: "request-" + suffix, Phase: dataplaneadapter.PhaseExecute, EffectKind: dataplaneadapter.CheckpointEffectKindV1, AttemptID: "attempt-" + suffix, TenantID: "tenant-1", PayloadDigest: digest("payload-" + suffix), RequestedNotAfterUnixNano: expires, SandboxAttempt: dataplaneadapter.ExactRefV1{ExpiresUnixNano: expires}, ExecutionBinding: dataplaneadapter.ExecutionBindingV1{ExpiresUnixNano: expires}, RuntimeEnforcement: dataplaneadapter.RuntimeEnforcementRefV1{ExpiresUnixNano: expires}, Payload: dataplaneadapter.ProviderPayloadV1{ProviderKind: "host_workspace"}, Digest: digest("request-" + suffix)}
	attempt := dataplaneadapter.ExactRefV1{ID: "host/tenant-1/" + request.AttemptID, Revision: 2, ExpiresUnixNano: expires}
	attempt.Digest = checkpointDataPlaneDigestV2(t, "ProviderAttemptRefV1", attempt)
	subjectDigest := digest("subject-" + suffix)
	observation := dataplaneadapter.ProviderObservationV1{Provider: "host_workspace", Attempt: attempt, State: "prepared", PayloadDigest: request.PayloadDigest, ObservedUnixNano: now.UnixNano(), CheckpointArtifact: &dataplaneadapter.CheckpointArtifactObservationV1{ContractVersion: "praxis.sandbox/checkpoint-artifact-observation/v1", ArtifactID: "artifact-" + subjectDigest[7:], SubjectDigest: subjectDigest, ContentDigest: digest("content-" + suffix), ContentLength: 1, State: "prepared", CheckpointPhase: "checkpoint_prepare", RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}}
	observation.Digest = checkpointDataPlaneDigestV2(t, "ProviderObservationV1", observation)
	receipt := dataplaneadapter.ProviderReceiptV1{Provider: "host_workspace", Attempt: attempt, Phase: string(dataplaneadapter.PhaseExecute), ObservationDigest: observation.Digest, RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	receipt.Digest = checkpointDataPlaneDigestV2(t, "ProviderReceiptV1", receipt)
	response := dataplaneadapter.DispatchResponseV1{ContractVersion: dataplaneadapter.ContractVersionV1, RequestID: request.RequestID, RequestDigest: request.Digest, Accepted: true, ProviderAttempt: &attempt, ProviderObservation: &observation, ProviderReceipt: &receipt, ObservationDigest: &observation.Digest, ReceiptDigest: &receipt.Digest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	response.Digest = checkpointDataPlaneDigestV2(t, "DispatchResponseV1", response)
	if err := response.Validate(request); err != nil {
		t.Fatal(err)
	}
	return request, response
}

func checkpointDataPlaneDigestV2(t *testing.T, kind string, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(dataplaneadapter.ContractVersionV1))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(data)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
