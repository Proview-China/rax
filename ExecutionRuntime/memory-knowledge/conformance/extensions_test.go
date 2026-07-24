package conformance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/consolidation"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/retrieval"
)

func cr(id string) contract.Ref { return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id} }

type retriever struct {
	observation  retrieval.ChannelObservationV1
	ignoreCancel bool
}

func (r retriever) RetrieveChannel(ctx context.Context, _ retrieval.HybridRequestV1, _ retrieval.ChannelBudgetV1) (retrieval.ChannelObservationV1, error) {
	if !r.ignoreCancel {
		if err := ctx.Err(); err != nil {
			return retrieval.ChannelObservationV1{}, err
		}
	}
	return r.observation, nil
}

type indexer struct{ observation projection.BuildObservationV1 }

func (i indexer) BuildIndex(ctx context.Context, _ projection.BuildRequestV1) (projection.BuildObservationV1, error) {
	if err := ctx.Err(); err != nil {
		return projection.BuildObservationV1{}, err
	}
	return i.observation, nil
}

type consolidator struct{ batch consolidation.BatchV1 }

func (c consolidator) Consolidate(ctx context.Context, _ consolidation.InputV1) (consolidation.BatchV1, error) {
	if err := ctx.Err(); err != nil {
		return consolidation.BatchV1{}, err
	}
	return c.batch, nil
}

type admissionPolicy struct{ advice contract.AdmissionAdviceV1 }

func (p admissionPolicy) AdviseAdmission(ctx context.Context, _ contract.AdmissionPolicyRequestV1) (contract.AdmissionAdviceV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.AdmissionAdviceV1{}, err
	}
	return p.advice, nil
}

type sourceConnector struct {
	observation knowledge.AcquireObservationV1
	inspection  knowledge.AcquireInspectionV1
}

func (c sourceConnector) Acquire(ctx context.Context, _ knowledge.AcquireRequestV1) (knowledge.AcquireObservationV1, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.AcquireObservationV1{}, err
	}
	return c.observation, nil
}
func (c sourceConnector) InspectAcquire(ctx context.Context, _, _ contract.Ref) (knowledge.AcquireInspectionV1, *knowledge.AcquireObservationV1, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.AcquireInspectionV1{}, nil, err
	}
	observation := c.observation
	return c.inspection, &observation, nil
}

func TestExtensionConformanceAcceptsObservationAndProposalOnlyProviders(t *testing.T) {
	now := time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)
	query := contract.RetrievalQuery{ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: cr("view"), Purpose: "assist", Text: "alpha", Scopes: []string{"private"}, SensitivityMax: "internal", Limit: 5, RequestedAt: now, ExpiresAt: now.Add(time.Hour)}
	request, err := retrieval.SealHybridRequestV1(retrieval.HybridRequestV1{Query: query, Channels: []retrieval.ChannelBudgetV1{{Kind: contract.IndexLexical, Limit: 5, Weight: 1}}, RRFK: 60, MaxCandidates: 10})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := retrieval.SealChannelObservationV1(retrieval.ChannelObservationV1{Kind: contract.IndexLexical, ProjectionRef: cr("projection"), ViewRef: query.ViewRef, WatermarkRef: cr("watermark"), Coverage: contract.Coverage{Status: contract.CoverageNone}, ObservedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckChannelRetrieverV1(context.Background(), now, retriever{observation: observation}, request, request.Channels[0]); err != nil {
		t.Fatal(err)
	}

	buildRequest, err := projection.SealBuildRequestV1(projection.BuildRequestV1{Owner: contract.OwnerMemory, Kind: contract.IndexLexical, ViewRef: cr("view"), BoundaryRef: cr("watermark"), RecordRefs: []contract.Ref{cr("record")}, BuilderRef: cr("builder"), BuilderVersion: "v1", IndexVersion: "v1", RequestedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	buildObservation, err := projection.SealBuildObservationV1(projection.BuildObservationV1{Ref: contract.Ref{ID: "build-observation", Revision: 1}, Owner: buildRequest.Owner, Kind: buildRequest.Kind, ViewRef: buildRequest.ViewRef, BoundaryRef: buildRequest.BoundaryRef, RecordRefs: buildRequest.RecordRefs, BuilderRef: buildRequest.BuilderRef, BuilderVersion: buildRequest.BuilderVersion, IndexVersion: buildRequest.IndexVersion, ArtifactRef: cr("artifact"), Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, ObservedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckIndexerV1(context.Background(), now, indexer{observation: buildObservation}, buildRequest); err != nil {
		t.Fatal(err)
	}

	input, err := consolidation.SealInputV1(consolidation.InputV1{Ref: contract.Ref{ID: "consolidation-input", Revision: 1}, TenantID: "tenant", ScopeRef: cr("scope"), PolicyRef: cr("policy"), TimelineStartRef: cr("timeline-start"), TimelineEndRef: cr("timeline-end"), Facts: []consolidation.InputFactV1{{Kind: consolidation.InputOutcome, FactRef: cr("outcome"), SettlementRef: cr("settlement"), EvidenceRefs: []contract.Ref{cr("evidence")}}}, RuleRef: cr("rule"), InputDigest: "sha256:input", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := consolidation.SealBatchV1(consolidation.BatchV1{Ref: contract.Ref{ID: "batch", Revision: 1}, InputRef: input.Ref, JobAttemptRef: cr("job"), Proposals: []consolidation.ProposalV1{{ID: "proposal", Subject: "alpha", Scope: "private", ContentRef: contract.ContentRef{ID: "content", Digest: "sha256:content", Length: 5, MediaType: "text/plain"}, SourceRefs: []contract.Ref{cr("source")}, EvidenceRefs: []contract.Ref{cr("evidence")}, Sensitivity: "internal", FutureUse: "similar deployment", Verifiable: true, Decision: consolidation.DecisionReviewRequired, Reason: "needs owner review"}}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckConsolidatorV1(context.Background(), now, consolidator{batch: batch}, input); err != nil {
		t.Fatal(err)
	}

	policyRequest, err := contract.SealAdmissionPolicyRequestV1(contract.AdmissionPolicyRequestV1{Owner: contract.OwnerMemory, TenantID: "tenant", CandidateRef: cr("candidate"), AuthorityRef: cr("authority"), PolicyRef: cr("policy"), ScopeRef: cr("scope"), Purpose: "assist", Sensitivity: "internal", RiskFlags: []string{"shared_scope"}, RequestedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	advice, err := contract.SealAdmissionAdviceV1(contract.AdmissionAdviceV1{Ref: contract.Ref{ID: "advice", Revision: 1}, RequestDigest: policyRequest.Digest, PolicyAdapterRef: cr("policy-adapter"), Decision: contract.AdviceReview, ReasonCodes: []string{"shared_scope_requires_review"}, ObservedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckAdmissionPolicyAdapterV1(context.Background(), now, admissionPolicy{advice: advice}, policyRequest); err != nil {
		t.Fatal(err)
	}

	acquire, err := knowledge.SealAcquireRequestV1(knowledge.AcquireRequestV1{Ref: contract.Ref{ID: "acquire", Revision: 1}, TenantID: "tenant", SourceKind: knowledge.SourceAPI, SourceSubjectRef: cr("source-subject"), ConnectorRef: cr("connector"), AuthorityRef: cr("authority"), PolicyRef: cr("policy"), ScopeRef: cr("scope"), BudgetRef: cr("budget"), OperationRef: cr("operation"), AttemptRef: cr("attempt"), PermitRef: cr("permit"), PrepareEnforcementRef: cr("prepare-enforcement"), ExecuteEnforcementRef: cr("execute-enforcement"), RequestedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := knowledge.SealAcquireObservationV1(knowledge.AcquireObservationV1{Ref: contract.Ref{ID: "acquired", Revision: 1}, RequestRef: acquire.Ref, AttemptRef: acquire.AttemptRef, ProviderReceiptRef: cr("receipt"), AssetRef: cr("asset"), ContentDigest: "sha256:content", SourceVersion: "v1", ProvenanceRefs: []contract.Ref{cr("provenance")}, License: "internal-use", Scope: "project", Sensitivity: "internal", ObservedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	acquireInspection, err := knowledge.SealAcquireInspectionV1(knowledge.AcquireInspectionV1{Ref: contract.Ref{ID: "acquire-inspection", Revision: 1}, RequestRef: acquire.Ref, AttemptRef: acquire.AttemptRef, Outcome: knowledge.AcquireObserved, ObservationRef: acquired.Ref, InspectedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckSourceConnectorV1(context.Background(), now, sourceConnector{observation: acquired, inspection: acquireInspection}, acquire); err != nil {
		t.Fatal(err)
	}
}

func TestExtensionConformanceRejectsCancellationAndBindingViolations(t *testing.T) {
	now := time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)
	request, err := retrieval.SealHybridRequestV1(retrieval.HybridRequestV1{Query: contract.RetrievalQuery{ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: cr("view"), Purpose: "assist", Text: "alpha", Limit: 1}, Channels: []retrieval.ChannelBudgetV1{{Kind: contract.IndexLexical, Limit: 1, Weight: 1}}, RRFK: 60, MaxCandidates: 1})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := retrieval.SealChannelObservationV1(retrieval.ChannelObservationV1{Kind: contract.IndexLexical, ProjectionRef: cr("projection"), ViewRef: request.Query.ViewRef, WatermarkRef: cr("watermark"), Coverage: contract.Coverage{Status: contract.CoverageNone}, ObservedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckChannelRetrieverV1(context.Background(), now, retriever{observation: observation, ignoreCancel: true}, request, request.Channels[0]); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("provider ignoring cancellation accepted: %v", err)
	}
	drift := observation
	drift.ViewRef = cr("other-view")
	drift, err = retrieval.SealChannelObservationV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	if err := conformance.CheckChannelRetrieverV1(context.Background(), now, retriever{observation: drift}, request, request.Channels[0]); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("view drift accepted: %v", err)
	}
}
