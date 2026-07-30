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
	Package    contract.AgentPackageV1
	Generation ports.AssemblyGenerationArtifactV1
	Manifest   ports.AssemblyManifestArtifactV1
	Graph      ports.CompiledHarnessGraphArtifactV1
	Handoff    ports.AssemblyHandoffArtifactV1
}

type LoaderV1 struct {
	packages  ports.AgentPackageExactReaderV1
	artifacts ports.HarnessArtifactExactReaderV1
}

func NewV1(packages ports.AgentPackageExactReaderV1, artifacts ports.HarnessArtifactExactReaderV1) (*LoaderV1, error) {
	if nilInterface(packages) || nilInterface(artifacts) {
		return nil, invalid("agent package loader requires exact package and Harness artifact readers")
	}
	return &LoaderV1{packages: packages, artifacts: artifacts}, nil
}

func (l *LoaderV1) LoadExactV1(ctx context.Context, ref contract.AgentPackageRefV1) (VerifiedAgentPackageClosureV1, error) {
	if l == nil || nilInterface(l.packages) || nilInterface(l.artifacts) {
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

	generation, err := l.artifacts.InspectExactGenerationV1(ctx, pkg.Lock.GenerationRef)
	if err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	manifest, err := l.artifacts.InspectExactManifestV1(ctx, pkg.Lock.ManifestRef)
	if err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	graph, err := l.artifacts.InspectExactGraphV1(ctx, pkg.Lock.GraphRef)
	if err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	handoff, err := l.artifacts.InspectExactHandoffV1(ctx, pkg.Lock.HandoffRef)
	if err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}

	if err = validateGeneration(pkg.Lock.GenerationRef, generation); err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	if err = validateManifest(pkg.Lock.ManifestRef, manifest); err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	if err = validateGraph(pkg.Lock.GraphRef, graph); err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	if err = validateHandoff(pkg.Lock.HandoffRef, handoff); err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}

	g, m, graphValue, h := generation.Value, manifest.Value, graph.Value, handoff.Value
	if g.InputDigest != pkg.Lock.AssemblyInputDigest || m.InputDigest != pkg.Lock.AssemblyInputDigest || graphValue.InputDigest != pkg.Lock.AssemblyInputDigest || g.CompilerVersion != pkg.Lock.HarnessCompilerVersion {
		return VerifiedAgentPackageClosureV1{}, drift("Package lock and Harness generation inputs diverged")
	}
	if g.ManifestDigest != m.Digest || g.GraphDigest != graphValue.Digest || h.GenerationRef != pkg.Lock.GenerationRef || h.ManifestDigest != m.Digest || h.GraphDigest != graphValue.Digest {
		return VerifiedAgentPackageClosureV1{}, drift("Harness generation, manifest, graph and handoff do not form one closure")
	}
	if m.CatalogDigest != graphValue.CatalogDigest || h.CatalogDigest != m.CatalogDigest {
		return VerifiedAgentPackageClosureV1{}, drift("Harness artifact catalog digests diverged")
	}
	return clone(VerifiedAgentPackageClosureV1{Package: pkg, Generation: generation, Manifest: manifest, Graph: graph, Handoff: handoff}), nil
}

func validateGeneration(ref assemblycontract.ObjectRefV1, artifact ports.AssemblyGenerationArtifactV1) error {
	value := artifact.Value
	if artifact.Ref != ref || value.GenerationID != ref.ID || value.Revision != ref.Revision || value.Digest != ref.Digest || value.ContractVersion != assemblycontract.ContractVersionV1 || value.CompilerVersion != assemblycontract.CompilerVersionV1 || value.State != assemblycontract.AssemblyStateSealedV1 || value.CreatedUnixNano <= 0 {
		return drift("Harness generation exact identity or discriminator drifted")
	}
	digest, err := assemblycontract.GenerationDigestV1(value)
	if err != nil {
		return err
	}
	if digest != value.Digest {
		return digestDrift("Harness generation body digest drifted")
	}
	return nil
}

func validateManifest(ref assemblycontract.ObjectRefV1, artifact ports.AssemblyManifestArtifactV1) error {
	value := artifact.Value
	if artifact.Ref != ref || value.Digest != ref.Digest || value.ContractVersion != assemblycontract.ContractVersionV1 {
		return drift("Harness manifest exact identity or discriminator drifted")
	}
	digest, err := assemblycontract.ManifestDigestV1(value)
	if err != nil {
		return err
	}
	if digest != value.Digest {
		return digestDrift("Harness manifest body digest drifted")
	}
	return nil
}

func validateGraph(ref assemblycontract.ObjectRefV1, artifact ports.CompiledHarnessGraphArtifactV1) error {
	value := artifact.Value
	if artifact.Ref != ref || value.Digest != ref.Digest || value.ContractVersion != assemblycontract.ContractVersionV1 {
		return drift("Harness graph exact identity or discriminator drifted")
	}
	digest, err := assemblycontract.GraphDigestV1(value)
	if err != nil {
		return err
	}
	if digest != value.Digest {
		return digestDrift("Harness graph body digest drifted")
	}
	return nil
}

func validateHandoff(ref assemblycontract.ObjectRefV1, artifact ports.AssemblyHandoffArtifactV1) error {
	value := artifact.Value
	if artifact.Ref != ref || value.Digest != ref.Digest {
		return drift("Harness handoff exact identity drifted")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return nil
}

func invalid(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, message)
}
func drift(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, message)
}
func digestDrift(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, message)
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
