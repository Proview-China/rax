package contract

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func SealDefinitionCurrentV1(value DefinitionCurrentV1) (DefinitionCurrentV1, error) {
	value.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(DigestDomainV1, DigestVersionV1, "DefinitionCurrentV1", value)
	if err != nil {
		return DefinitionCurrentV1{}, err
	}
	value.ProjectionDigest = digest
	if err := value.Validate(); err != nil {
		return DefinitionCurrentV1{}, err
	}
	return value, nil
}
