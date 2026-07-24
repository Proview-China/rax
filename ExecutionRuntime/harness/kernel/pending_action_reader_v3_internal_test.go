package kernel

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestNewCommittedPendingActionReaderV3RejectsEveryTypedNilDependencyWithoutReads(t *testing.T) {
	valid := &nilDependencyStubV3{}
	var typedNil *nilDependencyStubV3
	clock := func() time.Time { return time.Unix(1_750_000_000, 0) }
	tests := []struct {
		name string
		call func() error
	}{
		{"sessions", func() error {
			_, err := NewCommittedPendingActionReaderV3(typedNil, valid, valid, valid, valid, valid, valid, valid, valid, valid, clock)
			return err
		}},
		{"candidates", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, typedNil, valid, valid, valid, valid, valid, valid, valid, valid, clock)
			return err
		}},
		{"domain-results", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, valid, typedNil, valid, valid, valid, valid, valid, valid, valid, clock)
			return err
		}},
		{"models", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, valid, valid, typedNil, valid, valid, valid, valid, valid, valid, clock)
			return err
		}},
		{"settlements", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, valid, valid, valid, typedNil, valid, valid, valid, valid, valid, clock)
			return err
		}},
		{"associations", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, valid, valid, valid, valid, typedNil, valid, valid, valid, valid, clock)
			return err
		}},
		{"generations", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, valid, valid, valid, valid, valid, typedNil, valid, valid, valid, clock)
			return err
		}},
		{"routes", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, valid, valid, valid, valid, valid, valid, typedNil, valid, valid, clock)
			return err
		}},
		{"bindings", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, valid, valid, valid, valid, valid, valid, valid, typedNil, valid, clock)
			return err
		}},
		{"contexts", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, valid, valid, valid, valid, valid, valid, valid, valid, typedNil, clock)
			return err
		}},
		{"clock", func() error {
			_, err := NewCommittedPendingActionReaderV3(valid, valid, valid, valid, valid, valid, valid, valid, valid, valid, nil)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if !core.HasCategory(err, core.ErrorUnavailable) || !core.HasReason(err, core.ReasonComponentMissing) || valid.reads != 0 {
				t.Fatalf("typed nil classification/zero-read mismatch: reads=%d err=%v", valid.reads, err)
			}
		})
	}
}

func TestNormalizeProviderBindingRolesV3GroupsOneExactRefAndSortsRoles(t *testing.T) {
	ref := providerBindingRefFixtureV3(1, "shared")
	values := providerBindingRolesFixtureV3(ref)
	groups, err := normalizeProviderBindingRolesV3(values)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Roles) != len(closedProviderBindingRolesV3) {
		t.Fatalf("same exact ref did not form one closed group: %#v", groups)
	}
	for index := 1; index < len(groups[0].Roles); index++ {
		if groups[0].Roles[index-1].Role >= groups[0].Roles[index].Role {
			t.Fatal("roles are not exact sorted within the canonical ref group")
		}
	}
}

func TestNormalizeProviderBindingRolesV3RejectsTypoMissingAndExtra(t *testing.T) {
	ref := providerBindingRefFixtureV3(1, "closed")
	base := providerBindingRolesFixtureV3(ref)
	for name, values := range map[string][]providerBindingRoleV3{
		"typo": func() []providerBindingRoleV3 {
			copy := append([]providerBindingRoleV3(nil), base...)
			copy[0].Role = "session-endpoint"
			return copy
		}(),
		"missing": append([]providerBindingRoleV3(nil), base[:len(base)-1]...),
		"extra":   append(append([]providerBindingRoleV3(nil), base...), providerBindingRoleV3{Role: "extra", Ref: ref}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeProviderBindingRolesV3(values); err == nil {
				t.Fatal("non-closed role set was accepted")
			}
		})
	}
}

func TestNormalizeProviderBindingRolesV3UsesFullRevisionWithoutCollision(t *testing.T) {
	low := providerBindingRefFixtureV3(1, "revision")
	high := providerBindingRefFixtureV3(65_537, "revision")
	values := providerBindingRolesFixtureV3(low)
	for index := len(values) / 2; index < len(values); index++ {
		values[index].Ref = high
	}
	groups, err := normalizeProviderBindingRolesV3(values)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].key == groups[1].key {
		t.Fatalf("large revisions collided in canonical grouping: %#v", groups)
	}
}

func TestInspectProviderBindingRoleGroupsV3ReadsSharedRefOnce(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	ref := providerBindingRefFixtureV3(1, "once")
	groups, err := normalizeProviderBindingRolesV3(providerBindingRolesFixtureV3(ref))
	if err != nil {
		t.Fatal(err)
	}
	set := runtimeports.GenerationBindingSetCurrentProjectionV1{BindingSetDigest: testkit.Digest("set"), BindingSetSemanticDigest: testkit.Digest("semantic")}
	projection, err := runtimeports.SealProviderBindingCurrentProjectionV2(runtimeports.ProviderBindingCurrentProjectionV2{
		ContractVersion: runtimeports.ProviderBindingCurrentnessContractVersionV2,
		Ref:             ref, State: runtimeports.ProviderBindingCurrentActiveV2,
		BindingSetDigest: set.BindingSetDigest, BindingSetSemanticDigest: set.BindingSetSemanticDigest,
		BindingID: "binding-once", BindingRevision: 1, GrantDigest: testkit.Digest("grant"),
		IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &countingProviderBindingReaderV3{projection: projection, driftOnSecond: true}
	expires, err := inspectProviderBindingRoleGroupsV3(context.Background(), reader, groups, set, now)
	if err != nil || reader.calls != 1 || len(expires) != 1 || expires[0] != projection.ExpiresUnixNano {
		t.Fatalf("shared ref was not read exactly once: calls=%d expires=%v err=%v", reader.calls, expires, err)
	}
}

func TestInspectProviderBindingRoleGroupsV3RejectsSetDigestSplice(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	ref := providerBindingRefFixtureV3(1, "splice")
	groups, err := normalizeProviderBindingRolesV3(providerBindingRolesFixtureV3(ref))
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtimeports.SealProviderBindingCurrentProjectionV2(runtimeports.ProviderBindingCurrentProjectionV2{
		ContractVersion: runtimeports.ProviderBindingCurrentnessContractVersionV2,
		Ref:             ref, State: runtimeports.ProviderBindingCurrentActiveV2,
		BindingSetDigest: testkit.Digest("wrong-set"), BindingSetSemanticDigest: testkit.Digest("wrong-semantic"),
		BindingID: "binding-splice", BindingRevision: 1, GrantDigest: testkit.Digest("grant-splice"),
		IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	set := runtimeports.GenerationBindingSetCurrentProjectionV1{BindingSetDigest: testkit.Digest("set"), BindingSetSemanticDigest: testkit.Digest("semantic")}
	reader := &countingProviderBindingReaderV3{projection: projection}
	if _, err := inspectProviderBindingRoleGroupsV3(context.Background(), reader, groups, set, now); err == nil || reader.calls != 1 {
		t.Fatalf("BindingSet digest/semantic splice was accepted or reread: calls=%d err=%v", reader.calls, err)
	}
}

type countingProviderBindingReaderV3 struct {
	projection    runtimeports.ProviderBindingCurrentProjectionV2
	calls         int
	driftOnSecond bool
}

func (r *countingProviderBindingReaderV3) InspectProviderBindingCurrentV2(context.Context, runtimeports.ProviderBindingRefV2) (runtimeports.ProviderBindingCurrentProjectionV2, error) {
	r.calls++
	if r.driftOnSecond && r.calls > 1 {
		changed := r.projection
		changed.BindingSetDigest = testkit.Digest("second-read-drift")
		return changed, nil
	}
	return r.projection, nil
}

func providerBindingRolesFixtureV3(ref runtimeports.ProviderBindingRefV2) []providerBindingRoleV3 {
	values := make([]providerBindingRoleV3, len(closedProviderBindingRolesV3))
	for index, role := range closedProviderBindingRolesV3 {
		values[index] = providerBindingRoleV3{Role: role, Ref: ref}
	}
	return values
}

func providerBindingRefFixtureV3(revision uint64, suffix string) runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-set-" + suffix, BindingSetRevision: core.Revision(revision),
		ComponentID:    runtimeports.ComponentIDV2("custom.provider/" + suffix),
		ManifestDigest: testkit.Digest("manifest-" + suffix), ArtifactDigest: testkit.Digest("artifact-" + suffix),
		Capability: "praxis.tool/execute",
	}
}

type nilDependencyStubV3 struct{ reads int }

func (s *nilDependencyStubV3) CreateSessionV4(context.Context, contract.GovernedSessionV4) (contract.GovernedSessionV4, error) {
	s.reads++
	return contract.GovernedSessionV4{}, nil
}
func (s *nilDependencyStubV3) InspectSessionV4(context.Context, contract.RunRef, string) (contract.GovernedSessionV4, error) {
	s.reads++
	return contract.GovernedSessionV4{}, nil
}
func (s *nilDependencyStubV3) CompareAndSwapSessionV4(context.Context, contract.SessionCASRequestV4) (contract.GovernedSessionV4, error) {
	s.reads++
	return contract.GovernedSessionV4{}, nil
}
func (s *nilDependencyStubV3) CreateCandidateV2(context.Context, contract.ModelTurnCandidateV2) (contract.ModelTurnCandidateV2, error) {
	s.reads++
	return contract.ModelTurnCandidateV2{}, nil
}
func (s *nilDependencyStubV3) InspectCandidateV2(context.Context, contract.RunRef, string) (contract.ModelTurnCandidateV2, error) {
	s.reads++
	return contract.ModelTurnCandidateV2{}, nil
}
func (s *nilDependencyStubV3) InspectExact(context.Context, contract.SettledTurnDomainResultFactRefV3) (contract.SettledTurnDomainResultFactV3, error) {
	s.reads++
	return contract.SettledTurnDomainResultFactV3{}, nil
}
func (s *nilDependencyStubV3) InspectExactProjectionV1(context.Context, modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	s.reads++
	return modelinvoker.ToolCallCandidateObservationProjectionV1{}, nil
}
func (s *nilDependencyStubV3) InspectOperationSettlementV3(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID) (runtimeports.OperationSettlementRefV3, error) {
	s.reads++
	return runtimeports.OperationSettlementRefV3{}, nil
}
func (s *nilDependencyStubV3) AssociateGenerationBindingV1(context.Context, runtimeports.GenerationBindingAssociationCandidateV1) (runtimeports.GenerationBindingAssociationFactV1, error) {
	s.reads++
	return runtimeports.GenerationBindingAssociationFactV1{}, nil
}
func (s *nilDependencyStubV3) InspectCurrentGenerationBindingAssociationV1(context.Context, string) (runtimeports.GenerationBindingAssociationFactV1, error) {
	s.reads++
	return runtimeports.GenerationBindingAssociationFactV1{}, nil
}
func (s *nilDependencyStubV3) InspectGenerationCurrentV1(context.Context, runtimeports.GenerationArtifactRefV1) (runtimeports.GenerationCurrentProjectionV1, error) {
	s.reads++
	return runtimeports.GenerationCurrentProjectionV1{}, nil
}
func (s *nilDependencyStubV3) InspectCurrentControlledOperationProviderRouteV2(context.Context, runtimeports.ControlledOperationProviderRouteCurrentRefV2, runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	s.reads++
	return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, nil
}
func (s *nilDependencyStubV3) InspectProviderBindingCurrentV2(context.Context, runtimeports.ProviderBindingRefV2) (runtimeports.ProviderBindingCurrentProjectionV2, error) {
	s.reads++
	return runtimeports.ProviderBindingCurrentProjectionV2{}, nil
}
func (s *nilDependencyStubV3) InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Context, runtimeports.OperationScopeEvidenceApplicabilityFactRefV3) (runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3, error) {
	s.reads++
	return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, nil
}
