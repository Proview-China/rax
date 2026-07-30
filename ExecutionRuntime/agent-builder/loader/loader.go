package loader

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type LoaderV1 struct {
	packages     ports.AgentPackageExactReaderV1
	publications ports.HarnessAssemblyPublicationHistoricalReaderV2
}

// VerifiedAgentPackageClosureV1 preserves the loader package's original
// explicit result type while the authoritative contract now lives in contract.
type VerifiedAgentPackageClosureV1 = contract.VerifiedAgentPackageClosureV1

func NewV1(packages ports.AgentPackageExactReaderV1, publications ports.HarnessAssemblyPublicationHistoricalReaderV2) (*LoaderV1, error) {
	if nilInterface(packages) || nilInterface(publications) {
		return nil, invalid("agent package loader requires exact package and Harness historical publication readers")
	}
	return &LoaderV1{packages: packages, publications: publications}, nil
}

func (l *LoaderV1) LoadExactV1(ctx context.Context, ref contract.AgentPackageRefV1) (VerifiedAgentPackageClosureV1, error) {
	return l.LoadVerifiedAgentPackageClosureV1(ctx, ref)
}

func (l *LoaderV1) LoadVerifiedAgentPackageClosureV1(ctx context.Context, ref contract.AgentPackageRefV1) (contract.VerifiedAgentPackageClosureV1, error) {
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
	verified, err := contract.SealVerifiedAgentPackageClosureV1(pkg, bundle)
	if err != nil {
		return VerifiedAgentPackageClosureV1{}, err
	}
	return contract.CloneVerifiedAgentPackageClosureV1(verified), nil
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
