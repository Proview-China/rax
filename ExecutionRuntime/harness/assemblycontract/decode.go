package assemblycontract

import "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"

// DecodePortSpecV1 is intentionally strict. In particular, the removed
// run_requirement_ref field is not migrated implicitly because Run Start and
// Run Settlement requirements are different contracts.
func DecodePortSpecV1(payload []byte) (PortSpecV1, error) {
	var value PortSpecV1
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return PortSpecV1{}, err
	}
	if err := value.Validate(); err != nil {
		return PortSpecV1{}, err
	}
	return value, nil
}
