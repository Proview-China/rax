package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationScopeEvidenceGatewayV3 coordinates current facts. Facts owns every
// mutation; the gateway neither calls a Provider nor commits Domain facts or
// Operation settlement.
type OperationScopeEvidenceGatewayV3 struct {
	Facts                   ports.OperationScopeEvidenceFactPortV3
	Runtime                 ports.OperationScopeEvidenceRuntimeCurrentReaderV3
	Generation              ports.OperationScopeEvidenceGenerationCurrentReaderV3
	Applicability           ports.OperationScopeEvidenceApplicabilityCurrentReaderV3
	ActionApplicability     ports.OperationScopeEvidenceActionApplicabilityCurrentRouterV3
	MCPConnectApplicability ports.OperationScopeEvidenceMCPConnectApplicabilityCurrentRouterV1
	Clock                   func() time.Time
}

func (g OperationScopeEvidenceGatewayV3) IssueOperationScopeEvidenceV3(ctx context.Context, request ports.IssueOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	if err := g.validate(); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	if existing, err := g.Facts.InspectOperationScopeEvidenceQualificationV3(ctx, request.QualificationID); err == nil {
		if err := existing.Validate(); err != nil {
			return ports.OperationScopeEvidenceQualificationFactV3{}, err
		}
		if qualificationMatchesIssue(existing, request) {
			return existing, nil
		}
		return ports.OperationScopeEvidenceQualificationFactV3{}, evidenceConflict("qualification id changed immutable Issue content")
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	now, err := g.now()
	if err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	inputs, err := g.inspectCurrentInputs(ctx, request.Scope, request.PermitID, request.EvidencePolicy, request.Reservation.Registration, now)
	if err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	if inputs.runtime.PermitFactRevision != request.PermitFactRevision || inputs.runtime.PermitDigest != request.PermitDigest || inputs.runtime.AdmissionDigest != request.AdmissionDigest || inputs.runtime.Authorization != request.Authorization || inputs.runtime.Phase != request.PhaseRef || inputs.source.SourceEpoch != request.Reservation.Source.SourceEpoch || inputs.source.NextSequence != request.Reservation.Source.SourceSequence || inputs.source.Policy != request.EvidencePolicy || request.Reservation.Schema != inputs.policy.ExpectedSchema {
		return ports.OperationScopeEvidenceQualificationFactV3{}, staleEvidence("Issue expected stale Runtime, source, policy or phase facts")
	}
	expires := minUnixNano(now.Add(request.RequestedTTL).UnixNano(), inputs.runtime.ExpiresUnixNano, inputs.generation.ExpiresUnixNano, inputs.policy.ExpiresUnixNano, inputs.appPolicy.ExpiresUnixNano, inputs.source.ExpiresUnixNano, inputs.applicabilityExpires)
	policyTTL := now.Add(inputs.policy.MaximumQualificationTTL).UnixNano()
	if policyTTL < expires {
		expires = policyTTL
	}
	if expires <= now.UnixNano() {
		return ports.OperationScopeEvidenceQualificationFactV3{}, staleEvidence("qualification has no current TTL")
	}
	ingestNotAfter := expires + inputs.policy.MaximumIngestGrace.Nanoseconds()
	if inputs.policy.ExpiresUnixNano < ingestNotAfter {
		ingestNotAfter = inputs.policy.ExpiresUnixNano
	}
	if inputs.source.ExpiresUnixNano < ingestNotAfter {
		ingestNotAfter = inputs.source.ExpiresUnixNano
	}
	fact, err := ports.SealOperationScopeEvidenceQualificationFactV3(ports.OperationScopeEvidenceQualificationFactV3{
		ID: request.QualificationID, Revision: 1, State: ports.OperationScopeEvidenceIssuedV3, Scope: request.Scope, Runtime: inputs.runtime,
		EvidencePolicy: request.EvidencePolicy, Reservation: request.Reservation, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires, IngestNotAfterUnixNano: ingestNotAfter,
		RequestedTTL: request.RequestedTTL,
	})
	if err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	created, err := g.Facts.CreateOperationScopeEvidenceQualificationV3(ctx, fact)
	if err == nil {
		if created.Digest != fact.Digest {
			return ports.OperationScopeEvidenceQualificationFactV3{}, evidenceConflict("Fact Owner returned another qualification")
		}
		return created, nil
	}
	if !recoverableEvidence(err) {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	recovered, inspectErr := g.Facts.InspectOperationScopeEvidenceQualificationV3(context.WithoutCancel(ctx), fact.ID)
	if inspectErr != nil || recovered.Digest != fact.Digest {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	return recovered, nil
}

func (g OperationScopeEvidenceGatewayV3) InspectOperationScopeEvidenceV3(ctx context.Context, request ports.InspectOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	if g.Facts == nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, missingComponent("Operation Evidence Fact Owner is required")
	}
	if err := request.Validate(); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	fact, err := g.Facts.InspectOperationScopeEvidenceQualificationV3(ctx, request.QualificationID)
	if err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	return fact, fact.Validate()
}

func (g OperationScopeEvidenceGatewayV3) InspectCurrentOperationScopeEvidenceV3(ctx context.Context, request ports.InspectCurrentOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	if err := g.validate(); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	now, err := g.now()
	if err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	fact, err := g.Facts.InspectOperationScopeEvidenceQualificationV3(ctx, request.Qualification.ID)
	if err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	if fact.RefV3() != request.Qualification || fact.State != ports.OperationScopeEvidenceIssuedV3 || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.OperationScopeEvidenceQualificationFactV3{}, staleEvidence("qualification is not current")
	}
	inputs, err := g.inspectCurrentInputs(ctx, fact.Scope, fact.Runtime.PermitID, fact.EvidencePolicy, fact.Reservation.Registration, now)
	if err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	if inputs.runtime.Digest != fact.Runtime.Digest || inputs.source.SourceEpoch != fact.Reservation.Source.SourceEpoch || inputs.source.NextSequence != fact.Reservation.Source.SourceSequence || inputs.source.Policy != fact.EvidencePolicy {
		return ports.OperationScopeEvidenceQualificationFactV3{}, staleEvidence("qualification current facts drifted")
	}
	return fact, nil
}

func (g OperationScopeEvidenceGatewayV3) HandoffOperationScopeEvidenceV3(ctx context.Context, request ports.HandoffOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	if err := g.validate(); err != nil {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	if current, err := g.Facts.InspectOperationScopeEvidenceProviderHandoffV3(ctx, request.HandoffID); err == nil {
		if current.Qualification == request.Qualification {
			return current, nil
		}
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, evidenceConflict("handoff id changed qualification")
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	qualification, err := g.InspectCurrentOperationScopeEvidenceV3(ctx, ports.InspectCurrentOperationScopeEvidenceRequestV3{Qualification: request.Qualification})
	if err != nil {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	now, err := g.now()
	if err != nil {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	fact, err := ports.SealOperationScopeEvidenceProviderHandoffFactV3(ports.OperationScopeEvidenceProviderHandoffFactV3{ID: request.HandoffID, Revision: 1, Qualification: qualification.RefV3(), Phase: qualification.Runtime.Phase, CheckedUnixNano: now.UnixNano(), NotAfterUnixNano: qualification.ExpiresUnixNano})
	if err != nil {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	created, err := g.Facts.CreateOperationScopeEvidenceProviderHandoffV3(ctx, fact)
	if err == nil {
		if created.Digest != fact.Digest {
			return ports.OperationScopeEvidenceProviderHandoffFactV3{}, evidenceConflict("Fact Owner returned another handoff")
		}
		return created, nil
	}
	if !recoverableEvidence(err) {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	recovered, inspectErr := g.Facts.InspectOperationScopeEvidenceProviderHandoffV3(context.WithoutCancel(ctx), fact.ID)
	if inspectErr != nil || recovered.Digest != fact.Digest {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	return recovered, nil
}

func (g OperationScopeEvidenceGatewayV3) ConsumeOperationScopeEvidenceV3(ctx context.Context, request ports.ConsumeOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceConsumeResultV3, error) {
	if err := g.validate(); err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	if existing, err := g.Facts.InspectOperationScopeEvidenceConsumptionV3(ctx, request.ConsumptionID); err == nil {
		return g.recoverConsume(ctx, existing, request)
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	now, err := g.now()
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	qualification, err := g.Facts.InspectOperationScopeEvidenceQualificationV3(ctx, request.Candidate.Qualification.ID)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	if err := qualification.Validate(); err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	if qualification.State != ports.OperationScopeEvidenceIssuedV3 {
		current, inspectErr := g.Facts.InspectOperationScopeEvidenceConsumptionV3(context.WithoutCancel(ctx), request.ConsumptionID)
		if inspectErr != nil {
			return ports.OperationScopeEvidenceConsumeResultV3{}, staleEvidence("qualification was already consumed or invalidated")
		}
		return g.recoverConsume(context.WithoutCancel(ctx), current, request)
	}
	handoff, err := g.Facts.InspectOperationScopeEvidenceProviderHandoffV3(ctx, request.Handoff.ID)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	if err := handoff.Validate(); err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	if qualification.RefV3() != request.Candidate.Qualification || handoff.RefV3() != request.Handoff || handoff.Qualification != qualification.RefV3() || handoff.Phase != qualification.Runtime.Phase || request.Candidate.Source != qualification.Reservation.Source || request.Candidate.EventID != qualification.Reservation.EventID {
		return ports.OperationScopeEvidenceConsumeResultV3{}, evidenceConflict("consume candidate changed qualification or handoff")
	}
	policy, err := g.Facts.InspectOperationScopeEvidencePolicyV3(ctx, qualification.EvidencePolicy.ID)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	source, err := g.Facts.InspectOperationScopeEvidenceSourceV3(ctx, qualification.Reservation.Source.RegistrationID)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	if policy.RefV3() != qualification.EvidencePolicy || policy.State != ports.OperationScopeEvidencePolicyActiveV3 || source.SourceEpoch != qualification.Reservation.Source.SourceEpoch || source.Policy != qualification.EvidencePolicy || now.UnixNano() >= source.ExpiresUnixNano || now.UnixNano() >= policy.ExpiresUnixNano || request.Candidate.Payload.Schema != policy.ExpectedSchema || request.Candidate.Payload.Length > policy.MaximumPayloadBytes {
		return g.recoverConcurrentConsume(ctx, request, staleEvidence("consume policy, source or payload drifted"))
	}
	late := !now.Before(time.Unix(0, qualification.ExpiresUnixNano))
	if late {
		if !now.Before(time.Unix(0, qualification.IngestNotAfterUnixNano)) {
			return ports.OperationScopeEvidenceConsumeResultV3{}, staleEvidence("late observation crossed bounded ingest TTL")
		}
	} else {
		if _, err := g.InspectCurrentOperationScopeEvidenceV3(ctx, ports.InspectCurrentOperationScopeEvidenceRequestV3{Qualification: qualification.RefV3()}); err != nil {
			return g.recoverConcurrentConsume(ctx, request, err)
		}
	}
	result, err := g.Facts.ConsumeOperationScopeEvidenceV3(ctx, ports.OperationScopeEvidenceAtomicConsumeRequestV3{ExpectedQualificationRevision: qualification.Revision, ExpectedSourceRevision: source.Revision, ConsumptionID: request.ConsumptionID, Handoff: request.Handoff, Candidate: request.Candidate, LateObservation: late, ConsumedUnixNano: now.UnixNano()})
	if err == nil {
		return validateConsumeResult(result, request)
	}
	if !recoverableEvidence(err) {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	current, inspectErr := g.Facts.InspectOperationScopeEvidenceConsumptionV3(context.WithoutCancel(ctx), request.ConsumptionID)
	if inspectErr != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	recovered, recoverErr := g.recoverConsume(context.WithoutCancel(ctx), current, request)
	if recoverErr != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	return recovered, nil
}

func (g OperationScopeEvidenceGatewayV3) recoverConcurrentConsume(ctx context.Context, request ports.ConsumeOperationScopeEvidenceRequestV3, original error) (ports.OperationScopeEvidenceConsumeResultV3, error) {
	current, err := g.Facts.InspectOperationScopeEvidenceConsumptionV3(context.WithoutCancel(ctx), request.ConsumptionID)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, original
	}
	result, err := g.recoverConsume(context.WithoutCancel(ctx), current, request)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, original
	}
	return result, nil
}

type operationScopeEvidenceInputs struct {
	runtime              ports.OperationScopeEvidenceRuntimeCurrentProjectionV3
	generation           ports.OperationScopeEvidenceFactRefV3
	policy               ports.OperationScopeEvidencePolicyFactV3
	appPolicy            ports.OperationScopeEvidenceApplicabilityPolicyFactV3
	source               ports.OperationScopeEvidenceSourceRegistrationFactV3
	applicabilityExpires int64
}

func (g OperationScopeEvidenceGatewayV3) inspectCurrentInputs(ctx context.Context, scope ports.OperationScopeEvidenceScopeV3, permitID string, policyRef ports.OperationScopeEvidencePolicyRefV3, expectedSource ports.OperationScopeEvidenceFactRefV3, now time.Time) (operationScopeEvidenceInputs, error) {
	action := scope.Operation.Kind == ports.OperationScopeRunV3 && scope.EffectKind == ports.OperationScopeEvidenceActionEffectKindV3
	mcpConnect := scope.Operation.Kind == ports.OperationScopeRunV3 && scope.EffectKind == ports.OperationScopeEvidenceMCPConnectEffectKindV1
	if scope.Operation.Kind == ports.OperationScopeActivationV3 {
		if !preRunApplicability(scope.Applicability) {
			return operationScopeEvidenceInputs{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "activation first slice forbids Run, Session, Turn, Action and Context applicability")
		}
	} else if action {
		if err := ports.ValidateOperationScopeEvidenceActionApplicabilityV3(scope.Applicability); err != nil {
			return operationScopeEvidenceInputs{}, err
		}
		if g.ActionApplicability == nil {
			return operationScopeEvidenceInputs{}, missingComponent("Action Evidence applicability Router is required")
		}
	} else if mcpConnect {
		if err := ports.ValidateOperationScopeEvidenceMCPConnectApplicabilityV1(scope.Applicability); err != nil {
			return operationScopeEvidenceInputs{}, err
		}
		if g.MCPConnectApplicability == nil {
			return operationScopeEvidenceInputs{}, missingComponent("MCP Connect Evidence applicability Router is required")
		}
	} else {
		return operationScopeEvidenceInputs{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "Operation Evidence operation/effect matrix is unsupported")
	}
	runtime, err := g.Runtime.InspectOperationScopeEvidenceRuntimeCurrentV3(ctx, scope, permitID)
	if err != nil {
		return operationScopeEvidenceInputs{}, err
	}
	if err := runtime.Validate(now); err != nil {
		return operationScopeEvidenceInputs{}, err
	}
	if !sameOperationScopeEvidenceScope(runtime.Scope, scope) {
		return operationScopeEvidenceInputs{}, staleEvidence("Runtime current projection binds another Operation Evidence scope")
	}
	generation, err := g.Generation.InspectOperationScopeEvidenceGenerationCurrentV3(ctx, scope.Generation)
	if err != nil {
		return operationScopeEvidenceInputs{}, err
	}
	if generation.ID != scope.Generation.ID || generation.Revision != scope.Generation.Revision || generation.Digest != scope.Generation.Digest || !now.Before(time.Unix(0, generation.ExpiresUnixNano)) {
		return operationScopeEvidenceInputs{}, staleEvidence("generation association drifted")
	}
	policy, err := g.Facts.InspectOperationScopeEvidencePolicyV3(ctx, policyRef.ID)
	if err != nil {
		return operationScopeEvidenceInputs{}, err
	}
	if err := policy.Validate(); err != nil {
		return operationScopeEvidenceInputs{}, err
	}
	appPolicy, err := g.Facts.InspectOperationScopeEvidenceApplicabilityPolicyV3(ctx, scope.ApplicabilityPolicy.ID)
	if err != nil {
		return operationScopeEvidenceInputs{}, err
	}
	if err := appPolicy.Validate(); err != nil {
		return operationScopeEvidenceInputs{}, err
	}
	source, err := g.Facts.InspectOperationScopeEvidenceSourceV3(ctx, expectedSource.ID)
	if err != nil {
		return operationScopeEvidenceInputs{}, err
	}
	if err := source.Validate(); err != nil {
		return operationScopeEvidenceInputs{}, err
	}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope.Operation.ExecutionScope)
	if policy.RefV3() != policyRef || appPolicy.RefV3() != scope.ApplicabilityPolicy || sourceRef(source) != expectedSource || policy.State != ports.OperationScopeEvidencePolicyActiveV3 || appPolicy.State != ports.OperationScopeEvidencePolicyActiveV3 || source.State != ports.EvidenceSourceActive || policy.OperationKind != scope.Operation.Kind || appPolicy.OperationKind != scope.Operation.Kind || policy.EffectKind != scope.EffectKind || appPolicy.EffectKind != scope.EffectKind || appPolicy.ExecutionScopeDigest != scopeDigest || !sameApplicability(appPolicy.Applicability, scope.Applicability) || !now.Before(time.Unix(0, policy.ExpiresUnixNano)) || !now.Before(time.Unix(0, appPolicy.ExpiresUnixNano)) || !now.Before(time.Unix(0, source.ExpiresUnixNano)) {
		return operationScopeEvidenceInputs{}, staleEvidence("policy, applicability or source currentness drifted")
	}
	applicabilityExpires := appPolicy.ExpiresUnixNano
	if action {
		for _, value := range ports.NormalizeOperationScopeEvidenceApplicabilityV3(scope.Applicability) {
			projection, err := g.ActionApplicability.InspectOperationScopeEvidenceActionApplicabilityCurrentV3(ctx, value.Dimension, *value.Fact, scopeDigest)
			if err != nil {
				return operationScopeEvidenceInputs{}, err
			}
			if projection.ExpiresUnixNano < applicabilityExpires {
				applicabilityExpires = projection.ExpiresUnixNano
			}
		}
	} else if mcpConnect {
		for _, value := range ports.NormalizeOperationScopeEvidenceApplicabilityV3(scope.Applicability) {
			if value.Mode != ports.OperationScopeEvidenceRequiredV3 {
				continue
			}
			projection, err := g.MCPConnectApplicability.InspectOperationScopeEvidenceMCPConnectApplicabilityCurrentV1(ctx, value.Dimension, *value.Fact, scopeDigest)
			if err != nil {
				return operationScopeEvidenceInputs{}, err
			}
			if projection.ExpiresUnixNano < applicabilityExpires {
				applicabilityExpires = projection.ExpiresUnixNano
			}
		}
	}
	return operationScopeEvidenceInputs{runtime: runtime, generation: generation, policy: policy, appPolicy: appPolicy, source: source, applicabilityExpires: applicabilityExpires}, nil
}
func (g OperationScopeEvidenceGatewayV3) recoverConsume(ctx context.Context, fact ports.OperationScopeEvidenceConsumptionFactV3, request ports.ConsumeOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceConsumeResultV3, error) {
	digest, _ := request.Candidate.DigestV3()
	if fact.CandidateDigest != digest || fact.Handoff != request.Handoff {
		return ports.OperationScopeEvidenceConsumeResultV3{}, evidenceConflict("consumption id changed candidate")
	}
	record, err := g.Facts.InspectOperationScopeEvidenceRecordV3(ctx, fact.Record)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	qualification, err := g.Facts.InspectOperationScopeEvidenceQualificationV3(ctx, fact.Qualification.ID)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	source, err := g.Facts.InspectOperationScopeEvidenceSourceV3(ctx, request.Candidate.Source.RegistrationID)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	return validateConsumeResult(ports.OperationScopeEvidenceConsumeResultV3{Qualification: qualification, Consumption: fact, Record: record, Source: source}, request)
}
func validateConsumeResult(r ports.OperationScopeEvidenceConsumeResultV3, request ports.ConsumeOperationScopeEvidenceRequestV3) (ports.OperationScopeEvidenceConsumeResultV3, error) {
	d, _ := request.Candidate.DigestV3()
	if r.Consumption.CandidateDigest != d || r.Consumption.Handoff != request.Handoff || r.Record.CandidateDigest != d || r.Qualification.Consumption == nil || *r.Qualification.Consumption != r.Consumption.RefV3() {
		return ports.OperationScopeEvidenceConsumeResultV3{}, evidenceConflict("Fact Owner returned another atomic consume result")
	}
	return r, nil
}
func (g OperationScopeEvidenceGatewayV3) validate() error {
	if g.Facts == nil || g.Runtime == nil || g.Generation == nil || g.Clock == nil {
		return missingComponent("Operation Evidence Fact Owner, Runtime, Generation and clock are required")
	}
	return nil
}
func (g OperationScopeEvidenceGatewayV3) now() (time.Time, error) {
	n := g.Clock()
	if n.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Operation Evidence clock returned zero")
	}
	return n, nil
}
func qualificationMatchesIssue(f ports.OperationScopeEvidenceQualificationFactV3, r ports.IssueOperationScopeEvidenceRequestV3) bool {
	return f.ID == r.QualificationID && sameOperationScopeEvidenceScope(f.Scope, r.Scope) && f.EvidencePolicy == r.EvidencePolicy && f.Reservation == r.Reservation && f.RequestedTTL == r.RequestedTTL && f.Runtime.PermitID == r.PermitID && f.Runtime.PermitFactRevision == r.PermitFactRevision && f.Runtime.PermitDigest == r.PermitDigest && f.Runtime.AdmissionDigest == r.AdmissionDigest && f.Runtime.Authorization == r.Authorization && f.Runtime.Phase == r.PhaseRef
}
func preRunApplicability(a []ports.OperationScopeEvidenceApplicabilityV3) bool {
	if len(a) != 5 {
		return false
	}
	for _, v := range a {
		if v.Mode != ports.OperationScopeEvidenceForbiddenV3 || v.Fact != nil {
			return false
		}
	}
	return true
}
func sameApplicability(a, b []ports.OperationScopeEvidenceApplicabilityV3) bool {
	aa := ports.NormalizeOperationScopeEvidenceApplicabilityV3(a)
	bb := ports.NormalizeOperationScopeEvidenceApplicabilityV3(b)
	if len(aa) != len(bb) {
		return false
	}
	for i := range aa {
		if aa[i].Dimension != bb[i].Dimension || aa[i].Mode != bb[i].Mode {
			return false
		}
		if (aa[i].Fact == nil) != (bb[i].Fact == nil) {
			return false
		}
		if aa[i].Fact != nil && *aa[i].Fact != *bb[i].Fact {
			return false
		}
	}
	return true
}
func sameOperationScopeEvidenceScope(a, b ports.OperationScopeEvidenceScopeV3) bool {
	a.Applicability = ports.NormalizeOperationScopeEvidenceApplicabilityV3(a.Applicability)
	b.Applicability = ports.NormalizeOperationScopeEvidenceApplicabilityV3(b.Applicability)
	left, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", ports.OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceScopeV3", a)
	right, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", ports.OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceScopeV3", b)
	return leftErr == nil && rightErr == nil && left == right
}
func sourceRef(f ports.OperationScopeEvidenceSourceRegistrationFactV3) ports.OperationScopeEvidenceFactRefV3 {
	return ports.OperationScopeEvidenceFactRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest, ExpiresUnixNano: f.ExpiresUnixNano}
}
func minUnixNano(v int64, rest ...int64) int64 {
	m := v
	for _, x := range rest {
		if x < m {
			m = x
		}
	}
	return m
}
func recoverableEvidence(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorConflict)
}
func evidenceConflict(m string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, m)
}
func staleEvidence(m string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, m)
}
func missingComponent(m string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, m)
}
