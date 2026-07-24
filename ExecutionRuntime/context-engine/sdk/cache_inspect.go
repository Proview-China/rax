package sdk

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

// InspectCachePlanRequestV1 evaluates an already constructed provider-neutral
// plan. The supplied profile remains non-authoritative offline input.
type InspectCachePlanRequestV1 struct {
	Meta                 OfflineRequestMetaV1          `json:"meta"`
	CachePlan            contract.CachePlan            `json:"cache_plan"`
	ProviderCacheProfile contract.ProviderCacheProfile `json:"provider_cache_profile"`
	CheckedUnixNano      int64                         `json:"checked_unix_nano"`
}

type InspectCachePlanResponseV1 struct {
	Meta               OfflineResponseMetaV1          `json:"meta"`
	Current            bool                           `json:"current"`
	PlanRef            contract.FactRef               `json:"plan_ref"`
	PartitionDigest    contract.Digest                `json:"partition_digest"`
	KeyDigest          contract.Digest                `json:"key_digest"`
	ProviderProfileRef contract.FactRef               `json:"provider_profile_ref"`
	EconomicDecision   contract.CacheEconomicDecision `json:"economic_decision"`
	CheckedUnixNano    int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                          `json:"expires_unix_nano"`
	Diagnostics        []OfflineDiagnosticV1          `json:"diagnostics"`
	InspectionDigest   contract.Digest                `json:"inspection_digest"`
	limits             OfflineSDKLimitsV1
}

type cacheInspectResponsePrivateV1 struct {
	Current            bool                           `json:"current"`
	PlanRef            contract.FactRef               `json:"plan_ref"`
	PartitionDigest    contract.Digest                `json:"partition_digest"`
	KeyDigest          contract.Digest                `json:"key_digest"`
	ProviderProfileRef contract.FactRef               `json:"provider_profile_ref"`
	EconomicDecision   contract.CacheEconomicDecision `json:"economic_decision"`
	CheckedUnixNano    int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                          `json:"expires_unix_nano"`
	Diagnostics        []OfflineDiagnosticV1          `json:"diagnostics"`
	InspectionDigest   contract.Digest                `json:"inspection_digest"`
}

func cacheInspectRequestDigestV1(request InspectCachePlanRequestV1, contexts ...context.Context) (contract.Digest, error) {
	meta := request.Meta
	meta.RequestDigest = ""
	return canonicalDigestV1("inspect-cache-plan-request", struct {
		Meta                 OfflineRequestMetaV1          `json:"meta"`
		CachePlan            contract.CachePlan            `json:"cache_plan"`
		ProviderCacheProfile contract.ProviderCacheProfile `json:"provider_cache_profile"`
		CheckedUnixNano      int64                         `json:"checked_unix_nano"`
	}{meta, request.CachePlan, request.ProviderCacheProfile, request.CheckedUnixNano}, contexts...)
}

func cacheInspectPrivateV1(response InspectCachePlanResponseV1) cacheInspectResponsePrivateV1 {
	return cacheInspectResponsePrivateV1{
		Current: response.Current, PlanRef: response.PlanRef, PartitionDigest: response.PartitionDigest,
		KeyDigest: response.KeyDigest, ProviderProfileRef: response.ProviderProfileRef,
		EconomicDecision: response.EconomicDecision, CheckedUnixNano: response.CheckedUnixNano,
		ExpiresUnixNano: response.ExpiresUnixNano, Diagnostics: cloneDiagnosticsV1(response.Diagnostics),
		InspectionDigest: response.InspectionDigest,
	}
}

func cacheInspectionDigestV1(response InspectCachePlanResponseV1, contexts ...context.Context) (contract.Digest, error) {
	private := cacheInspectPrivateV1(response)
	private.InspectionDigest = ""
	return canonicalDigestV1("cache-plan-inspection", private, contexts...)
}

func validateInspectCacheRequestV1(request InspectCachePlanRequestV1) error {
	if request.CheckedUnixNano <= 0 {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, OfflineInspectCachePlanV1, "checked_unix_nano", "checked time must be positive", contract.ErrInvalid)
	}
	return nil
}

func SealInspectCachePlanRequestV1(ctx context.Context, request InspectCachePlanRequestV1) (InspectCachePlanRequestV1, error) {
	const op = OfflineInspectCachePlanV1
	if err := validateSealContextAndMetaV1(ctx, request.Meta, op); err != nil {
		return InspectCachePlanRequestV1{}, err
	}
	if err := validateInspectCacheRequestV1(request); err != nil {
		return InspectCachePlanRequestV1{}, err
	}
	digest, err := cacheInspectRequestDigestV1(request, ctx)
	if err != nil {
		return InspectCachePlanRequestV1{}, mapErrorV1(op, "meta.request_digest", err)
	}
	if err := acceptOrSetRequestDigestV1(&request.Meta, digest, op); err != nil {
		return InspectCachePlanRequestV1{}, err
	}
	return request, validateContextV1(ctx, op)
}

func EncodeInspectCachePlanRequestV1(ctx context.Context, request InspectCachePlanRequestV1) ([]byte, error) {
	sealed, err := SealInspectCachePlanRequestV1(ctx, request)
	if err != nil {
		return nil, err
	}
	return encodeBoundedRequestV1(ctx, OfflineInspectCachePlanV1, sealed.Meta, func(buffer *boundedCodecBufferV1) (uint64, error) {
		return 0, buffer.writeJSON(sealed)
	})
}

func DecodeInspectCachePlanRequestV1(ctx context.Context, payload []byte) (InspectCachePlanRequestV1, error) {
	var request InspectCachePlanRequestV1
	if err := decodeStrictV1(ctx, OfflineInspectCachePlanV1, payload, hardWire48MiBV1, &request); err != nil {
		return InspectCachePlanRequestV1{}, err
	}
	if err := validateRequestMetaV1(request.Meta, OfflineInspectCachePlanV1); err != nil {
		return InspectCachePlanRequestV1{}, err
	}
	if uint64(len(payload)) > request.Meta.Limits.MaxWireRequestBytes {
		return InspectCachePlanRequestV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, OfflineInspectCachePlanV1, "payload", "request wire limit exceeded", contract.ErrLimitExceeded)
	}
	if err := validateWireAccountingV1(payload, 0, request.Meta.Limits, OfflineInspectCachePlanV1); err != nil {
		return InspectCachePlanRequestV1{}, err
	}
	if err := validateInspectCacheRequestV1(request); err != nil {
		return InspectCachePlanRequestV1{}, err
	}
	return request, nil
}

func InspectCachePlanV1(ctx context.Context, request InspectCachePlanRequestV1) (InspectCachePlanResponseV1, error) {
	const op = OfflineInspectCachePlanV1
	if err := validateContextV1(ctx, op); err != nil {
		return InspectCachePlanResponseV1{}, err
	}
	if err := validateRequestMetaV1(request.Meta, op); err != nil {
		return InspectCachePlanResponseV1{}, err
	}
	if err := validateInspectCacheRequestV1(request); err != nil {
		return InspectCachePlanResponseV1{}, err
	}
	wantRequest, err := cacheInspectRequestDigestV1(request, ctx)
	if err != nil {
		return InspectCachePlanResponseV1{}, mapErrorV1(op, "meta.request_digest", err)
	}
	if wantRequest != request.Meta.RequestDigest {
		return InspectCachePlanResponseV1{}, sdkErrorV1(OfflineErrorConflictV1, op, "meta.request_digest", "request digest mismatch", contract.ErrConflict)
	}
	if err := request.CachePlan.ValidateCurrent(request.ProviderCacheProfile, request.CheckedUnixNano); err != nil {
		return InspectCachePlanResponseV1{}, mapErrorV1(op, "cache_plan", err)
	}
	planDigest, err := contract.DigestJSON(request.CachePlan)
	if err != nil {
		return InspectCachePlanResponseV1{}, mapErrorV1(op, "cache_plan", err)
	}
	partitionDigest, err := request.CachePlan.Partition.DigestValue()
	if err != nil {
		return InspectCachePlanResponseV1{}, mapErrorV1(op, "cache_plan.partition", err)
	}
	keyDigest, err := request.CachePlan.Partition.KeyDigest()
	if err != nil {
		return InspectCachePlanResponseV1{}, mapErrorV1(op, "cache_plan.partition", err)
	}
	profileDigest, err := request.ProviderCacheProfile.DigestValue(request.CheckedUnixNano)
	if err != nil {
		return InspectCachePlanResponseV1{}, mapErrorV1(op, "provider_cache_profile", err)
	}
	decision, err := contract.CompareCacheEconomics(request.CachePlan)
	if err != nil {
		return InspectCachePlanResponseV1{}, mapErrorV1(op, "cache_plan", err)
	}
	expires := request.CachePlan.ExpiresUnixNano
	if request.ProviderCacheProfile.ExpiresUnixNano < expires {
		expires = request.ProviderCacheProfile.ExpiresUnixNano
	}
	response := InspectCachePlanResponseV1{
		Meta:    OfflineResponseMetaV1{ContractVersion: OfflineSDKContractVersionV1, RequestID: request.Meta.RequestID, Operation: op, RequestDigest: request.Meta.RequestDigest},
		Current: true, PlanRef: contract.FactRef{ID: request.CachePlan.ID, Revision: request.CachePlan.Revision, Digest: planDigest},
		PartitionDigest: partitionDigest, KeyDigest: keyDigest,
		ProviderProfileRef: contract.FactRef{ID: request.ProviderCacheProfile.ID, Revision: request.ProviderCacheProfile.Revision, Digest: profileDigest},
		EconomicDecision:   decision, CheckedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: expires,
		Diagnostics: []OfflineDiagnosticV1{}, limits: request.Meta.Limits,
	}
	response.InspectionDigest, err = cacheInspectionDigestV1(response, ctx)
	if err != nil {
		return InspectCachePlanResponseV1{}, mapErrorV1(op, "inspection_digest", err)
	}
	response.Meta.ResultDigest, err = validateResponseResultDigestV1("inspect-cache-plan-response", &response.Meta, cacheInspectPrivateV1(response), ctx)
	if err != nil {
		return InspectCachePlanResponseV1{}, mapErrorV1(op, "meta.result_digest", err)
	}
	if err := validateContextV1(ctx, op); err != nil {
		return InspectCachePlanResponseV1{}, err
	}
	return response, nil
}

func validateInspectCacheResponseV1(ctx context.Context, response InspectCachePlanResponseV1) error {
	const op = OfflineInspectCachePlanV1
	if err := validateContextV1(ctx, op); err != nil {
		return err
	}
	if err := validateResponseEnvelopeV1(response.Meta, response.limits, op); err != nil {
		return err
	}
	if err := validateDiagnosticsV1(response.Diagnostics, response.limits, op); err != nil {
		return err
	}
	if !response.Current || response.PlanRef.Validate() != nil || response.PartitionDigest.Validate() != nil || response.KeyDigest.Validate() != nil || response.ProviderProfileRef.Validate() != nil || response.CheckedUnixNano <= 0 || response.ExpiresUnixNano <= response.CheckedUnixNano {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "response", "invalid cache inspection closure", contract.ErrInvalid)
	}
	wantInspection, err := cacheInspectionDigestV1(response, ctx)
	if err != nil {
		return mapErrorV1(op, "inspection_digest", err)
	}
	if wantInspection != response.InspectionDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "inspection_digest", "inspection digest mismatch", contract.ErrConflict)
	}
	wantResult, err := validateResponseResultDigestV1("inspect-cache-plan-response", &response.Meta, cacheInspectPrivateV1(response), ctx)
	if err != nil {
		return mapErrorV1(op, "meta.result_digest", err)
	}
	if wantResult != response.Meta.ResultDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "meta.result_digest", "result digest mismatch", contract.ErrConflict)
	}
	return nil
}

func EncodeInspectCachePlanResponseV1(ctx context.Context, response InspectCachePlanResponseV1) ([]byte, error) {
	if err := validateInspectCacheResponseV1(ctx, response); err != nil {
		return nil, err
	}
	return encodeResponseV1(ctx, OfflineInspectCachePlanV1, response, response.Meta, response.limits)
}
