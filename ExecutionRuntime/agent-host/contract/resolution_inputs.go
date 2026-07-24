package contract

const ResolutionInputsCurrentKindV1 = "praxis.agent-host/resolution-inputs-current"

// ResolutionInputsCurrentV1 is the Host-owned atomic current projection from
// opaque configuration ids to the exact Agent Assembler owner inputs.
type ResolutionInputsCurrentV1 struct {
	ContractVersion         string     `json:"contract_version"`
	ObjectKind              string     `json:"object_kind"`
	CatalogStableID         string     `json:"catalog_stable_id"`
	ResolutionFactsStableID string     `json:"resolution_facts_stable_id"`
	CatalogExactRef         ExactRefV1 `json:"catalog_exact_ref"`
	ResolutionFactsExactRef ExactRefV1 `json:"resolution_facts_exact_ref"`
	Revision                uint64     `json:"revision"`
	CheckedUnixNano         int64      `json:"checked_unix_nano"`
	ExpiresUnixNano         int64      `json:"expires_unix_nano"`
	ProjectionDigest        DigestV1   `json:"projection_digest"`
}

func (v ResolutionInputsCurrentV1) Validate(nowUnixNano int64) error {
	if err := v.validateStructureV1(); err != nil {
		return err
	}
	if nowUnixNano <= 0 || v.CheckedUnixNano > nowUnixNano || nowUnixNano >= v.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "resolution_inputs_current_stale", "resolution inputs current projection is stale or observed in the future")
	}
	digest, err := resolutionInputsCurrentDigestV1(v)
	if err != nil {
		return err
	}
	if digest != v.ProjectionDigest {
		return NewError(ErrorPrecondition, "resolution_inputs_current_digest_drift", "resolution inputs current projection digest drifted")
	}
	return nil
}

func (v ResolutionInputsCurrentV1) validateStructureV1() error {
	if v.ContractVersion != ContractVersionV1 || v.ObjectKind != ResolutionInputsCurrentKindV1 {
		return NewError(ErrorInvalidArgument, "resolution_inputs_current_contract_invalid", "resolution inputs current contract discriminator is unsupported")
	}
	if err := ValidateIdentifierV1("catalog stable id", v.CatalogStableID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("resolution facts stable id", v.ResolutionFactsStableID); err != nil {
		return err
	}
	if err := v.CatalogExactRef.Validate(); err != nil {
		return err
	}
	if err := v.ResolutionFactsExactRef.Validate(); err != nil {
		return err
	}
	if v.Revision == 0 || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "resolution_inputs_current_window_invalid", "resolution inputs current coordinates are invalid")
	}
	return nil
}

func SealResolutionInputsCurrentV1(value ResolutionInputsCurrentV1) (ResolutionInputsCurrentV1, error) {
	if err := value.validateStructureV1(); err != nil {
		return ResolutionInputsCurrentV1{}, err
	}
	value.ProjectionDigest = ""
	digest, err := resolutionInputsCurrentDigestV1(value)
	if err != nil {
		return ResolutionInputsCurrentV1{}, err
	}
	value.ProjectionDigest = digest
	if err := value.Validate(value.CheckedUnixNano); err != nil {
		return ResolutionInputsCurrentV1{}, err
	}
	return value, nil
}

func resolutionInputsCurrentDigestV1(value ResolutionInputsCurrentV1) (DigestV1, error) {
	value.ProjectionDigest = ""
	return DigestJSONV1(struct {
		Domain  string                    `json:"domain"`
		Version string                    `json:"version"`
		Kind    string                    `json:"kind"`
		Value   ResolutionInputsCurrentV1 `json:"value"`
	}{Domain: "praxis.agent-host", Version: "v1", Kind: "ResolutionInputsCurrentV1", Value: value})
}
