package loader

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type VerifiedAgentPackageClosureV1 struct {
	Package     contract.AgentPackageV1
	Publication assemblycontract.AssemblyPublicationBundleV2
}

type LoaderV1 struct {
	packages     ports.AgentPackageExactReaderV1
	publications ports.HarnessAssemblyPublicationHistoricalReaderV2
}

func NewV1(packages ports.AgentPackageExactReaderV1, publications ports.HarnessAssemblyPublicationHistoricalReaderV2) (*LoaderV1, error) {
	if nilInterface(packages) || nilInterface(publications) {
		return nil, invalid("agent package loader requires exact package and Harness historical publication readers")
	}
	return &LoaderV1{packages: packages, publications: publications}, nil
}

func (l *LoaderV1) LoadExactV1(ctx context.Context, ref contract.AgentPackageRefV1) (VerifiedAgentPackageClosureV1, error) {
	if l == nil || nilInterface(l.packages) || nilInterface(l.publications) {
		return VerifiedAgentPackageClosureV1{}, invalid("agent package loader is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return VerifiedAgentPackageClosureV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "agent package load requires live context")
	}
	if err := ref.Validate(); err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	pkg, err := l.packages.InspectExactAgentPackageV1(ctx, ref)
	if err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	if err = pkg.Validate(); err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	if pkg.RefV1() != ref {
		return VerifiedAgentPackageClosureV1{}, drift("package reader returned a different exact ref")
	}

	bundle, err := l.publications.InspectAssemblyPublicationHistoricalV2(ctx, pkg.Lock.PublicationRef)
	if err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	if err = bundle.Validate(); err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	publicationRef := assemblycontract.AssemblyPublicationRefV2{PublicationID: bundle.Publication.PublicationID, Revision: bundle.Publication.Revision, Digest: bundle.Publication.Digest}
	if publicationRef != pkg.Lock.PublicationRef {
		return VerifiedAgentPackageClosureV1{}, drift("Harness historical reader returned a different exact Publication ref")
	}
	artifacts := bundle.Publication.Artifacts
	if artifacts.Generation != pkg.Lock.GenerationRef || artifacts.Manifest != pkg.Lock.ManifestRef || artifacts.Graph != pkg.Lock.GraphRef || artifacts.Handoff != pkg.Lock.HandoffRef {
		return VerifiedAgentPackageClosureV1{}, drift("Package lock and Harness Publication artifact refs diverged")
	}
	if bundle.Publication.InputDigest != pkg.Lock.AssemblyInputDigest || bundle.Generation.InputDigest != pkg.Lock.AssemblyInputDigest || bundle.Manifest.InputDigest != pkg.Lock.AssemblyInputDigest || bundle.Graph.InputDigest != pkg.Lock.AssemblyInputDigest {
		return VerifiedAgentPackageClosureV1{}, drift("Package lock and Harness Publication input closure diverged")
	}
	if bundle.Generation.CompilerVersion != pkg.Lock.HarnessCompilerVersion || bundle.Generation.CreatedUnixNano != pkg.Lock.FrozenUnixNano {
		return VerifiedAgentPackageClosureV1{}, drift("Package lock and Harness Publication compiler closure diverged")
	}
	if bundle.Handoff.GenerationRef != pkg.Lock.GenerationRef || bundle.Handoff.ManifestDigest != pkg.Lock.ManifestRef.Digest || bundle.Handoff.GraphDigest != pkg.Lock.GraphRef.Digest {
		return VerifiedAgentPackageClosureV1{}, drift("Package lock and Harness Publication handoff closure diverged")
	}
	return clone(VerifiedAgentPackageClosureV1{Package: pkg, Publication: bundle}), nil
}

func invalid(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, message)
}
func drift(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, message)
}
func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return reflected.IsNil()
	}
	return false
}
func clone[T any](value T) T {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result T
	if json.Unmarshal(raw, &result) != nil {
		return value
	}
	return result
}
