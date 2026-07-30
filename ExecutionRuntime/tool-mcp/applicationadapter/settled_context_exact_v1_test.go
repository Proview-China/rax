package applicationadapter

import (
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type settledExactFixtureV1 struct {
	now        time.Time
	request    applicationcontract.SingleCallToolActionRequestV2
	result     applicationcontract.SingleCallToolActionResultV2
	toolResult toolcontract.ToolResultV2
	domain     toolcontract.DomainResultFact
	projection toolcontract.SettledToolResultProjectionV1
}

func TestBuildSettledToolResultExactChainV1(t *testing.T) {
	f := newSettledExactFixtureV1(t)
	chain, err := BuildSettledToolResultExactChainV1(f.request, f.result, f.toolResult, f.domain, f.projection, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if chain.DomainResult.AttemptID != f.domain.AttemptID || chain.ExpiresUnixNano != f.projection.ExpiresUnixNano || chain.Projection.ProjectionDigest != f.projection.ProjectionDigest {
		t.Fatalf("exact chain lost Attempt, TTL or projection: %#v", chain)
	}
}

func TestBuildSettledToolResultExactChainV1RejectsSpliceAndExpiry(t *testing.T) {
	f := newSettledExactFixtureV1(t)

	t.Run("Application result splice", func(t *testing.T) {
		spliced := f.result
		spliced.Coordinate.ToolResult.Digest = testkit.Digest("another-tool-result")
		if _, err := BuildSettledToolResultExactChainV1(f.request, spliced, f.toolResult, f.domain, f.projection, f.now); err == nil {
			t.Fatal("spliced Application result was accepted")
		}
	})

	t.Run("Application owner Action splice", func(t *testing.T) {
		spliced := f.result
		spliced.Coordinate.ToolResult.ActionRevision++
		coordinate, err := applicationcontract.SealSingleCallToolActionResultCoordinateV2(spliced.Coordinate, f.request, f.now)
		if err != nil {
			t.Fatal(err)
		}
		spliced, err = applicationcontract.SealSingleCallToolActionResultV2(applicationcontract.SingleCallToolActionResultV2{Coordinate: coordinate}, f.request, f.now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = BuildSettledToolResultExactChainV1(f.request, spliced, f.toolResult, f.domain, f.projection, f.now); err == nil {
			t.Fatal("spliced Application owner Action was accepted")
		}
	})

	t.Run("DomainResult splice", func(t *testing.T) {
		spliced := f.domain
		spliced.Digest = testkit.Digest("another-domain-result")
		if _, err := BuildSettledToolResultExactChainV1(f.request, f.result, f.toolResult, spliced, f.projection, f.now); err == nil {
			t.Fatal("spliced DomainResult was accepted")
		}
	})

	t.Run("DomainResult Runtime semantic splice", func(t *testing.T) {
		cases := []struct {
			name   string
			mutate func(*toolcontract.DomainResultFact)
		}{
			{name: "Attempt", mutate: func(value *toolcontract.DomainResultFact) { value.AttemptID = "another-tool-attempt" }},
			{name: "Payload", mutate: func(value *toolcontract.DomainResultFact) { value.Payload = testkit.Payload("another-settled-body") }},
			{name: "AuthoritativeTime", mutate: func(value *toolcontract.DomainResultFact) { value.CreatedUnixNano++ }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				spliced := f.domain
				tc.mutate(&spliced)
				spliced.Digest = ""
				spliced, err := toolcontract.SealDomainResult(spliced)
				if err != nil {
					t.Fatal(err)
				}
				if err = validateDomainResultRuntimeExactV1(spliced, f.toolResult.Inspection.DomainResult); err == nil {
					t.Fatal("spliced Tool/Runtime DomainResult semantics were accepted")
				}
			})
		}
	})

	t.Run("Runtime DomainResult exact ref splice", func(t *testing.T) {
		cases := []struct {
			name   string
			mutate func(*runtimeports.OperationSettlementDomainResultFactRefV4)
		}{
			{name: "ID", mutate: func(value *runtimeports.OperationSettlementDomainResultFactRefV4) {
				value.ID = "another-runtime-domain-result"
			}},
			{name: "Revision", mutate: func(value *runtimeports.OperationSettlementDomainResultFactRefV4) { value.Revision++ }},
			{name: "Digest", mutate: func(value *runtimeports.OperationSettlementDomainResultFactRefV4) {
				value.Digest = testkit.Digest("another-runtime-domain-result")
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				spliced := f.toolResult.Inspection.DomainResult
				tc.mutate(&spliced)
				if err := validateDomainResultRuntimeExactV1(f.domain, spliced); err == nil {
					t.Fatal("spliced Runtime DomainResult exact ref was accepted")
				}
			})
		}
	})

	t.Run("projection artifact splice", func(t *testing.T) {
		spliced := f.projection
		artifact := *spliced.Artifact
		artifact.ID = "another-artifact"
		spliced.Artifact = &artifact
		spliced.ProjectionDigest = ""
		spliced, err := toolcontract.SealSettledToolResultProjectionV1(spliced)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = BuildSettledToolResultExactChainV1(f.request, f.result, f.toolResult, f.domain, spliced, f.now); err == nil {
			t.Fatal("spliced projection artifact was accepted")
		}
	})

	t.Run("TTL crossing", func(t *testing.T) {
		if _, err := BuildSettledToolResultExactChainV1(f.request, f.result, f.toolResult, f.domain, f.projection, time.Unix(0, f.projection.ExpiresUnixNano)); err == nil {
			t.Fatal("expired projection was accepted")
		}
	})
}

func newSettledExactFixtureV1(t *testing.T) settledExactFixtureV1 {
	t.Helper()
	base := newAdapterV2Fixture(t)
	now := base.binding.now
	request := base.binding.request.ApplicationRequest
	candidate := base.projection.CandidateClosure.Candidate
	payload := testkit.Payload("settled exact body")
	old := base.execution.result.Inspection
	domain, err := toolcontract.SealDomainResult(toolcontract.DomainResultFact{
		ID: "tool-domain-result-exact-v1", Action: candidate.ObjectRef(), AttemptID: old.DomainResult.Attempt.AttemptID,
		ObservationDigest: testkit.Digest("tool-observation-exact-v1"), Payload: payload, CreatedUnixNano: now.Add(-time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	runtimeDomain := old.DomainResult
	runtimeDomain.ID = domain.ID
	runtimeDomain.Revision = core.Revision(domain.Revision)
	runtimeDomain.Digest = domain.Digest
	runtimeDomain.Schema = payload.Schema
	runtimeDomain.PayloadDigest = payload.ContentDigest
	runtimeDomain.PayloadRevision = 1
	facts := testkit.ApplicationG6AFixture(now)
	evidence := []runtimeports.OperationSettlementEvidenceBindingV4{facts.Association.Prepare, facts.Association.Execute}
	scopeSet, err := runtimeports.DigestOperationSettlementScopeSetV4(evidence)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := runtimeports.SealOperationSettlementSubmissionV4(runtimeports.OperationSettlementSubmissionV4{
		ID: "runtime-settlement-exact-v1", TenantID: runtimeDomain.TenantID, Operation: runtimeDomain.Operation,
		OperationDigest: runtimeDomain.OperationDigest, OperationScopeDigest: scopeSet, EffectID: runtimeDomain.EffectID,
		ExpectedEffectRevision: 3, Owner: old.Owner, DomainResult: runtimeDomain, Evidence: evidence,
		IdempotencyKey: "runtime-settlement-exact-v1", ConflictDomain: testkit.Digest("runtime-conflict-exact-v1"), SettledUnixNano: now.Add(-time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	settlementFact, err := runtimeports.SealOperationSettlementFactV4(runtimeports.OperationSettlementFactV4{Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	settlement := settlementFact.RefV4()
	association, err := runtimeports.SealOperationSettlementEvidenceAssociationV4(runtimeports.OperationSettlementEvidenceAssociationV4{ID: "runtime-association-exact-v1", Settlement: settlement, Prepare: evidence[0], Execute: evidence[1]})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := runtimeports.SealOperationSettlementTerminalGuardV4(runtimeports.OperationSettlementTerminalGuardV4{ID: "runtime-guard-exact-v1", TenantID: runtimeDomain.TenantID, OperationDigest: runtimeDomain.OperationDigest, EffectID: runtimeDomain.EffectID, Settlement: settlement})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := runtimeports.SealOperationSettlementTerminalProjectionV4(runtimeports.OperationSettlementTerminalProjectionV4{ID: "runtime-terminal-exact-v1", TenantID: runtimeDomain.TenantID, OperationDigest: runtimeDomain.OperationDigest, EffectID: runtimeDomain.EffectID, Settlement: settlement, Association: association.RefV4(), Guard: guard.RefV4(), DomainResult: runtimeDomain})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: settlement, Association: association.RefV4(), Guard: guard.RefV4(), Projection: terminal.RefV4(), DomainResult: runtimeDomain, EffectFactRevision: 4, Owner: old.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}

	artifact := toolcontract.ObjectRef{ID: "tool-result-artifact-exact-v1", Revision: 1, Digest: payload.ContentDigest}
	toolResult := base.execution.result
	toolResult.DomainResult = toolcontract.ObjectRef{ID: domain.ID, Revision: domain.Revision, Digest: domain.Digest}
	toolResult.Inspection = inspection
	toolResult.Schema = payload.Schema
	toolResult.PayloadDigest = payload.ContentDigest
	toolResult.PayloadRevision = 1
	toolResult.Artifacts = []toolcontract.ObjectRef{artifact}
	toolResult.Apply.ID, err = toolcontract.StableID("tool-apply-v2", candidate.ID, domain.ID, string(inspection.Digest))
	if err != nil {
		t.Fatal(err)
	}
	toolResult.Apply.Digest = testkit.Digest("tool-apply-exact-v1")
	toolResult.ID, err = toolcontract.StableID("tool-result-v2", candidate.ID, domain.ID, toolResult.Apply.ID, string(toolResult.Apply.Digest))
	if err != nil {
		t.Fatal(err)
	}
	toolResult, err = toolcontract.SealToolResultV2(toolResult)
	if err != nil {
		t.Fatal(err)
	}

	projection, err := toolcontract.SealSettledToolResultProjectionV1(toolcontract.SettledToolResultProjectionV1{
		Result: toolcontract.ObjectRef{ID: toolResult.ID, Revision: toolResult.Revision, Digest: toolResult.Digest}, Tool: candidate.Tool,
		Inspection: inspection, Schema: toolResult.Schema, PayloadDigest: toolResult.PayloadDigest, PayloadRevision: toolResult.PayloadRevision,
		Artifact: &artifact, Classification: testkit.Digest("classification-exact-v1"), Complete: true,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := applicationcontract.SingleCallToolOwnerResultRefCoordinateV2{
		OwnerContractVersion: applicationcontract.SingleCallToolOwnerResultContractVersionV2, ID: toolResult.ID, Revision: toolResult.Revision, Digest: toolResult.Digest,
		ActionID: request.Action.PendingSubject.PendingActionRef, ActionRevision: candidate.PendingAction.Revision, ActionDigest: request.Action.Digest,
		ApplyID: toolResult.Apply.ID, ApplyRevision: toolResult.Apply.Revision, ApplyDigest: toolResult.Apply.Digest,
		Inspection: inspection, Schema: toolResult.Schema, PayloadDigest: toolResult.PayloadDigest, PayloadRevision: toolResult.PayloadRevision,
		FinalizedUnixNano: toolResult.FinalizedUnixNano,
	}
	coordinate, err := applicationcontract.SealSingleCallToolActionResultCoordinateV2(applicationcontract.SingleCallToolActionResultCoordinateV2{
		ToolResult: owner, Inspection: inspection, Association: association.RefV4(), AssociationCheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := applicationcontract.SealSingleCallToolActionResultV2(applicationcontract.SingleCallToolActionResultV2{Coordinate: coordinate}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	return settledExactFixtureV1{now: now, request: request, result: result, toolResult: toolResult, domain: domain, projection: projection}
}
