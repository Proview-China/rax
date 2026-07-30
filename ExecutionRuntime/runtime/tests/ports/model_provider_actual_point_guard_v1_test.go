package ports_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelProviderBoundaryV1ExactCoordinatesAndCanonicalTamper(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	request := modelActualPointRequestV1(t, now)
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	projection, err := ports.SealModelProviderBoundaryCurrentProjectionV1(ports.ModelProviderBoundaryCurrentProjectionV1{
		Ref:             request.ModelBoundary,
		State:           ports.ModelProviderBoundaryCrossedV1,
		Provider:        modelProviderBindingV1(),
		CheckedUnixNano: now.UnixNano(),
		ExpiresUnixNano: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	tampered := projection
	tampered.Ref.DispatchSequence++
	if err := tampered.Validate(); err == nil {
		t.Fatal("changed dispatch sequence retained a valid boundary projection")
	}
	tampered = projection
	tampered.Ref.ProviderAttemptOrdinal++
	if err := tampered.Validate(); err == nil {
		t.Fatal("changed provider ordinal retained a valid boundary projection")
	}
	tampered = projection
	tampered.Ref.AttemptRequestDigest = modelGuardDigestV1("other-request")
	if err := tampered.Validate(); err == nil {
		t.Fatal("changed request digest retained a valid boundary projection")
	}
	tampered = projection
	tampered.Ref.AcknowledgementDigest = modelGuardDigestV1("other-ack")
	if err := tampered.Validate(); err == nil {
		t.Fatal("changed acknowledgement retained a valid boundary projection")
	}
}

func TestModelProviderActualPointV1RequiresRunAndExactAttempt(t *testing.T) {
	now := time.Unix(1_000_100, 0)
	request := modelActualPointRequestV1(t, now)
	tampered := request
	tampered.Attempt.AttemptID = "another-attempt"
	if err := tampered.Validate(); err == nil {
		t.Fatal("request accepted another Runtime attempt")
	}
	tampered = request
	tampered.Operation.Kind = ports.OperationScopeActivationV3
	tampered.Operation.RunID = ""
	tampered.Operation.ActivationAttemptID = "activation"
	if err := tampered.Validate(); err == nil {
		t.Fatal("activation-scoped Model Turn entered the run-only slice")
	}
}

func TestModelProviderActualPointV1ProjectionHasNoAllowedFlagAndExpires(t *testing.T) {
	now := time.Unix(1_000_200, 0)
	request := modelActualPointRequestV1(t, now)
	projection := modelActualPointProjectionV1(t, now, request)
	if err := projection.ValidateCurrent(now.Add(2 * time.Second)); err == nil {
		t.Fatal("expired actual-point projection remained current")
	}
	if err := projection.ValidateCurrent(now.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("consumer clock rollback was not rejected precisely: %v", err)
	}
	if err := projection.ValidateCurrent(time.Unix(0, projection.NotAfterUnixNano)); err == nil {
		t.Fatal("projection remained current at its exclusive NotAfter boundary")
	}
	typ := reflect.TypeOf(projection)
	if _, present := typ.FieldByName("Allowed"); present {
		t.Fatal("actual-point projection exposed an Allowed boolean")
	}
}

func TestModelProviderActualPointV1ProjectionValidateAgainstExactRequest(t *testing.T) {
	now := time.Unix(1_000_300, 0)
	request := modelActualPointRequestV1(t, now)
	projection := modelActualPointProjectionV1(t, now, request)
	if err := projection.ValidateAgainst(request, now); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*ports.InspectCurrentModelProviderActualPointRequestV1)
	}{
		{name: "effect", mutate: func(r *ports.InspectCurrentModelProviderActualPointRequestV1) { r.ExpectedEffectRevision++ }},
		{name: "permit fact", mutate: func(r *ports.InspectCurrentModelProviderActualPointRequestV1) { r.ExpectedPermitFactRevision++ }},
		{name: "permit digest", mutate: func(r *ports.InspectCurrentModelProviderActualPointRequestV1) {
			r.PermitDigest = modelGuardDigestV1("other-permit")
		}},
		{name: "admission", mutate: func(r *ports.InspectCurrentModelProviderActualPointRequestV1) {
			r.AdmissionDigest = modelGuardDigestV1("other-admission")
		}},
		{name: "review", mutate: func(r *ports.InspectCurrentModelProviderActualPointRequestV1) {
			r.ReviewAuthorization.Digest = modelGuardDigestV1("other-review")
		}},
		{name: "attempt", mutate: func(r *ports.InspectCurrentModelProviderActualPointRequestV1) {
			r.Attempt.IntentDigest = modelGuardDigestV1("other-intent")
			r.ModelBoundary.RuntimeAttempt = r.Attempt
			r.ModelBoundary, _ = ports.SealModelProviderBoundaryCurrentRefV1(r.ModelBoundary)
		}},
		{name: "verifier", mutate: func(r *ports.InspectCurrentModelProviderActualPointRequestV1) {
			r.Verifier.ArtifactDigest = modelGuardDigestV1("other-verifier")
		}},
		{name: "fence", mutate: func(r *ports.InspectCurrentModelProviderActualPointRequestV1) {
			r.FenceDigest = modelGuardDigestV1("other-fence")
		}},
		{name: "boundary", mutate: func(r *ports.InspectCurrentModelProviderActualPointRequestV1) {
			r.ModelBoundary.DispatchSequence++
			r.ModelBoundary, _ = ports.SealModelProviderBoundaryCurrentRefV1(r.ModelBoundary)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			changed := request
			testCase.mutate(&changed)
			if err := projection.ValidateAgainst(changed, now); err == nil {
				t.Fatal("projection validated against a changed request")
			}
		})
	}
}

func modelActualPointProjectionV1(t *testing.T, now time.Time, request ports.InspectCurrentModelProviderActualPointRequestV1) ports.ModelProviderActualPointCurrentProjectionV1 {
	t.Helper()
	operationDigest, _ := request.Operation.DigestV3()
	requestDigest, _ := request.DigestV1()
	projection, err := ports.SealModelProviderActualPointCurrentProjectionV1(ports.ModelProviderActualPointCurrentProjectionV1{
		RequestDigest:        requestDigest,
		OperationDigest:      operationDigest,
		EffectID:             request.EffectID,
		EffectFactRevision:   request.ExpectedEffectRevision,
		PermitID:             request.PermitID,
		PermitFactRevision:   request.ExpectedPermitFactRevision,
		PermitDigest:         request.PermitDigest,
		AdmissionDigest:      request.AdmissionDigest,
		ReviewAuthorization:  request.ReviewAuthorization,
		Attempt:              request.Attempt,
		FenceDigest:          request.FenceDigest,
		RuntimeControlDigest: modelGuardDigestV1("control"),
		ModelBoundary:        request.ModelBoundary,
		Provider:             modelProviderBindingV1(),
		Verifier:             modelProviderBindingV1(),
		CheckedUnixNano:      now.UnixNano(),
		NotAfterUnixNano:     now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func modelActualPointRequestV1(t *testing.T, now time.Time) ports.InspectCurrentModelProviderActualPointRequestV1 {
	t.Helper()
	lease := &core.SandboxLeaseRef{ID: "lease-model", Epoch: 7}
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-model", ID: "agent-model", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-model", PlanDigest: modelGuardDigestV1("plan")},
		Instance:       core.InstanceRef{ID: "instance-model", Epoch: 4},
		SandboxLease:   lease,
		AuthorityEpoch: 3,
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := ports.OperationSubjectV3{
		Kind:                      ports.OperationScopeRunV3,
		ExecutionScope:            scope,
		ExecutionScopeDigest:      scopeDigest,
		RunID:                     "run-model",
		SubjectRevision:           1,
		CurrentProjectionRef:      "run-current-model",
		CurrentProjectionRevision: 1,
		CurrentProjectionDigest:   modelGuardDigestV1("run-current"),
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest,
		EffectID:        "effect-model",
		IntentRevision:  1,
		IntentDigest:    modelGuardDigestV1("intent"),
		PermitID:        "permit-model",
		PermitRevision:  1,
		PermitDigest:    modelGuardDigestV1("permit"),
		AttemptID:       "attempt-model",
	}
	boundary, err := ports.SealModelProviderBoundaryCurrentRefV1(ports.ModelProviderBoundaryCurrentRefV1{
		Owner:                  core.OwnerRef{Domain: "praxis.model", ID: "model-invoker"},
		ID:                     "model-boundary",
		Revision:               1,
		OperationDigest:        operationDigest,
		EffectID:               attempt.EffectID,
		RuntimeAttempt:         attempt,
		DispatchSequence:       9,
		ProviderAttemptOrdinal: 1,
		AttemptRequestDigest:   modelGuardDigestV1("attempt-request"),
		AcknowledgementDigest:  modelGuardDigestV1("ack"),
		ExpiresUnixNano:        now.Add(2 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ports.InspectCurrentModelProviderActualPointRequestV1{
		Operation:                  operation,
		EffectID:                   attempt.EffectID,
		ExpectedEffectRevision:     3,
		PermitID:                   attempt.PermitID,
		ExpectedPermitFactRevision: 2,
		PermitDigest:               attempt.PermitDigest,
		AdmissionDigest:            modelGuardDigestV1("admission"),
		ReviewAuthorization:        ports.OperationReviewAuthorizationRefV4{ID: "review-authorization", Revision: 1, Digest: modelGuardDigestV1("review")},
		Attempt:                    attempt,
		Verifier:                   modelProviderBindingV1(),
		FenceDigest:                modelGuardDigestV1("fence"),
		ModelBoundary:              boundary,
		RequestedNotAfterUnixNano:  now.Add(time.Second).UnixNano(),
	}
}

func modelProviderBindingV1() ports.ProviderBindingRefV2 {
	return ports.ProviderBindingRefV2{
		BindingSetID:       "binding-model",
		BindingSetRevision: 1,
		ComponentID:        "praxis.model/provider",
		ManifestDigest:     modelGuardDigestV1("manifest"),
		ArtifactDigest:     modelGuardDigestV1("artifact"),
		Capability:         ports.ModelInvokeCapabilityV1,
	}
}

func modelGuardDigestV1(value string) core.Digest {
	digest, _ := core.DigestJSON(value)
	return digest
}
