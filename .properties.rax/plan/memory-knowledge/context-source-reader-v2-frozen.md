# Memory/Knowledge Context Source Current Reader V2冻结合同

> 状态：**design_frozen / owner_local_and_reference_integration_software_test_yes**。两Owner各七个V2 struct、StableClosureDigest V2及Memory六个/Knowledge八个set digest算法已按本合同实现；V1/V2属于同一Owner Reader合同族并共享唯一Store/Journal/current。reference fixture已使用Memory=1、Knowledge=1；production root仍未启用。

## 1. 唯一合同族与Go nominal

| Owner | package | V2接口 | 合同版本 |
|---|---|---|---|
| Memory | `ExecutionRuntime/memory-knowledge/memory/contextsource` | `MemoryContextSourceCurrentReaderV2` | `praxis.memory/context-source-current-reader/v2` |
| Knowledge | `ExecutionRuntime/memory-knowledge/knowledge/contextsource` | `KnowledgeContextSourceCurrentReaderV2` | `praxis.knowledge/context-source-current-reader/v2` |

每个package只新增以下同名V2 nominal，不建立neutral/facade副本：

```text
AttemptCoordinateV2
AttemptInspectionV2
CurrentRequestV2
ProjectionItemV2
CurrentProjectionV2
ExactContentRequestV2
ExactContentObservationV2
```

冻结接口保持V1三方法集合：

```text
InspectAttempt(context.Context, AttemptCoordinateV2) -> AttemptInspectionV2
InspectForTurn(context.Context, CurrentRequestV2) -> CurrentProjectionV2
ReadContentExact(context.Context, ExactContentRequestV2) -> ExactContentObservationV2 + bounded []byte
```

`context.Context`只作为Go参数，不序列化、不进入任何digest。Memory与Knowledge的nominal、ObjectKind、Owner常量和package保持独立；不得用类型别名合并。

## 2. 七个V2 struct逐字段schema

### 2.1 discriminator

| nominal | Memory ObjectKind | Knowledge ObjectKind |
|---|---|---|
| `AttemptCoordinateV2` | `memory_context_source_attempt_coordinate` | `knowledge_context_source_attempt_coordinate` |
| `AttemptInspectionV2` | `memory_local_attempt_inspection` | `knowledge_local_attempt_inspection` |
| `CurrentRequestV2` | `memory_context_source_current_request` | `knowledge_context_source_current_request` |
| `ProjectionItemV2` | `memory_context_source_projection_item` | `knowledge_context_source_projection_item` |
| `CurrentProjectionV2` | `memory_contribution_current_projection` | `knowledge_contribution_current_projection` |
| `ExactContentRequestV2` | `memory_local_exact_content_request` | `knowledge_local_exact_content_request` |
| `ExactContentObservationV2` | `memory_local_exact_content_observation` | `knowledge_local_exact_content_observation` |

每个字段均显式带JSON tag，字段声明顺序就是`encoding/json` canonical body顺序；V2 public struct禁止`omitempty`。两Owner分别在自己的`contextsource` package声明同名nominal，不使用alias或neutral DTO。共同辅助类型只有：

```go
type CheckPhaseV2 string

const (
    CheckPhaseS1V2 CheckPhaseV2 = "s1"
    CheckPhaseS2V2 CheckPhaseV2 = "s2"
)
```

### 2.2 Memory七个struct

```go
type AttemptCoordinateV2 struct {
    ContractVersion      string        `json:"contract_version"`
    ObjectKind           string        `json:"object_kind"`
    TenantID             string        `json:"tenant_id"`
    IdentityRef          contract.Ref  `json:"identity_ref"`
    IdentityEpoch        uint64        `json:"identity_epoch"`
    ExecutionScopeDigest string        `json:"execution_scope_digest"`
    RunID                string        `json:"run_id"`
    SessionRef           contract.Ref  `json:"session_ref"`
    SessionEvidenceRef   contract.Ref  `json:"session_evidence_ref"`
    SessionCheckedAt     time.Time     `json:"session_checked_at"`
    SessionExpiresAt     time.Time     `json:"session_expires_at"`
    SourceTurnOrdinal    uint32        `json:"source_turn_ordinal"`
    SourceTurnRef        contract.Ref  `json:"source_turn_ref"`
    TurnEvidenceRef      contract.Ref  `json:"turn_evidence_ref"`
    TurnCheckedAt        time.Time     `json:"turn_checked_at"`
    TurnExpiresAt        time.Time     `json:"turn_expires_at"`
    LegacyTurnID         string        `json:"legacy_turn_id"`
    AttemptRef           contract.Ref  `json:"attempt_ref"`
    RequestDigest        string        `json:"request_digest"`
    IdempotencyKey       string        `json:"idempotency_key"`
    ObservationRef       contract.Ref  `json:"observation_ref"`
    ResultRef            contract.Ref  `json:"result_ref"`
    Digest               string        `json:"digest"`
}

type AttemptInspectionV2 struct {
    ContractVersion string               `json:"contract_version"`
    ObjectKind      string               `json:"object_kind"`
    Ref             contract.Ref         `json:"ref"`
    Owner           contract.OwnerDomain `json:"owner"`
    Coordinate      AttemptCoordinateV2  `json:"coordinate"`
    ObservationRef  *contract.Ref        `json:"observation_ref"`
    ResultRef       *contract.Ref        `json:"result_ref"`
    Status          AttemptStatus        `json:"status"`
    OwnerCheckedAt  time.Time            `json:"owner_checked_at"`
    ExpiresAt       time.Time            `json:"expires_at"`
    Digest          string               `json:"digest"`
}

type CurrentRequestV2 struct {
    ContractVersion         string               `json:"contract_version"`
    ObjectKind              string               `json:"object_kind"`
    Coordinate              AttemptCoordinateV2  `json:"coordinate"`
    CurrentStateRef         contract.Ref         `json:"current_state_ref"`
    ExpectedQueryRef        contract.Ref         `json:"expected_query_ref"`
    ExpectedViewRef         contract.Ref         `json:"expected_view_ref"`
    ExpectedWatermarkRef    contract.Ref         `json:"expected_watermark_ref"`
    AuthorityRef            contract.Ref         `json:"authority_ref"`
    AuthorityEpoch          uint64               `json:"authority_epoch"`
    PolicyRef               contract.Ref         `json:"policy_ref"`
    Purpose                 string               `json:"purpose"`
    Scopes                  []string             `json:"scopes"`
    SensitivityMax          string               `json:"sensitivity_max"`
    CheckPhase              CheckPhaseV2         `json:"check_phase"`
    ExpectedS1ClosureDigest string               `json:"expected_s1_closure_digest"`
    MaxItems                int                  `json:"max_items"`
    MaxBytes                int64                `json:"max_bytes"`
    MaxTokens               int                  `json:"max_tokens"`
    PerItemMaxBytes         int64                `json:"per_item_max_bytes"`
    EstimatorRef            contract.Ref         `json:"estimator_ref"`
    CheckedUpperBound       time.Time            `json:"checked_upper_bound"`
    NotAfter                time.Time            `json:"not_after"`
    ProjectionID            string               `json:"projection_id"`
    ProjectionRevision      uint64               `json:"projection_revision"`
    Digest                  string               `json:"digest"`
}

type ProjectionItemV2 struct {
    ContractVersion  string               `json:"contract_version"`
    ObjectKind       string               `json:"object_kind"`
    Rank             int                  `json:"rank"`
    Score            int                  `json:"score"`
    RecordRef        contract.Ref         `json:"record_ref"`
    ContentRef       contract.ContentRef  `json:"content_ref"`
    SourceRefs       []contract.Ref       `json:"source_refs"`
    EvidenceRefs     []contract.Ref       `json:"evidence_refs"`
    ProjectionRefs   []contract.Ref       `json:"projection_refs"`
    CitationDigest   string               `json:"citation_digest"`
    DomainResultRef  contract.Ref         `json:"domain_result_ref"`
    AssociationDigest string              `json:"association_digest"`
    ApplicationRef   contract.Ref         `json:"application_ref"`
    TokenEstimate    int                  `json:"token_estimate"`
    EstimatorRef     contract.Ref         `json:"estimator_ref"`
    ExpiresAt        time.Time            `json:"expires_at"`
    Digest           string               `json:"digest"`
}

type CurrentProjectionV2 struct {
    ContractVersion       string               `json:"contract_version"`
    ObjectKind            string               `json:"object_kind"`
    Ref                   contract.Ref         `json:"ref"`
    Owner                 contract.OwnerDomain `json:"owner"`
    Current               bool                 `json:"current"`
    Coordinate            AttemptCoordinateV2  `json:"coordinate"`
    AttemptInspectionRef  contract.Ref         `json:"attempt_inspection_ref"`
    CurrentStateRef       contract.Ref         `json:"current_state_ref"`
    StatePlaneBindingRef  contract.Ref         `json:"state_plane_binding_ref"`
    QueryRef              contract.Ref         `json:"query_ref"`
    ViewRef               contract.Ref         `json:"view_ref"`
    WatermarkRef          contract.Ref         `json:"watermark_ref"`
    AuthorityRef          contract.Ref         `json:"authority_ref"`
    AuthorityEpoch        uint64               `json:"authority_epoch"`
    PolicyRef             contract.Ref         `json:"policy_ref"`
    Purpose               string               `json:"purpose"`
    Scopes                []string             `json:"scopes"`
    SensitivityMax        string               `json:"sensitivity_max"`
    Coverage              contract.Coverage    `json:"coverage"`
    NextCursor            string               `json:"next_cursor"`
    ResultDigest          string               `json:"result_digest"`
    EvidenceDigest        string               `json:"evidence_digest"`
    Items                 []ProjectionItemV2   `json:"items"`
    OrderedItemSetDigest  string               `json:"ordered_item_set_digest"`
    ContentSetDigest      string               `json:"content_set_digest"`
    SourceSetDigest       string               `json:"source_set_digest"`
    EvidenceSetDigest     string               `json:"evidence_set_digest"`
    ProjectionSetDigest   string               `json:"projection_set_digest"`
    CitationSetDigest     string               `json:"citation_set_digest"`
    MaxItems              int                  `json:"max_items"`
    MaxBytes              int64                `json:"max_bytes"`
    MaxTokens             int                  `json:"max_tokens"`
    PerItemMaxBytes       int64                `json:"per_item_max_bytes"`
    EstimatorRef          contract.Ref         `json:"estimator_ref"`
    StableClosureDigest   string               `json:"stable_closure_digest"`
    CheckPhase            CheckPhaseV2         `json:"check_phase"`
    OwnerCheckedAt        time.Time            `json:"owner_checked_at"`
    ExpiresAt             time.Time            `json:"expires_at"`
    Digest                string               `json:"digest"`
}

type ExactContentRequestV2 struct {
    ContractVersion            string              `json:"contract_version"`
    ObjectKind                 string              `json:"object_kind"`
    Coordinate                 AttemptCoordinateV2 `json:"coordinate"`
    Projection                 CurrentProjectionV2 `json:"projection"`
    Rank                       int                 `json:"rank"`
    CheckPhase                 CheckPhaseV2        `json:"check_phase"`
    ExpectedStableClosureDigest string             `json:"expected_stable_closure_digest"`
    MaxBodyBytes               int64               `json:"max_body_bytes"`
    CheckedUpperBound          time.Time           `json:"checked_upper_bound"`
    NotAfter                   time.Time           `json:"not_after"`
    Digest                     string              `json:"digest"`
}

type ExactContentObservationV2 struct {
    ContractVersion       string               `json:"contract_version"`
    ObjectKind            string               `json:"object_kind"`
    Ref                   contract.Ref         `json:"ref"`
    Owner                 contract.OwnerDomain `json:"owner"`
    ProjectionRef         contract.Ref         `json:"projection_ref"`
    ProjectionItemDigest  string               `json:"projection_item_digest"`
    StatePlaneBindingRef  contract.Ref         `json:"state_plane_binding_ref"`
    StableClosureDigest   string               `json:"stable_closure_digest"`
    Coordinate            AttemptCoordinateV2  `json:"coordinate"`
    Rank                  int                  `json:"rank"`
    RecordRef             contract.Ref         `json:"record_ref"`
    ContentRef            contract.ContentRef  `json:"content_ref"`
    DomainResultRef       contract.Ref         `json:"domain_result_ref"`
    AssociationDigest     string               `json:"association_digest"`
    ApplicationRef        contract.Ref         `json:"application_ref"`
    ObservedLength        int64                `json:"observed_length"`
    ObservedMediaType     string               `json:"observed_media_type"`
    ObservedDigest        string               `json:"observed_digest"`
    CheckPhase            CheckPhaseV2         `json:"check_phase"`
    OwnerObservedAt       time.Time            `json:"owner_observed_at"`
    ExpiresAt             time.Time            `json:"expires_at"`
    Digest                string               `json:"digest"`
}
```

### 2.3 Knowledge七个struct

```go
type AttemptCoordinateV2 struct {
    ContractVersion      string        `json:"contract_version"`
    ObjectKind           string        `json:"object_kind"`
    TenantID             string        `json:"tenant_id"`
    IdentityRef          contract.Ref  `json:"identity_ref"`
    IdentityEpoch        uint64        `json:"identity_epoch"`
    ExecutionScopeDigest string        `json:"execution_scope_digest"`
    RunID                string        `json:"run_id"`
    SessionRef           contract.Ref  `json:"session_ref"`
    SessionEvidenceRef   contract.Ref  `json:"session_evidence_ref"`
    SessionCheckedAt     time.Time     `json:"session_checked_at"`
    SessionExpiresAt     time.Time     `json:"session_expires_at"`
    SourceTurnOrdinal    uint32        `json:"source_turn_ordinal"`
    SourceTurnRef        contract.Ref  `json:"source_turn_ref"`
    TurnEvidenceRef      contract.Ref  `json:"turn_evidence_ref"`
    TurnCheckedAt        time.Time     `json:"turn_checked_at"`
    TurnExpiresAt        time.Time     `json:"turn_expires_at"`
    LegacyTurnID         string        `json:"legacy_turn_id"`
    AttemptRef           contract.Ref  `json:"attempt_ref"`
    RequestDigest        string        `json:"request_digest"`
    IdempotencyKey       string        `json:"idempotency_key"`
    ObservationRef       contract.Ref  `json:"observation_ref"`
    ResultRef            contract.Ref  `json:"result_ref"`
    Digest               string        `json:"digest"`
}

type AttemptInspectionV2 struct {
    ContractVersion string               `json:"contract_version"`
    ObjectKind      string               `json:"object_kind"`
    Ref             contract.Ref         `json:"ref"`
    Owner           contract.OwnerDomain `json:"owner"`
    Coordinate      AttemptCoordinateV2  `json:"coordinate"`
    ObservationRef  *contract.Ref        `json:"observation_ref"`
    ResultRef       *contract.Ref        `json:"result_ref"`
    Status          AttemptStatus        `json:"status"`
    OwnerCheckedAt  time.Time            `json:"owner_checked_at"`
    ExpiresAt       time.Time            `json:"expires_at"`
    Digest          string               `json:"digest"`
}

type CurrentRequestV2 struct {
    ContractVersion         string               `json:"contract_version"`
    ObjectKind              string               `json:"object_kind"`
    Coordinate              AttemptCoordinateV2  `json:"coordinate"`
    CurrentStateRef         contract.Ref         `json:"current_state_ref"`
    ExpectedQueryRef        contract.Ref         `json:"expected_query_ref"`
    ExpectedViewRef         contract.Ref         `json:"expected_view_ref"`
    ExpectedSnapshotRef     contract.Ref         `json:"expected_snapshot_ref"`
    ExpectedPointerRef      contract.Ref         `json:"expected_pointer_ref"`
    AuthorityRef            contract.Ref         `json:"authority_ref"`
    AuthorityEpoch          uint64               `json:"authority_epoch"`
    PolicyRef               contract.Ref         `json:"policy_ref"`
    Purpose                 string               `json:"purpose"`
    Scopes                  []string             `json:"scopes"`
    AllowedLicenses         []string             `json:"allowed_licenses"`
    SensitivityMax          string               `json:"sensitivity_max"`
    CheckPhase              CheckPhaseV2         `json:"check_phase"`
    ExpectedS1ClosureDigest string               `json:"expected_s1_closure_digest"`
    MaxItems                int                  `json:"max_items"`
    MaxBytes                int64                `json:"max_bytes"`
    MaxTokens               int                  `json:"max_tokens"`
    PerItemMaxBytes         int64                `json:"per_item_max_bytes"`
    EstimatorRef            contract.Ref         `json:"estimator_ref"`
    CheckedUpperBound       time.Time            `json:"checked_upper_bound"`
    NotAfter                time.Time            `json:"not_after"`
    ProjectionID            string               `json:"projection_id"`
    ProjectionRevision      uint64               `json:"projection_revision"`
    Digest                  string               `json:"digest"`
}

type ProjectionItemV2 struct {
    ContractVersion   string               `json:"contract_version"`
    ObjectKind        string               `json:"object_kind"`
    Rank              int                  `json:"rank"`
    Score             int                  `json:"score"`
    RecordRef         contract.Ref         `json:"record_ref"`
    PackageRef        contract.Ref         `json:"package_ref"`
    SnapshotRef       contract.Ref         `json:"snapshot_ref"`
    ContentRef        contract.ContentRef  `json:"content_ref"`
    SourceRefs        []contract.Ref       `json:"source_refs"`
    EvidenceRefs      []contract.Ref       `json:"evidence_refs"`
    ProjectionRefs    []contract.Ref       `json:"projection_refs"`
    CitationDigest    string               `json:"citation_digest"`
    License           string               `json:"license"`
    LicenseDigest     string               `json:"license_digest"`
    TrustState        string               `json:"trust_state"`
    ConflictGroup     string               `json:"conflict_group"`
    ConflictDigest    string               `json:"conflict_digest"`
    DomainResultRef   contract.Ref         `json:"domain_result_ref"`
    AssociationDigest string               `json:"association_digest"`
    ApplicationRef    contract.Ref         `json:"application_ref"`
    TokenEstimate     int                  `json:"token_estimate"`
    EstimatorRef      contract.Ref         `json:"estimator_ref"`
    ExpiresAt         time.Time            `json:"expires_at"`
    Digest            string               `json:"digest"`
}

type CurrentProjectionV2 struct {
    ContractVersion       string               `json:"contract_version"`
    ObjectKind            string               `json:"object_kind"`
    Ref                   contract.Ref         `json:"ref"`
    Owner                 contract.OwnerDomain `json:"owner"`
    Current               bool                 `json:"current"`
    Coordinate            AttemptCoordinateV2  `json:"coordinate"`
    AttemptInspectionRef  contract.Ref         `json:"attempt_inspection_ref"`
    CurrentStateRef       contract.Ref         `json:"current_state_ref"`
    StatePlaneBindingRef  contract.Ref         `json:"state_plane_binding_ref"`
    QueryRef              contract.Ref         `json:"query_ref"`
    ViewRef               contract.Ref         `json:"view_ref"`
    SnapshotRef           contract.Ref         `json:"snapshot_ref"`
    PointerRef            contract.Ref         `json:"pointer_ref"`
    AuthorityRef          contract.Ref         `json:"authority_ref"`
    AuthorityEpoch        uint64               `json:"authority_epoch"`
    PolicyRef             contract.Ref         `json:"policy_ref"`
    Purpose               string               `json:"purpose"`
    Scopes                []string             `json:"scopes"`
    AllowedLicenses       []string             `json:"allowed_licenses"`
    SensitivityMax        string               `json:"sensitivity_max"`
    Coverage              contract.Coverage    `json:"coverage"`
    NextCursor            string               `json:"next_cursor"`
    ResultDigest          string               `json:"result_digest"`
    EvidenceDigest        string               `json:"evidence_digest"`
    Items                 []ProjectionItemV2   `json:"items"`
    OrderedItemSetDigest  string               `json:"ordered_item_set_digest"`
    ContentSetDigest      string               `json:"content_set_digest"`
    SourceSetDigest       string               `json:"source_set_digest"`
    EvidenceSetDigest     string               `json:"evidence_set_digest"`
    ProjectionSetDigest   string               `json:"projection_set_digest"`
    CitationSetDigest     string               `json:"citation_set_digest"`
    LicenseSetDigest      string               `json:"license_set_digest"`
    ConflictSetDigest     string               `json:"conflict_set_digest"`
    MaxItems              int                  `json:"max_items"`
    MaxBytes              int64                `json:"max_bytes"`
    MaxTokens             int                  `json:"max_tokens"`
    PerItemMaxBytes       int64                `json:"per_item_max_bytes"`
    EstimatorRef          contract.Ref         `json:"estimator_ref"`
    StableClosureDigest   string               `json:"stable_closure_digest"`
    CheckPhase            CheckPhaseV2         `json:"check_phase"`
    OwnerCheckedAt        time.Time            `json:"owner_checked_at"`
    ExpiresAt             time.Time            `json:"expires_at"`
    Digest                string               `json:"digest"`
}

type ExactContentRequestV2 struct {
    ContractVersion            string              `json:"contract_version"`
    ObjectKind                 string              `json:"object_kind"`
    Coordinate                 AttemptCoordinateV2 `json:"coordinate"`
    Projection                 CurrentProjectionV2 `json:"projection"`
    Rank                       int                 `json:"rank"`
    CheckPhase                 CheckPhaseV2        `json:"check_phase"`
    ExpectedStableClosureDigest string             `json:"expected_stable_closure_digest"`
    MaxBodyBytes               int64               `json:"max_body_bytes"`
    CheckedUpperBound          time.Time           `json:"checked_upper_bound"`
    NotAfter                   time.Time           `json:"not_after"`
    Digest                     string              `json:"digest"`
}

type ExactContentObservationV2 struct {
    ContractVersion       string               `json:"contract_version"`
    ObjectKind            string               `json:"object_kind"`
    Ref                   contract.Ref         `json:"ref"`
    Owner                 contract.OwnerDomain `json:"owner"`
    ProjectionRef         contract.Ref         `json:"projection_ref"`
    ProjectionItemDigest  string               `json:"projection_item_digest"`
    StatePlaneBindingRef  contract.Ref         `json:"state_plane_binding_ref"`
    StableClosureDigest   string               `json:"stable_closure_digest"`
    Coordinate            AttemptCoordinateV2  `json:"coordinate"`
    Rank                  int                  `json:"rank"`
    RecordRef             contract.Ref         `json:"record_ref"`
    PackageRef            contract.Ref         `json:"package_ref"`
    SnapshotRef           contract.Ref         `json:"snapshot_ref"`
    ContentRef            contract.ContentRef  `json:"content_ref"`
    License               string               `json:"license"`
    LicenseDigest         string               `json:"license_digest"`
    DomainResultRef       contract.Ref         `json:"domain_result_ref"`
    AssociationDigest     string               `json:"association_digest"`
    ApplicationRef        contract.Ref         `json:"application_ref"`
    ObservedLength        int64                `json:"observed_length"`
    ObservedMediaType     string               `json:"observed_media_type"`
    ObservedDigest        string               `json:"observed_digest"`
    CheckPhase            CheckPhaseV2         `json:"check_phase"`
    OwnerObservedAt       time.Time            `json:"owner_observed_at"`
    ExpiresAt             time.Time            `json:"expires_at"`
    Digest                string               `json:"digest"`
}
```

`AttemptStatus`沿用live闭集。`AttemptInspectionV2.ObservationRef/ResultRef`是唯一允许nil的字段：`confirmed_not_persisted`必须二者均nil，两个persisted状态必须二者均非nil；JSON仍显式输出`null`，不使用`omitempty`。所有slice canonical前排序并拒绝semantic duplicate；Knowledge `ConflictGroup`为空时仍编码空字符串。

强制等式保持：`SourceTurnRef/Ordinal == Tool.Execution.Turn == ExpectedCurrent.Turn`。`SourceTurnRef`的Go类型是`contract.Ref`，本组件不得在Ref上伪造`Ordinal`字段；Harness committed PendingAction current与public Adapter无损提供同一exact Turn。Session exact证据同样只来自Harness current并经Application Port传递。

## 3. Validate字段顺序

每个public V2入口必须按以下顺序Fail Closed，并返回对应零值；untrusted输入路径不得调用`Must*`或panic：

1. `ctx != nil`且未cancel/deadline；
2. ContractVersion、ObjectKind、Owner exact discriminator；
3. required字段、时间顺序、正数预算与hard caps；
4. 所有nested exact refs逐项Validate，slice排序并拒绝semantic duplicate；
5. Identity/ExecutionScope/Run、由Harness committed PendingAction current经Application无损传递的Session/Turn exact evidence；
6. SourceTurn ordinal四项等式与`LegacyTurnID == SourceTurn.ID`；
7. Attempt/Observation/Result、DomainResult、Association、SettlementApplication逐项exact绑定；
8. Owner current facts、withdraw/tombstone/poison/license/scope/sensitivity；
9. stable ClosureDigest与ordered exact集合；
10. phase、fresh owner clock、TTL、NotAfter及clock rollback；
11. fresh Projection/Observation canonical digest重算；
12. 返回前再次检查ctx；ReadContent另执行Get后S2与body cap。

错误优先级继续沿用原error set：contract/type → coordinate/evidence → authority/scope/license/sensitivity → current/TTL → local content → unsupported。不得以NotFound或成功空列表掩盖coordinate、tamper或cancel。

## 4. StableClosureDigest V2精确冻结

### 4.1 常量与digest算法

以下值由第八审输入授权冻结；Go实现不得换字符串、复用V1 helper或增加隐式prefix：

```go
const (
    MemoryStableClosureDigestDomainV2     = "praxis.memory/context-source-current-reader/stable-closure"
    MemoryStableClosureContractVersionV2  = "praxis.memory/context-source-current-reader/stable-closure/v2"
    MemoryStableClosureObjectKindV2       = "memory_context_source_stable_closure"

    KnowledgeStableClosureDigestDomainV2    = "praxis.knowledge/context-source-current-reader/stable-closure"
    KnowledgeStableClosureContractVersionV2 = "praxis.knowledge/context-source-current-reader/stable-closure/v2"
    KnowledgeStableClosureObjectKindV2      = "knowledge_context_source_stable_closure"
)
```

算法精确为`contract.Digest(body)`，其中body先逐项Validate、所有slice按既有canonical规则正规化并拒绝semantic duplicate；不得使用`MustDigest`。`DigestDomain`、`ContractVersion`与`ObjectKind`是body前三个字段，因此已经进入JSON摘要，不再额外拼接字节prefix。

### 4.2 Memory canonical body

```go
type memoryStableClosureItemV2 struct {
    ContractVersion   string              `json:"contract_version"`
    ObjectKind        string              `json:"object_kind"`
    Rank              int                 `json:"rank"`
    Score             int                 `json:"score"`
    RecordRef         contract.Ref        `json:"record_ref"`
    ContentRef        contract.ContentRef `json:"content_ref"`
    SourceRefs        []contract.Ref      `json:"source_refs"`
    EvidenceRefs      []contract.Ref      `json:"evidence_refs"`
    ProjectionRefs    []contract.Ref      `json:"projection_refs"`
    CitationDigest    string              `json:"citation_digest"`
    DomainResultRef   contract.Ref        `json:"domain_result_ref"`
    AssociationDigest string              `json:"association_digest"`
    ApplicationRef    contract.Ref        `json:"application_ref"`
    TokenEstimate     int                 `json:"token_estimate"`
    EstimatorRef      contract.Ref        `json:"estimator_ref"`
}

type memoryStableClosureCanonicalBodyV2 struct {
    DigestDomain          string                      `json:"digest_domain"`
    ContractVersion       string                      `json:"contract_version"`
    ObjectKind            string                      `json:"object_kind"`
    Owner                 contract.OwnerDomain        `json:"owner"`
    TenantID              string                      `json:"tenant_id"`
    IdentityRef           contract.Ref                `json:"identity_ref"`
    IdentityEpoch         uint64                      `json:"identity_epoch"`
    ExecutionScopeDigest  string                      `json:"execution_scope_digest"`
    RunID                 string                      `json:"run_id"`
    SessionRef            contract.Ref                `json:"session_ref"`
    SessionEvidenceRef    contract.Ref                `json:"session_evidence_ref"`
    SourceTurnOrdinal     uint32                      `json:"source_turn_ordinal"`
    SourceTurnRef         contract.Ref                `json:"source_turn_ref"`
    TurnEvidenceRef       contract.Ref                `json:"turn_evidence_ref"`
    LegacyTurnID          string                      `json:"legacy_turn_id"`
    AttemptRef            contract.Ref                `json:"attempt_ref"`
    RequestDigest         string                      `json:"request_digest"`
    IdempotencyKey        string                      `json:"idempotency_key"`
    ObservationRef        contract.Ref                `json:"observation_ref"`
    ResultRef             contract.Ref                `json:"result_ref"`
    CurrentStateRef       contract.Ref                `json:"current_state_ref"`
    StatePlaneBindingRef  contract.Ref                `json:"state_plane_binding_ref"`
    QueryRef              contract.Ref                `json:"query_ref"`
    ViewRef               contract.Ref                `json:"view_ref"`
    WatermarkRef          contract.Ref                `json:"watermark_ref"`
    AuthorityRef          contract.Ref                `json:"authority_ref"`
    AuthorityEpoch        uint64                      `json:"authority_epoch"`
    PolicyRef             contract.Ref                `json:"policy_ref"`
    Purpose               string                      `json:"purpose"`
    Scopes                []string                    `json:"scopes"`
    SensitivityMax        string                      `json:"sensitivity_max"`
    Coverage              contract.Coverage           `json:"coverage"`
    NextCursor            string                      `json:"next_cursor"`
    ResultDigest          string                      `json:"result_digest"`
    EvidenceDigest        string                      `json:"evidence_digest"`
    Items                 []memoryStableClosureItemV2 `json:"items"`
    OrderedItemSetDigest  string                      `json:"ordered_item_set_digest"`
    ContentSetDigest      string                      `json:"content_set_digest"`
    SourceSetDigest       string                      `json:"source_set_digest"`
    EvidenceSetDigest     string                      `json:"evidence_set_digest"`
    ProjectionSetDigest   string                      `json:"projection_set_digest"`
    CitationSetDigest     string                      `json:"citation_set_digest"`
    MaxItems              int                         `json:"max_items"`
    MaxBytes              int64                       `json:"max_bytes"`
    MaxTokens             int                         `json:"max_tokens"`
    PerItemMaxBytes       int64                       `json:"per_item_max_bytes"`
    EstimatorRef          contract.Ref                `json:"estimator_ref"`
}
```

### 4.3 Knowledge canonical body

```go
type knowledgeStableClosureItemV2 struct {
    ContractVersion   string              `json:"contract_version"`
    ObjectKind        string              `json:"object_kind"`
    Rank              int                 `json:"rank"`
    Score             int                 `json:"score"`
    RecordRef         contract.Ref        `json:"record_ref"`
    PackageRef        contract.Ref        `json:"package_ref"`
    SnapshotRef       contract.Ref        `json:"snapshot_ref"`
    ContentRef        contract.ContentRef `json:"content_ref"`
    SourceRefs        []contract.Ref      `json:"source_refs"`
    EvidenceRefs      []contract.Ref      `json:"evidence_refs"`
    ProjectionRefs    []contract.Ref      `json:"projection_refs"`
    CitationDigest    string              `json:"citation_digest"`
    License           string              `json:"license"`
    LicenseDigest     string              `json:"license_digest"`
    TrustState        string              `json:"trust_state"`
    ConflictGroup     string              `json:"conflict_group"`
    ConflictDigest    string              `json:"conflict_digest"`
    DomainResultRef   contract.Ref        `json:"domain_result_ref"`
    AssociationDigest string              `json:"association_digest"`
    ApplicationRef    contract.Ref        `json:"application_ref"`
    TokenEstimate     int                 `json:"token_estimate"`
    EstimatorRef      contract.Ref        `json:"estimator_ref"`
}

type knowledgeStableClosureCanonicalBodyV2 struct {
    DigestDomain          string                         `json:"digest_domain"`
    ContractVersion       string                         `json:"contract_version"`
    ObjectKind            string                         `json:"object_kind"`
    Owner                 contract.OwnerDomain           `json:"owner"`
    TenantID              string                         `json:"tenant_id"`
    IdentityRef           contract.Ref                   `json:"identity_ref"`
    IdentityEpoch         uint64                         `json:"identity_epoch"`
    ExecutionScopeDigest  string                         `json:"execution_scope_digest"`
    RunID                 string                         `json:"run_id"`
    SessionRef            contract.Ref                   `json:"session_ref"`
    SessionEvidenceRef    contract.Ref                   `json:"session_evidence_ref"`
    SourceTurnOrdinal     uint32                         `json:"source_turn_ordinal"`
    SourceTurnRef         contract.Ref                   `json:"source_turn_ref"`
    TurnEvidenceRef       contract.Ref                   `json:"turn_evidence_ref"`
    LegacyTurnID          string                         `json:"legacy_turn_id"`
    AttemptRef            contract.Ref                   `json:"attempt_ref"`
    RequestDigest         string                         `json:"request_digest"`
    IdempotencyKey        string                         `json:"idempotency_key"`
    ObservationRef        contract.Ref                   `json:"observation_ref"`
    ResultRef             contract.Ref                   `json:"result_ref"`
    CurrentStateRef       contract.Ref                   `json:"current_state_ref"`
    StatePlaneBindingRef  contract.Ref                   `json:"state_plane_binding_ref"`
    QueryRef              contract.Ref                   `json:"query_ref"`
    ViewRef               contract.Ref                   `json:"view_ref"`
    SnapshotRef           contract.Ref                   `json:"snapshot_ref"`
    PointerRef            contract.Ref                   `json:"pointer_ref"`
    AuthorityRef          contract.Ref                   `json:"authority_ref"`
    AuthorityEpoch        uint64                         `json:"authority_epoch"`
    PolicyRef             contract.Ref                   `json:"policy_ref"`
    Purpose               string                         `json:"purpose"`
    Scopes                []string                       `json:"scopes"`
    AllowedLicenses       []string                       `json:"allowed_licenses"`
    SensitivityMax        string                         `json:"sensitivity_max"`
    Coverage              contract.Coverage              `json:"coverage"`
    NextCursor            string                         `json:"next_cursor"`
    ResultDigest          string                         `json:"result_digest"`
    EvidenceDigest        string                         `json:"evidence_digest"`
    Items                 []knowledgeStableClosureItemV2 `json:"items"`
    OrderedItemSetDigest  string                         `json:"ordered_item_set_digest"`
    ContentSetDigest      string                         `json:"content_set_digest"`
    SourceSetDigest       string                         `json:"source_set_digest"`
    EvidenceSetDigest     string                         `json:"evidence_set_digest"`
    ProjectionSetDigest   string                         `json:"projection_set_digest"`
    CitationSetDigest     string                         `json:"citation_set_digest"`
    LicenseSetDigest      string                         `json:"license_set_digest"`
    ConflictSetDigest     string                         `json:"conflict_set_digest"`
    MaxItems              int                            `json:"max_items"`
    MaxBytes              int64                          `json:"max_bytes"`
    MaxTokens             int                            `json:"max_tokens"`
    PerItemMaxBytes       int64                          `json:"per_item_max_bytes"`
    EstimatorRef          contract.Ref                   `json:"estimator_ref"`
}
```

### 4.4 Set digest精确算法

所有set digest都是stable派生值，不允许对完整`ProjectionItemV2`或fresh `CurrentProjectionV2`直接摘要。以下domain/version/ObjectKind冻结：

```go
const (
    MemorySetDigestDomainV2            = "praxis.memory/context-source-current-reader/set-digest"
    MemorySetDigestContractVersionV2   = "praxis.memory/context-source-current-reader/set-digest/v2"
    MemoryOrderedItemSetObjectKindV2   = "memory_ordered_item_set"
    MemoryContentSetObjectKindV2       = "memory_content_set"
    MemorySourceSetObjectKindV2        = "memory_source_ref_set"
    MemoryEvidenceSetObjectKindV2      = "memory_evidence_ref_set"
    MemoryProjectionSetObjectKindV2    = "memory_projection_ref_set"
    MemoryCitationSetObjectKindV2      = "memory_citation_digest_set"

    KnowledgeSetDigestDomainV2          = "praxis.knowledge/context-source-current-reader/set-digest"
    KnowledgeSetDigestContractVersionV2 = "praxis.knowledge/context-source-current-reader/set-digest/v2"
    KnowledgeOrderedItemSetObjectKindV2 = "knowledge_ordered_item_set"
    KnowledgeContentSetObjectKindV2     = "knowledge_content_set"
    KnowledgeSourceSetObjectKindV2      = "knowledge_source_ref_set"
    KnowledgeEvidenceSetObjectKindV2    = "knowledge_evidence_ref_set"
    KnowledgeProjectionSetObjectKindV2  = "knowledge_projection_ref_set"
    KnowledgeCitationSetObjectKindV2    = "knowledge_citation_digest_set"
    KnowledgeLicenseSetObjectKindV2     = "knowledge_license_digest_set"
    KnowledgeConflictSetObjectKindV2    = "knowledge_conflict_digest_set"
)
```

canonical body字段顺序冻结为：

```go
type memoryOrderedItemSetCanonicalBodyV2 struct {
    DigestDomain    string                      `json:"digest_domain"`
    ContractVersion string                      `json:"contract_version"`
    ObjectKind      string                      `json:"object_kind"`
    Items           []memoryStableClosureItemV2 `json:"items"`
}

type memoryContentSetCanonicalBodyV2 struct {
    DigestDomain    string                `json:"digest_domain"`
    ContractVersion string                `json:"contract_version"`
    ObjectKind      string                `json:"object_kind"`
    ContentRefs     []contract.ContentRef `json:"content_refs"`
}

type memoryRefSetCanonicalBodyV2 struct {
    DigestDomain    string         `json:"digest_domain"`
    ContractVersion string         `json:"contract_version"`
    ObjectKind      string         `json:"object_kind"`
    Refs            []contract.Ref `json:"refs"`
}

type memoryStringSetCanonicalBodyV2 struct {
    DigestDomain    string   `json:"digest_domain"`
    ContractVersion string   `json:"contract_version"`
    ObjectKind      string   `json:"object_kind"`
    Values          []string `json:"values"`
}

type knowledgeOrderedItemSetCanonicalBodyV2 struct {
    DigestDomain    string                         `json:"digest_domain"`
    ContractVersion string                         `json:"contract_version"`
    ObjectKind      string                         `json:"object_kind"`
    Items           []knowledgeStableClosureItemV2 `json:"items"`
}

type knowledgeContentSetCanonicalBodyV2 struct {
    DigestDomain    string                `json:"digest_domain"`
    ContractVersion string                `json:"contract_version"`
    ObjectKind      string                `json:"object_kind"`
    ContentRefs     []contract.ContentRef `json:"content_refs"`
}

type knowledgeRefSetCanonicalBodyV2 struct {
    DigestDomain    string         `json:"digest_domain"`
    ContractVersion string         `json:"contract_version"`
    ObjectKind      string         `json:"object_kind"`
    Refs            []contract.Ref `json:"refs"`
}

type knowledgeStringSetCanonicalBodyV2 struct {
    DigestDomain    string   `json:"digest_domain"`
    ContractVersion string   `json:"contract_version"`
    ObjectKind      string   `json:"object_kind"`
    Values          []string `json:"values"`
}
```

每个字段的唯一重算映射：

| Projection字段 | Memory ObjectKind/body | Knowledge ObjectKind/body | 提取与正规化 |
|---|---|---|---|
| `OrderedItemSetDigest` | `MemoryOrderedItemSetObjectKindV2` / `memoryOrderedItemSetCanonicalBodyV2` | `KnowledgeOrderedItemSetObjectKindV2` / `knowledgeOrderedItemSetCanonicalBodyV2` | 先把每个`ProjectionItemV2`逐字段投影为对应stable item body，明确排除item `ExpiresAt/Digest`；保持Owner排序后的原顺序，要求`Rank == index`且`Score desc -> Record ID asc -> Revision desc -> Digest asc`，拒绝重复Record ID |
| `ContentSetDigest` | `MemoryContentSetObjectKindV2` / `memoryContentSetCanonicalBodyV2` | `KnowledgeContentSetObjectKindV2` / `knowledgeContentSetCanonicalBodyV2` | 展开stable items的`ContentRef`；逐项Validate，按ID asc→Digest asc→Length asc→MediaType asc排序并compact exact duplicate |
| `SourceSetDigest` | `MemorySourceSetObjectKindV2` / `memoryRefSetCanonicalBodyV2` | `KnowledgeSourceSetObjectKindV2` / `knowledgeRefSetCanonicalBodyV2` | 展开全部`SourceRefs`，逐项Validate后调用`contract.NormalizeRefs` |
| `EvidenceSetDigest` | `MemoryEvidenceSetObjectKindV2` / `memoryRefSetCanonicalBodyV2` | `KnowledgeEvidenceSetObjectKindV2` / `knowledgeRefSetCanonicalBodyV2` | 展开全部`EvidenceRefs`，逐项Validate后调用`contract.NormalizeRefs` |
| `ProjectionSetDigest` | `MemoryProjectionSetObjectKindV2` / `memoryRefSetCanonicalBodyV2` | `KnowledgeProjectionSetObjectKindV2` / `knowledgeRefSetCanonicalBodyV2` | 展开全部`ProjectionRefs`，逐项Validate后调用`contract.NormalizeRefs` |
| `CitationSetDigest` | `MemoryCitationSetObjectKindV2` / `memoryStringSetCanonicalBodyV2` | `KnowledgeCitationSetObjectKindV2` / `knowledgeStringSetCanonicalBodyV2` | 展开`CitationDigest`；要求非空且无首尾空白，字节序升序并compact exact duplicate |
| `LicenseSetDigest` | 不适用 | `KnowledgeLicenseSetObjectKindV2` / `knowledgeStringSetCanonicalBodyV2` | 展开`LicenseDigest`；要求非空且无首尾空白，字节序升序并compact exact duplicate |
| `ConflictSetDigest` | 不适用 | `KnowledgeConflictSetObjectKindV2` / `knowledgeStringSetCanonicalBodyV2` | 展开`ConflictDigest`；要求非空且无首尾空白，字节序升序并compact exact duplicate |

每个digest都精确执行`contract.Digest(body)`；`DigestDomain/ContractVersion/ObjectKind`使用对应Owner常量，禁止`MustDigest`、map遍历、JSON map、额外prefix或二次hash。空集合也必须编码为非nil空slice`[]`并产生确定摘要，不能使用`null`。重算顺序固定为：Validate stable items → 构造stable item bodies → 分别正规化各集合 → 计算Memory适用的六个或Knowledge适用的八个set digest → 构造StableClosure canonical body → `contract.Digest(stableBody)`。任何set digest与按上述算法重算值不一致均为tamper，禁止只验证字符串非空。

构造body时必须使用上述冻结常量，并从已经Validate的`CurrentProjectionV2`逐字段复制；不得直接marshal整个Projection。`Coverage`后必须严格按`NextCursor string -> ResultDigest string -> EvidenceDigest string -> Items`声明和编码；三字段全部进入fresh Projection与stable closure，任一变化都必须改变相应canonical digest。无下一页时`NextCursor`以明确空字符串编码，禁止`omitempty`或省略字段。两个item body明确排除`ExpiresAt`和item `Digest`；两个closure body明确排除Session/Turn checked/expiry、`AttemptInspectionRef`、`CheckPhase`、`OwnerCheckedAt/OwnerObservedAt`、所有`ExpiresAt/NotAfter`、Projection/Observation self ref及其Digest。`AttemptInspectionV2`的canonical包含fresh `OwnerCheckedAt/ExpiresAt`，重新Inspect允许产生不同Inspection exact ref；该Ref只留在fresh `CurrentProjectionV2` digest，绝不能进入stable closure。S2 `CurrentRequestV2`无需携带S1 Inspection ref，只比较Source coordinate、stable closure与ordered exact集合。TTL或复检时间变化不改变stable closure，但会改变fresh digest并可能使S2失败。

### 4.5 fresh Projection/Observation（第八审已通过，不重开）

- Projection digest包含ContractVersion/ObjectKind、self ID/revision（self digest置空）、Owner、完整stable ClosureDigest、CheckPhase、fresh OwnerCheckedAt、ExpiresAt与全部输出字段。
- Observation digest包含ContractVersion/ObjectKind、self ID/revision（self digest置空）、Projection exact ref、stable ClosureDigest、CheckPhase、fresh OwnerObservedAt、ExpiresAt、StatePlaneBinding、Content exact字段及observed length/media/digest；Knowledge另含License。
- S1/S2 fresh Projection/Observation refs与digest允许不同；只要求Source coordinate、stable ClosureDigest与ordered exact集合相同。fresh ref相同不能豁免stable集合复核。

## 5. Owner evidence持久化与禁止迁移（第八审已通过，不重开）

V2 Owner Attempt记录必须原样持久化以下字段，且全部进入Attempt canonical digest：

```text
IdentityRef, IdentityEpoch,
Session exact coordinate, SessionEvidenceRef,
SourceTurnOrdinal, SourceTurn exact coordinate, TurnEvidenceRef,
LegacyTurnID, ExecutionScopeDigest, RunID,
evidence OwnerCheckedAt, evidence ExpiresAt,
Attempt/Observation/Result refs and RequestDigest
```

规则：

1. `PutAttempt`只接受具名Owner Reader输出的原始字段；不得接受Adapter派生或Context/Application回填值。
2. Inspection、Current Projection与Content Observation只回显已持久字段，并fresh复读对应evidence currentness。
3. V1若缺Identity exact ref/epoch、Session/Turn exact coordinate或evidence refs，不做后台补列、字符串解析、Context反查或默认值迁移。
4. 缺字段的V1只允许历史exact Inspect；要成为V2 current source必须创建新的V2 Attempt并重新取得Owner evidence。
5. V1/V2切换保持单一Capability/Binding generation；禁止双current或双写权威Attempt。

## 6. DomainResultAssociation digest（第八审已通过，不重开）

第八审已接受以下domain/version/canonical为Owner V2设计基线；本轮只在七个struct与stable closure body中引用`AssociationDigest string`，不修改算法：

```json
{
  "contract_version": "praxis.memory-knowledge/domain-result-association/v2",
  "object_kind": "domain_result_association",
  "owner": "praxis.memory | praxis.knowledge",
  "domain_result_ref": {"id":"...","revision":1,"digest":"sha256:..."}
}
```

算法为`AssociationDigest = sha256(canonical_json(payload))`。Digest字段本身不进入payload；SettlementApplicationRef不进入AssociationDigest，而是作为ProjectionItem与stable Closure中的独立exact ref。domain/version/object kind任一未来变更都必须升版并生成新golden，禁止兼容翻译。

Validate/Verify必须：

1. strict version/kind/owner；
2. DomainResultRef ID/revision/digest逐项有效；
3. 重算AssociationDigest；
4. 回读DomainResultFact并重算其canonical digest；
5. Owner与exact ref逐项相等；同ID换revision/digest、错Owner或tamper均`ErrSettlementMismatch`/evidence conflict；
6. 不解读或复制opaque Runtime Settlement。

Memory与Knowledge使用同一算法和domain separator、仅Owner值不同；该设计结论已通过，本轮不重开。

## 7. ctx-aware bounded local reader与可取消锁（第八审已通过，不重开）

V2 `LocalContentReader`已通过的设计语义为：

```text
StatePlaneBinding(context.Context) -> exact local binding
GetExact(context.Context, ContentRef, MaxBodyBytes) -> bounded copied bytes
```

它仍是sealed Owner-local接口，零network、Provider、Resolver和remote fallback。实现不得仅把现有不可取消`sync.RWMutex`等待包一层ctx检查后宣称支持取消；必须使用context-aware read gate或经测试证明等价的可取消获取机制。

三个public方法都必须：

1. 入参前检查ctx；
2. 等待Owner一致性锁域期间响应cancel/deadline；
3. 获锁后读取fresh owner clock并检测rollback；
4. `ReadContentExact`在同一Owner一致性锁域内执行S1 current/binding → `GetExact(ctx, ref, max)` → S2 fresh current/binding/closure；
5. Get前验证`ContentRef.Length <= MaxBodyBytes`，Get后验证`len(body) <= MaxBodyBytes`且length/media/digest exact；禁止截断后成功；
6. Get期间TTL/binding/current漂移或ctx取消时返回零Observation、nil body；
7. 返回前再次检查ctx并copy isolation；不得泄漏goroutine、锁或部分body。

取消返回兼容`context.Canceled`/`context.DeadlineExceeded`，不创建新Attempt、不改变Owner state、不降级NotFound/Unknown成功。

## 8. Context/Application边界

- Context是TransitionProof唯一Owner；Application只协调已发布的public三阶段Port。
- pre-frame request只含SourceTurnOrdinal与ExpectedTargetOrdinal及已有exact inputs；不含未来Frame/Generation refs，也不是proof。
- Context先seal pending DomainResult/Manifest/Frame/Generation，再seal final proof；随后Owner S2，最后atomic ApplySettlement+Generation CAS并publish。
- Context Owner-local Refresh/Apply/Inspect、single backend/lock、atomic CAS、两Owner Adapter、Application Port、`knowledge_reference`与nonzero reference fixture当前YES；production G6B/root仍NO-GO。

## 9. 当前双轴门禁

资产轻门要求：Memory与Knowledge各七个public struct均有完整Go type/JSON tag/字段顺序，两个`CurrentProjectionV2`不得含占位语；StableClosure常量与canonical body逐字段完整；Owner隔离、V1 closure不可复用、链接、尾随空白与diff-check通过。

- Owner-local：P0=0、P1=0、P2=0。七struct与StableClosure精确schema已经冻结；其他Owner设计项保持Review YES，不因未写Go重新计入设计缺口。
- Cross-owner reference：P0=0、P1=0、P2=0，Harness exact Turn、Context TransitionProof、Application三阶段Port、两Owner Adapter/nonzero fixture及`knowledge_reference` exact source chain均已实现。
- 实现：V2 Owner-local Reader与reference integration已完成软件验收；production root仍不得启用。
