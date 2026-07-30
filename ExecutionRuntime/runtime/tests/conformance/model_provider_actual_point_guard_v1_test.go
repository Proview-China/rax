package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelProviderActualPointConformanceV1RequiresEveryProviderPath(t *testing.T) {
	request, projection := conformanceModelFixtureV1(t)
	guard := conformanceGuardV1{expected: request, projection: projection}
	all := []string{"direct", "stream", "continuation", "realtime", "routegateway", "raw"}
	fixture := conformanceFixtureV1(request, projection, all)
	report, err := conformance.CheckModelProviderActualPointGuardV1(context.Background(), guard, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !report.CurrentProjectionValid || !report.TamperFailsClosed || !report.OwnerLocalPathInventoryExact || report.ProductionEligible {
		t.Fatalf("unexpected conformance report: %+v", report)
	}
	for index := range all {
		missing := append([]string(nil), all[:index]...)
		missing = append(missing, all[index+1:]...)
		fixture.Wiring.GuardedPaths = missing
		report, err = conformance.CheckModelProviderActualPointGuardV1(context.Background(), guard, fixture)
		if err != nil {
			t.Fatal(err)
		}
		if report.OwnerLocalPathInventoryExact {
			t.Fatalf("missing %q provider path passed conformance", all[index])
		}
	}
}

func TestModelProviderActualPointConformanceV1RejectsTypedNilGuard(t *testing.T) {
	request, projection := conformanceModelFixtureV1(t)
	var typedNil *conformancePointerGuardV1
	_, err := conformance.CheckModelProviderActualPointGuardV1(
		context.Background(),
		typedNil,
		conformanceFixtureV1(request, projection, []string{"direct", "stream", "continuation", "realtime", "routegateway", "raw"}),
	)
	if !core.HasCategory(err, core.ErrorUnavailable) || !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil guard was not rejected safely: %v", err)
	}
}

func TestModelProviderActualPointConformanceV1RequiresExactRequestClosure(t *testing.T) {
	request, projection := conformanceModelFixtureV1(t)
	tests := []struct {
		name   string
		mutate func(*ports.ModelProviderActualPointCurrentProjectionV1)
	}{
		{name: "verifier", mutate: func(value *ports.ModelProviderActualPointCurrentProjectionV1) {
			value.Verifier.ArtifactDigest = conformanceDigestV1("other-verifier")
		}},
		{name: "permit", mutate: func(value *ports.ModelProviderActualPointCurrentProjectionV1) {
			value.PermitDigest = conformanceDigestV1("other-permit")
		}},
		{name: "attempt", mutate: func(value *ports.ModelProviderActualPointCurrentProjectionV1) {
			value.Attempt.AttemptID = "other-attempt"
			value.ModelBoundary.RuntimeAttempt = value.Attempt
			value.ModelBoundary, _ = ports.SealModelProviderBoundaryCurrentRefV1(value.ModelBoundary)
		}},
		{name: "fence", mutate: func(value *ports.ModelProviderActualPointCurrentProjectionV1) {
			value.FenceDigest = conformanceDigestV1("other-fence")
		}},
		{name: "request", mutate: func(value *ports.ModelProviderActualPointCurrentProjectionV1) {
			value.RequestDigest = conformanceDigestV1("other-request")
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			changed := projection
			testCase.mutate(&changed)
			changed, err := ports.SealModelProviderActualPointCurrentProjectionV1(changed)
			if err != nil {
				t.Fatal(err)
			}
			report, err := conformance.CheckModelProviderActualPointGuardV1(
				context.Background(),
				conformanceGuardV1{expected: request, projection: changed},
				conformanceFixtureV1(request, projection, []string{"direct", "stream", "continuation", "realtime", "routegateway", "raw"}),
			)
			if err != nil {
				t.Fatal(err)
			}
			if report.CurrentProjectionValid || report.ProductionEligible {
				t.Fatalf("%s drift passed exact conformance: %+v", testCase.name, report)
			}
		})
	}
}

func TestModelProviderActualPointConformanceV1RejectsProjectionLeakedWithTamperError(t *testing.T) {
	request, projection := conformanceModelFixtureV1(t)
	report, err := conformance.CheckModelProviderActualPointGuardV1(
		context.Background(),
		conformanceGuardV1{expected: request, projection: projection, leakOnTamper: true},
		conformanceFixtureV1(request, projection, []string{"direct", "stream", "continuation", "realtime", "routegateway", "raw"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.TamperFailsClosed || report.ProductionEligible {
		t.Fatalf("Guard leaking a projection with an error passed conformance: %+v", report)
	}
}

type conformancePointerGuardV1 struct{}

func (*conformancePointerGuardV1) InspectCurrentModelProviderActualPointV1(context.Context, ports.InspectCurrentModelProviderActualPointRequestV1) (ports.ModelProviderActualPointCurrentProjectionV1, error) {
	panic("typed-nil guard must not be called")
}

type conformanceGuardV1 struct {
	expected     ports.InspectCurrentModelProviderActualPointRequestV1
	projection   ports.ModelProviderActualPointCurrentProjectionV1
	leakOnTamper bool
}

func (g conformanceGuardV1) InspectCurrentModelProviderActualPointV1(_ context.Context, request ports.InspectCurrentModelProviderActualPointRequestV1) (ports.ModelProviderActualPointCurrentProjectionV1, error) {
	if !ports.SameModelProviderBoundaryCurrentRefV1(request.ModelBoundary, g.expected.ModelBoundary) {
		if g.leakOnTamper {
			return g.projection, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tampered boundary with leaked projection")
		}
		return ports.ModelProviderActualPointCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tampered boundary")
	}
	return g.projection, nil
}

func conformanceModelFixtureV1(t *testing.T) (ports.InspectCurrentModelProviderActualPointRequestV1, ports.ModelProviderActualPointCurrentProjectionV1) {
	t.Helper()
	now := time.Unix(1_200_000, 0)
	lease := &core.SandboxLeaseRef{ID: "lease-conformance", Epoch: 1}
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-conformance", ID: "agent-conformance", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage-conformance", PlanDigest: conformanceDigestV1("plan")},
		Instance: core.InstanceRef{ID: "instance-conformance", Epoch: 1}, SandboxLease: lease, AuthorityEpoch: 1,
	}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-conformance",
		SubjectRevision: 1, CurrentProjectionRef: "run-current-conformance", CurrentProjectionRevision: 1, CurrentProjectionDigest: conformanceDigestV1("run-current"),
	}
	operationDigest, _ := operation.DigestV3()
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: "effect-conformance", IntentRevision: 1, IntentDigest: conformanceDigestV1("intent"),
		PermitID: "permit-conformance", PermitRevision: 1, PermitDigest: conformanceDigestV1("permit"), AttemptID: "attempt-conformance",
	}
	boundary, err := ports.SealModelProviderBoundaryCurrentRefV1(ports.ModelProviderBoundaryCurrentRefV1{
		Owner: core.OwnerRef{Domain: "praxis.model", ID: "model-invoker"}, ID: "boundary-conformance", Revision: 1,
		OperationDigest: operationDigest, EffectID: attempt.EffectID, RuntimeAttempt: attempt, DispatchSequence: 1, ProviderAttemptOrdinal: 1,
		AttemptRequestDigest: conformanceDigestV1("request"), AcknowledgementDigest: conformanceDigestV1("ack"), ExpiresUnixNano: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := ports.ProviderBindingRefV2{
		BindingSetID: "binding-conformance", BindingSetRevision: 1, ComponentID: "praxis.model/provider",
		ManifestDigest: conformanceDigestV1("manifest"), ArtifactDigest: conformanceDigestV1("artifact"), Capability: ports.ModelInvokeCapabilityV1,
	}
	request := ports.InspectCurrentModelProviderActualPointRequestV1{
		Operation: operation, EffectID: attempt.EffectID, ExpectedEffectRevision: 3, PermitID: attempt.PermitID, ExpectedPermitFactRevision: 2,
		PermitDigest: attempt.PermitDigest, AdmissionDigest: conformanceDigestV1("admission"),
		ReviewAuthorization: ports.OperationReviewAuthorizationRefV4{ID: "review-conformance", Revision: 1, Digest: conformanceDigestV1("review")},
		Attempt:             attempt, Verifier: provider, FenceDigest: conformanceDigestV1("fence"), ModelBoundary: boundary, RequestedNotAfterUnixNano: now.Add(time.Second).UnixNano(),
	}
	requestDigest, _ := request.DigestV1()
	projection, err := ports.SealModelProviderActualPointCurrentProjectionV1(ports.ModelProviderActualPointCurrentProjectionV1{
		RequestDigest: requestDigest, OperationDigest: operationDigest, EffectID: request.EffectID, EffectFactRevision: request.ExpectedEffectRevision,
		PermitID: request.PermitID, PermitFactRevision: request.ExpectedPermitFactRevision, PermitDigest: request.PermitDigest,
		AdmissionDigest: request.AdmissionDigest, ReviewAuthorization: request.ReviewAuthorization, Attempt: attempt, FenceDigest: request.FenceDigest,
		RuntimeControlDigest: conformanceDigestV1("control"), ModelBoundary: boundary, Provider: provider, Verifier: provider,
		CheckedUnixNano: now.UnixNano(), NotAfterUnixNano: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request, projection
}

func conformanceFixtureV1(request ports.InspectCurrentModelProviderActualPointRequestV1, projection ports.ModelProviderActualPointCurrentProjectionV1, paths []string) conformance.ModelProviderActualPointConformanceFixtureV1 {
	return conformance.ModelProviderActualPointConformanceFixtureV1{
		Request:                      request,
		CheckedUnixNano:              projection.CheckedUnixNano,
		ExpectedProvider:             projection.Provider,
		ExpectedRuntimeControlDigest: projection.RuntimeControlDigest,
		ExpectedNotAfterUnixNano:     projection.NotAfterUnixNano,
		Wiring:                       conformance.ModelProviderActualPointWiringV1{GuardedPaths: paths},
	}
}

func conformanceDigestV1(value string) core.Digest {
	digest, _ := core.DigestJSON(value)
	return digest
}
