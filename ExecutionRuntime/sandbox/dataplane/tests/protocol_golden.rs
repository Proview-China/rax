use praxis_sandbox_dataplane::contract::{ProviderBindingV1, canonical_digest};
use serde::Deserialize;

#[derive(Deserialize)]
struct GoldenProviderBinding {
    kind: String,
    canonical: ProviderBindingV1,
    expected_digest: String,
}

#[test]
fn provider_binding_digest_matches_go_wire_golden() {
    let golden: GoldenProviderBinding = serde_json::from_str(include_str!(
        "../../protocol/v1/golden/provider-binding-v1.json"
    ))
    .unwrap_or_else(|error| panic!("decode golden: {error}"));
    let digest = canonical_digest(&golden.kind, &golden.canonical)
        .unwrap_or_else(|error| panic!("digest golden: {error}"));
    assert_eq!(digest, golden.expected_digest);
}
