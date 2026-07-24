package owneradapter

import (
	"context"
	"sort"
	"time"

	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblypublication"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type AssemblyPublicationAdapterV2 struct {
	store   assemblypublication.OwnerStoreV2
	journal hostports.JournalFactPortV2
	owner   core.OwnerRef
	clock   func() time.Time
}

var _ hostports.AssemblyPublisherV2 = (*AssemblyPublicationAdapterV2)(nil)
var _ hostports.AssemblyPublicationInspectorV2 = (*AssemblyPublicationAdapterV2)(nil)

func NewAssemblyPublicationAdapterV2(store assemblypublication.OwnerStoreV2, journal hostports.JournalFactPortV2, owner core.OwnerRef, clock func() time.Time) (*AssemblyPublicationAdapterV2, error) {
	if hostcontract.IsTypedNilV1(store) || hostcontract.IsTypedNilV1(journal) || clock == nil {
		return nil, hostcontract.NewError(hostcontract.ErrorInvalidArgument, "assembly_publication_dependencies_missing", "Assembly publication store, Host Journal and clock are required")
	}
	if err := owner.Validate(); err != nil {
		return nil, ownerErrorV1(err, "assembly_publication_owner_invalid")
	}
	return &AssemblyPublicationAdapterV2{store: store, journal: journal, owner: owner, clock: clock}, nil
}

func (a *AssemblyPublicationAdapterV2) PublishAssemblyV2(ctx context.Context, request hostcontract.AssemblyPublicationRequestV2) (hostcontract.AssemblyPublicationResultV2, error) {
	if a == nil || hostcontract.IsTypedNilV1(a.store) || hostcontract.IsTypedNilV1(a.journal) || a.clock == nil {
		return hostcontract.AssemblyPublicationResultV2{}, hostcontract.NewError(hostcontract.ErrorUnavailable, "assembly_publication_adapter_unavailable", "Assembly publication Adapter is unavailable")
	}
	if ctx == nil {
		return hostcontract.AssemblyPublicationResultV2{}, hostcontract.NewError(hostcontract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	now, err := a.nowAfterV2(0)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	if err := request.ValidateAt(now); err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	bundle, err := assemblycontract.NewAssemblyPublicationBundleV2(request.Artifacts.ScopeRef, request.Artifacts.Harness)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, ownerErrorV1(err, "assembly_publication_bundle_invalid")
	}
	nextRevision := core.Revision(1)
	if request.ExpectedCurrent.Exists {
		nextRevision = request.ExpectedCurrent.Revision + 1
	}
	current, err := assemblycontract.NewAssemblyPublicationCurrentV2(bundle, request.AttemptID, nextRevision, now, request.RequestedExpiresUnixNano)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, ownerErrorV1(err, "assembly_publication_current_invalid")
	}
	recovered := false
	artifacts := []publicationStepV2{
		{name: "generation", coordinate: hostExactArtifactCoordinateV2(GenerationKindV1, request.Artifacts.Compiled.GenerationRef), dispatch: func(c context.Context) error {
			return a.store.StageGenerationV2(c, bundle.Publication.PublicationID, bundle.Generation)
		}},
		{name: "manifest", coordinate: hostExactArtifactCoordinateV2(ManifestKindV1, request.Artifacts.Compiled.ManifestRef), dispatch: func(c context.Context) error {
			return a.store.StageManifestV2(c, bundle.Publication.PublicationID, bundle.Manifest)
		}},
		{name: "graph", coordinate: hostExactArtifactCoordinateV2(GraphKindV1, request.Artifacts.Compiled.Graph.GraphRef), dispatch: func(c context.Context) error {
			return a.store.StageGraphV2(c, bundle.Publication.PublicationID, bundle.Graph)
		}},
		{name: "handoff", coordinate: hostExactArtifactCoordinateV2(HandoffKindV1, request.Artifacts.Compiled.HandoffRef), dispatch: func(c context.Context) error {
			return a.store.StageHandoffV2(c, bundle.Publication.PublicationID, bundle.Handoff)
		}},
	}
	for index := range artifacts {
		step := artifacts[index]
		step.inspect = func(c context.Context) (bool, error) {
			inspection, inspectErr := a.store.InspectStagedPublicationV2(c, bundle.Publication.PublicationID)
			if inspectErr != nil {
				if core.HasCategory(inspectErr, core.ErrorNotFound) {
					return false, nil
				}
				return false, ownerErrorV1(inspectErr, "assembly_stage_inspect_failed")
			}
			got := map[string]core.Digest{"generation": inspection.GenerationDigest, "manifest": inspection.ManifestDigest, "graph": inspection.GraphDigest, "handoff": inspection.HandoffDigest}[step.name]
			if got == "" {
				return false, nil
			}
			if got != core.Digest(step.coordinate.Digest) {
				return false, hostcontract.NewError(hostcontract.ErrorConflict, "assembly_stage_splice", "staged Assembly object drifted")
			}
			return true, nil
		}
		wasRecovered, runErr := a.runPublicationStepV2(ctx, request, step)
		if runErr != nil {
			return hostcontract.AssemblyPublicationResultV2{}, runErr
		}
		recovered = recovered || wasRecovered
	}
	publicationRef := assemblycontract.AssemblyPublicationRefV2{PublicationID: bundle.Publication.PublicationID, Revision: bundle.Publication.Revision, Digest: bundle.Publication.Digest}
	commitCoordinate := hostcontract.HostOperationCoordinateV2{ContractKind: assemblycontract.PublicationContractVersionV2, OwnerID: string(a.owner.ID), ID: current.ScopeRef, Revision: uint64(current.Revision), Digest: hostcontract.DigestV1(current.Digest), Current: true, ExpiresUnixNano: current.ExpiresUnixNano}
	commitStep := publicationStepV2{name: "commit", coordinate: commitCoordinate, inputs: publicationInputsV2(artifacts), dispatch: func(c context.Context) error {
		_, commitErr := a.store.CommitPublicationCurrentV2(c, assemblypublication.CommitPublicationCurrentRequestV2{Expected: request.ExpectedCurrent, Bundle: bundle, Current: current})
		return commitErr
	}}
	commitStep.inspect = func(c context.Context) (bool, error) {
		historical, historyErr := a.store.InspectHistoricalPublicationV2(c, publicationRef)
		if historyErr != nil {
			if core.HasCategory(historyErr, core.ErrorNotFound) {
				return false, nil
			}
			return false, ownerErrorV1(historyErr, "assembly_publication_history_inspect_failed")
		}
		committed, currentErr := a.store.InspectCommittedPublicationCurrentV2(c, publicationRef)
		if currentErr != nil {
			return false, ownerErrorV1(currentErr, "assembly_publication_current_inspect_failed")
		}
		if historical.Publication.Digest != bundle.Publication.Digest || committed != current {
			return false, hostcontract.NewError(hostcontract.ErrorConflict, "assembly_publication_commit_splice", "committed Assembly publication drifted")
		}
		return true, nil
	}
	wasRecovered, err := a.runPublicationStepV2(ctx, request, commitStep)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	recovered = recovered || wasRecovered
	finalNow, err := a.nowAfterV2(now.UnixNano())
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	if finalNow.UnixNano() >= current.ExpiresUnixNano {
		return hostcontract.AssemblyPublicationResultV2{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "assembly_publication_expired", "Assembly publication expired before return")
	}
	ownerCurrent, err := hostcontract.AssemblyPublicationOwnerCurrentV2(a.owner, current)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	result := hostcontract.AssemblyPublicationResultV2{ContractVersion: hostcontract.AssemblyPublicationAdapterContractVersionV2, OwnerCurrent: ownerCurrent, Publication: hostcontract.ExactRefV1{Kind: "praxis.harness/assembly-publication", ID: publicationRef.PublicationID, Revision: uint64(publicationRef.Revision), Digest: hostcontract.DigestV1(publicationRef.Digest)}, Generation: request.Artifacts.Compiled.GenerationRef, Manifest: request.Artifacts.Compiled.ManifestRef, Graph: request.Artifacts.Compiled.Graph.GraphRef, Handoff: request.Artifacts.Compiled.HandoffRef, Recovered: recovered}
	if err := result.ValidateAt(finalNow); err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	return result, nil
}

func (a *AssemblyPublicationAdapterV2) InspectAssemblyPublicationV2(ctx context.Context, request hostcontract.AssemblyPublicationRequestV2) (hostcontract.AssemblyPublicationResultV2, error) {
	if a == nil || hostcontract.IsTypedNilV1(a.store) || a.clock == nil {
		return hostcontract.AssemblyPublicationResultV2{}, hostcontract.NewError(hostcontract.ErrorUnavailable, "assembly_publication_adapter_unavailable", "Assembly publication Adapter is unavailable")
	}
	if ctx == nil {
		return hostcontract.AssemblyPublicationResultV2{}, hostcontract.NewError(hostcontract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	now, err := a.nowAfterV2(0)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	if err = request.ValidateAt(now); err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	bundle, err := assemblycontract.NewAssemblyPublicationBundleV2(request.Artifacts.ScopeRef, request.Artifacts.Harness)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, ownerErrorV1(err, "assembly_publication_bundle_invalid")
	}
	publicationRef := assemblycontract.AssemblyPublicationRefV2{PublicationID: bundle.Publication.PublicationID, Revision: bundle.Publication.Revision, Digest: bundle.Publication.Digest}
	historical, err := a.store.InspectHistoricalPublicationV2(ctx, publicationRef)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, ownerErrorV1(err, "assembly_publication_history_inspect_failed")
	}
	current, err := a.store.InspectCommittedPublicationCurrentV2(ctx, publicationRef)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, ownerErrorV1(err, "assembly_publication_current_inspect_failed")
	}
	if historical.Publication.Digest != bundle.Publication.Digest || current.Publication != publicationRef || current.CommitAttemptID != request.AttemptID {
		return hostcontract.AssemblyPublicationResultV2{}, hostcontract.NewError(hostcontract.ErrorConflict, "assembly_publication_commit_splice", "committed Assembly publication drifted from the exact request")
	}
	if now.UnixNano() >= current.ExpiresUnixNano || current.ExpiresUnixNano > request.RequestedExpiresUnixNano {
		return hostcontract.AssemblyPublicationResultV2{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "assembly_publication_expired", "Assembly publication is no longer current")
	}
	ownerCurrent, err := hostcontract.AssemblyPublicationOwnerCurrentV2(a.owner, current)
	if err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	result := hostcontract.AssemblyPublicationResultV2{ContractVersion: hostcontract.AssemblyPublicationAdapterContractVersionV2, OwnerCurrent: ownerCurrent, Publication: hostcontract.ExactRefV1{Kind: "praxis.harness/assembly-publication", ID: publicationRef.PublicationID, Revision: uint64(publicationRef.Revision), Digest: hostcontract.DigestV1(publicationRef.Digest)}, Generation: request.Artifacts.Compiled.GenerationRef, Manifest: request.Artifacts.Compiled.ManifestRef, Graph: request.Artifacts.Compiled.Graph.GraphRef, Handoff: request.Artifacts.Compiled.HandoffRef, Recovered: true}
	if err := result.ValidateAt(now); err != nil {
		return hostcontract.AssemblyPublicationResultV2{}, err
	}
	return result, nil
}

type publicationStepV2 struct {
	name       string
	coordinate hostcontract.HostOperationCoordinateV2
	inputs     []hostcontract.HostOperationCoordinateV2
	dispatch   func(context.Context) error
	inspect    func(context.Context) (bool, error)
}

func (a *AssemblyPublicationAdapterV2) runPublicationStepV2(ctx context.Context, request hostcontract.AssemblyPublicationRequestV2, step publicationStepV2) (bool, error) {
	journal, err := a.journal.InspectHostJournalV2(ctx, request.HostID, request.StartID)
	if err != nil {
		return false, err
	}
	if err := journal.Validate(); err != nil {
		return false, err
	}
	if journal.Phase != hostcontract.HostCompilingV2 {
		return false, hostcontract.NewError(hostcontract.ErrorPrecondition, "assembly_publication_phase_invalid", "Assembly publication requires the Host compiling phase")
	}
	stepID := request.AttemptID + "-publication-" + step.name
	requestDigest, err := request.DigestV2()
	if err != nil {
		return false, err
	}
	operationDigest, err := hostcontract.DigestJSONV1(struct {
		Request    hostcontract.DigestV1                  `json:"request"`
		Step       string                                 `json:"step"`
		Coordinate hostcontract.HostOperationCoordinateV2 `json:"coordinate"`
	}{requestDigest, step.name, step.coordinate})
	if err != nil {
		return false, err
	}
	owned := false
	index := findOperationV2(journal, stepID)
	if index < 0 {
		inputs := append([]hostcontract.HostOperationCoordinateV2(nil), step.inputs...)
		if len(inputs) == 0 {
			inputs = []hostcontract.HostOperationCoordinateV2{step.coordinate}
		}
		sort.Slice(inputs, func(i, j int) bool { return operationInputKeyV2(inputs[i]) < operationInputKeyV2(inputs[j]) })
		now, nowErr := a.nowAfterV2(journal.UpdatedUnixNano)
		if nowErr != nil {
			return false, nowErr
		}
		attempt, sealErr := hostcontract.SealHostOperationAttemptV2(hostcontract.HostOperationAttemptV2{ContractVersion: hostcontract.HostJournalContractVersionV2, AttemptID: stepID, Revision: 1, OperationKind: "praxis.agent-host/assembly-publication-" + step.name, Phase: hostcontract.HostCompilingV2, RequestDigest: operationDigest, Inputs: inputs, State: hostcontract.HostOperationIntentRecordedV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
		if sealErr != nil {
			return false, sealErr
		}
		next := journal
		next.Revision++
		next.UpdatedUnixNano = now.UnixNano()
		next.Operations = append(append([]hostcontract.HostOperationAttemptV2(nil), journal.Operations...), attempt)
		next.Digest = ""
		next, sealErr = hostcontract.SealHostJournalV2(next)
		if sealErr != nil {
			return false, sealErr
		}
		expected, _ := journal.RefV2()
		actual, writeErr := a.journal.CompareAndSwapHostJournalV2(ctx, expected, next)
		if writeErr == nil && actual.Digest == next.Digest {
			journal, index, owned = actual, len(actual.Operations)-1, true
		} else {
			journal, err = a.journal.InspectHostJournalV2(context.WithoutCancel(ctx), request.HostID, request.StartID)
			if err != nil {
				if writeErr != nil {
					return false, writeErr
				}
				return false, err
			}
			index = findOperationV2(journal, stepID)
			if index < 0 {
				if writeErr != nil {
					return false, writeErr
				}
				return false, hostcontract.NewError(hostcontract.ErrorConflict, "assembly_step_intent_missing", "Assembly step intent CAS did not linearize")
			}
		}
	}
	attempt := journal.Operations[index]
	if attempt.RequestDigest != operationDigest || attempt.OperationKind != "praxis.agent-host/assembly-publication-"+step.name {
		return false, hostcontract.NewError(hostcontract.ErrorConflict, "assembly_step_attempt_splice", "Assembly publication step attempt drifted")
	}
	if attempt.State == hostcontract.HostOperationResultRecordedV2 {
		if attempt.Result == nil || *attempt.Result != step.coordinate {
			return false, hostcontract.NewError(hostcontract.ErrorConflict, "assembly_step_result_splice", "Assembly publication step result drifted")
		}
		return true, nil
	}
	if !owned {
		ok, inspectErr := step.inspect(context.WithoutCancel(ctx))
		if inspectErr != nil {
			return false, inspectErr
		}
		if !ok {
			return false, hostcontract.NewError(hostcontract.ErrorUnknownOutcome, "assembly_step_requires_inspect", "Assembly publication step intent exists without exact inspectable result")
		}
		return true, a.settlePublicationStepV2(context.WithoutCancel(ctx), request, stepID, operationDigest, step.coordinate)
	}
	dispatchNow, nowErr := a.nowAfterV2(journal.UpdatedUnixNano)
	if nowErr != nil {
		return false, nowErr
	}
	if err := request.ValidateAt(dispatchNow); err != nil {
		return false, err
	}
	dispatchErr := step.dispatch(ctx)
	if dispatchErr != nil {
		ok, inspectErr := step.inspect(context.WithoutCancel(ctx))
		if inspectErr == nil && ok {
			return true, a.settlePublicationStepV2(context.WithoutCancel(ctx), request, stepID, operationDigest, step.coordinate)
		}
		_ = a.markPublicationStepUnknownV2(context.WithoutCancel(ctx), request, stepID, operationDigest)
		return false, ownerErrorV1(dispatchErr, "assembly_publication_write_failed")
	}
	return false, a.settlePublicationStepV2(ctx, request, stepID, operationDigest, step.coordinate)
}

func (a *AssemblyPublicationAdapterV2) settlePublicationStepV2(ctx context.Context, request hostcontract.AssemblyPublicationRequestV2, stepID string, requestDigest hostcontract.DigestV1, result hostcontract.HostOperationCoordinateV2) error {
	journal, err := a.journal.InspectHostJournalV2(ctx, request.HostID, request.StartID)
	if err != nil {
		return err
	}
	index := findOperationV2(journal, stepID)
	if index < 0 || index != len(journal.Operations)-1 {
		return hostcontract.NewError(hostcontract.ErrorConflict, "assembly_step_journal_order", "Assembly publication step is not the active Journal operation")
	}
	current := journal.Operations[index]
	if current.RequestDigest != requestDigest {
		return hostcontract.NewError(hostcontract.ErrorConflict, "assembly_step_attempt_splice", "Assembly publication step request digest drifted")
	}
	if current.State == hostcontract.HostOperationResultRecordedV2 {
		if current.Result != nil && *current.Result == result {
			return nil
		}
		return hostcontract.NewError(hostcontract.ErrorConflict, "assembly_step_result_splice", "Assembly publication result drifted")
	}
	now, err := a.nowAfterV2(journal.UpdatedUnixNano)
	if err != nil {
		return err
	}
	nextAttempt := current
	nextAttempt.Revision++
	nextAttempt.State = hostcontract.HostOperationResultRecordedV2
	nextAttempt.Result = &result
	nextAttempt.UpdatedUnixNano = now.UnixNano()
	nextAttempt.Digest = ""
	nextAttempt, err = hostcontract.SealHostOperationAttemptV2(nextAttempt)
	if err != nil {
		return err
	}
	next := journal
	next.Revision++
	next.UpdatedUnixNano = now.UnixNano()
	next.Operations = append([]hostcontract.HostOperationAttemptV2(nil), journal.Operations...)
	next.Operations[index] = nextAttempt
	next.Digest = ""
	next, err = hostcontract.SealHostJournalV2(next)
	if err != nil {
		return err
	}
	expected, _ := journal.RefV2()
	actual, writeErr := a.journal.CompareAndSwapHostJournalV2(ctx, expected, next)
	if writeErr == nil && actual.Digest == next.Digest {
		return nil
	}
	inspected, inspectErr := a.journal.InspectHostJournalV2(context.WithoutCancel(ctx), request.HostID, request.StartID)
	if inspectErr == nil {
		idx := findOperationV2(inspected, stepID)
		if idx >= 0 && inspected.Operations[idx].Digest == nextAttempt.Digest {
			return nil
		}
	}
	if writeErr != nil {
		return writeErr
	}
	return hostcontract.NewError(hostcontract.ErrorConflict, "assembly_step_settlement_splice", "Assembly step settlement returned another Journal successor")
}

func (a *AssemblyPublicationAdapterV2) markPublicationStepUnknownV2(ctx context.Context, request hostcontract.AssemblyPublicationRequestV2, stepID string, requestDigest hostcontract.DigestV1) error {
	journal, err := a.journal.InspectHostJournalV2(ctx, request.HostID, request.StartID)
	if err != nil {
		return err
	}
	index := findOperationV2(journal, stepID)
	if index < 0 || index != len(journal.Operations)-1 {
		return nil
	}
	current := journal.Operations[index]
	if current.RequestDigest != requestDigest || current.State != hostcontract.HostOperationIntentRecordedV2 {
		return nil
	}
	now, err := a.nowAfterV2(journal.UpdatedUnixNano)
	if err != nil {
		return err
	}
	nextAttempt := current
	nextAttempt.Revision++
	nextAttempt.State = hostcontract.HostOperationOutcomeUnknownV2
	nextAttempt.UpdatedUnixNano = now.UnixNano()
	nextAttempt.Digest = ""
	nextAttempt, err = hostcontract.SealHostOperationAttemptV2(nextAttempt)
	if err != nil {
		return err
	}
	next := journal
	next.Revision++
	next.UpdatedUnixNano = now.UnixNano()
	next.Operations = append([]hostcontract.HostOperationAttemptV2(nil), journal.Operations...)
	next.Operations[index] = nextAttempt
	next.Digest = ""
	next, err = hostcontract.SealHostJournalV2(next)
	if err != nil {
		return err
	}
	expected, _ := journal.RefV2()
	_, err = a.journal.CompareAndSwapHostJournalV2(ctx, expected, next)
	return err
}

func (a *AssemblyPublicationAdapterV2) nowAfterV2(previous int64) (time.Time, error) {
	now := a.clock()
	if now.IsZero() || now.UnixNano() <= 0 || now.UnixNano() < previous {
		return time.Time{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "assembly_publication_clock_regression", "Assembly publication clock regressed")
	}
	return now, nil
}
func findOperationV2(j hostcontract.HostJournalV2, id string) int {
	for i := range j.Operations {
		if j.Operations[i].AttemptID == id {
			return i
		}
	}
	return -1
}
func operationInputKeyV2(v hostcontract.HostOperationCoordinateV2) string {
	return v.ContractKind + "\x00" + v.OwnerID + "\x00" + v.ID
}
func hostExactArtifactCoordinateV2(kind string, ref hostcontract.ExactRefV1) hostcontract.HostOperationCoordinateV2 {
	return hostcontract.HostOperationCoordinateV2{ContractKind: kind, OwnerID: "praxis.harness", ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
}
func publicationInputsV2(steps []publicationStepV2) []hostcontract.HostOperationCoordinateV2 {
	result := make([]hostcontract.HostOperationCoordinateV2, 0, len(steps))
	for _, step := range steps {
		result = append(result, step.coordinate)
	}
	sort.Slice(result, func(i, j int) bool { return operationInputKeyV2(result[i]) < operationInputKeyV2(result[j]) })
	return result
}
