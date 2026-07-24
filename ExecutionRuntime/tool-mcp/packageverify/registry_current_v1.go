package packageverify

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type ClockV1 interface{ Now() time.Time }

type PackageRegistryCurrentReaderV1 struct {
	registry *registry.Registry
	clock    ClockV1
}

func NewPackageRegistryCurrentReaderV1(source *registry.Registry, clock ClockV1) (*PackageRegistryCurrentReaderV1, error) {
	if source == nil || isNilV1(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "package Registry current dependencies are nil")
	}
	return &PackageRegistryCurrentReaderV1{registry: source, clock: clock}, nil
}

func (r *PackageRegistryCurrentReaderV1) InspectCurrentToolPackageRegistryV1(ctx context.Context, exact toolcontract.ToolPackageRegistryCurrentRefV1) (toolcontract.ToolPackageRegistryCurrentProjectionV1, error) {
	if isNilV1(ctx) || r == nil || r.registry == nil || isNilV1(r.clock) || exact.Validate() != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "package Registry current inspect request is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "package Registry current inspect requires caller deadline")
	}
	now := r.clock.Now()
	if now.IsZero() || !now.Before(deadline) {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "package Registry current clock or deadline is invalid")
	}
	snapshot, err := r.registry.Snapshot()
	if err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, err
	}
	var candidate toolcontract.ObjectRef
	matches := 0
	for _, record := range snapshot.Records {
		if record.Kind != "package" || record.RegistryRevision != exact.Revision {
			continue
		}
		object := toolcontract.ObjectRef{ID: record.ID, Revision: record.ObjectRevision, Digest: record.ObjectDigest}
		id, deriveErr := toolcontract.DeriveToolPackageRegistryCurrentIDV1(object)
		if deriveErr == nil && id == exact.ID {
			candidate = object
			matches++
		}
	}
	if matches != 1 {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "exact package Registry current source was not found")
	}
	manifest, record, ok := r.registry.InspectExactPackageRecordV1(candidate, exact.Revision, exact.Digest)
	if !ok {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "package Registry current source drifted")
	}
	source, err := toolcontract.SealToolPackageRegistryRecordSourceV1(toolcontract.ToolPackageRegistryRecordSourceV1{
		Kind: record.Kind, ID: record.ID, ObjectRevision: record.ObjectRevision, ObjectDigest: record.ObjectDigest,
		State: string(record.State), RegistryRevision: record.RegistryRevision, UpdatedUnixNano: record.UpdatedUnixNano,
	})
	if err != nil || source.Digest != exact.Digest {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "package Registry current source digest drifted")
	}
	projection, err := toolcontract.SealToolPackageRegistryCurrentProjectionV1(toolcontract.ToolPackageRegistryCurrentProjectionV1{
		Ref: exact, Source: source, Package: candidate, Manifest: manifest, State: string(record.State),
		RegistryRevision: record.RegistryRevision, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: deadline.UnixNano(),
	})
	if err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, err
	}
	fresh := r.clock.Now()
	if fresh.Before(now) {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "package Registry current clock regressed after read")
	}
	if err := projection.ValidateCurrent(exact, fresh); err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, err
	}
	return projection.Clone(), nil
}
