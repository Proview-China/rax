package contract

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	AutoReviewerOutputSchemaContractVersionV1 = "praxis.review/auto-reviewer-output-schema-v1"
	AutoReviewerOutputRubricSchemaIDV1        = "praxis.review/attestation-v1"
)

// AutoReviewerOutputSchemaDocumentV1 is an immutable Review-owned schema
// asset. Its SchemaRef content digest binds the exact bytes passed to Model;
// Rubric currentness controls whether that immutable schema may be used.
type AutoReviewerOutputSchemaDocumentV1 struct {
	ContractVersion string                   `json:"contract_version"`
	RubricSchemaID  string                   `json:"rubric_schema_id"`
	Schema          runtimeports.SchemaRefV2 `json:"schema"`
	Document        json.RawMessage          `json:"document"`
	Digest          core.Digest              `json:"digest"`
}

func (d AutoReviewerOutputSchemaDocumentV1) Clone() AutoReviewerOutputSchemaDocumentV1 {
	d.Document = append(json.RawMessage(nil), d.Document...)
	return d
}

func (d AutoReviewerOutputSchemaDocumentV1) digestValue() AutoReviewerOutputSchemaDocumentV1 {
	d = d.Clone()
	d.Digest = ""
	return d
}

func (d AutoReviewerOutputSchemaDocumentV1) Validate() error {
	if d.ContractVersion != AutoReviewerOutputSchemaContractVersionV1 || d.RubricSchemaID != AutoReviewerOutputRubricSchemaIDV1 || d.Schema.Validate() != nil || d.Schema.Namespace != "praxis.review" || d.Schema.Name != "auto-reviewer-output-draft" || d.Schema.Version != "1.0.0" || d.Schema.MediaType != "application/schema+json" || len(d.Document) == 0 || d.Schema.ContentDigest != core.DigestBytes(d.Document) || d.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "auto reviewer output schema document is incomplete")
	}
	var strict map[string]json.RawMessage
	if err := core.DecodeStrictJSON(d.Document, &strict); err != nil || strict == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "auto reviewer output schema must be one strict JSON object")
	}
	if _, err := compileAutoReviewerOutputSchemaV1(d.Document); err != nil {
		return err
	}
	want, err := core.CanonicalJSONDigest("praxis.review.auto-reviewer-output-schema", AutoReviewerOutputSchemaContractVersionV1, "AutoReviewerOutputSchemaDocumentV1", d.digestValue())
	if err != nil || want != d.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "auto reviewer output schema document digest drifted")
	}
	return nil
}

func SealAutoReviewerOutputSchemaDocumentV1(d AutoReviewerOutputSchemaDocumentV1) (AutoReviewerOutputSchemaDocumentV1, error) {
	d = d.Clone()
	d.ContractVersion = AutoReviewerOutputSchemaContractVersionV1
	d.RubricSchemaID = AutoReviewerOutputRubricSchemaIDV1
	d.Schema = runtimeports.SchemaRefV2{Namespace: "praxis.review", Name: "auto-reviewer-output-draft", Version: "1.0.0", MediaType: "application/schema+json", ContentDigest: core.DigestBytes(d.Document)}
	d.Digest = ""
	var err error
	d.Digest, err = core.CanonicalJSONDigest("praxis.review.auto-reviewer-output-schema", AutoReviewerOutputSchemaContractVersionV1, "AutoReviewerOutputSchemaDocumentV1", d.digestValue())
	if err != nil {
		return AutoReviewerOutputSchemaDocumentV1{}, err
	}
	return d, d.Validate()
}

// ValidateDraftV1 independently validates strict JSON at the Review boundary;
// successful Model validation is only an Observation and is never trusted.
func (d AutoReviewerOutputSchemaDocumentV1) ValidateDraftV1(payload json.RawMessage) (AutoReviewerStructuredOutputV1, error) {
	if err := d.Validate(); err != nil {
		return AutoReviewerStructuredOutputV1{}, err
	}
	var draft AutoReviewerStructuredOutputDraftV1
	if err := core.DecodeStrictJSON(payload, &draft); err != nil {
		return AutoReviewerStructuredOutputV1{}, err
	}
	schema, err := compileAutoReviewerOutputSchemaV1(d.Document)
	if err != nil {
		return AutoReviewerStructuredOutputV1{}, err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil || schema.Validate(value) != nil {
		return AutoReviewerStructuredOutputV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "auto reviewer output does not satisfy the exact Review schema")
	}
	return SealAutoReviewerStructuredOutputDraftV1(draft)
}

func (d AutoReviewerOutputSchemaDocumentV1) ValidateForRubricV1(rubric RubricDefinitionV1, expected runtimeports.SchemaRefV2) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if err := rubric.Validate(); err != nil {
		return err
	}
	if rubric.OutputSchema.SchemaID != d.RubricSchemaID || expected != d.Schema {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "auto reviewer output schema drifted from the exact Rubric or Attempt")
	}
	return nil
}

func compileAutoReviewerOutputSchemaV1(document json.RawMessage) (*jsonschema.Schema, error) {
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "auto reviewer JSON schema cannot be decoded")
	}
	if err := rejectExternalAutoReviewerSchemaRefsV1(value); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	const resource = "urn:praxis:review:auto-reviewer-output-schema:v1"
	if err := compiler.AddResource(resource, value); err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "auto reviewer JSON schema resource is invalid")
	}
	schema, err := compiler.Compile(resource)
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "auto reviewer JSON schema cannot be compiled")
	}
	return schema, nil
}

func rejectExternalAutoReviewerSchemaRefsV1(value any) error {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if key == "$id" {
				return core.NewError(core.ErrorForbidden, core.ReasonInvalidReference, "auto reviewer schema cannot redefine its resource ID")
			}
			if key == "$ref" || key == "$dynamicRef" || key == "$recursiveRef" {
				ref, ok := child.(string)
				if !ok || !strings.HasPrefix(ref, "#") {
					return core.NewError(core.ErrorForbidden, core.ReasonInvalidReference, "auto reviewer schema references must stay inside the sealed document")
				}
			}
			if err := rejectExternalAutoReviewerSchemaRefsV1(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range node {
			if err := rejectExternalAutoReviewerSchemaRefsV1(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func BuiltinAutoReviewerOutputSchemaDocumentV1() (AutoReviewerOutputSchemaDocumentV1, error) {
	return SealAutoReviewerOutputSchemaDocumentV1(AutoReviewerOutputSchemaDocumentV1{Document: json.RawMessage(autoReviewerOutputSchemaJSONV1)})
}

const autoReviewerOutputSchemaJSONV1 = `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "type":"object",
  "additionalProperties":false,
  "required":["resolution","reason_codes","findings","evidence"],
  "properties":{
    "resolution":{"type":"string","enum":["accept","conditional_acceptance","escalate_human","insufficient_evidence","reject","request_changes"]},
    "reason_codes":{"type":"array","minItems":1,"maxItems":128,"uniqueItems":true,"items":{"type":"string","minLength":1}},
    "findings":{"type":"array","maxItems":64,"items":{"type":"object","additionalProperties":false,"required":["category","priority","anchor","claim","impact","evidence"],"properties":{"category":{"type":"string","minLength":1},"priority":{"type":"string","minLength":1},"anchor":{"type":"string","minLength":1},"claim":{"type":"string","minLength":1},"impact":{"type":"string","minLength":1},"evidence":{"type":"array","minItems":1,"maxItems":128,"uniqueItems":true,"items":{"$ref":"#/$defs/evidence"}}}}},
    "evidence":{"type":"array","minItems":1,"maxItems":128,"uniqueItems":true,"items":{"$ref":"#/$defs/evidence"}},
    "conditions":{"type":"array","minItems":1,"maxItems":64,"items":{"$ref":"#/$defs/condition"}}
  },
  "allOf":[
    {"if":{"properties":{"resolution":{"const":"conditional_acceptance"}},"required":["resolution"]},"then":{"required":["conditions"]},"else":{"not":{"required":["conditions"]}}}
  ],
  "$defs":{
    "digest":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"},
    "name":{"type":"string","pattern":"^[a-z0-9][a-z0-9._/-]*$"},
    "evidence":{"type":"object","additionalProperties":false,"required":["ref","classification","digest"],"properties":{"ref":{"type":"string","minLength":1},"classification":{"$ref":"#/$defs/name"},"digest":{"$ref":"#/$defs/digest"}}},
    "schema":{"type":"object","additionalProperties":false,"required":["namespace","name","version","media_type","content_digest"],"properties":{"namespace":{"type":"string","minLength":1},"name":{"type":"string","minLength":1},"version":{"type":"string","minLength":1},"media_type":{"type":"string","minLength":1},"content_digest":{"$ref":"#/$defs/digest"}}},
    "component_binding":{"type":"object","additionalProperties":false,"required":["binding_set_id","binding_set_revision","component_id","manifest_digest","artifact_digest","capability"],"properties":{"binding_set_id":{"type":"string","minLength":1},"binding_set_revision":{"type":"integer","minimum":1},"component_id":{"$ref":"#/$defs/name"},"manifest_digest":{"$ref":"#/$defs/digest"},"artifact_digest":{"$ref":"#/$defs/digest"},"capability":{"$ref":"#/$defs/name"}}},
    "authority":{"type":"object","additionalProperties":false,"required":["ref","digest","revision","epoch"],"properties":{"ref":{"type":"string","minLength":1},"digest":{"$ref":"#/$defs/digest"},"revision":{"type":"integer","minimum":1},"epoch":{"type":"integer","minimum":1}}},
    "condition":{"type":"object","additionalProperties":false,"required":["id","revision","schema","constraint_digest","satisfaction_owner","scope_digest","authority","expires_unix_nano"],"properties":{"id":{"$ref":"#/$defs/name"},"revision":{"type":"integer","minimum":1},"schema":{"$ref":"#/$defs/schema"},"constraint_digest":{"$ref":"#/$defs/digest"},"satisfaction_owner":{"$ref":"#/$defs/component_binding"},"scope_digest":{"$ref":"#/$defs/digest"},"authority":{"$ref":"#/$defs/authority"},"expires_unix_nano":{"type":"integer","minimum":1}}}
  }
}`
