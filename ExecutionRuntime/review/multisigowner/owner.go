// Package multisigowner owns Human V2 quorum decisions. It consumes an
// injected external-current cut; it does not mint or cache Policy, Identity,
// Authority, Delegation, Responsibility, Binding, Scope, or Evidence facts.
package multisigowner

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type Clock func() time.Time

const ExternalCurrentCutContractV2 = "praxis.review.human-multisig-external-cut/v2"
const lostReplyRecoveryTimeoutV2 = 5 * time.Second

// ExternalCurrentProofV2 is a Review-owned receipt over one completed external
// S1/S2 read cut. It does not replace any external Owner fact.
type ExternalCurrentProofV2 struct {
	ContractVersion string        `json:"contract_version"`
	TenantID        core.TenantID `json:"tenant_id"`
	SubjectDigest   core.Digest   `json:"subject_digest"`
	CheckedUnixNano int64         `json:"checked_unix_nano"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
	Digest          core.Digest   `json:"digest"`
}

func (p ExternalCurrentProofV2) digestValue() ExternalCurrentProofV2 { p.Digest = ""; return p }
func SealExternalCurrentProofV2(p ExternalCurrentProofV2) (ExternalCurrentProofV2, error) {
	p.ContractVersion = ExternalCurrentCutContractV2
	p.Digest = ""
	d, err := core.CanonicalJSONDigest("praxis.review.human-multisig", ExternalCurrentCutContractV2, "ExternalCurrentProofV2", p)
	if err != nil {
		return ExternalCurrentProofV2{}, err
	}
	p.Digest = d
	return p, p.Validate(p.TenantID, p.SubjectDigest, time.Unix(0, p.CheckedUnixNano))
}
func (p ExternalCurrentProofV2) Validate(tenant core.TenantID, subject core.Digest, now time.Time) error {
	if p.ContractVersion != ExternalCurrentCutContractV2 || p.TenantID != tenant || p.SubjectDigest != subject || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human multisign external current cut is stale or drifted")
	}
	d, err := core.CanonicalJSONDigest("praxis.review.human-multisig", ExternalCurrentCutContractV2, "ExternalCurrentProofV2", p.digestValue())
	if err != nil {
		return err
	}
	if d != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "human multisign external current cut digest drifted")
	}
	return nil
}

// ExternalCurrentCutV2 must implement the public Owner readers and their S1 ->
// exact reread -> S2 protocol. Review only invokes it and never interprets a
// nominal ref as currentness.
type ExternalCurrentCutV2 interface {
	ValidatePanelCurrentV2(context.Context, contract.HumanReviewPanelV2, []contract.HumanPanelAssignmentV2, contract.HumanReviewPanelV2, time.Time) (ExternalCurrentProofV2, error)
	ValidateAttestationCurrentV2(context.Context, contract.HumanReviewPanelV2, contract.HumanPanelAssignmentV2, contract.HumanAttestationV2, time.Time) (ExternalCurrentProofV2, error)
	ValidateDecisionCurrentV2(context.Context, contract.HumanReviewPanelV2, contract.HumanQuorumDecisionV2, contract.HumanVerdictV2, time.Time) (ExternalCurrentProofV2, error)
}

type Owner struct {
	store    reviewport.StoreV2
	external ExternalCurrentCutV2
	clock    Clock
}

func New(store reviewport.StoreV2, external ExternalCurrentCutV2, clock Clock) (*Owner, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(external) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "human multisign Owner requires Store, external-current cut and clock")
	}
	return &Owner{store: store, external: external, clock: clock}, nil
}

func (o *Owner) OpenPanelV2(ctx context.Context, m reviewport.CreateHumanPanelMutationV2) (reviewport.CreateHumanPanelResultV2, error) {
	if err := reviewport.ValidateCreateHumanPanelTraceV2(m); err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	baseline := o.clock()
	if baseline.IsZero() {
		return reviewport.CreateHumanPanelResultV2{}, clockError()
	}
	identities := map[string]bool{}
	for _, a := range m.Assignments {
		k := string(a.ReviewerIdentity.TenantID) + "\x00" + a.ReviewerIdentity.Ref
		if identities[k] {
			return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "one human identity cannot hold multiple Panel assignments")
		}
		identities[k] = true
	}
	subject, err := subjectDigestV2("open", struct {
		Proposed    contract.HumanReviewPanelV2       `json:"proposed"`
		Assignments []contract.HumanPanelAssignmentV2 `json:"assignments"`
		Open        contract.HumanReviewPanelV2       `json:"open"`
	}{m.ProposedPanel, m.Assignments, m.OpenPanel})
	if err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	proof, err := o.external.ValidatePanelCurrentV2(ctx, m.ProposedPanel, m.Assignments, m.OpenPanel, baseline)
	if err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	if err := proof.Validate(m.OpenPanel.TenantID, subject, baseline); err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	wantExpiry := minExpires(proof.ExpiresUnixNano, m.OpenPanel.QuorumPolicy.ExpiresUnixNano, m.OpenPanel.CreatedUnixNano+m.OpenPanel.MaxPanelDurationNanos)
	for _, a := range m.Assignments {
		wantExpiry = minExpires(wantExpiry, a.ExpiresUnixNano)
	}
	if m.ProposedPanel.ExpiresUnixNano != wantExpiry || m.OpenPanel.ExpiresUnixNano != wantExpiry {
		return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human Panel expiry is not the exact current-input minimum")
	}
	now := o.clock()
	if now.IsZero() || now.Before(baseline) {
		return reviewport.CreateHumanPanelResultV2{}, clockError()
	}
	if err := proof.Validate(m.OpenPanel.TenantID, subject, now); err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	if err := m.OpenPanel.ValidateCurrent(m.OpenPanel.ExactRef(), now); err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	result, err := o.store.CreateHumanPanelV2(ctx, m)
	if err == nil || (!core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorUnavailable)) {
		return result, err
	}
	recovery, cancel, ok := boundedRecoveryContextV2(ctx, o.clock, now, panelRecoveryExpiryV2(m)...)
	if !ok {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	defer cancel()
	panel, inspectErr := o.store.InspectHumanPanelExactV2(recovery, m.OpenPanel.ExactRef())
	if inspectErr != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	assignments, inspectErr := o.store.ListHumanPanelAssignmentsV2(recovery, m.OpenPanel.ExactRef())
	if inspectErr != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	proposed, inspectErr := o.store.InspectHumanPanelExactV2(recovery, m.ProposedPanel.ExactRef())
	if inspectErr != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	trace, inspectErr := o.store.InspectTraceExactV1(recovery, m.Trace.TenantID, reviewport.ExactV1(m.Trace.ID, m.Trace.Revision, m.Trace.Digest))
	if inspectErr != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	if !reflect.DeepEqual(panel, m.OpenPanel) || !reflect.DeepEqual(proposed, m.ProposedPanel) || !samePanelAssignmentsV2(assignments, m.Assignments) || !reflect.DeepEqual(trace, m.Trace) || !recoveryStillCurrentV2(o.clock, now, panelRecoveryExpiryV2(m)...) {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	return reviewport.CreateHumanPanelResultV2{Panel: panel, Assignments: assignments}, nil
}

func (o *Owner) SubmitAttestationV2(ctx context.Context, m reviewport.RecordHumanAttestationMutationV2) (reviewport.RecordHumanAttestationResultV2, error) {
	if err := reviewport.ValidateRecordHumanAttestationTracesV2(m); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	baseline := o.clock()
	if baseline.IsZero() {
		return reviewport.RecordHumanAttestationResultV2{}, clockError()
	}
	panel, err := o.store.InspectHumanPanelExactV2(ctx, m.ExpectedPanel)
	if err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	assignment, err := o.store.InspectHumanPanelAssignmentExactV2(ctx, m.Attestation.Assignment)
	if err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	target, err := o.store.InspectTargetExactV1(ctx, m.Attestation.Target.TenantID, reviewport.ExactV1(m.Attestation.Target.ID, m.Attestation.Target.Revision, m.Attestation.Target.Digest))
	if err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	for _, condition := range m.Attestation.Conditions {
		if condition.ScopeDigest != target.ActionScopeDigest || condition.ExpiresUnixNano <= baseline.UnixNano() {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "human condition Scope or TTL drifted")
		}
	}
	if err := panel.ValidateCurrent(m.ExpectedPanel, baseline); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if err := assignment.ValidateCurrent(m.Attestation.Assignment, baseline); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	subject, err := subjectDigestV2("attest", struct {
		Panel       contract.HumanReviewPanelV2     `json:"panel"`
		Assignment  contract.HumanPanelAssignmentV2 `json:"assignment"`
		Attestation contract.HumanAttestationV2     `json:"attestation"`
	}{panel, assignment, m.Attestation})
	if err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	proof, err := o.external.ValidateAttestationCurrentV2(ctx, panel, assignment, m.Attestation, baseline)
	if err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if err := proof.Validate(panel.TenantID, subject, baseline); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	existing, err := o.store.ListHumanAttestationsByPanelV2(ctx, m.ExpectedPanel)
	if err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	all := append(existing, m.Attestation.Clone())
	for _, old := range existing {
		if old.ReviewerIdentity.TenantID == m.Attestation.ReviewerIdentity.TenantID && old.ReviewerIdentity.Ref == m.Attestation.ReviewerIdentity.Ref {
			if old.ID == m.Attestation.ID && old.Revision == m.Attestation.Revision && old.Digest == m.Attestation.Digest {
				continue
			}
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "one human identity cannot cast multiple Panel votes")
		}
	}
	assignments, err := o.store.ListHumanPanelAssignmentsV2(ctx, panel.ExactRef())
	if err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	expectedState, terminal := evaluate(panel, assignments, all)
	if terminal != (m.Quorum != nil) || m.NextPanel.State != expectedState {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum candidate does not match K-of-N/veto/waiting result")
	}
	if m.Quorum != nil {
		if err := validateQuorum(panel, assignments, all, *m.Quorum); err != nil {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
	}
	voteExpiry := minExpires(panel.ExpiresUnixNano, panel.QuorumPolicy.ExpiresUnixNano, assignment.ExpiresUnixNano, proof.ExpiresUnixNano, m.Attestation.ObservedUnixNano+panel.MaxVoteTTLNanos)
	for _, condition := range m.Attestation.Conditions {
		voteExpiry = minExpires(voteExpiry, condition.ExpiresUnixNano)
	}
	if assignment.LeaseExpiresUnixNano > 0 {
		voteExpiry = minExpires(voteExpiry, assignment.LeaseExpiresUnixNano)
	}
	if m.Attestation.ExpiresUnixNano != voteExpiry {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human Attestation expiry is not the exact current-input minimum")
	}
	if m.Quorum != nil {
		qExpiry := minExpires(panel.ExpiresUnixNano, panel.QuorumPolicy.ExpiresUnixNano, proof.ExpiresUnixNano)
		for _, as := range assignments {
			qExpiry = minExpires(qExpiry, as.ExpiresUnixNano)
			if as.LeaseExpiresUnixNano > 0 {
				qExpiry = minExpires(qExpiry, as.LeaseExpiresUnixNano)
			}
		}
		for _, a := range all {
			qExpiry = minExpires(qExpiry, a.ExpiresUnixNano)
			for _, condition := range a.Conditions {
				qExpiry = minExpires(qExpiry, condition.ExpiresUnixNano)
			}
		}
		if m.Quorum.ExpiresUnixNano != qExpiry {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human Quorum expiry is not the exact counted-input minimum")
		}
	}
	now := o.clock()
	if now.IsZero() || now.Before(baseline) {
		return reviewport.RecordHumanAttestationResultV2{}, clockError()
	}
	if err := proof.Validate(panel.TenantID, subject, now); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if err := panel.ValidateCurrent(m.ExpectedPanel, now); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if err := assignment.ValidateCurrent(m.Attestation.Assignment, now); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if err := m.Attestation.ValidateCurrent(m.Attestation.ExactRef(), now); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	result, err := o.store.RecordHumanAttestationV2(ctx, m)
	if err == nil || (!core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorUnavailable)) {
		return result, err
	}
	recovery, cancel, ok := boundedRecoveryContextV2(ctx, o.clock, now, attestationRecoveryExpiryV2(m)...)
	if !ok {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	defer cancel()
	att, inspectErr := o.store.InspectHumanAttestationExactV2(recovery, m.Attestation.ExactRef())
	if inspectErr != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	next, inspectErr := o.store.InspectHumanPanelExactV2(recovery, m.NextPanel.ExactRef())
	if inspectErr != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	out := reviewport.RecordHumanAttestationResultV2{Panel: next, Attestation: att}
	if m.Quorum != nil {
		q, e := o.store.InspectHumanQuorumDecisionExactV2(recovery, m.Quorum.ExactRef())
		if e != nil {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
		if !reflect.DeepEqual(q, *m.Quorum) {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
		out.Quorum = &q
	}
	if m.NextCase != nil {
		c, e := o.store.InspectCaseExactV1(recovery, m.NextCase.TenantID, reviewport.ExactV1(m.NextCase.ID, m.NextCase.Revision, m.NextCase.Digest))
		if e != nil {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
		if !reflect.DeepEqual(c, *m.NextCase) {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
		out.Case = &c
	}
	for _, event := range append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...) {
		stored, e := o.store.InspectTraceExactV1(recovery, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest))
		if e != nil || !reflect.DeepEqual(stored, event) {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
	}
	if !reflect.DeepEqual(att, m.Attestation) || !reflect.DeepEqual(next, m.NextPanel) || !recoveryStillCurrentV2(o.clock, now, attestationRecoveryExpiryV2(m)...) {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	return out, nil
}

// DecideV2 is for the Review Owner worker. It is intentionally not exposed by
// service.HumanMultiSignServiceV2.
func (o *Owner) DecideV2(ctx context.Context, m reviewport.DecideHumanPanelMutationV2) (reviewport.DecideHumanPanelResultV2, error) {
	if err := reviewport.ValidateDecideHumanPanelTracesV2(m); err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	baseline := o.clock()
	if baseline.IsZero() {
		return reviewport.DecideHumanPanelResultV2{}, clockError()
	}
	p, err := o.store.InspectHumanPanelExactV2(ctx, m.ExpectedPanel)
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	q, err := o.store.InspectHumanQuorumDecisionExactV2(ctx, m.Quorum)
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	subject, err := subjectDigestV2("decide", struct {
		Panel   contract.HumanReviewPanelV2    `json:"panel"`
		Quorum  contract.HumanQuorumDecisionV2 `json:"quorum"`
		Verdict contract.HumanVerdictV2        `json:"verdict"`
	}{p, q, m.Verdict})
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	proof, err := o.external.ValidateDecisionCurrentV2(ctx, p, q, m.Verdict, baseline)
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	if err := proof.Validate(p.TenantID, subject, baseline); err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	c, err := o.store.InspectCaseExactV1(ctx, m.ExpectedCase.TenantID, reviewport.ExactV1(m.ExpectedCase.ID, m.ExpectedCase.Revision, m.ExpectedCase.Digest))
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	target, err := o.store.InspectTargetExactV1(ctx, m.Verdict.Target.TenantID, reviewport.ExactV1(m.Verdict.Target.ID, m.Verdict.Target.Revision, m.Verdict.Target.Digest))
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	round, err := o.store.InspectRoundExactV1(ctx, m.Verdict.Round.TenantID, reviewport.ExactV1(m.Verdict.Round.ID, m.Verdict.Round.Revision, m.Verdict.Round.Digest))
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	assignments, err := o.store.ListHumanPanelAssignmentsV2(ctx, p.ExactRef())
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	atts, err := o.store.ListHumanAttestationsByPanelV2(ctx, p.ExactRef())
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	wantExpiry := minExpires(proof.ExpiresUnixNano, p.ExpiresUnixNano, p.QuorumPolicy.ExpiresUnixNano, q.ExpiresUnixNano, c.ExpiresUnixNano, target.ExpiresUnixNano, round.ExpiresUnixNano)
	for _, a := range assignments {
		wantExpiry = minExpires(wantExpiry, a.ExpiresUnixNano)
		if a.LeaseExpiresUnixNano > 0 {
			wantExpiry = minExpires(wantExpiry, a.LeaseExpiresUnixNano)
		}
	}
	byAtt := map[string]contract.HumanAttestationV2{}
	for _, a := range atts {
		byAtt[a.ID] = a
	}
	for _, ref := range m.Verdict.AttestationRefs {
		a, ok := byAtt[ref.ID]
		if !ok || a.Revision != ref.Revision || a.Digest != ref.Digest {
			return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Verdict Attestation exact set drifted")
		}
		wantExpiry = minExpires(wantExpiry, a.ExpiresUnixNano)
		for _, condition := range a.Conditions {
			wantExpiry = minExpires(wantExpiry, condition.ExpiresUnixNano)
		}
	}
	if m.Verdict.ConditionsDigest != q.ConditionsDigest || !reflect.DeepEqual(m.Verdict.Conditions, q.Conditions) {
		return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "human Verdict condition set differs from Quorum")
	}
	if m.Verdict.ExpiresUnixNano != wantExpiry {
		return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human Verdict expiry is not the exact all-input minimum")
	}
	now := o.clock()
	if now.IsZero() || now.Before(baseline) {
		return reviewport.DecideHumanPanelResultV2{}, clockError()
	}
	if err := proof.Validate(p.TenantID, subject, now); err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	if err := p.ValidateCurrent(m.ExpectedPanel, now); err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	if err := q.ValidateCurrent(m.Quorum, now); err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	result, err := o.store.DecideHumanPanelV2(ctx, m)
	if err == nil || (!core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorUnavailable)) {
		return result, err
	}
	recovery, cancel, ok := boundedRecoveryContextV2(ctx, o.clock, now, m.Verdict.ExpiresUnixNano, m.NextPanel.ExpiresUnixNano, m.NextCase.ExpiresUnixNano)
	if !ok {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	defer cancel()
	v, inspectErr := o.store.InspectHumanVerdictExactV2(recovery, m.Verdict.ExactRef())
	if inspectErr != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	next, inspectErr := o.store.InspectHumanPanelExactV2(recovery, m.NextPanel.ExactRef())
	if inspectErr != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	c, inspectErr = o.store.InspectCaseExactV1(recovery, m.NextCase.TenantID, reviewport.ExactV1(m.NextCase.ID, m.NextCase.Revision, m.NextCase.Digest))
	if inspectErr != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	for _, event := range append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...) {
		stored, inspectErr := o.store.InspectTraceExactV1(recovery, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest))
		if inspectErr != nil || !reflect.DeepEqual(stored, event) {
			return reviewport.DecideHumanPanelResultV2{}, err
		}
	}
	if !reflect.DeepEqual(v, m.Verdict) || !reflect.DeepEqual(next, m.NextPanel) || !reflect.DeepEqual(c, m.NextCase) || !recoveryStillCurrentV2(o.clock, now, m.Verdict.ExpiresUnixNano, m.NextPanel.ExpiresUnixNano, m.NextCase.ExpiresUnixNano) {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	return reviewport.DecideHumanPanelResultV2{Panel: next, Case: c, Verdict: v}, nil
}

func (o *Owner) BeginDecisionV2(ctx context.Context, m reviewport.BeginHumanPanelDecisionMutationV2) (contract.HumanReviewPanelV2, contract.ReviewCaseV1, error) {
	baseline := o.clock()
	if baseline.IsZero() {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, clockError()
	}
	p, err := o.store.InspectHumanPanelExactV2(ctx, m.ExpectedPanel)
	if err != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	q, err := o.store.InspectHumanQuorumDecisionCurrentByPanelIDV2(ctx, p.TenantID, p.ID)
	if err != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	if err := q.ValidateCurrent(q.ExactRef(), baseline); err != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	if err := reviewport.ValidateBeginHumanPanelDecisionTraceV2(m, q.ID); err != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	now := o.clock()
	if now.IsZero() || now.Before(baseline) {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, clockError()
	}
	panel, c, err := o.store.BeginHumanPanelDecisionV2(ctx, m)
	if err == nil || (!core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorUnavailable)) {
		return panel, c, err
	}
	recovery, cancel, ok := boundedRecoveryContextV2(ctx, o.clock, now, m.NextPanel.ExpiresUnixNano, m.NextCase.ExpiresUnixNano, q.ExpiresUnixNano)
	if !ok {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	defer cancel()
	panel, inspectErr := o.store.InspectHumanPanelExactV2(recovery, m.NextPanel.ExactRef())
	if inspectErr != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	c, inspectErr = o.store.InspectCaseExactV1(recovery, m.NextCase.TenantID, reviewport.ExactV1(m.NextCase.ID, m.NextCase.Revision, m.NextCase.Digest))
	if inspectErr != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	trace, inspectErr := o.store.InspectTraceExactV1(recovery, m.Trace.TenantID, reviewport.ExactV1(m.Trace.ID, m.Trace.Revision, m.Trace.Digest))
	if inspectErr != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	if !reflect.DeepEqual(panel, m.NextPanel) || !reflect.DeepEqual(c, m.NextCase) || !reflect.DeepEqual(trace, m.Trace) || !recoveryStillCurrentV2(o.clock, now, m.NextPanel.ExpiresUnixNano, m.NextCase.ExpiresUnixNano, q.ExpiresUnixNano) {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	return panel, c, nil
}

func panelRecoveryExpiryV2(m reviewport.CreateHumanPanelMutationV2) []int64 {
	values := []int64{m.ProposedPanel.ExpiresUnixNano, m.OpenPanel.ExpiresUnixNano, m.OpenPanel.QuorumPolicy.ExpiresUnixNano}
	for _, assignment := range m.Assignments {
		values = append(values, assignment.ExpiresUnixNano)
		if assignment.LeaseExpiresUnixNano > 0 {
			values = append(values, assignment.LeaseExpiresUnixNano)
		}
	}
	return values
}

func attestationRecoveryExpiryV2(m reviewport.RecordHumanAttestationMutationV2) []int64 {
	values := []int64{m.Attestation.ExpiresUnixNano, m.NextPanel.ExpiresUnixNano}
	if m.Quorum != nil {
		values = append(values, m.Quorum.ExpiresUnixNano)
	}
	if m.NextCase != nil {
		values = append(values, m.NextCase.ExpiresUnixNano)
	}
	for _, condition := range m.Attestation.Conditions {
		values = append(values, condition.ExpiresUnixNano)
	}
	return values
}

func boundedRecoveryContextV2(parent context.Context, clock Clock, baseline time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	now := clock()
	if now.IsZero() || now.Before(baseline) {
		return nil, nil, false
	}
	remaining := lostReplyRecoveryTimeoutV2
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		if expiry <= now.UnixNano() {
			return nil, nil, false
		}
		if d := time.Duration(expiry - now.UnixNano()); d < remaining {
			remaining = d
		}
	}
	if remaining <= 0 {
		return nil, nil, false
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(parent), remaining)
	return recovery, cancel, true
}

func recoveryStillCurrentV2(clock Clock, baseline time.Time, expiries ...int64) bool {
	now := clock()
	if now.IsZero() || now.Before(baseline) {
		return false
	}
	for _, expiry := range expiries {
		if expiry > 0 && now.UnixNano() >= expiry {
			return false
		}
	}
	return true
}

func samePanelAssignmentsV2(got, want []contract.HumanPanelAssignmentV2) bool {
	if len(got) != len(want) {
		return false
	}
	byRef := make(map[contract.HumanPanelAssignmentExactRefV2]contract.HumanPanelAssignmentV2, len(want))
	for _, assignment := range want {
		byRef[assignment.ExactRef()] = assignment
	}
	for _, assignment := range got {
		expected, ok := byRef[assignment.ExactRef()]
		if !ok || !reflect.DeepEqual(assignment, expected) {
			return false
		}
		delete(byRef, assignment.ExactRef())
	}
	return len(byRef) == 0
}

func evaluate(panel contract.HumanReviewPanelV2, assignments []contract.HumanPanelAssignmentV2, atts []contract.HumanAttestationV2) (contract.HumanPanelStateV2, bool) {
	byID := map[string]contract.HumanPanelAssignmentV2{}
	for _, a := range assignments {
		byID[a.ID] = a
	}
	accept := uint32(0)
	roles := map[string]uint32{}
	for _, a := range atts {
		as := byID[a.Assignment.ID]
		switch a.Resolution {
		case contract.ResolutionRejectV1:
			if as.CanVeto && intersects(as.Roles, panel.RejectVetoRoles) {
				return contract.HumanPanelVetoedV2, true
			}
		case contract.ResolutionRequestChangesV1:
			return contract.HumanPanelWaitingRevisionV2, true
		case contract.ResolutionInsufficientEvidenceV1:
			return contract.HumanPanelWaitingEvidenceV2, true
		case contract.ResolutionEscalateHumanV1:
			return contract.HumanPanelWaitingHigherAuthorityV2, true
		case contract.ResolutionAcceptV1, contract.ResolutionConditionalV1:
			accept++
			for _, r := range as.Roles {
				roles[r]++
			}
		}
	}
	if accept >= panel.AcceptThreshold {
		for _, r := range panel.RoleRequirements {
			if roles[r.Role] < r.Minimum {
				return contract.HumanPanelOpenV2, false
			}
		}
		return contract.HumanPanelQuorumSatisfiedV2, true
	}
	return contract.HumanPanelOpenV2, false
}

func validateQuorum(panel contract.HumanReviewPanelV2, assignments []contract.HumanPanelAssignmentV2, atts []contract.HumanAttestationV2, q contract.HumanQuorumDecisionV2) error {
	byAssign := map[string]contract.HumanPanelAssignmentV2{}
	byAtt := map[string]contract.HumanAttestationV2{}
	for _, a := range assignments {
		byAssign[a.ID] = a
	}
	for _, a := range atts {
		if old, ok := byAtt[a.ID]; ok && old.Digest != a.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human quorum duplicate Attestation ID drifted")
		}
		byAtt[a.ID] = a
	}
	if q.Threshold != panel.AcceptThreshold || q.Policy != panel.QuorumPolicy {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum threshold/policy drifted")
	}
	refs := append(append([]contract.HumanAttestationExactRefV2{}, q.AcceptedAttestationRefs...), q.OtherAttestationRefs...)
	if len(refs) != len(byAtt) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum does not audit every Panel vote")
	}
	identities := make(map[string]bool)
	roles := map[string]uint32{}
	accept := uint32(0)
	conditional := false
	var veto *contract.HumanAttestationV2
	for _, r := range refs {
		a, ok := byAtt[r.ID]
		if !ok || a.Revision != r.Revision || a.Digest != r.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum Attestation exact set drifted")
		}
		identity := string(a.ReviewerIdentity.TenantID) + "\x00" + a.ReviewerIdentity.Ref
		if identities[identity] {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human quorum counted one identity more than once")
		}
		identities[identity] = true
		as, ok := byAssign[a.Assignment.ID]
		if !ok {
			return core.NewError(core.ErrorConflict, core.ReasonStaleLeaseRevision, "human quorum Assignment is absent")
		}
		if a.Resolution == contract.ResolutionAcceptV1 || a.Resolution == contract.ResolutionConditionalV1 {
			accept++
			if a.Resolution == contract.ResolutionConditionalV1 {
				conditional = true
			}
			for _, role := range as.Roles {
				roles[role]++
			}
		}
		if a.Resolution == contract.ResolutionRejectV1 && as.CanVeto && intersects(as.Roles, panel.RejectVetoRoles) {
			x := a
			veto = &x
		}
	}
	if q.AcceptCount != accept {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum AcceptCount drifted")
	}
	for _, x := range q.SatisfiedRoleCounts {
		if roles[x.Role] != x.DistinctCurrentCount {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum role count drifted")
		}
	}
	state, _ := evaluate(panel, assignments, atts)
	switch state {
	case contract.HumanPanelVetoedV2:
		if veto == nil || !q.Vetoed || q.VetoAttestationRef == nil || q.VetoAttestationRef.ID != veto.ID {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum veto proof drifted")
		}
	case contract.HumanPanelQuorumSatisfiedV2:
		want := contract.ResolutionAcceptV1
		if conditional {
			want = contract.ResolutionConditionalV1
		}
		if q.Resolution != want {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum accepting resolution drifted")
		}
	case contract.HumanPanelWaitingRevisionV2:
		if q.Resolution != contract.ResolutionRequestChangesV1 {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum request-changes resolution drifted")
		}
	case contract.HumanPanelWaitingEvidenceV2:
		if q.Resolution != contract.ResolutionInsufficientEvidenceV1 {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum insufficient-evidence resolution drifted")
		}
	case contract.HumanPanelWaitingHigherAuthorityV2:
		if q.Resolution != contract.ResolutionEscalateHumanV1 {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human quorum escalation resolution drifted")
		}
	}
	conditions, digest, err := contract.CanonicalAcceptedConditionsV2(atts, q.AcceptedAttestationRefs)
	if err != nil {
		return err
	}
	if digest != q.ConditionsDigest || !reflect.DeepEqual(conditions, q.Conditions) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "human quorum condition union drifted")
	}
	return nil
}

func intersects(a, b []string) bool {
	i, j := 0, 0
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i < len(aa) && j < len(bb) {
		if aa[i] == bb[j] {
			return true
		}
		if aa[i] < bb[j] {
			i++
		} else {
			j++
		}
	}
	return false
}
func clockError() error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "human multisign Owner clock regressed or is unavailable")
}

func subjectDigestV2(kind string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.review.human-multisig", ExternalCurrentCutContractV2, "ExternalCurrentSubjectV2/"+kind, value)
}
func PanelCurrentSubjectDigestV2(proposed contract.HumanReviewPanelV2, assignments []contract.HumanPanelAssignmentV2, open contract.HumanReviewPanelV2) (core.Digest, error) {
	return subjectDigestV2("open", struct {
		Proposed    contract.HumanReviewPanelV2       `json:"proposed"`
		Assignments []contract.HumanPanelAssignmentV2 `json:"assignments"`
		Open        contract.HumanReviewPanelV2       `json:"open"`
	}{proposed, assignments, open})
}
func AttestationCurrentSubjectDigestV2(panel contract.HumanReviewPanelV2, assignment contract.HumanPanelAssignmentV2, attestation contract.HumanAttestationV2) (core.Digest, error) {
	return subjectDigestV2("attest", struct {
		Panel       contract.HumanReviewPanelV2     `json:"panel"`
		Assignment  contract.HumanPanelAssignmentV2 `json:"assignment"`
		Attestation contract.HumanAttestationV2     `json:"attestation"`
	}{panel, assignment, attestation})
}
func DecisionCurrentSubjectDigestV2(panel contract.HumanReviewPanelV2, quorum contract.HumanQuorumDecisionV2, verdict contract.HumanVerdictV2) (core.Digest, error) {
	return subjectDigestV2("decide", struct {
		Panel   contract.HumanReviewPanelV2    `json:"panel"`
		Quorum  contract.HumanQuorumDecisionV2 `json:"quorum"`
		Verdict contract.HumanVerdictV2        `json:"verdict"`
	}{panel, quorum, verdict})
}
func minExpires(v ...int64) int64 {
	var out int64
	for _, x := range v {
		if x > 0 && (out == 0 || x < out) {
			out = x
		}
	}
	return out
}
