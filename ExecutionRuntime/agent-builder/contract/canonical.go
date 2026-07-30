package contract

import (
	"encoding/json"
	"sort"
	"strings"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const digestDomainV1 = "praxis.agent.package"

func packageIDV1(lockDigest core.Digest) string {
	return "agent-package-" + strings.TrimPrefix(string(lockDigest), "sha256:")[:24]
}

func clone[T any](value T) T {
	payload, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result T
	if json.Unmarshal(payload, &result) != nil {
		return value
	}
	return result
}

func normalizeLockV1(value AgentPackageLockManifestV1) AgentPackageLockManifestV1 {
	value = clone(value)
	sort.Slice(value.ComponentReleaseRefs, func(i, j int) bool {
		if value.ComponentReleaseRefs[i].ReleaseID != value.ComponentReleaseRefs[j].ReleaseID {
			return value.ComponentReleaseRefs[i].ReleaseID < value.ComponentReleaseRefs[j].ReleaseID
		}
		return value.ComponentReleaseRefs[i].Revision < value.ComponentReleaseRefs[j].Revision
	})
	if value.ComponentReleaseRefs == nil {
		value.ComponentReleaseRefs = []assemblercontract.ComponentReleaseRefV1{}
	}
	return value
}

func LockDigestV1(value AgentPackageLockManifestV1) (core.Digest, error) {
	value = normalizeLockV1(value)
	value.Digest = ""
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, LockObjectKindV1, value)
}

func SealLockManifestV1(value AgentPackageLockManifestV1) (AgentPackageLockManifestV1, error) {
	value = normalizeLockV1(value)
	value.ContractVersion = ContractVersionV1
	value.SchemaVersion = SchemaVersionV1
	value.ObjectKind = LockObjectKindV1
	value.Digest = ""
	digest, err := LockDigestV1(value)
	if err != nil {
		return AgentPackageLockManifestV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func PackageDigestV1(value AgentPackageV1) (core.Digest, error) {
	value = clone(value)
	value.Digest = ""
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, PackageObjectKindV1, value)
}

func SealPackageV1(value AgentPackageV1) (AgentPackageV1, error) {
	if err := value.Lock.Validate(); err != nil {
		return AgentPackageV1{}, err
	}
	value = clone(value)
	value.ContractVersion = ContractVersionV1
	value.SchemaVersion = SchemaVersionV1
	value.ObjectKind = PackageObjectKindV1
	value.Revision = 1
	value.CreatedUnixNano = value.Lock.FrozenUnixNano
	value.PackageID = packageIDV1(value.Lock.Digest)
	value.Digest = ""
	digest, err := PackageDigestV1(value)
	if err != nil {
		return AgentPackageV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func CloneLockManifestV1(value AgentPackageLockManifestV1) AgentPackageLockManifestV1 {
	return clone(value)
}
func ClonePackageV1(value AgentPackageV1) AgentPackageV1 { return clone(value) }
