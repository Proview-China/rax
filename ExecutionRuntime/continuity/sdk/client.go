// Package sdk exposes a transport-neutral Continuity client. Reads are
// revalidated at the SDK boundary; writes can only submit or inspect governed
// Application workflows. It never owns a Fact Store and intentionally has no
// direct Checkpoint, Restore, Retention purge, or external Effect write method.
package sdk

import (
	"context"
	"reflect"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type TimelineReader interface {
	Inspect(context.Context, string) (contract.TimelineEventRecord, error)
	Query(context.Context, contract.TimelineQuery) (contract.TimelinePage, error)
	Watch(context.Context, contract.TimelineQuery) (contract.TimelinePage, error)
}

type RetentionReader interface {
	Inspect(context.Context, string) (contract.RetentionFact, error)
}

// GovernedWorkflowGateway is the SDK's only write capability. Implementations
// must route through the public Application gateway; this is not a Fact Store
// or provider execution seam.
type GovernedWorkflowGateway interface {
	Submit(context.Context, appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error)
	Inspect(context.Context, appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error)
}

type Config struct {
	Timeline     TimelineReader
	Checkpoints  ports.CheckpointManifestReaderV2
	RestorePlans ports.RestorePlanReaderV2
	RewindPlans  ports.RewindPlanReaderV2
	Artifacts    ports.ArtifactRelationReaderV1
	Integrity    ports.ContentIntegrityAuditReaderV1
	Deltas       ports.ContentDeltaReaderV1
	Derivations  ports.HistoryDerivationCandidateReaderV1
	Retention    RetentionReader
	Workflows    GovernedWorkflowGateway
	Clock        func() time.Time
}

type Client struct {
	timeline     TimelineReader
	checkpoints  ports.CheckpointManifestReaderV2
	restorePlans ports.RestorePlanReaderV2
	rewindPlans  ports.RewindPlanReaderV2
	artifacts    ports.ArtifactRelationReaderV1
	integrity    ports.ContentIntegrityAuditReaderV1
	deltas       ports.ContentDeltaReaderV1
	derivations  ports.HistoryDerivationCandidateReaderV1
	retention    RetentionReader
	workflows    GovernedWorkflowGateway
	clock        func() time.Time
}

func New(config Config) (*Client, error) {
	if config.Clock == nil {
		return nil, contract.NewError(contract.ErrInvalidArgument, "clock", "SDK clock is required")
	}
	if nilOrTypedNil(config.Timeline) && nilOrTypedNil(config.Checkpoints) && nilOrTypedNil(config.RestorePlans) && nilOrTypedNil(config.RewindPlans) && nilOrTypedNil(config.Artifacts) && nilOrTypedNil(config.Integrity) && nilOrTypedNil(config.Deltas) && nilOrTypedNil(config.Derivations) && nilOrTypedNil(config.Retention) && nilOrTypedNil(config.Workflows) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "readers", "at least one Continuity read capability is required")
	}
	return &Client{
		timeline: config.Timeline, checkpoints: config.Checkpoints,
		restorePlans: config.RestorePlans, rewindPlans: config.RewindPlans, artifacts: config.Artifacts, integrity: config.Integrity, deltas: config.Deltas, derivations: config.Derivations, retention: config.Retention, workflows: config.Workflows, clock: config.Clock,
	}, nil
}

func (c *Client) SubmitGovernedWorkflow(ctx context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	if c == nil || nilOrTypedNil(c.workflows) {
		return appcontract.ContinuityWorkflowInspectionV1{}, unavailable("governed workflow")
	}
	if err := request.Validate(c.now()); err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	result, err := c.workflows.Submit(ctx, request)
	if err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	return cloneGovernedWorkflowInspection(request, result)
}

func (c *Client) InspectGovernedWorkflow(ctx context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	if c == nil || nilOrTypedNil(c.workflows) {
		return appcontract.ContinuityWorkflowInspectionV1{}, unavailable("governed workflow")
	}
	if err := request.Validate(time.Time{}); err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	result, err := c.workflows.Inspect(ctx, request)
	if err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	return cloneGovernedWorkflowInspection(request, result)
}

func cloneGovernedWorkflowInspection(request appcontract.ContinuityWorkflowRequestV1, result appcontract.ContinuityWorkflowInspectionV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	if err := result.ValidateFor(request); err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	result.Steps = append([]appcontract.ContinuityWorkflowStepRefV1(nil), result.Steps...)
	if result.Journal != nil {
		journal := *result.Journal
		result.Journal = &journal
	}
	return result, nil
}

func (c *Client) InspectEvent(ctx context.Context, evidenceRef string) (contract.TimelineEventRecord, error) {
	if c == nil || nilOrTypedNil(c.timeline) {
		return contract.TimelineEventRecord{}, unavailable("timeline")
	}
	record, err := c.timeline.Inspect(ctx, evidenceRef)
	if err != nil {
		return contract.TimelineEventRecord{}, err
	}
	if err := record.Validate(); err != nil {
		return contract.TimelineEventRecord{}, contract.NewError(contract.ErrContentDigestMismatch, "timeline_event", "reader returned an invalid Event")
	}
	return record.Clone(), nil
}

func (c *Client) QueryTimeline(ctx context.Context, query contract.TimelineQuery) (contract.TimelinePage, error) {
	if c == nil || nilOrTypedNil(c.timeline) {
		return contract.TimelinePage{}, unavailable("timeline")
	}
	if err := query.Validate(); err != nil {
		return contract.TimelinePage{}, err
	}
	if _, err := validateTimelineInputCursor(query, c.now()); err != nil {
		return contract.TimelinePage{}, err
	}
	page, err := c.timeline.Query(ctx, query)
	if err != nil {
		return contract.TimelinePage{}, err
	}
	return validateAndCloneTimelinePage(page, query, c.now())
}

func (c *Client) WatchTimeline(ctx context.Context, query contract.TimelineQuery) (contract.TimelinePage, error) {
	if c == nil || nilOrTypedNil(c.timeline) {
		return contract.TimelinePage{}, unavailable("timeline")
	}
	if err := query.Validate(); err != nil {
		return contract.TimelinePage{}, err
	}
	if _, err := validateTimelineInputCursor(query, c.now()); err != nil {
		return contract.TimelinePage{}, err
	}
	page, err := c.timeline.Watch(ctx, query)
	if err != nil {
		return contract.TimelinePage{}, err
	}
	return validateAndCloneTimelinePage(page, query, c.now())
}

func (c *Client) InspectCheckpointManifest(ctx context.Context, ref contract.CheckpointManifestRefV2) (contract.CheckpointManifestFactV2, error) {
	if c == nil || nilOrTypedNil(c.checkpoints) {
		return contract.CheckpointManifestFactV2{}, unavailable("checkpoint Manifest")
	}
	fact, err := c.checkpoints.InspectCheckpointManifestV2(ctx, ports.InspectCheckpointManifestRequestV2{Ref: ref})
	if err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	if err := fact.Validate(); err != nil || !fact.Ref().Exact().Equal(ref.Exact()) {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrRevisionConflict, "checkpoint_manifest_ref", "reader returned a non-exact Manifest")
	}
	return fact.Clone(), nil
}

func (c *Client) InspectCurrentCheckpointManifest(ctx context.Context, request ports.InspectCurrentCheckpointManifestRequestV2) (contract.CheckpointManifestFactV2, error) {
	if c == nil || nilOrTypedNil(c.checkpoints) {
		return contract.CheckpointManifestFactV2{}, unavailable("checkpoint Manifest")
	}
	fact, err := c.checkpoints.InspectCurrentCheckpointManifestV2(ctx, request)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	if err := fact.Validate(); err != nil || fact.Owner != request.Owner || fact.Scope.TenantID != request.TenantID || fact.Scope.ExecutionScopeDigest != request.ScopeDigest || fact.ManifestID != request.ManifestID {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrRevisionConflict, "checkpoint_manifest_current", "reader returned a mismatched current Manifest")
	}
	return fact.Clone(), nil
}

func (c *Client) InspectCheckpointManifestSeal(ctx context.Context, ref contract.CheckpointManifestSealRefV2) (contract.CheckpointManifestSealFactV2, error) {
	if c == nil || nilOrTypedNil(c.checkpoints) {
		return contract.CheckpointManifestSealFactV2{}, unavailable("checkpoint Manifest Seal")
	}
	fact, err := c.checkpoints.InspectCheckpointManifestSealV2(ctx, ports.InspectCheckpointManifestSealRequestV2{Ref: ref})
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	if err := fact.Validate(); err != nil || !fact.Ref().Exact().Equal(ref.Exact()) {
		return contract.CheckpointManifestSealFactV2{}, contract.NewError(contract.ErrRevisionConflict, "checkpoint_manifest_seal_ref", "reader returned a non-exact Manifest Seal")
	}
	return fact.Clone(), nil
}

func (c *Client) InspectRestorePlan(ctx context.Context, ref contract.RestorePlanRefV2) (contract.RestorePlanFactV2, error) {
	if c == nil || nilOrTypedNil(c.restorePlans) {
		return contract.RestorePlanFactV2{}, unavailable("Restore Plan")
	}
	plan, err := c.restorePlans.InspectRestorePlanV2(ctx, ports.InspectRestorePlanRequestV2{Ref: ref})
	if err != nil {
		return contract.RestorePlanFactV2{}, err
	}
	if err := plan.Validate(); err != nil || !plan.Ref().Exact().Equal(ref.Exact()) {
		return contract.RestorePlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "restore_plan_ref", "reader returned a non-exact Restore Plan")
	}
	return plan.Clone(), nil
}

func (c *Client) InspectCurrentRestorePlan(ctx context.Context, request ports.InspectCurrentRestorePlanRequestV2) (contract.RestorePlanFactV2, error) {
	if c == nil || nilOrTypedNil(c.restorePlans) {
		return contract.RestorePlanFactV2{}, unavailable("Restore Plan")
	}
	plan, err := c.restorePlans.InspectCurrentRestorePlanV2(ctx, request)
	if err != nil {
		return contract.RestorePlanFactV2{}, err
	}
	if err := plan.ValidateCurrent(c.clock()); err != nil || plan.Owner != request.Owner || plan.Scope.TenantID != request.TenantID || plan.Scope.ExecutionScopeDigest != request.ScopeDigest || plan.PlanID != request.PlanID {
		if err != nil {
			return contract.RestorePlanFactV2{}, err
		}
		return contract.RestorePlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "restore_plan_current", "reader returned a mismatched current Restore Plan")
	}
	return plan.Clone(), nil
}

func (c *Client) InspectRewindPlan(ctx context.Context, ref contract.RewindPlanRefV2) (contract.RewindPlanFactV2, error) {
	if c == nil || nilOrTypedNil(c.rewindPlans) {
		return contract.RewindPlanFactV2{}, unavailable("Rewind Plan")
	}
	plan, err := c.rewindPlans.InspectRewindPlanV2(ctx, ports.InspectRewindPlanRequestV2{Ref: ref})
	if err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if err := plan.Validate(); err != nil || !plan.Ref().Exact().Equal(ref.Exact()) {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_ref", "reader returned a non-exact Rewind Plan")
	}
	return plan.Clone(), nil
}

func (c *Client) InspectCurrentRewindPlan(ctx context.Context, request ports.InspectCurrentRewindPlanRequestV2) (contract.RewindPlanFactV2, error) {
	if c == nil || nilOrTypedNil(c.rewindPlans) {
		return contract.RewindPlanFactV2{}, unavailable("Rewind Plan")
	}
	plan, err := c.rewindPlans.InspectCurrentRewindPlanV2(ctx, request)
	if err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if err := plan.ValidateCurrent(c.clock()); err != nil {
		return contract.RewindPlanFactV2{}, err
	}
	if plan.Owner != request.Owner || plan.Scope.TenantID != request.TenantID || plan.Scope.ExecutionScopeDigest != request.ScopeDigest || plan.PlanID != request.PlanID {
		return contract.RewindPlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "rewind_plan_current", "reader returned a mismatched current Rewind Plan")
	}
	return plan.Clone(), nil
}

func (c *Client) InspectArtifactRelation(ctx context.Context, ref contract.ArtifactRelationRefV1) (contract.ArtifactRelationFactV1, error) {
	if c == nil || nilOrTypedNil(c.artifacts) {
		return contract.ArtifactRelationFactV1{}, unavailable("Artifact Relation")
	}
	fact, err := c.artifacts.InspectArtifactRelationV1(ctx, ports.InspectArtifactRelationRequestV1{Ref: ref})
	if err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	if err := fact.Validate(); err != nil || !fact.Ref().Exact().Equal(ref.Exact()) {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrRevisionConflict, "artifact_relation_ref", "reader returned a non-exact Artifact Relation")
	}
	return fact.Clone(), nil
}

func (c *Client) ListArtifactRelations(ctx context.Context, ref contract.ExactFactRefV2) ([]contract.ArtifactRelationFactV1, error) {
	if c == nil || nilOrTypedNil(c.artifacts) {
		return nil, unavailable("Artifact Relation")
	}
	facts, err := c.artifacts.ListArtifactRelationsV1(ctx, ports.ListArtifactRelationsRequestV1{ArtifactFactRef: ref})
	if err != nil {
		return nil, err
	}
	return validateAndCloneArtifactRelations(facts, func(fact contract.ArtifactRelationFactV1) bool {
		return fact.SourceProjection.Artifact.ArtifactFactRef.Equal(ref)
	})
}

func (c *Client) ListRelatedArtifactRelations(ctx context.Context, ref contract.ExactFactRefV2) ([]contract.ArtifactRelationFactV1, error) {
	if c == nil || nilOrTypedNil(c.artifacts) {
		return nil, unavailable("Artifact Relation")
	}
	facts, err := c.artifacts.ListRelatedArtifactRelationsV1(ctx, ports.ListRelatedArtifactRelationsRequestV1{RelatedFactRef: ref})
	if err != nil {
		return nil, err
	}
	return validateAndCloneArtifactRelations(facts, func(fact contract.ArtifactRelationFactV1) bool {
		return fact.SourceProjection.RelatedFactRef.Equal(ref)
	})
}

func (c *Client) InspectContentIntegrityAudit(ctx context.Context, ref contract.ContentIntegrityAuditRefV1) (contract.ContentIntegrityAuditFactV1, error) {
	if c == nil || nilOrTypedNil(c.integrity) {
		return contract.ContentIntegrityAuditFactV1{}, unavailable("Content Integrity Audit")
	}
	fact, err := c.integrity.InspectContentIntegrityAuditV1(ctx, ports.InspectContentIntegrityAuditRequestV1{Ref: ref})
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, err
	}
	if err := fact.Validate(); err != nil || !fact.Ref().Exact().Equal(ref.Exact()) {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrRevisionConflict, "content_integrity_audit_ref", "reader returned a non-exact Content Integrity Audit")
	}
	return fact.Clone(), nil
}

func (c *Client) InspectContentDelta(ctx context.Context, ref contract.ContentDeltaRefV1) (contract.ContentDeltaFactV1, error) {
	if c == nil || nilOrTypedNil(c.deltas) {
		return contract.ContentDeltaFactV1{}, unavailable("Content Delta")
	}
	fact, err := c.deltas.InspectContentDeltaV1(ctx, ports.InspectContentDeltaRequestV1{Ref: ref})
	if err != nil {
		return contract.ContentDeltaFactV1{}, err
	}
	if err := fact.Validate(); err != nil || !fact.Ref().Exact().Equal(ref.Exact()) {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrRevisionConflict, "content_delta_ref", "reader returned a non-exact Content Delta")
	}
	return fact.Clone(), nil
}

func (c *Client) InspectHistoryDerivationCandidate(ctx context.Context, ref contract.HistoryDerivationCandidateRefV1) (contract.HistoryDerivationCandidateFactV1, error) {
	if c == nil || nilOrTypedNil(c.derivations) {
		return contract.HistoryDerivationCandidateFactV1{}, unavailable("History Derivation Candidate")
	}
	fact, err := c.derivations.InspectHistoryDerivationCandidateV1(ctx, ports.InspectHistoryDerivationCandidateRequestV1{Ref: ref})
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	if err := fact.Validate(); err != nil || !fact.Ref().Exact().Equal(ref.Exact()) {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrRevisionConflict, "history_derivation_ref", "reader returned a non-exact History Derivation Candidate")
	}
	return fact.Clone(), nil
}

func (c *Client) InspectRetention(ctx context.Context, objectID string) (contract.RetentionFact, error) {
	if c == nil || nilOrTypedNil(c.retention) {
		return contract.RetentionFact{}, unavailable("retention")
	}
	if err := contract.ValidateToken("object_id", objectID); err != nil {
		return contract.RetentionFact{}, err
	}
	fact, err := c.retention.Inspect(ctx, objectID)
	if err != nil {
		return contract.RetentionFact{}, err
	}
	if fact.ObjectID != objectID {
		return contract.RetentionFact{}, contract.NewError(contract.ErrRevisionConflict, "retention", "reader returned another object")
	}
	if err := fact.Validate(); err != nil {
		return contract.RetentionFact{}, contract.NewError(contract.ErrContentDigestMismatch, "retention", "reader returned an invalid Retention Fact")
	}
	return fact, nil
}

func (c *Client) ValidateForkPlan(plan contract.ForkPlan) error     { return plan.Validate(c.now()) }
func (c *Client) ValidateRewindPlan(plan contract.RewindPlan) error { return plan.Validate(c.now()) }
func (c *Client) ValidateRewindPlanV2(plan contract.RewindPlanFactV2) error {
	return plan.ValidateCurrent(c.now())
}
func (c *Client) ValidateRestorePlan(plan contract.RestorePlan) error { return plan.Validate(c.now()) }
func (c *Client) ValidateRestorePlanV2(plan contract.RestorePlanFactV2) error {
	return plan.ValidateCurrent(c.now())
}
func (c *Client) ValidateRecoveryCredential(credential contract.RecoveryCredentialV1) error {
	return credential.Validate(c.now())
}

func (c *Client) now() time.Time {
	if c == nil || c.clock == nil {
		return time.Time{}
	}
	return c.clock()
}

func cloneTimelinePage(page contract.TimelinePage) contract.TimelinePage {
	result := page
	result.Records = make([]contract.TimelineEventRecord, len(page.Records))
	for i := range page.Records {
		result.Records[i] = page.Records[i].Clone()
	}
	return result
}

func validateAndCloneTimelinePage(page contract.TimelinePage, query contract.TimelineQuery, now time.Time) (contract.TimelinePage, error) {
	inputAfter, err := validateTimelineInputCursor(query, now)
	if err != nil {
		return contract.TimelinePage{}, err
	}
	if len(page.Records) > query.PageLimit || page.NextCursor == "" || !page.Exhausted && len(page.Records) != query.PageLimit {
		return contract.TimelinePage{}, contract.NewError(contract.ErrRevisionConflict, "timeline_page", "Reader returned an invalid page bound or cursor")
	}
	previous := inputAfter
	for _, record := range page.Records {
		if err := record.Validate(); err != nil {
			return contract.TimelinePage{}, contract.NewError(contract.ErrContentDigestMismatch, "timeline_page", "Reader returned an invalid Event")
		}
		if record.LedgerSequence <= previous || !contract.TimelineEventMatchesQuery(record, query) {
			return contract.TimelinePage{}, contract.NewError(contract.ErrRevisionConflict, "timeline_page", "Reader returned an out-of-order or mismatched Event")
		}
		previous = record.LedgerSequence
	}
	outputCursor, err := contract.DecodeTimelineCursor(page.NextCursor)
	if err != nil {
		return contract.TimelinePage{}, err
	}
	if err := outputCursor.ValidateFor(query, now); err != nil {
		return contract.TimelinePage{}, err
	}
	if outputCursor.AfterSequence != previous {
		return contract.TimelinePage{}, contract.NewError(contract.ErrRevisionConflict, "timeline_cursor", "Reader cursor does not follow the exact page watermark")
	}
	return cloneTimelinePage(page), nil
}

func validateTimelineInputCursor(query contract.TimelineQuery, now time.Time) (uint64, error) {
	if query.Cursor == "" {
		return 0, nil
	}
	cursor, err := contract.DecodeTimelineCursor(query.Cursor)
	if err != nil {
		return 0, err
	}
	if err := cursor.ValidateFor(query, now); err != nil {
		return 0, err
	}
	return cursor.AfterSequence, nil
}

func validateAndCloneArtifactRelations(facts []contract.ArtifactRelationFactV1, matches func(contract.ArtifactRelationFactV1) bool) ([]contract.ArtifactRelationFactV1, error) {
	result := make([]contract.ArtifactRelationFactV1, len(facts))
	for i := range facts {
		if err := facts[i].Validate(); err != nil || !matches(facts[i]) {
			return nil, contract.NewError(contract.ErrRevisionConflict, "artifact_relation_index", "reader returned a mismatched Artifact Relation")
		}
		result[i] = facts[i].Clone()
	}
	return result, nil
}

func unavailable(capability string) error {
	return contract.NewError(contract.ErrUnsupported, "sdk", capability+" reader is not configured")
}

func nilOrTypedNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
