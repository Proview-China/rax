package packageverify

import (
	"context"
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type VerifiedPackageAdmissionRegistryV1 interface {
	AdmitVerifiedPackageV1(toolcontract.ToolPackageVerifiedAdmissionRequestV1, time.Time) (registry.Record, error)
	InspectVerifiedPackageAdmissionV1(toolcontract.ObjectRef, toolcontract.ToolPackageVerificationCurrentRefV1) (registry.Record, bool)
}

func (s *ServiceV1) AdmitPackageV1(ctx context.Context, target VerifiedPackageAdmissionRegistryV1, command toolcontract.ToolPackageAdmissionCommandV1) (registry.Record, error) {
	if isNilV1(ctx) || s == nil || isNilV1(target) || isNilV1(s.config.Clock) || command.Validate() != nil {
		return registry.Record{}, packageInvalidV1("Package Admission request or dependency is invalid")
	}
	if err := ctx.Err(); err != nil {
		return registry.Record{}, err
	}
	current, err := s.config.Repository.InspectCurrentToolPackageVerificationV1(ctx, command.VerificationCurrent)
	if err != nil {
		return registry.Record{}, err
	}
	if command.ExpectedRegistryRevision != current.CurrentPackageRegistry.RegistryRevision {
		return registry.Record{}, packageConflictV1("Package Admission command changed the exact Registry CAS source")
	}
	if winner, ok := target.InspectVerifiedPackageAdmissionV1(current.Fact.Package, command.VerificationCurrent); ok {
		return winner, nil
	}
	nowS1 := s.config.Clock.Now()
	if err := current.ValidateCurrent(command.VerificationCurrent, nowS1); err != nil {
		return recoverPackageAdmissionV1(target, current, err)
	}
	observation, err := s.config.Repository.InspectExactToolPackageVerificationObservationV1(ctx, current.Fact.Observation)
	if err != nil {
		return recoverPackageAdmissionV1(target, current, err)
	}
	request := toolcontract.ToolPackageVerifyRequestV1{
		ContractVersion: toolcontract.PackageVerificationContractVersionV1,
		Subject:         observation.Request.Subject, TrustPolicyCurrent: current.TrustPolicy.Ref,
		RequestedExpiresUnixNano: current.ExpiresUnixNano,
	}
	bounded, cancel, err := boundedContextV1(ctx, current.ExpiresUnixNano)
	if err != nil {
		return recoverPackageAdmissionV1(target, current, err)
	}
	defer cancel()
	packageS1, trustS1, _, materialsS1, err := s.readClosureV1(bounded, request, nowS1)
	if err != nil {
		return recoverPackageAdmissionV1(target, current, err)
	}
	nowS2 := s.config.Clock.Now()
	if nowS2.Before(nowS1) {
		return recoverPackageAdmissionV1(target, current, packageClockV1("Package Admission clock regressed before S2"))
	}
	packageS2, trustS2, _, materialsS2, err := s.readClosureV1(bounded, request, nowS2)
	if err != nil {
		return recoverPackageAdmissionV1(target, current, err)
	}
	if !samePackageCurrentSourceV1(packageS1, packageS2) || trustS1 != trustS2 || !equalMaterialClosureV1(materialsS1, materialsS2) {
		return recoverPackageAdmissionV1(target, current, packageConflictV1("Package Admission S1/S2 closure drifted"))
	}
	actual := s.config.Clock.Now()
	if actual.Before(nowS2) {
		return recoverPackageAdmissionV1(target, current, packageClockV1("Package Admission clock regressed at CAS"))
	}
	if err := current.ValidateCurrent(command.VerificationCurrent, actual); err != nil {
		return recoverPackageAdmissionV1(target, current, err)
	}
	requestCAS := toolcontract.ToolPackageVerifiedAdmissionRequestV1{
		ContractVersion: toolcontract.PackageVerificationContractVersionV1,
		PackageCurrent:  packageS2, VerificationCurrent: current,
		ExpectedRegistryRevision: command.ExpectedRegistryRevision,
	}
	if err := requestCAS.ValidateCurrent(actual); err != nil {
		return recoverPackageAdmissionV1(target, current, err)
	}
	winner, err := target.AdmitVerifiedPackageV1(requestCAS, actual)
	if err != nil {
		if recovered, ok := target.InspectVerifiedPackageAdmissionV1(current.Fact.Package, command.VerificationCurrent); ok {
			return recovered, nil
		}
		return registry.Record{}, err
	}
	return winner, nil
}

func recoverPackageAdmissionV1(target VerifiedPackageAdmissionRegistryV1, current toolcontract.ToolPackageVerificationCurrentProjectionV1, cause error) (registry.Record, error) {
	if recovered, ok := target.InspectVerifiedPackageAdmissionV1(current.Fact.Package, current.Ref); ok {
		return recovered, nil
	}
	return registry.Record{}, cause
}

func equalMaterialClosureV1(left, right materialClosureV1) bool {
	return string(left.ociManifest) == string(right.ociManifest) && string(left.packageArtifact) == string(right.packageArtifact) && string(left.sigstoreBundle) == string(right.sigstoreBundle) && string(left.inTotoStatement) == string(right.inTotoStatement) && string(left.trustedRoot) == string(right.trustedRoot) && string(left.policyDocument) == string(right.policyDocument)
}
