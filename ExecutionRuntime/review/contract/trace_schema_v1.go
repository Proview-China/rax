package contract

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// traceFactJSONSchemaV1 is the immutable wire schema for TraceFactV1. The
// SchemaRef content digest is computed from these exact bytes; it is not a
// digest of a label or type name.
const traceFactJSONSchemaV1 = `{"$id":"praxis.review/trace-fact@1.0.0","additionalProperties":false,"properties":{"case_id":{"type":"string"},"case_revision":{"minimum":1,"type":"integer"},"causation_id":{"type":"string"},"contract_version":{"const":"1.0.0"},"correlation_id":{"type":"string"},"created_unix_nano":{"minimum":1,"type":"integer"},"digest":{"type":"string"},"event":{"type":"string"},"fact_refs":{"items":{"type":"string"},"type":"array"},"id":{"type":"string"},"revision":{"const":1},"source_epoch":{"minimum":1,"type":"integer"},"source_id":{"type":"string"},"source_sequence":{"minimum":1,"type":"integer"},"target_digest":{"type":"string"},"target_id":{"type":"string"},"target_revision":{"minimum":1,"type":"integer"},"tenant_id":{"type":"string"},"updated_unix_nano":{"minimum":1,"type":"integer"}},"required":["contract_version","tenant_id","id","revision","digest","created_unix_nano","updated_unix_nano","case_id","case_revision","target_id","target_revision","target_digest","event","source_id","source_epoch","source_sequence","causation_id","correlation_id","fact_refs"],"type":"object"}`

func TraceFactJSONSchemaV1() []byte {
	return append([]byte(nil), traceFactJSONSchemaV1...)
}

func TraceFactSchemaRefV1() runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{
		Namespace: "praxis.review",
		Name:      "trace-fact",
		Version:   "1.0.0",
		MediaType: "application/json",
		ContentDigest: core.DigestBytes(
			[]byte(traceFactJSONSchemaV1),
		),
	}
}
