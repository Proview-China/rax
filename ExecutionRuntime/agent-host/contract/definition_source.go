package contract

const DefinitionSourceCurrentKindV1 = "praxis.agent-host/definition-source-current"

type DefinitionSourceCurrentV1 struct {
	ContractVersion    string     `json:"contract_version"`
	ObjectKind         string     `json:"object_kind"`
	SourceStableID     string     `json:"source_stable_id"`
	DefinitionExactRef ExactRefV1 `json:"definition_exact_ref"`
	Revision           uint64     `json:"revision"`
	CheckedUnixNano    int64      `json:"checked_unix_nano"`
	ExpiresUnixNano    int64      `json:"expires_unix_nano"`
	ProjectionDigest   DigestV1   `json:"projection_digest"`
}

func (v DefinitionSourceCurrentV1) Validate(nowUnixNano int64) error {
	if err := v.validateStructureV1(); err != nil {
		return err
	}
	if nowUnixNano <= 0 || v.CheckedUnixNano > nowUnixNano || nowUnixNano >= v.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "definition_source_current_stale", "definition source current projection is stale or observed in the future")
	}
	digest, err := definitionSourceCurrentDigestV1(v)
	if err != nil {
		return err
	}
	if digest != v.ProjectionDigest {
		return NewError(ErrorPrecondition, "definition_source_current_digest_drift", "definition source current projection digest drifted")
	}
	return nil
}

func (v DefinitionSourceCurrentV1) validateStructureV1() error {
	if v.ContractVersion != ContractVersionV1 || v.ObjectKind != DefinitionSourceCurrentKindV1 {
		return NewError(ErrorInvalidArgument, "definition_source_current_contract_invalid", "definition source current contract discriminator is unsupported")
	}
	if err := ValidateIdentifierV1("definition source stable id", v.SourceStableID); err != nil {
		return err
	}
	if err := v.DefinitionExactRef.Validate(); err != nil {
		return err
	}
	if v.Revision == 0 || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "definition_source_current_window_invalid", "definition source current coordinates are invalid")
	}
	return nil
}

func SealDefinitionSourceCurrentV1(value DefinitionSourceCurrentV1) (DefinitionSourceCurrentV1, error) {
	if err := value.validateStructureV1(); err != nil {
		return DefinitionSourceCurrentV1{}, err
	}
	value.ProjectionDigest = ""
	digest, err := definitionSourceCurrentDigestV1(value)
	if err != nil {
		return DefinitionSourceCurrentV1{}, err
	}
	value.ProjectionDigest = digest
	if err := value.Validate(value.CheckedUnixNano); err != nil {
		return DefinitionSourceCurrentV1{}, err
	}
	return value, nil
}

func definitionSourceCurrentDigestV1(value DefinitionSourceCurrentV1) (DigestV1, error) {
	value.ProjectionDigest = ""
	return DigestJSONV1(struct {
		Domain  string                    `json:"domain"`
		Version string                    `json:"version"`
		Kind    string                    `json:"kind"`
		Value   DefinitionSourceCurrentV1 `json:"value"`
	}{Domain: "praxis.agent-host", Version: "v1", Kind: "DefinitionSourceCurrentV1", Value: value})
}
