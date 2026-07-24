package kernel

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BindingAdmissionGatewayV1 is the governed Binding V2 admission coordinator.
// Persistence and currentness remain owned by injected Ports.
type BindingAdmissionGatewayV1 struct {
	facts    control.BindingFactPortV2
	attempts control.BindingAdmissionAttemptFactPortV1
	inputs   ports.BindingAdmissionInputCurrentReaderV1
	now      func() time.Time
	clockMu  sync.Mutex
	last     time.Time
}

func NewBindingAdmissionGatewayV1(facts control.BindingFactPortV2, attempts control.BindingAdmissionAttemptFactPortV1, inputs ports.BindingAdmissionInputCurrentReaderV1, now func() time.Time) (*BindingAdmissionGatewayV1, error) {
	if bindingAdmissionNilV1(facts) || bindingAdmissionNilV1(attempts) || bindingAdmissionNilV1(inputs) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Binding admission Gateway dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &BindingAdmissionGatewayV1{facts: facts, attempts: attempts, inputs: inputs, now: now}, nil
}

func bindingAdmissionNilV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (g *BindingAdmissionGatewayV1) freshNowV1() (time.Time, error) {
	g.clockMu.Lock()
	defer g.clockMu.Unlock()
	now := g.now()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Binding admission clock returned zero")
	}
	if !g.last.IsZero() && now.Before(g.last) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Binding admission clock regressed")
	}
	g.last = now
	return now, nil
}

func (g *BindingAdmissionGatewayV1) StartOrInspectBindingAdmissionV1(ctx context.Context, request ports.BindingAdmissionRequestV1) (ports.BindingAdmissionResultV1, error) {
	if g == nil || bindingAdmissionNilV1(g.facts) || bindingAdmissionNilV1(g.attempts) {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Binding admission Gateway is unavailable")
	}
	if ctx == nil {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context is required")
	}
	now, err := g.freshNowV1()
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	attempt, err := g.attempts.InspectBindingAdmissionAttemptV1(ctx, request.AttemptID)
	if err == nil {
		return g.resumeBindingAdmissionV1(ctx, attempt, request)
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.BindingAdmissionResultV1{}, err
	}
	inputs, err := g.readBindingAdmissionInputsV1(ctx, request, now)
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	intent, err := buildBindingAdmissionIntentV1(request, inputs, now)
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	created, writeErr := g.attempts.CreateBindingAdmissionAttemptV1(ctx, intent)
	if writeErr != nil {
		actual, inspectErr := g.attempts.InspectBindingAdmissionAttemptV1(context.WithoutCancel(ctx), request.AttemptID)
		if inspectErr != nil {
			return ports.BindingAdmissionResultV1{}, writeErr
		}
		if actual.Request.RequestDigest != request.RequestDigest {
			return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Binding admission Attempt binds another request")
		}
		return g.resumeBindingAdmissionV1(ctx, actual, request)
	}
	if created.Digest != intent.Digest {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission Attempt create returned another fact")
	}
	return g.executeOwnedBindingAdmissionV1(ctx, created)
}

func (g *BindingAdmissionGatewayV1) InspectBindingAdmissionV1(ctx context.Context, request ports.BindingAdmissionInspectRequestV1) (ports.BindingAdmissionResultV1, error) {
	if g == nil || bindingAdmissionNilV1(g.attempts) {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Binding admission Gateway is unavailable")
	}
	if ctx == nil {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context is required")
	}
	if err := request.Validate(); err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	attempt, err := g.attempts.InspectBindingAdmissionAttemptV1(ctx, request.AttemptID)
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	if attempt.Request.RequestDigest != request.RequestDigest {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Binding admission Inspect request digest drifted")
	}
	if attempt.State != control.BindingAdmissionResultRecordedV1 || attempt.Result == nil {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Binding admission Attempt has no settled result")
	}
	return *attempt.Result, nil
}

func (g *BindingAdmissionGatewayV1) readBindingAdmissionInputsV1(ctx context.Context, r ports.BindingAdmissionRequestV1, now time.Time) (control.BindingAdmissionInputSnapshotV1, error) {
	definition, err := g.inputs.InspectBindingAdmissionDefinitionCurrentV1(ctx, r.DefinitionCurrent)
	if err != nil || definition != r.DefinitionCurrent {
		return control.BindingAdmissionInputSnapshotV1{}, bindingAdmissionExactErrV1(err, "Definition")
	}
	plan, err := g.inputs.InspectBindingAdmissionPlanCurrentV1(ctx, r.PlanCurrent)
	if err != nil || plan.Ref != r.PlanCurrent {
		return control.BindingAdmissionInputSnapshotV1{}, bindingAdmissionExactErrV1(err, "Plan")
	}
	assembly, err := g.inputs.InspectBindingAdmissionAssemblyCurrentV1(ctx, r.AssemblyCurrent)
	if err != nil || assembly.Ref != r.AssemblyCurrent {
		return control.BindingAdmissionInputSnapshotV1{}, bindingAdmissionExactErrV1(err, "Assembly")
	}
	catalog, err := g.inputs.InspectBindingAdmissionCatalogCurrentV1(ctx, r.CatalogCurrent)
	if err != nil || catalog.Ref != r.CatalogCurrent {
		return control.BindingAdmissionInputSnapshotV1{}, bindingAdmissionExactErrV1(err, "Catalog")
	}
	resolution, err := g.inputs.InspectBindingAdmissionResolutionCurrentV1(ctx, r.ResolutionCurrent)
	if err != nil || resolution != r.ResolutionCurrent {
		return control.BindingAdmissionInputSnapshotV1{}, bindingAdmissionExactErrV1(err, "Resolution")
	}
	releases := make([]ports.BindingAdmissionReleaseCurrentV1, 0, len(r.Releases))
	for _, expected := range r.Releases {
		release, inspectErr := g.inputs.InspectBindingAdmissionReleaseCurrentV1(ctx, expected)
		if inspectErr != nil || release.Expected != expected {
			return control.BindingAdmissionInputSnapshotV1{}, bindingAdmissionExactErrV1(inspectErr, "Release")
		}
		releases = append(releases, release)
	}
	resources, err := g.inputs.InspectBindingAdmissionResourceBindingSetCurrentV1(ctx, r.ResourceBindingSet)
	if err != nil {
		return control.BindingAdmissionInputSnapshotV1{}, err
	}
	authority, err := g.inputs.InspectBindingAdmissionAuthorityCurrentV1(ctx, r.AuthorityCurrent)
	if err != nil || authority != r.AuthorityCurrent {
		return control.BindingAdmissionInputSnapshotV1{}, bindingAdmissionExactErrV1(err, "Authority")
	}
	policy, err := g.inputs.InspectBindingAdmissionPolicyCurrentV1(ctx, r.PolicyCurrent)
	if err != nil || policy != r.PolicyCurrent {
		return control.BindingAdmissionInputSnapshotV1{}, bindingAdmissionExactErrV1(err, "Policy")
	}
	for _, ref := range []ports.OwnerCurrentRefV1{definition, resolution, authority, policy} {
		if !now.Before(time.Unix(0, ref.ExpiresUnixNano)) {
			return control.BindingAdmissionInputSnapshotV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Binding admission owner current expired")
		}
	}
	for _, window := range [][2]int64{{plan.CheckedUnixNano, plan.ExpiresUnixNano}, {assembly.CheckedUnixNano, assembly.ExpiresUnixNano}, {catalog.CheckedUnixNano, catalog.ExpiresUnixNano}} {
		if err := ports.ValidateBindingAdmissionProjectionCurrentV1(window[0], window[1], now); err != nil {
			return control.BindingAdmissionInputSnapshotV1{}, err
		}
	}
	for _, release := range releases {
		if err := ports.ValidateBindingAdmissionProjectionCurrentV1(release.CheckedUnixNano, release.ExpiresUnixNano, now); err != nil {
			return control.BindingAdmissionInputSnapshotV1{}, err
		}
	}
	if err := resources.ValidateCurrent(r.ResourceBindingSet, now); err != nil {
		return control.BindingAdmissionInputSnapshotV1{}, err
	}
	return control.SealBindingAdmissionInputSnapshotV1(control.BindingAdmissionInputSnapshotV1{Definition: definition, Plan: plan, Assembly: assembly, Catalog: catalog, Resolution: resolution, Releases: releases, Resources: resources, Authority: authority, Policy: policy})
}

func bindingAdmissionExactErrV1(err error, subject string) error {
	if err != nil {
		return err
	}
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding admission "+subject+" current drifted")
}

func buildBindingAdmissionIntentV1(r ports.BindingAdmissionRequestV1, inputs control.BindingAdmissionInputSnapshotV1, now time.Time) (control.BindingAdmissionAttemptFactV1, error) {
	if err := inputs.ValidateAgainstRequestV1(r); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	catalogDigest, err := inputs.Catalog.Catalog.DigestV2()
	if err != nil || catalogDigest != inputs.Plan.Plan.GovernanceDigest {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Binding admission Plan and Catalog governance drifted")
	}
	manifestByComponent := make(map[ports.ComponentIDV2]ports.ComponentManifestV2, len(inputs.Assembly.Manifests))
	for _, manifest := range inputs.Assembly.Manifests {
		manifestByComponent[manifest.ComponentID] = manifest
	}
	declared := make([]control.BindingFactV2, 0, len(inputs.Releases))
	probed := make([]control.BindingFactV2, 0, len(inputs.Releases))
	certified := make([]control.BindingFactV2, 0, len(inputs.Releases))
	for _, release := range inputs.Releases {
		manifest, exists := manifestByComponent[release.Expected.ComponentID]
		if !exists {
			return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Binding admission Release has no Assembly manifest")
		}
		manifestDigest, digestErr := manifest.BindingDigestV2()
		if digestErr != nil || manifestDigest != release.ManifestDigest {
			return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding admission Release and Assembly manifest drifted")
		}
		if err := ports.ValidateManifestAgainstCatalogV2(manifest, inputs.Catalog.Catalog); err != nil {
			return control.BindingAdmissionAttemptFactV1{}, err
		}
		idDigest, digestErr := core.CanonicalJSONDigest("praxis.runtime.binding-admission-binding-id", ports.BindingAdmissionContractVersionV1, "BindingAdmissionBindingIDV1", struct {
			RequestDigest core.Digest         `json:"request_digest"`
			ComponentID   ports.ComponentIDV2 `json:"component_id"`
		}{r.RequestDigest, manifest.ComponentID})
		if digestErr != nil {
			return control.BindingAdmissionAttemptFactV1{}, digestErr
		}
		id := "binding/" + string(idDigest)
		d := control.BindingFactV2{ID: id, ComponentID: manifest.ComponentID, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: inputs.Plan.Plan.GovernanceDigest, State: control.BindingDeclared, Revision: 1, Grants: []ports.CapabilityGrantV2{}, RenewalEvidence: []ports.EvidenceRecordRefV2{}}
		if err := d.Validate(); err != nil {
			return control.BindingAdmissionAttemptFactV1{}, err
		}
		p := d
		p.State = control.BindingProbed
		p.Revision = 2
		p.Grants = append([]ports.CapabilityGrantV2{}, release.Grants...)
		p.ProbedUnixNano = now.UnixNano()
		p.ExpiresUnixNano = minimumGrantExpiryV1(p.Grants)
		if err := control.ValidateBindingFactTransitionV2(d, p, now); err != nil {
			return control.BindingAdmissionAttemptFactV1{}, err
		}
		c := p
		c.State = control.BindingCertified
		c.Revision = 3
		c.CertifiedUnixNano = now.UnixNano()
		c.ConformanceEvidenceDigest = release.ConformanceEvidenceDigest
		if err := control.ValidateBindingFactTransitionV2(p, c, now); err != nil {
			return control.BindingAdmissionAttemptFactV1{}, err
		}
		declared, probed, certified = append(declared, d), append(probed, p), append(certified, c)
	}
	set, err := control.BuildBindingSetV2(r.ExpectedBindingSetID, inputs.Plan.Plan, inputs.Catalog.Catalog, certified, now)
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if r.RequestedNotAfterUnixNano < set.ExpiresUnixNano {
		set.ExpiresUnixNano = r.RequestedNotAfterUnixNano
	}
	if err := set.Validate(); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	committed := set
	committed.Members = append([]control.BindingMemberV2{}, set.Members...)
	for i := range committed.Members {
		committed.Members[i].BindingRevision++
	}
	if err := committed.Validate(); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	return control.SealBindingAdmissionAttemptFactV1(control.BindingAdmissionAttemptFactV1{AttemptID: r.AttemptID, Revision: 1, Request: r, Inputs: inputs, DeclaredCandidates: declared, ProbedCandidates: probed, CertifiedCandidates: certified, BindingSetCandidate: set, CommittedBindingSetCandidate: committed, State: control.BindingAdmissionIntentRecordedV1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
}

func minimumGrantExpiryV1(grants []ports.CapabilityGrantV2) int64 {
	minimum := int64(^uint64(0) >> 1)
	for _, grant := range grants {
		if grant.ExpiresUnixNano < minimum {
			minimum = grant.ExpiresUnixNano
		}
	}
	return minimum
}

func (g *BindingAdmissionGatewayV1) executeOwnedBindingAdmissionV1(ctx context.Context, attempt control.BindingAdmissionAttemptFactV1) (ports.BindingAdmissionResultV1, error) {
	for i := range attempt.DeclaredCandidates {
		if err := g.writeBindingLifecycleV1(ctx, attempt.DeclaredCandidates[i], attempt.ProbedCandidates[i], attempt.CertifiedCandidates[i]); err != nil {
			_, _ = g.advanceBindingAdmissionAttemptV1(context.WithoutCancel(ctx), attempt, control.BindingAdmissionOutcomeUnknownV1, nil)
			return ports.BindingAdmissionResultV1{}, err
		}
	}
	now, err := g.freshNowV1()
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	s2, err := g.readBindingAdmissionInputsV1(ctx, attempt.Request, now)
	if err != nil || s2.SnapshotDigest != attempt.Inputs.SnapshotDigest {
		_, _ = g.advanceBindingAdmissionAttemptV1(context.WithoutCancel(ctx), attempt, control.BindingAdmissionOutcomeUnknownV1, nil)
		return ports.BindingAdmissionResultV1{}, bindingAdmissionExactErrV1(err, "S2 input snapshot")
	}
	expected := make([]control.ExpectedBindingRevisionV2, 0, len(attempt.BindingSetCandidate.Members))
	for _, member := range attempt.BindingSetCandidate.Members {
		expected = append(expected, control.ExpectedBindingRevisionV2{BindingID: member.BindingID, ExpectedRevision: member.BindingRevision})
	}
	set, writeErr := g.facts.CommitBindingSet(ctx, control.CommitBindingSetRequestV2{Set: attempt.BindingSetCandidate, Expected: expected})
	if writeErr != nil {
		set, _ = g.facts.InspectBindingSet(context.WithoutCancel(ctx), attempt.BindingSetCandidate.ID)
	}
	if !sameBindingSetContentV1(set, attempt.CommittedBindingSetCandidate) {
		_, _ = g.advanceBindingAdmissionAttemptV1(context.WithoutCancel(ctx), attempt, control.BindingAdmissionOutcomeUnknownV1, nil)
		if writeErr != nil {
			return ports.BindingAdmissionResultV1{}, writeErr
		}
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "BindingSet commit outcome is unknown")
	}
	latest, err := g.attempts.InspectBindingAdmissionAttemptV1(context.WithoutCancel(ctx), attempt.AttemptID)
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	return g.recordBindingAdmissionResultV1(ctx, latest)
}

func (g *BindingAdmissionGatewayV1) writeBindingLifecycleV1(ctx context.Context, declared, probed, certified control.BindingFactV2) error {
	actual, err := g.facts.CreateBinding(ctx, declared)
	if err != nil {
		actual, _ = g.facts.InspectBinding(context.WithoutCancel(ctx), declared.ID)
	}
	if !sameBindingFactContentV1(actual, declared) {
		if err != nil {
			return err
		}
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "declared Binding Fact drifted")
	}
	actual, err = g.facts.CompareAndSwapBinding(ctx, control.BindingFactCASRequestV2{ExpectedRevision: declared.Revision, Next: probed})
	if err != nil {
		actual, _ = g.facts.InspectBinding(context.WithoutCancel(ctx), probed.ID)
	}
	if !sameBindingFactContentV1(actual, probed) {
		if err != nil {
			return err
		}
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "probed Binding Fact drifted")
	}
	actual, err = g.facts.CompareAndSwapBinding(ctx, control.BindingFactCASRequestV2{ExpectedRevision: probed.Revision, Next: certified})
	if err != nil {
		actual, _ = g.facts.InspectBinding(context.WithoutCancel(ctx), certified.ID)
	}
	if !sameBindingFactContentV1(actual, certified) {
		if err != nil {
			return err
		}
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "certified Binding Fact drifted")
	}
	return nil
}

func sameBindingFactContentV1(left, right control.BindingFactV2) bool {
	l, lerr := control.BindingFactContentDigestV2(left)
	r, rerr := control.BindingFactContentDigestV2(right)
	return lerr == nil && rerr == nil && l == r
}

func sameBindingSetContentV1(left, right control.BindingSetFactV2) bool {
	l, lerr := control.BindingSetFactContentDigestV2(left)
	r, rerr := control.BindingSetFactContentDigestV2(right)
	return lerr == nil && rerr == nil && l == r
}

func (g *BindingAdmissionGatewayV1) resumeBindingAdmissionV1(ctx context.Context, attempt control.BindingAdmissionAttemptFactV1, request ports.BindingAdmissionRequestV1) (ports.BindingAdmissionResultV1, error) {
	if err := attempt.Validate(); err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	if attempt.Request.RequestDigest != request.RequestDigest {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Binding admission Attempt binds another request")
	}
	if attempt.State == control.BindingAdmissionResultRecordedV1 && attempt.Result != nil {
		return *attempt.Result, nil
	}
	set, setErr := g.facts.InspectBindingSet(ctx, attempt.BindingSetCandidate.ID)
	if setErr == nil && sameBindingSetContentV1(set, attempt.CommittedBindingSetCandidate) {
		return g.recordBindingAdmissionResultV1(ctx, attempt)
	}
	if setErr == nil {
		return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "persisted BindingSet drifted from admission candidate")
	}
	if !core.HasCategory(setErr, core.ErrorNotFound) {
		return ports.BindingAdmissionResultV1{}, setErr
	}
	if attempt.State == control.BindingAdmissionIntentRecordedV1 {
		var err error
		attempt, err = g.advanceBindingAdmissionAttemptV1(ctx, attempt, control.BindingAdmissionOutcomeUnknownV1, nil)
		if err != nil {
			return ports.BindingAdmissionResultV1{}, err
		}
	}
	if attempt.State == control.BindingAdmissionOutcomeUnknownV1 {
		var err error
		attempt, err = g.advanceBindingAdmissionAttemptV1(ctx, attempt, control.BindingAdmissionReconciliationRequiredV1, nil)
		if err != nil {
			return ports.BindingAdmissionResultV1{}, err
		}
	}
	return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Binding admission requires reconciliation and cannot redispatch")
}

func (g *BindingAdmissionGatewayV1) recordBindingAdmissionResultV1(ctx context.Context, attempt control.BindingAdmissionAttemptFactV1) (ports.BindingAdmissionResultV1, error) {
	if attempt.State == control.BindingAdmissionResultRecordedV1 && attempt.Result != nil {
		return *attempt.Result, nil
	}
	now, err := g.freshNowV1()
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	bindings := make([]ports.BindingAdmissionBindingRefV1, 0, len(attempt.CommittedBindingSetCandidate.Members))
	for _, member := range attempt.CommittedBindingSetCandidate.Members {
		fact, inspectErr := g.facts.InspectBinding(ctx, member.BindingID)
		if inspectErr != nil {
			return ports.BindingAdmissionResultV1{}, inspectErr
		}
		if fact.State != control.BindingBound || fact.BindingSetID != attempt.BindingSetCandidate.ID || fact.Revision != member.BindingRevision {
			return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "bound Binding Fact drifted from committed set")
		}
		digest, digestErr := control.BindingFactContentDigestV2(fact)
		if digestErr != nil {
			return ports.BindingAdmissionResultV1{}, digestErr
		}
		bindings = append(bindings, ports.BindingAdmissionBindingRefV1{ComponentID: fact.ComponentID, ID: fact.ID, Revision: fact.Revision, Digest: digest, ExpiresUnixNano: fact.ExpiresUnixNano})
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].ComponentID < bindings[j].ComponentID })
	setDigest, err := control.BindingSetFactContentDigestV2(attempt.CommittedBindingSetCandidate)
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	result, err := ports.SealBindingAdmissionResultV1(ports.BindingAdmissionResultV1{AttemptID: attempt.AttemptID, RequestDigest: attempt.Request.RequestDigest, BindingSet: ports.BindingAdmissionBindingSetRefV1{ID: attempt.CommittedBindingSetCandidate.ID, Revision: attempt.CommittedBindingSetCandidate.Revision, Digest: setDigest}, Bindings: bindings, ResourceBindingSet: attempt.Request.ResourceBindingSet, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: attempt.CommittedBindingSetCandidate.ExpiresUnixNano})
	if err != nil {
		return ports.BindingAdmissionResultV1{}, err
	}
	for range 4 {
		next, advanceErr := g.advanceBindingAdmissionAttemptV1(ctx, attempt, control.BindingAdmissionResultRecordedV1, &result)
		if advanceErr == nil && next.Result != nil {
			return *next.Result, nil
		}
		latest, inspectErr := g.attempts.InspectBindingAdmissionAttemptV1(context.WithoutCancel(ctx), attempt.AttemptID)
		if inspectErr != nil {
			return ports.BindingAdmissionResultV1{}, advanceErr
		}
		attempt = latest
		if attempt.State == control.BindingAdmissionResultRecordedV1 && attempt.Result != nil {
			return *attempt.Result, nil
		}
	}
	return ports.BindingAdmissionResultV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Binding admission result CAS remained contended")
}

func (g *BindingAdmissionGatewayV1) advanceBindingAdmissionAttemptV1(ctx context.Context, current control.BindingAdmissionAttemptFactV1, state control.BindingAdmissionAttemptStateV1, result *ports.BindingAdmissionResultV1) (control.BindingAdmissionAttemptFactV1, error) {
	now, err := g.freshNowV1()
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	next := current.CloneV1()
	next.Revision++
	next.State = state
	next.Result = result
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	next, err = control.SealBindingAdmissionAttemptFactV1(next)
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	written, writeErr := g.attempts.CompareAndSwapBindingAdmissionAttemptV1(ctx, control.BindingAdmissionAttemptCASRequestV1{ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if writeErr == nil {
		return written, nil
	}
	actual, inspectErr := g.attempts.InspectBindingAdmissionAttemptV1(context.WithoutCancel(ctx), current.AttemptID)
	if inspectErr == nil && actual.Digest == next.Digest {
		return actual, nil
	}
	if inspectErr == nil && actual.State == control.BindingAdmissionResultRecordedV1 && actual.Result != nil {
		return actual, nil
	}
	return control.BindingAdmissionAttemptFactV1{}, writeErr
}

var _ ports.BindingAdmissionGovernancePortV1 = (*BindingAdmissionGatewayV1)(nil)
