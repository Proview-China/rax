package effect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

type rejectingSchemaLoader struct{}

func (rejectingSchemaLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("%w: %s", ErrExternalSchemaRef, url)
}

type StructuredValidation struct {
	Effect       union.EffectRecord
	Verification union.VerificationRecord
}

type StructuredMechanism struct {
	Kind      union.StructuredOutputMechanism
	Origin    union.CapabilityOrigin
	Fidelity  union.SemanticFidelity
	Transport string
}

type StructuredRepairer interface {
	Repair(context.Context, int, []byte, json.RawMessage) ([]byte, error)
}

type StructuredRepairFunc func(context.Context, int, []byte, json.RawMessage) ([]byte, error)

func (function StructuredRepairFunc) Repair(ctx context.Context, attempt int, raw []byte, schema json.RawMessage) ([]byte, error) {
	if function == nil {
		return nil, fmt.Errorf("%w: repair function is nil", ErrInvalidPolicy)
	}
	return function(ctx, attempt, raw, schema)
}

// ValidateStructuredOutputWithRepair emits no Effect until one candidate is
// strict JSON that satisfies the schema. Invalid schemas never enter a repair
// loop because changing the model output cannot repair the contract itself.
func ValidateStructuredOutputWithRepair(
	ctx context.Context,
	effectID union.EffectID,
	verificationID union.VerificationID,
	intentID union.IntentID,
	attemptID union.MechanismAttemptID,
	raw []byte,
	schemaDocument json.RawMessage,
	mechanism StructuredMechanism,
	maxRepairs int,
	repairer StructuredRepairer,
	completedAt time.Time,
) (StructuredValidation, error) {
	if ctx == nil || maxRepairs < 0 || (maxRepairs > 0 && repairer == nil) {
		return StructuredValidation{}, fmt.Errorf("%w: repair context, limit, and repairer are invalid", ErrInvalidPolicy)
	}
	candidate := append([]byte(nil), raw...)
	var lastErr error
	for repairAttempt := 0; repairAttempt <= maxRepairs; repairAttempt++ {
		if err := ctx.Err(); err != nil {
			return StructuredValidation{}, err
		}
		validated, err := ValidateStructuredOutputWithMechanism(
			effectID, verificationID, intentID, attemptID, candidate, schemaDocument, mechanism, repairAttempt, completedAt,
		)
		if err == nil {
			return validated, nil
		}
		if errors.Is(err, ErrInvalidSchema) || errors.Is(err, ErrExternalSchemaRef) {
			return StructuredValidation{}, err
		}
		if !errors.Is(err, ErrInvalidJSON) && !errors.Is(err, ErrSchemaViolation) {
			return StructuredValidation{}, err
		}
		lastErr = err
		if repairAttempt == maxRepairs {
			break
		}
		repaired, repairErr := repairer.Repair(ctx, repairAttempt+1, append([]byte(nil), candidate...), append(json.RawMessage(nil), schemaDocument...))
		if repairErr != nil {
			return StructuredValidation{}, fmt.Errorf("%w: repair attempt %d: %w", ErrRepairExhausted, repairAttempt+1, repairErr)
		}
		candidate = append(candidate[:0], repaired...)
	}
	return StructuredValidation{}, fmt.Errorf("%w: %v", ErrRepairExhausted, lastErr)
}

func ValidateStructuredOutput(
	effectID union.EffectID,
	verificationID union.VerificationID,
	intentID union.IntentID,
	attemptID union.MechanismAttemptID,
	raw []byte,
	schemaDocument json.RawMessage,
	repairAttempts int,
	completedAt time.Time,
) (StructuredValidation, error) {
	return ValidateStructuredOutputWithMechanism(
		effectID, verificationID, intentID, attemptID, raw, schemaDocument,
		StructuredMechanism{
			Kind: union.StructuredEmulatedSchema, Origin: union.CapabilityOriginEmulated,
			Fidelity: union.SemanticFidelityTransformed, Transport: "emulated_json_validated",
		},
		repairAttempts, completedAt,
	)
}

func ValidateStructuredOutputWithMechanism(
	effectID union.EffectID,
	verificationID union.VerificationID,
	intentID union.IntentID,
	attemptID union.MechanismAttemptID,
	raw []byte,
	schemaDocument json.RawMessage,
	mechanism StructuredMechanism,
	repairAttempts int,
	completedAt time.Time,
) (StructuredValidation, error) {
	if effectID == "" || verificationID == "" || intentID == "" || attemptID == "" || completedAt.IsZero() || repairAttempts < 0 {
		return StructuredValidation{}, fmt.Errorf("%w: structured validation identity is incomplete", ErrInvalidPolicy)
	}
	if mechanism.Kind == "" || mechanism.Origin == "" || mechanism.Fidelity == "" {
		return StructuredValidation{}, fmt.Errorf("%w: structured mechanism identity is incomplete", ErrInvalidPolicy)
	}
	value, err := decodeStrictJSON(raw)
	if err != nil {
		return StructuredValidation{}, err
	}
	if len(bytes.TrimSpace(schemaDocument)) == 0 {
		return StructuredValidation{}, fmt.Errorf("%w: schema is empty", ErrInvalidSchema)
	}
	schemaValue, err := decodeStrictJSON(schemaDocument)
	if err != nil {
		return StructuredValidation{}, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}
	canonicalSchema, err := json.Marshal(schemaValue)
	if err != nil {
		return StructuredValidation{}, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.AssertContent()
	compiler.UseLoader(rejectingSchemaLoader{})
	const schemaURL = "urn:praxis:structured-output-schema"
	if err := compiler.AddResource(schemaURL, schemaValue); err != nil {
		return StructuredValidation{}, fmt.Errorf("%w: %w", ErrInvalidSchema, err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		if strings.Contains(err.Error(), ErrExternalSchemaRef.Error()) {
			return StructuredValidation{}, fmt.Errorf("%w: %w", ErrInvalidSchema, ErrExternalSchemaRef)
		}
		return StructuredValidation{}, fmt.Errorf("%w: %w", ErrInvalidSchema, err)
	}
	if err := compiled.Validate(value); err != nil {
		return StructuredValidation{}, fmt.Errorf("%w: %v", ErrSchemaViolation, err)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return StructuredValidation{}, err
	}
	schemaDigest := digestBytes(canonicalSchema)
	finalDigest := digestBytes(canonical)
	rawDigest := digestBytes(raw)
	effect := union.EffectRecord{
		ID: effectID, IntentIDs: []union.IntentID{intentID}, MechanismAttemptID: attemptID,
		Kind: "structured_output_produced", Target: "structured_output",
		Payload: union.EffectPayload{StructuredOutput: &union.StructuredOutputEffect{
			Mechanism: mechanism.Kind, Origin: mechanism.Origin, Fidelity: mechanism.Fidelity, Transport: mechanism.Transport,
			RawRef: rawDigest, Parsed: append(json.RawMessage(nil), canonical...), SchemaDigest: schemaDigest,
			JSONValid: true, SchemaValid: true, RepairAttempts: repairAttempts, FinalDigest: finalDigest,
		}},
		EvidenceRefs:      []union.EvidenceRef{{Kind: "structured_output", Source: "praxis_schema_verifier", Digest: rawDigest, CapturedAt: completedAt.UTC(), Sensitivity: "internal"}},
		ObservationSource: "praxis_schema_verifier", VerificationStatus: union.VerificationVerified,
		VerificationRefs: []union.VerificationID{verificationID}, Confidence: "verified", OccurredAt: completedAt.UTC(),
	}
	verification := union.VerificationRecord{
		ID: verificationID, EffectIDs: []union.EffectID{effectID}, IntentIDs: []union.IntentID{intentID},
		Kind: "json_schema", Status: union.VerificationVerified,
		Verifier:     union.VersionedIdentity{ID: "praxis.strict-json-schema", Version: "v1"},
		EvidenceRefs: append([]union.EvidenceRef(nil), effect.EvidenceRefs...), CompletedAt: completedAt.UTC(),
	}
	if err := effect.Validate(); err != nil {
		return StructuredValidation{}, fmt.Errorf("%w: invalid structured Effect: %v", ErrInvalidPolicy, err)
	}
	if err := verification.Validate(); err != nil {
		return StructuredValidation{}, fmt.Errorf("%w: invalid structured verification: %v", ErrInvalidPolicy, err)
	}
	return StructuredValidation{Effect: effect, Verification: verification}, nil
}
