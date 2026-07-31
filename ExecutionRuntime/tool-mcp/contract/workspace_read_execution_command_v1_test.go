package contract_test

import (
	"testing"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestWorkspaceReadExecutionCommandV1ValidFact(t *testing.T) {
	fact := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "valid")

	if err := fact.Validate(); err != nil {
		t.Fatalf("valid workspace.read execution command rejected: %v", err)
	}
	if err := fact.ValidateCurrent(testkit.FixedTime.Add(time.Second)); err != nil {
		t.Fatalf("fresh workspace.read execution command rejected: %v", err)
	}
}

func TestWorkspaceReadExecutionCommandV1RejectsRuntimeEffectSplices(t *testing.T) {
	t.Run("intent digest", func(t *testing.T) {
		fact := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "intent-splice")
		fact.RuntimeEffectIntentDigest = testkit.Digest("different-runtime-intent")
		fact.Ref.Digest = ""

		if _, err := toolcontract.SealWorkspaceReadExecutionCommandV1(fact); err == nil {
			t.Fatal("Runtime Effect Intent digest different from Runtime Attempt was accepted")
		}
	})

	t.Run("state closed set", func(t *testing.T) {
		fact := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "state-splice")
		fact.RuntimeEffectState = "prepared"
		fact.Ref.Digest = ""

		if _, err := toolcontract.SealWorkspaceReadExecutionCommandV1(fact); err == nil {
			t.Fatal("Runtime Effect state outside dispatch_intent was accepted")
		}
	})
}

func TestWorkspaceReadExecutionCommandSourceV1RejectsExecutionStateAndClaimSplices(t *testing.T) {
	fact := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "source-splice")

	for _, test := range []struct {
		name   string
		mutate func(*toolcontract.WorkspaceReadExecutionCommandSourceV1)
	}{
		{
			name: "execution state revision",
			mutate: func(source *toolcontract.WorkspaceReadExecutionCommandSourceV1) {
				source.ExecutionStateRef.Revision = 2
			},
		},
		{
			name: "execution state kind",
			mutate: func(source *toolcontract.WorkspaceReadExecutionCommandSourceV1) {
				source.ExecutionStateKind = "inspect_only"
			},
		},
		{
			name: "claim identity",
			mutate: func(source *toolcontract.WorkspaceReadExecutionCommandSourceV1) {
				source.ClaimRef.ID = "workspace-read-forged-claim"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := fact.Source
			test.mutate(&source)
			source.Digest = ""

			if _, err := toolcontract.SealWorkspaceReadExecutionCommandSourceV1(source); err == nil {
				t.Fatalf("%s splice was accepted", test.name)
			}
		})
	}
}

func TestWorkspaceReadExecutionCommandIDV1IncludesPreparedExactRef(t *testing.T) {
	fact := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "prepared-exact")
	firstID, err := toolcontract.DeriveWorkspaceReadExecutionCommandIDV1(
		fact.Source,
		fact.Prepared,
		fact.RuntimeAttempt,
	)
	if err != nil {
		t.Fatal(err)
	}

	changedPrepared := fact.Prepared
	changedPrepared.ExpiresUnixNano++
	changedPrepared.Digest = ""
	changedPrepared, err = runtimeports.SealPreparedProviderAttemptRefV2(changedPrepared)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := toolcontract.DeriveWorkspaceReadExecutionCommandIDV1(
		fact.Source,
		changedPrepared,
		fact.RuntimeAttempt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if firstID == secondID {
		t.Fatal("Prepared exact Ref drift did not change workspace.read execution command identity")
	}
}

func TestWorkspaceReadExecutionCommandV1RejectsPermitAndDelegationSplices(t *testing.T) {
	t.Run("permit", func(t *testing.T) {
		fact := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "permit-splice")
		fact.RuntimeAttempt.PermitDigest = testkit.Digest("different-permit")
		attemptDigest, err := toolcontract.DigestWorkspaceReadExecutionRuntimeAttemptV1(fact.RuntimeAttempt)
		if err != nil {
			t.Fatal(err)
		}
		fact.RuntimeAttemptDigest = attemptDigest
		fact.Ref.Digest = ""

		if _, err := toolcontract.SealWorkspaceReadExecutionCommandV1(fact); err == nil {
			t.Fatal("Runtime Permit splice was accepted")
		}
	})

	t.Run("delegation under existing exact command", func(t *testing.T) {
		fact := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "delegation-splice")
		fact.RuntimeAttempt.Delegation.Digest = testkit.Digest("different-delegation")
		attemptDigest, err := toolcontract.DigestWorkspaceReadExecutionRuntimeAttemptV1(fact.RuntimeAttempt)
		if err != nil {
			t.Fatal(err)
		}
		fact.RuntimeAttemptDigest = attemptDigest
		fact.Ref.Digest = ""

		if _, err := toolcontract.SealWorkspaceReadExecutionCommandV1(fact); err == nil {
			t.Fatal("Runtime Delegation splice was accepted under the original exact command identity")
		}
	})
}

func TestWorkspaceReadExecutionCommandV1CreatedAcceptsAnyInstantAtOrAboveLowerBound(t *testing.T) {
	fact := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "created-lower")
	lower := fact.TTL.EffectiveCreatedLowerUnixNano

	for _, created := range []int64{lower, lower + 1} {
		candidate := toolcontract.CloneWorkspaceReadExecutionCommandV1(fact)
		candidate.CreatedUnixNano = created
		candidate.Ref.Digest = ""
		sealed, err := toolcontract.SealWorkspaceReadExecutionCommandV1(candidate)
		if err != nil {
			t.Fatalf("CreatedUnixNano=%d at or above lower bound was rejected: %v", created, err)
		}
		if sealed.CreatedUnixNano != created {
			t.Fatalf("CreatedUnixNano changed during seal: got %d want %d", sealed.CreatedUnixNano, created)
		}
	}

	below := toolcontract.CloneWorkspaceReadExecutionCommandV1(fact)
	below.CreatedUnixNano = lower - 1
	below.Ref.Digest = ""
	if _, err := toolcontract.SealWorkspaceReadExecutionCommandV1(below); err == nil {
		t.Fatal("CreatedUnixNano below the authoritative lower bound was accepted")
	}
}

func TestSameWorkspaceReadExecutionCommandStableClosureV1IgnoresOnlyCreatedAndFactDigest(t *testing.T) {
	base := testkit.WorkspaceReadExecutionCommandV1(testkit.FixedTime, "stable-closure")
	differentCreated := resealWorkspaceReadExecutionCommandV1(t, func(value *toolcontract.WorkspaceReadExecutionCommandV1) {
		value.CreatedUnixNano++
	}, base)
	if base.Ref.Digest == differentCreated.Ref.Digest {
		t.Fatal("Created drift did not produce a distinct exact Fact digest")
	}
	same, err := toolcontract.SameWorkspaceReadExecutionCommandStableClosureV1(base, differentCreated)
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Fatal("stable closure rejected commands differing only in CreatedUnixNano and exact Fact digest")
	}

	tests := []struct {
		name   string
		mutate func(*toolcontract.WorkspaceReadExecutionCommandV1)
	}{
		{
			name: "runtime effect fact revision",
			mutate: func(value *toolcontract.WorkspaceReadExecutionCommandV1) {
				value.RuntimeEffectFactRevision++
			},
		},
		{
			name: "prepared semantic digest",
			mutate: func(value *toolcontract.WorkspaceReadExecutionCommandV1) {
				value.PreparedSemanticDigest = testkit.Digest("different-prepared-semantic")
			},
		},
		{
			name: "source candidate closure",
			mutate: func(value *toolcontract.WorkspaceReadExecutionCommandV1) {
				source := value.Source
				source.CandidateClosureDigest = testkit.Digest("different-candidate-closure")
				source.Digest = ""
				var err error
				source, err = toolcontract.SealWorkspaceReadExecutionCommandSourceV1(source)
				if err != nil {
					t.Fatalf("seal mutated Source: %v", err)
				}
				value.Source = source
			},
		},
		{
			name: "ttl closure",
			mutate: func(value *toolcontract.WorkspaceReadExecutionCommandV1) {
				ttl := value.TTL
				ttl.RequestedNotAfterUnixNano--
				ttl.Digest = ""
				var err error
				ttl, err = toolcontract.SealWorkspaceReadExecutionCommandTTLClosureV1(ttl)
				if err != nil {
					t.Fatalf("seal mutated TTL closure: %v", err)
				}
				value.TTL = ttl
				value.NotAfterUnixNano = ttl.EffectiveNotAfterUnixNano
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := resealWorkspaceReadExecutionCommandV1(t, test.mutate, base)
			same, err := toolcontract.SameWorkspaceReadExecutionCommandStableClosureV1(base, changed)
			if err != nil {
				t.Fatal(err)
			}
			if same {
				t.Fatalf("%s drift was ignored by stable closure", test.name)
			}
		})
	}
}

func resealWorkspaceReadExecutionCommandV1(
	t *testing.T,
	mutate func(*toolcontract.WorkspaceReadExecutionCommandV1),
	value toolcontract.WorkspaceReadExecutionCommandV1,
) toolcontract.WorkspaceReadExecutionCommandV1 {
	t.Helper()
	value = toolcontract.CloneWorkspaceReadExecutionCommandV1(value)
	mutate(&value)
	value.Ref = toolcontract.WorkspaceReadExecutionCommandRefV1{}
	sealed, err := toolcontract.SealWorkspaceReadExecutionCommandV1(value)
	if err != nil {
		t.Fatalf("seal mutated workspace.read execution command: %v", err)
	}
	return sealed
}
