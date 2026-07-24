# P0-2 Context TransitionProof 联合审查候选

> 状态：**design_frozen / context_owner_implemented / reference_integration_software_test_yes**。Context Owner已发布create-once TransitionRequest/TransitionProof、InspectExact、history与最终Apply exact binding；Application三阶段协调和双Owner非零reference fixture已通过。本文保留为联合设计依据；production root与远程Retrieval Gateway仍NO-GO。

## 1. Owner、Consumer 与非Owner

| 对象/动作 | 唯一Owner | Consumer | 明确非Owner |
|---|---|---|---|
| pre-frame transition request、TransitionProof、pending DomainResult/Manifest/Frame/Generation、ApplySettlement、Generation current CAS | Context Owner | Application协调；Harness最终只消费published exact Frame | Memory、Knowledge、Application、Harness、Runtime |
| SourceTurn exact Ref/Ordinal | Harness committed PendingAction current | Harness Adapter无损传递；Application、Context、Memory、Knowledge复读 | Context、Memory、Knowledge、Application不得补造 |
| Memory/Knowledge stable closure与fresh current projection | 各领域Owner | Application传递给Context S1/S2 | Context不得改写为领域事实 |
| Runtime Operation/Settlement | Runtime Owner | Application只传exact ref | Context/Memory/Knowledge不得写Runtime Outcome |

Application只协调调用和exact refs；它不能seal proof、创建Context Fact或把proof存在解释为Frame current。Harness不读Memory/Knowledge、不读proof，只在Context发布后消费exact Frame。

## 2. Live复用与外部门

复用live Context公开对象：`contract.FactRef`、`contract.Digest`、`contract.ExecutionBinding`、`contract.ContextGenerationCurrentPointerV1`、`contract.ContextTurnRefreshPendingDomainResultV1`及其Manifest/Frame/Generation refs。复用Memory/Knowledge已审Owner-local Current Reader合同族；不建立第二Owner DTO、第二current或私有兼容接口。

live实现复用Harness committed PendingAction current与Application中立Session/Turn nominal，不建立第二个Turn Store/Reader。其不变量为：

```text
SourceTurnOrdinal == TurnOwnerFact(SourceTurnRef).Ordinal
                  == Tool.Execution.Turn
                  == ExpectedCurrent.Turn
legacy TurnID     == SourceTurnRef.ID
ExpectedTargetOrdinal == SourceTurnOrdinal + 1
```

缺任一具名Turn Owner exact证据时Fail Closed；uint32、legacy字符串、Run、Session缓存或Application字段均不能生成ID/revision/digest。

## 3. Canonical envelope

候选常量：

| 项 | 值 |
|---|---|
| ContractVersion | `praxis.context/turn-transition-proof/v1` |
| Request ObjectKind | `context_turn_transition_request` |
| Proof Ref ObjectKind | `context_turn_transition_proof_ref` |
| StableBody ObjectKind | `context_turn_transition_proof_stable_body` |
| Projection ObjectKind | `context_turn_transition_proof_current_projection` |
| Request domain | `praxis.context.turn-transition-request` |
| Proof ID domain | `praxis.context.turn-transition-proof-id` |
| Stable domain | `praxis.context.turn-transition-proof-stable` |
| Projection domain | `praxis.context.turn-transition-proof-current` |
| Pending state | `proof_sealed_pending` |

所有digest候选统一为：

```text
sha256(CanonicalJSON({
  "domain": <constant>,
  "contract_version": "praxis.context/turn-transition-proof/v1",
  "object_kind": <constant>,
  "body": <按下表字段声明顺序、Digest字段置空后的对象>
}))
```

Canonical JSON禁止unknown/duplicate/trailing字段；整数使用十进制无损表示；必填字段不使用`omitempty`；exact ref逐字段校验`ID+Revision+Digest`。任何seal/recompute失败返回错误，禁止panic。

## 4. Pre-frame request候选

`ContextTurnTransitionRequestV1`按以下顺序seal；它在pending输出生成前存在，因此**不得**含PendingDomainResult/Manifest/Frame/Generation ref：

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `ContractVersion` | string | 固定合同版本 |
| 2 | `ObjectKind` | string | 固定Request ObjectKind |
| 3 | `RefreshAttemptRef` | exact FactRef | Context Refresh原attempt |
| 4 | `SourceTurnRef` | P0-1 `TurnExactRefV1` | 只能来自具名Turn Owner Reader |
| 5 | `SourceTurnOrdinal` | uint32 | 非0，满足四项ordinal等式 |
| 6 | `ExpectedTargetOrdinal` | uint32 | `SourceTurnOrdinal+1`，溢出拒绝 |
| 7 | `ExpectedCurrent` | Context current pointer V1 | exact current、同Run/Session/Turn/Scope |
| 8 | `ChildExecution` | Context ExecutionBinding | Turn等于ExpectedTargetOrdinal；Run/Scope/Authority与请求一致 |
| 9 | `RequestedNotAfterUnixNano` | int64 | 大于owner check时间，只是上界 |
| 10 | `Digest` | Digest | Request domain重算 |

Request exact ref为`{ObjectKind string, ID string, Revision uint64, Digest Digest}`。`Revision=1`；`ID="ctx-turn-transition-request:v1:" + hex(sha256(CanonicalJSON({refresh_attempt_ref,source_turn_ref,source_turn_ordinal,expected_target_ordinal,expected_current_digest,child_execution})))`。ID seed不含时间；Ref Digest等于Request canonical digest。

## 5. TransitionProof Ref、StableBody、Projection

### 5.1 `ContextTurnTransitionProofRefV1`

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `ObjectKind` | string | `context_turn_transition_proof_ref` |
| 2 | `ID` | string | Proof ID domain派生 |
| 3 | `Revision` | uint64 | v1固定1 |
| 4 | `Digest` | Digest | 等于StableBody canonical digest |

`ID="ctx-turn-transition-proof:v1:" + hex(sha256(CanonicalJSON({transition_request_ref,pending_domain_result_ref})))`。同ID不同revision/digest必须Conflict，不能NotFound或last-wins。

### 5.2 `ContextTurnTransitionProofStableBodyV1`

字段声明顺序即canonical body顺序：

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `ContractVersion` | string | 固定合同版本 |
| 2 | `ObjectKind` | string | StableBody ObjectKind |
| 3 | `TransitionRequestRef` | exact request ref | 指向第4节对象 |
| 4 | `RefreshAttemptRef` | exact FactRef | 与request一致 |
| 5 | `SourceTurnRef` | P0-1 `TurnExactRefV1` | 与request一致 |
| 6 | `SourceTurnOrdinal` | uint32 | 与request及Tool/ExpectedCurrent exact一致 |
| 7 | `TargetTurnOrdinal` | uint32 | 等于request.ExpectedTargetOrdinal |
| 8 | `ExpectedCurrent` | Context current pointer V1 | S1时复读的exact current |
| 9 | `ChildExecution` | Context ExecutionBinding | TargetTurn、Run、Scope、Authority exact |
| 10 | `PendingDomainResultRef` | exact FactRef | 已seal但不可见 |
| 11 | `ManifestRef` | exact FactRef | 已seal但不可见 |
| 12 | `FrameRef` | exact FactRef | 已seal但不可见 |
| 13 | `GenerationRef` | exact FactRef | 已seal但不可见 |
| 14 | `StableSourceSetDigest` | Digest | Tool/Memory/Knowledge稳定集合；不含fresh refs/times |
| 15 | `State` | string | 固定`proof_sealed_pending` |

StableBody明确排除：S1/S2 fresh Projection/Observation/Association refs与digest、CheckPhase、OwnerCheckedAt、ExpiresAt、RequestedNotAfter、Proof self ref/digest、ApplySettlement/current pointer、Provider body。TTL变化或重新Inspect产生不同fresh ref时，只要stable exact集合未变，StableBody digest必须不变。

### 5.3 `ContextTurnTransitionProofCurrentProjectionV1`

| 顺序 | 字段 | wire类型 | 约束 |
|---:|---|---|---|
| 1 | `ContractVersion` | string | 固定合同版本 |
| 2 | `ObjectKind` | string | Projection ObjectKind |
| 3 | `ProofRef` | `ContextTurnTransitionProofRefV1` | exact |
| 4 | `StableBody` | StableBody | canonical重算必须等于ProofRef.Digest |
| 5 | `S1SourceAssociationSetDigest` | Digest | S1 fresh refs/phase/time/expiry的Context封装digest |
| 6 | `PendingApplicable` | bool | 仅true可继续S2；不等于Frame current |
| 7 | `OwnerCheckedUnixNano` | int64 | Context Owner fresh clock |
| 8 | `ExpiresUnixNano` | int64 | 严格晚于checked，且为所有上界最小值 |
| 9 | `Digest` | Digest | Projection domain重算 |

Projection是fresh inspection；其digest必须包含phase/time/expiry及S1 fresh association。它不进入StableBody，不能成为Harness Frame或Runtime Outcome。

## 6. Reader候选与Validate

```text
InspectContextTurnTransitionProofCurrentV1(
  context.Context,
  ContextTurnTransitionProofCurrentRequestV1,
) -> ContextTurnTransitionProofCurrentProjectionV1
```

Request字段顺序：`ContractVersion string`、`ObjectKind string=context_turn_transition_proof_current_request`、`ProofRef ContextTurnTransitionProofRefV1`、`TransitionRequestRef exact request ref`、`RefreshAttemptRef FactRef`、`SourceTurnRef P0-1 TurnExactRefV1`、`SourceTurnOrdinal uint32`、`TargetTurnOrdinal uint32`、`ExpectedCurrent ContextGenerationCurrentPointerV1`、`RequestedNotAfterUnixNano int64`、`Digest Digest`。

Reader必须在Context Owner同一backend/lock与fresh clock域中：

1. 严格Validate contract/object kind、request digest、nested exact refs、ordinal等式与Run/Session/Scope/Authority；
2. exact读取原TransitionRequest、原Proof StableBody、原pending DomainResult/Manifest/Frame/Generation；
3. 复读ExpectedCurrent仍为current，且attempt仍是pending、未withdrawn/evicted/poisoned/applied；
4. fresh计算S1 association与TTL，seal Projection；
5. 同ID不存在返回NotFound；同ID但revision/digest或nested内容漂移返回Conflict。

`context.Context`只承载取消/deadline，不承载业务事实。取消或deadline返回零Projection且零状态变化。

## 7. S1/S2、Apply 与Unknown

固定时序：

```text
Runtime settled Tool exact refs
 -> P0-1 named Turn Owner current read
 -> Memory/Knowledge Owner S1 current/read exact（来源数获批后）
 -> Context seal pre-frame request
 -> Context seal pending DomainResult/Manifest/Frame/Generation（均不可见）
 -> Context seal final TransitionProof + S1 fresh Projection
 -> Memory/Knowledge Owner S2 fresh current/read exact
 -> Context复读proof/ExpectedCurrent/pending与S2 stable集合
 -> Context Owner atomic local ApplySettlement + Generation current CAS
 -> publish exact Frame/Manifest/Generation
 -> Harness只消费exact Frame
```

S2 fresh Projection/Observation/Association refs可以与S1不同；必须保持相同SourceTurn、Purpose/Scope/Authority/Sensitivity、StableClosureDigest与canonical ordered exact集合。Context future Apply输入必须exact携带`ProofRef`、`StableSourceSetDigest`、`S2SourceAssociationSetDigest`、`ExpectedCurrent`和原Attempt/Pending refs；该Apply nominal归Context Owner，本文不私造其Go接口。

任一步lost reply只Inspect原attempt/proof；禁止新建attempt或重跑外部动作。proof已seal但Apply回包丢失时，Application调用Context `InspectContextTurnRefresh`/未来proof Reader；Memory/Knowledge只Inspect自己的原attempt。Unknown不能推断成功，也不能发布Frame。

## 8. TTL/currentness

`now >= ExpiresUnixNano`即过期。Projection TTL取以下非零上界最小值：request NotAfter、P0-1 Session/Turn current projection、ExpectedCurrent、Context parent/Recipe/Authority、pending DomainResult/Manifest/Frame/Generation、S1 Memory/Knowledge projection/item/content observation。任何缺失、已过期、clock rollback或无法证明同一Owner lock域均Fail Closed。

S2和Apply前必须重新取Context Owner fresh clock并复读上述current事实；caller时间只作因果/上界，不能授current。Proof StableBody可长期作为历史exact事实，但其fresh Projection过期后不可用于Apply。

## 9. Closed errors

候选只映射live Context闭集：

| 条件 | error |
|---|---|
| malformed、unknown field、type/version mismatch、ordinal overflow | `ErrInvalid` |
| exact ref/digest/current pointer/stable集合漂移或重复语义键 | `ErrConflict` |
| `now >= expiry`、锁等待跨TTL、clock rollback | `ErrExpired` |
| Scope/Authority/Identity/Session/Turn不一致 | `ErrUnauthorized` |
| exact ID不存在、pending已evicted | `ErrNotFound` |
| Owner reader/backend不可用 | `ErrUnavailable` |
| inspection不完整、lost reply未闭表 | `ErrUnknown` / `ErrInspectOnly` |
| Context Owner未发布合同、非零来源或root未授权 | `ErrUnsupported` |

错误优先级：contract/type → exact coordinate/ref → authority/scope/session/turn → current/TTL → backend availability → unsupported。失败必须返回零Projection、零Frame可见性、零CAS变化。

## 10. 必须拒绝的反例

1. Application或Memory/Knowledge seal TransitionProof；
2. 从Tool uint32或legacy TurnID补造SourceTurn exact ref；
3. pre-frame request提前携带尚未seal的Frame/Generation ref；
4. proof未绑定pending DomainResult、ExpectedCurrent、childExecution或exact Frame/Generation；
5. proof sealed即发布Frame，随后才做S2；
6. S2 stable集合漂移仍沿用S1 proof；
7. 只因S1/S2 fresh ref或TTL不同就误判stable漂移；
8. 把fresh association/time/expiry/self ref加入StableBody使TTL变化改变stable digest；
9. Apply成功但Generation current CAS失败仍暴露Frame；
10. lost reply分配新attempt，或Unknown直接重试Execute；
11. stale/evicted/poisoned pending或proof仍返回PendingApplicable=true；
12. Memory/Knowledge或Context缓存成为Turn/领域权威事实；
13. Harness直接读取proof或Memory/Knowledge正文；
14. production root在未装配同一套exact Reader/Adapter/Context ports时直接启用非零来源。

## 11. 无SCC DAG与联合裁决门

```text
P0-1 named Turn exact Ref/Reader
        |
        v
P0-2 Context request/proof contract + Owner backend/reader
        |
        +----------------------+
        v                      v
P0-3 Application 3-stage Port  P0-5 knowledge_reference contract
        \                      /
         v                    v
       P0-4 two Owner adapters/nonzero/root
                    |
                    v
             Context exact Frame -> Harness
```

实现顺序已闭合为Harness exact current→Application三阶段→Context TransitionRequest/Proof→双Owner S2→Context atomic Apply/CAS。reference integration已关闭原P0-1至P0-5；production root仍须由装配Owner显式启用并验收。
