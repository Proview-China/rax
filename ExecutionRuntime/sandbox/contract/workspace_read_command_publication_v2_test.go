package contract_test

import (
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

type workspaceReadPublicationClosureV2 struct {
	fixture     testkit.WorkspaceReadCommandPublicationFixtureV2
	semantic    contract.WorkspaceReadCommandPublicationSemanticV2
	command     contract.WorkspaceReadCommandV1
	publication contract.WorkspaceReadCommandPublicationV2
	currentBody contract.WorkspaceReadCommandOwnerCurrentV2
	current     contract.WorkspaceReadCommandOwnerCurrentV2
	commitNow   time.Time
}

func newWorkspaceReadPublicationClosureV2(t *testing.T, suffix string) workspaceReadPublicationClosureV2 {
	t.Helper()
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	fixture := testkit.WorkspaceReadCommandPublicationV2(now, suffix)
	semantic, err := contract.SealWorkspaceReadCommandPublicationSemanticV2(
		fixture.Source,
		fixture.Effect,
		fixture.Prepared,
		fixture.Workspace,
		now,
	)
	if err != nil {
		t.Fatalf("seal semantic: %v", err)
	}
	commitNow := now.Add(time.Second)
	command, err := contract.SealWorkspaceReadPublishedCommandV2(semantic, commitNow)
	if err != nil {
		t.Fatalf("seal command: %v", err)
	}
	publication, err := contract.SealWorkspaceReadCommandPublicationV2(
		contract.WorkspaceReadCommandPublicationV2{Semantic: semantic},
		command,
		commitNow,
	)
	if err != nil {
		t.Fatalf("seal publication: %v", err)
	}
	currentBody := contract.WorkspaceReadCommandOwnerCurrentV2{
		Command:                         command.Meta.Ref(),
		Publication:                     publication.Meta.Ref(),
		PublicationSemanticDigest:       semantic.Digest,
		SourceCommand:                   semantic.SourceCommand,
		SourceSemanticDigest:            semantic.SourceSemanticDigest,
		SourceProjectionDigest:          fixture.Source.ProjectionDigest,
		SourceCheckedUnixNano:           fixture.Source.CheckedUnixNano,
		SourceExpiresUnixNano:           fixture.Source.ExpiresUnixNano,
		RuntimeEffectProjectionDigest:   fixture.Effect.Digest,
		RuntimeEffectCheckedUnixNano:    fixture.Effect.CheckedUnixNano,
		RuntimeEffectExpiresUnixNano:    fixture.Effect.ExpiresUnixNano,
		RuntimePreparedProjectionDigest: fixture.Prepared.ProjectionDigest,
		RuntimePreparedCheckedUnixNano:  fixture.Prepared.CheckedUnixNano,
		RuntimePreparedExpiresUnixNano:  fixture.Prepared.ExpiresUnixNano,
		WorkspaceView:                   fixture.Workspace.Meta.Ref(),
		WorkspaceSemanticDigest:         semantic.WorkspaceSemanticDigest,
		WorkspaceCheckedUnixNano:        commitNow.UnixNano(),
		WorkspaceExpiresUnixNano:        fixture.Workspace.Meta.ExpiresUnixNano,
		WorkspaceLeaseExpiresUnixNano:   fixture.Workspace.Lease.ExpiresUnixNano,
		SemanticNotAfterUnixNano:        semantic.SemanticNotAfterUnixNano,
	}
	current, err := contract.SealInitialWorkspaceReadCommandOwnerCurrentV2(currentBody, commitNow)
	if err != nil {
		t.Fatalf("seal initial owner current: %v", err)
	}
	if err := contract.ValidateWorkspaceReadCommandOwnerClosureV2(command, publication, current); err != nil {
		t.Fatalf("validate three-fact closure: %v", err)
	}
	return workspaceReadPublicationClosureV2{
		fixture: fixture, semantic: semantic, command: command,
		publication: publication, currentBody: currentBody, current: current,
		commitNow: commitNow,
	}
}

func TestWorkspaceReadCommandPublicationV2DelegationRevisionAndDigestClosure(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	fixture := testkit.WorkspaceReadCommandPublicationV2(now, "delegation")
	if got, want := fixture.Source.RuntimeAttempt.Delegation.Revision, fixture.Source.Prepared.DeclaredDelegation.Revision+1; got != want {
		t.Fatalf("fixture must exercise a newer attempt delegation: got %d want %d", got, want)
	}
	if _, err := contract.SealWorkspaceReadCommandPublicationSemanticV2(
		fixture.Source, fixture.Effect, fixture.Prepared, fixture.Workspace, now,
	); err != nil {
		t.Fatalf("greater delegation revision must pass: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*contract.WorkspaceReadSourceCurrentProjectionV2)
	}{
		{
			name: "equal revision",
			mutate: func(source *contract.WorkspaceReadSourceCurrentProjectionV2) {
				source.RuntimeAttempt.Delegation.Revision = source.Prepared.DeclaredDelegation.Revision
			},
		},
		{
			name: "lower revision",
			mutate: func(source *contract.WorkspaceReadSourceCurrentProjectionV2) {
				source.RuntimeAttempt.Delegation.Revision = source.Prepared.DeclaredDelegation.Revision - 1
			},
		},
		{
			name: "different id",
			mutate: func(source *contract.WorkspaceReadSourceCurrentProjectionV2) {
				source.RuntimeAttempt.Delegation.ID += "-splice"
			},
		},
		{
			name: "invalid digest",
			mutate: func(source *contract.WorkspaceReadSourceCurrentProjectionV2) {
				source.RuntimeAttempt.Delegation.Digest = "sha256:invalid"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := contract.CloneWorkspaceReadSourceCurrentProjectionV2(fixture.Source)
			test.mutate(&source)
			digest, _ := contract.WorkspaceReadSourceRuntimeAttemptDigestV2(source.RuntimeAttempt)
			source.RuntimeAttemptDigest = digest
			if _, err := contract.SealWorkspaceReadSourceCurrentProjectionV2(source); err == nil {
				t.Fatal("delegation drift must fail closed")
			}
		})
	}

	source := contract.CloneWorkspaceReadSourceCurrentProjectionV2(fixture.Source)
	source.RuntimeAttempt.Delegation.Digest = testkit.RuntimeDigest("attempt-delegation-splice")
	attemptDigest, err := contract.WorkspaceReadSourceRuntimeAttemptDigestV2(source.RuntimeAttempt)
	if err != nil {
		t.Fatalf("digest-spliced attempt is structurally valid: %v", err)
	}
	source.RuntimeAttemptDigest = attemptDigest
	source, err = contract.SealWorkspaceReadSourceCurrentProjectionV2(source)
	if err != nil {
		t.Fatalf("source seals its exact attempt coordinate before Prepared join: %v", err)
	}
	if _, err := contract.SealWorkspaceReadCommandPublicationSemanticV2(
		source, fixture.Effect, fixture.Prepared, fixture.Workspace, now,
	); err == nil {
		t.Fatal("Prepared exact snapshot must reject a delegation digest splice")
	}
}

func TestWorkspaceReadCommandPublicationV2SourceTimeClosure(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	fixture := testkit.WorkspaceReadCommandPublicationV2(now, "source-times")
	beforePrepared := contract.CloneWorkspaceReadSourceCurrentProjectionV2(fixture.Source)
	beforePrepared.SourceCreatedUnixNano = beforePrepared.Prepared.PreparedUnixNano - 1
	if _, err := contract.SealWorkspaceReadSourceCurrentProjectionV2(beforePrepared); err == nil {
		t.Fatal("Source creation cannot precede Prepared issuance")
	}

	afterRequested := contract.CloneWorkspaceReadSourceCurrentProjectionV2(fixture.Source)
	afterRequested.SourceNotAfterUnixNano = afterRequested.RequestedNotAfterUnixNano + 1
	if _, err := contract.SealWorkspaceReadSourceCurrentProjectionV2(afterRequested); err == nil {
		t.Fatal("Source lifetime cannot exceed caller requested-not-after")
	}

	semantic := contract.CloneWorkspaceReadCommandPublicationSemanticV2(
		newWorkspaceReadPublicationClosureV2(t, "source-effect-time").semantic,
	)
	semantic.RuntimeEffectIntent.ExpiresUnixNano = semantic.SourceNotAfterUnixNano - 1
	semantic.Digest = ""
	digest, err := semantic.ComputeDigestV2()
	if err != nil {
		t.Fatalf("compute tampered semantic digest: %v", err)
	}
	semantic.Digest = digest
	if err := semantic.Validate(); err == nil {
		t.Fatal("Source lifetime cannot exceed the Runtime Effect intent")
	}
}

func TestWorkspaceReadCommandPublicationV2WorkspaceAndOperationAxes(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	fixture := testkit.WorkspaceReadCommandPublicationV2(now, "workspace-axes")
	if got, wantNot := fixture.Workspace.Lease.ObservedRevision, uint64(fixture.Source.Operation.CurrentProjectionRevision); got == wantNot {
		t.Fatal("fixture must prove Workspace lease observation is not the Operation projection revision")
	}
	tests := []struct {
		name   string
		mutate func(*contract.WorkspaceView)
	}{
		{"tenant", func(v *contract.WorkspaceView) { v.Lease.TenantID += "-splice" }},
		{"instance id", func(v *contract.WorkspaceView) { v.Lease.InstanceID += "-splice" }},
		{"instance epoch", func(v *contract.WorkspaceView) { v.Lease.InstanceEpoch++ }},
		{"lease id", func(v *contract.WorkspaceView) { v.Lease.LeaseID += "-splice" }},
		{"lease epoch", func(v *contract.WorkspaceView) { v.Lease.LeaseEpoch++ }},
		{"fence epoch", func(v *contract.WorkspaceView) { v.Lease.FenceEpoch++ }},
		{"scope digest", func(v *contract.WorkspaceView) { v.Lease.ScopeDigest = testkit.Ref("scope-splice").Digest }},
		{"read scope", func(v *contract.WorkspaceView) { v.ReadScopes = []string{"other"} }},
		{"hidden scope", func(v *contract.WorkspaceView) { v.HiddenScopes = []string{"src"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := fixture.Workspace
			workspace.ReadScopes = append([]string(nil), workspace.ReadScopes...)
			workspace.WriteScopes = append([]string(nil), workspace.WriteScopes...)
			workspace.HiddenScopes = append([]string(nil), workspace.HiddenScopes...)
			test.mutate(&workspace)
			if _, err := contract.SealWorkspaceReadCommandPublicationSemanticV2(
				fixture.Source, fixture.Effect, fixture.Prepared, workspace, now,
			); err == nil {
				t.Fatal("Operation to Workspace axis drift must fail closed")
			}
		})
	}
}

func TestWorkspaceReadCommandPublicationV2WorkspaceBodyAndDigestAreSealed(t *testing.T) {
	closure := newWorkspaceReadPublicationClosureV2(t, "workspace-body")

	bodyTamper := contract.CloneWorkspaceReadCommandPublicationSemanticV2(closure.semantic)
	bodyTamper.Workspace.Lease.ObservedRevision++
	bodyTamper.Digest = ""
	digest, err := bodyTamper.ComputeDigestV2()
	if err != nil {
		t.Fatalf("compute outer digest: %v", err)
	}
	bodyTamper.Digest = digest
	if err := bodyTamper.Validate(); err == nil {
		t.Fatal("Workspace body tamper cannot be hidden by resealing the outer semantic")
	}

	digestTamper := contract.CloneWorkspaceReadCommandPublicationSemanticV2(closure.semantic)
	digestTamper.WorkspaceSemanticDigest = testkit.Ref("workspace-semantic-splice").Digest
	digestTamper.Digest = ""
	digest, err = digestTamper.ComputeDigestV2()
	if err != nil {
		t.Fatalf("compute outer digest: %v", err)
	}
	digestTamper.Digest = digest
	if err := digestTamper.Validate(); err == nil {
		t.Fatal("Workspace semantic digest must be recomputed from the full body")
	}
}

func TestWorkspaceReadCommandPublicationV2InitialClockIDsAndExpectedFile(t *testing.T) {
	closure := newWorkspaceReadPublicationClosureV2(t, "initial-clock")
	commandID, err := contract.DeriveWorkspaceReadCommandIDV2(closure.semantic.SourceCommand)
	if err != nil || closure.command.Meta.ID != commandID || closure.command.Meta.Revision != 1 {
		t.Fatalf("derived Command identity drifted: %v", err)
	}
	publicationID, err := contract.DeriveWorkspaceReadCommandPublicationIDV2(closure.semantic.SourceCommand)
	if err != nil || closure.publication.Meta.ID != publicationID || closure.publication.Meta.Revision != 1 {
		t.Fatalf("derived Publication identity drifted: %v", err)
	}
	currentID, err := contract.DeriveWorkspaceReadCommandOwnerCurrentIDV2(closure.command.Meta.Ref())
	if err != nil || closure.current.Meta.ID != currentID || closure.current.Meta.Revision != 1 {
		t.Fatalf("derived Owner current identity drifted: %v", err)
	}
	if closure.command.ExpectedFileRef != nil {
		t.Fatal("published workspace.read Command must not carry ExpectedFileRef")
	}
	if closure.command.Meta.CreatedUnixNano != closure.publication.Meta.CreatedUnixNano ||
		closure.current.Meta.CreatedUnixNano != closure.command.Meta.CreatedUnixNano ||
		closure.current.CheckedUnixNano != closure.current.Meta.CreatedUnixNano {
		t.Fatal("initial Command, Publication, and Owner current must share one owner commit clock")
	}

	differentClock := closure.commitNow.Add(time.Second)
	command2, err := contract.SealWorkspaceReadPublishedCommandV2(closure.semantic, differentClock)
	if err != nil {
		t.Fatalf("seal concurrent command: %v", err)
	}
	publication2, err := contract.SealWorkspaceReadCommandPublicationV2(
		contract.WorkspaceReadCommandPublicationV2{Semantic: closure.semantic},
		command2,
		differentClock,
	)
	if err != nil {
		t.Fatalf("seal concurrent publication: %v", err)
	}
	if command2.Meta.Ref() != closure.command.Meta.Ref() ||
		publication2.Meta.Ref() != closure.publication.Meta.Ref() {
		t.Fatal("different owner commit clocks must not create sibling exact refs")
	}
	if command2.Meta.CreatedUnixNano == closure.command.Meta.CreatedUnixNano ||
		publication2.Meta.CreatedUnixNano == closure.publication.Meta.CreatedUnixNano {
		t.Fatal("winner bodies must preserve their real owner commit clock")
	}
	if _, err := contract.SealWorkspaceReadCommandPublicationV2(
		contract.WorkspaceReadCommandPublicationV2{Semantic: closure.semantic},
		closure.command,
		differentClock,
	); err == nil {
		t.Fatal("Command and Publication cannot be sealed with different initial clocks")
	}
	if err := contract.ValidateWorkspaceReadPublicationCommandV2(closure.command, publication2); err == nil {
		t.Fatal("same stable Ref does not permit cross-clock body splicing")
	}

	withExpected := closure.command
	withExpected.ExpectedFileRef = &contract.Ref{
		ID: "unexpected-file", Revision: 1, Digest: testkit.Ref("unexpected-file").Digest,
	}
	withExpected.Meta = contract.Meta{}
	withExpected, err = contract.SealWorkspaceReadCommandV1(
		withExpected,
		closure.command.Meta.ID,
		closure.commitNow,
		time.Unix(0, closure.semantic.SemanticNotAfterUnixNano),
	)
	if err != nil {
		t.Fatalf("legacy command shape can represent ExpectedFileRef: %v", err)
	}
	if err := contract.ValidateWorkspaceReadPublicationCommandV2(withExpected, closure.publication); err == nil {
		t.Fatal("V2 publication must reject ExpectedFileRef")
	}
}

func TestWorkspaceReadCommandOwnerCurrentV2SuccessorAndThreeFactJoin(t *testing.T) {
	closure := newWorkspaceReadPublicationClosureV2(t, "owner-current")
	nextChecked := closure.commitNow.Add(time.Second)
	nextBody := closure.currentBody
	nextBody.WorkspaceCheckedUnixNano = nextChecked.UnixNano()
	next, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(
		nextBody,
		closure.current,
		nextChecked,
	)
	if err != nil {
		t.Fatalf("seal next owner current: %v", err)
	}
	if next.Meta.Revision != closure.current.Meta.Revision+1 ||
		next.Meta.CreatedUnixNano != closure.current.Meta.CreatedUnixNano ||
		next.CheckedUnixNano != nextChecked.UnixNano() {
		t.Fatal("Owner current successor revision or clock closure drifted")
	}
	expectedExpiry := closure.fixture.Source.ExpiresUnixNano
	if next.ExpiresUnixNano != expectedExpiry {
		t.Fatalf("Owner current expiry must be the exact shortest bound: got %d want %d", next.ExpiresUnixNano, expectedExpiry)
	}
	if err := contract.ValidateWorkspaceReadCommandOwnerClosureV2(
		closure.command, closure.publication, next,
	); err != nil {
		t.Fatalf("validate successor three-fact closure: %v", err)
	}
	if _, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(
		closure.currentBody,
		closure.current,
		closure.commitNow.Add(-time.Nanosecond),
	); err == nil {
		t.Fatal("Owner current checked clock rollback must fail closed")
	}

	splicedBody := closure.currentBody
	splicedBody.PublicationSemanticDigest = testkit.RuntimeDigest("publication-semantic-splice")
	spliced, err := contract.SealInitialWorkspaceReadCommandOwnerCurrentV2(splicedBody, closure.commitNow)
	if err != nil {
		t.Fatalf("self-consistent spliced current is a valid envelope: %v", err)
	}
	if err := contract.ValidateWorkspaceReadCommandOwnerClosureV2(
		closure.command, closure.publication, spliced,
	); err == nil {
		t.Fatal("OwnerCurrent self-validation cannot replace the three-fact join")
	}
}

func TestWorkspaceReadCommandPublicationV2ExpiredPredecessorCanRefreshFromFreshReaders(t *testing.T) {
	closure := newWorkspaceReadPublicationClosureV2(t, "expiry")
	oldExpiry := time.Unix(0, closure.current.ExpiresUnixNano)
	if err := closure.current.ValidateCurrent(oldExpiry); err == nil {
		t.Fatal("Owner current uses half-open TTL")
	}

	source := contract.CloneWorkspaceReadSourceCurrentProjectionV2(closure.fixture.Source)
	source.CheckedUnixNano = oldExpiry.UnixNano()
	source.ExpiresUnixNano = oldExpiry.Add(8 * time.Second).UnixNano()
	source, err := contract.SealWorkspaceReadSourceCurrentProjectionV2(source)
	if err != nil {
		t.Fatalf("seal refreshed Source current: %v", err)
	}
	effect := closure.fixture.Effect
	effect.CheckedUnixNano = oldExpiry.UnixNano()
	effect.ExpiresUnixNano = oldExpiry.Add(10 * time.Second).UnixNano()
	effect, err = runtimeports.SealControlledOperationEffectCurrentProjectionV2(effect)
	if err != nil {
		t.Fatalf("seal refreshed Effect current: %v", err)
	}
	prepared := closure.fixture.Prepared
	prepared.CheckedUnixNano = oldExpiry.UnixNano()
	prepared.ExpiresUnixNano = oldExpiry.Add(10 * time.Second).UnixNano()
	prepared, err = runtimeports.SealControlledOperationPreparedCurrentProjectionV2(prepared)
	if err != nil {
		t.Fatalf("seal refreshed Prepared current: %v", err)
	}
	body := closure.currentBody
	body.SourceProjectionDigest = source.ProjectionDigest
	body.SourceCheckedUnixNano = source.CheckedUnixNano
	body.SourceExpiresUnixNano = source.ExpiresUnixNano
	body.RuntimeEffectProjectionDigest = effect.Digest
	body.RuntimeEffectCheckedUnixNano = effect.CheckedUnixNano
	body.RuntimeEffectExpiresUnixNano = effect.ExpiresUnixNano
	body.RuntimePreparedProjectionDigest = prepared.ProjectionDigest
	body.RuntimePreparedCheckedUnixNano = prepared.CheckedUnixNano
	body.RuntimePreparedExpiresUnixNano = prepared.ExpiresUnixNano
	body.WorkspaceCheckedUnixNano = oldExpiry.UnixNano()
	next, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(body, closure.current, oldExpiry)
	if err != nil {
		t.Fatalf("expired exact predecessor may be refreshed from fresh owner reads: %v", err)
	}
	if next.Meta.Revision != closure.current.Meta.Revision+1 ||
		next.Meta.CreatedUnixNano != closure.current.Meta.CreatedUnixNano {
		t.Fatal("refreshed current must be the exact no-ABA successor")
	}
	if err := contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
		next, source, effect, prepared, closure.fixture.Workspace, oldExpiry,
	); err != nil {
		t.Fatalf("fresh successor qualification: %v", err)
	}
	if err := next.ValidateCurrent(time.Unix(0, next.ExpiresUnixNano)); err == nil {
		t.Fatal("fresh successor also uses half-open TTL")
	}
}

func TestWorkspaceReadCommandOwnerFreshClosureV2RejectsTransientSplices(t *testing.T) {
	closure := newWorkspaceReadPublicationClosureV2(t, "fresh-splice")
	if err := contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
		closure.current,
		closure.fixture.Source,
		closure.fixture.Effect,
		closure.fixture.Prepared,
		closure.fixture.Workspace,
		closure.commitNow,
	); err != nil {
		t.Fatalf("valid fresh join: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*contract.WorkspaceReadCommandOwnerCurrentV2)
	}{
		{"source digest", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.SourceProjectionDigest = testkit.RuntimeDigest("source-current-splice")
		}},
		{"source checked", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.SourceCheckedUnixNano++
		}},
		{"source expires", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.SourceExpiresUnixNano--
		}},
		{"effect digest", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.RuntimeEffectProjectionDigest = testkit.RuntimeDigest("effect-current-splice")
		}},
		{"effect checked", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.RuntimeEffectCheckedUnixNano++
		}},
		{"effect expires", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.RuntimeEffectExpiresUnixNano--
		}},
		{"prepared digest", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.RuntimePreparedProjectionDigest = testkit.RuntimeDigest("prepared-current-splice")
		}},
		{"prepared checked", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.RuntimePreparedCheckedUnixNano++
		}},
		{"prepared expires", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.RuntimePreparedExpiresUnixNano--
		}},
		{"workspace digest", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.WorkspaceSemanticDigest = testkit.Ref("workspace-current-splice").Digest
		}},
		{"workspace future watermark", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.WorkspaceCheckedUnixNano = closure.commitNow.UnixNano() + 1
		}},
		{"workspace expires", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.WorkspaceExpiresUnixNano--
		}},
		{"workspace lease expires", func(p *contract.WorkspaceReadCommandOwnerCurrentV2) {
			p.WorkspaceLeaseExpiresUnixNano--
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := closure.currentBody
			test.mutate(&body)
			spliced, err := contract.SealInitialWorkspaceReadCommandOwnerCurrentV2(body, closure.commitNow)
			if err != nil {
				// A self-shape rejection is also fail-closed; the important
				// property is that no spliced qualification is returned.
				return
			}
			if err := contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
				spliced,
				closure.fixture.Source,
				closure.fixture.Effect,
				closure.fixture.Prepared,
				closure.fixture.Workspace,
				closure.commitNow,
			); err == nil {
				t.Fatal("fresh owner projection/time splice must fail closed")
			}
		})
	}

	workspace := closure.fixture.Workspace
	workspace.Lease.ObservedRevision++
	if err := contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
		closure.current,
		closure.fixture.Source,
		closure.fixture.Effect,
		closure.fixture.Prepared,
		workspace,
		closure.commitNow,
	); err == nil {
		t.Fatal("Workspace S1/S2 full-body ObservedRevision drift must fail closed")
	}
}

func TestWorkspaceReadCommandPublicationV2CoreDigestRepresentation(t *testing.T) {
	closure := newWorkspaceReadPublicationClosureV2(t, "digest-representation")
	if got := closure.command.OperationDigest; len(got) != len("sha256:")+contract.DigestSizeHex || got[:len("sha256:")] != "sha256:" {
		t.Fatalf("Runtime digest representation must remain prefixed: %q", got)
	}
	if got := closure.command.SourceToolCommand.Digest; len(got) != contract.DigestSizeHex {
		t.Fatalf("Sandbox Ref digest must remain raw lowercase hex: %q", got)
	}
	if runtimecore.Digest(closure.command.SourceToolPayloadDigest) != closure.semantic.PayloadDigest {
		t.Fatal("payload digest representation drifted")
	}
}
