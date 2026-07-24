// Package cli defines Continuity-owned command descriptors and argument
// mapping. Root CLI registration, endpoint selection and credentials remain
// outside this package.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	continuitycontract "github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ContractVersionV1 = "praxis.continuity.cli/v1"
	maxInputBytesV1   = 1 << 20
)

type GovernedWorkflowPortV1 interface {
	SubmitGovernedWorkflow(context.Context, appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error)
	InspectGovernedWorkflow(context.Context, appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error)
}

type TimelineReadPortV1 interface {
	InspectEvent(context.Context, string) (continuitycontract.TimelineEventRecord, error)
	WatchTimeline(context.Context, continuitycontract.TimelineQuery) (continuitycontract.TimelinePage, error)
}

type CheckpointReadPortV1 interface {
	InspectCheckpointManifest(context.Context, continuitycontract.CheckpointManifestRefV2) (continuitycontract.CheckpointManifestFactV2, error)
}

type TimelineShowRequestV1 struct {
	EvidenceRecordRef string `json:"evidence_record_ref"`
}

type TimelineWatchRequestV1 struct {
	Query  continuitycontract.TimelineQuery `json:"query"`
	Cursor string                           `json:"cursor,omitempty"`
}

type OutputV1 struct {
	ContractVersion string                                       `json:"contract_version"`
	Inspection      *appcontract.ContinuityWorkflowInspectionV1  `json:"inspection,omitempty"`
	Event           *continuitycontract.TimelineEventRecord      `json:"event,omitempty"`
	Page            *continuitycontract.TimelinePage             `json:"page,omitempty"`
	Checkpoint      *continuitycontract.CheckpointManifestFactV2 `json:"checkpoint,omitempty"`
}

type RunnerV1 struct {
	workflows  GovernedWorkflowPortV1
	timeline   TimelineReadPortV1
	checkpoint CheckpointReadPortV1
}

func NewRunnerV1(workflows GovernedWorkflowPortV1) (*RunnerV1, error) {
	if nilCLIV1(workflows) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Continuity governed workflow port is required")
	}
	return &RunnerV1{workflows: workflows}, nil
}

func NewRunnerWithReadersV1(workflows GovernedWorkflowPortV1, timeline TimelineReadPortV1, checkpoint CheckpointReadPortV1) (*RunnerV1, error) {
	if nilCLIV1(workflows) || nilCLIV1(timeline) || nilCLIV1(checkpoint) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Continuity workflow and read ports are required")
	}
	return &RunnerV1{workflows: workflows, timeline: timeline, checkpoint: checkpoint}, nil
}

func (r *RunnerV1) RunV1(ctx context.Context, args []string, input io.Reader, output io.Writer) error {
	if r == nil || nilCLIV1(r.workflows) || nilCLIV1(ctx) || nilCLIV1(input) || nilCLIV1(output) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Continuity CLI dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch strings.Join(args, " ") {
	case "timeline show":
		return r.runTimelineShowV1(ctx, input, output)
	case "timeline watch":
		return r.runTimelineWatchV1(ctx, input, output)
	case "checkpoint inspect":
		return r.runCheckpointInspectV1(ctx, input, output)
	}
	kind, inspect, ok := commandV1(args)
	if !ok {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Continuity CLI command is not registered")
	}
	var request appcontract.ContinuityWorkflowRequestV1
	if err := decodeInputV1(input, &request); err != nil {
		return err
	}
	if !inspect && request.Kind != kind {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMismatch, "Continuity CLI command and request kind differ")
	}
	var result appcontract.ContinuityWorkflowInspectionV1
	var err error
	if inspect {
		result, err = r.workflows.InspectGovernedWorkflow(ctx, request)
	} else {
		result, err = r.workflows.SubmitGovernedWorkflow(ctx, request)
	}
	if err != nil {
		return err
	}
	if err := result.ValidateFor(request); err != nil {
		return err
	}
	return writeOutputV1(output, OutputV1{ContractVersion: ContractVersionV1, Inspection: &result})
}

func (r *RunnerV1) runTimelineShowV1(ctx context.Context, input io.Reader, output io.Writer) error {
	if nilCLIV1(r.timeline) {
		return unsupportedReadCommandV1()
	}
	var request TimelineShowRequestV1
	if err := decodeInputV1(input, &request); err != nil {
		return err
	}
	if err := continuitycontract.ValidateToken("evidence_record_ref", request.EvidenceRecordRef); err != nil {
		return err
	}
	record, err := r.timeline.InspectEvent(ctx, request.EvidenceRecordRef)
	if err != nil {
		return err
	}
	if err := record.Validate(); err != nil || record.EvidenceRecordRef != request.EvidenceRecordRef {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "timeline show returned another Event")
	}
	clone := record.Clone()
	return writeOutputV1(output, OutputV1{ContractVersion: ContractVersionV1, Event: &clone})
}

func (r *RunnerV1) runTimelineWatchV1(ctx context.Context, input io.Reader, output io.Writer) error {
	if nilCLIV1(r.timeline) {
		return unsupportedReadCommandV1()
	}
	var request TimelineWatchRequestV1
	if err := decodeInputV1(input, &request); err != nil {
		return err
	}
	request.Query.Cursor = request.Cursor
	if err := request.Query.Validate(); err != nil {
		return err
	}
	page, err := r.timeline.WatchTimeline(ctx, request.Query)
	if err != nil {
		return err
	}
	return writeOutputV1(output, OutputV1{ContractVersion: ContractVersionV1, Page: &page})
}

func (r *RunnerV1) runCheckpointInspectV1(ctx context.Context, input io.Reader, output io.Writer) error {
	if nilCLIV1(r.checkpoint) {
		return unsupportedReadCommandV1()
	}
	var ref continuitycontract.CheckpointManifestRefV2
	if err := decodeInputV1(input, &ref); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	manifest, err := r.checkpoint.InspectCheckpointManifest(ctx, ref)
	if err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil || !manifest.Ref().Exact().Equal(ref.Exact()) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint inspect returned another Manifest")
	}
	clone := manifest.Clone()
	return writeOutputV1(output, OutputV1{ContractVersion: ContractVersionV1, Checkpoint: &clone})
}

func decodeInputV1(input io.Reader, target any) error {
	payload, err := io.ReadAll(io.LimitReader(input, maxInputBytesV1+1))
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > maxInputBytesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Continuity CLI request is empty or too large")
	}
	return core.DecodeStrictJSON(payload, target)
}

func writeOutputV1(output io.Writer, value OutputV1) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Continuity CLI output cannot be encoded")
	}
	_, err := output.Write(encoded.Bytes())
	return err
}

func unsupportedReadCommandV1() error {
	return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Continuity CLI read capability is not configured")
}

func commandV1(args []string) (appcontract.ContinuityWorkflowKindV1, bool, bool) {
	switch strings.Join(args, " ") {
	case "timeline project":
		return appcontract.ContinuityTimelineProjectV1, false, true
	case "checkpoint create":
		return appcontract.ContinuityCheckpointCreateV1, false, true
	case "fork":
		return appcontract.ContinuityForkV1, false, true
	case "rewind plan":
		return appcontract.ContinuityRewindPlanV1, false, true
	case "restore":
		return appcontract.ContinuityRestoreV1, false, true
	case "artifact attach":
		return appcontract.ContinuityArtifactAttachV1, false, true
	case "retention resolve":
		return appcontract.ContinuityRetentionResolveV1, false, true
	case "workflow inspect":
		return "", true, true
	default:
		return "", false, false
	}
}

func nilCLIV1(value any) bool {
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
