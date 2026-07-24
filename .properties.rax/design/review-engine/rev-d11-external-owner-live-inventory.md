# REV-D11 外部 Owner live 依赖盘点

## 1. 结论与边界

- 盘点快照：`2026-07-17T23:13:07+08:00`。
- 固定状态：**Review-owned asset P0/P1/P2=0；production准入P0=6（Binding/Evidence/Policy/Authority/Scope各1项，composition/root 1项）；五类Owner与production root全部OPEN/NO-GO**。
- 本文件只把 live public 类型、Reader、缺口和准入证据对齐到已冻结的 [REV-D11 Review 侧合同](rev-d11-external-current-reader-v1.md)，不增加业务语义，不定义外部 Owner 的 Go 合同，不授权 Go、Adapter、fixture、benchmark 或 production root。
- `DecisionExternalCurrentReaderV1` 仍只是 Review-owned 聚合 seam；`memory`、`internal/testkit` 与 Store conformance 不是 production external current source。
- R-CTY `EvidenceSubjectCurrentReaderV1` 已是 Evidence Owner 的通用subject-current底座，但它不自动证明`ReviewEvidenceRefV2`对Review Target/Scope的applicability；复用底座不等于关闭Evidence P0。

## 2. live 文件指纹

以下 SHA-256 是本次结论的 stale 边界。任一文件变化后，本盘点只可视为旧快照，必须由 Review Owner 重新逐字段复读，不能沿用本结论授予实现准入。

| live public 文件 | SHA-256 |
|---|---|
| `ExecutionRuntime/runtime/ports/review_v2.go` | `69b8719eed7dc519db23c7decbcefb9f53e02c614931259d5b2913f158e8b4d2` |
| `ExecutionRuntime/runtime/ports/binding_currentness_v2.go` | `43c960df736a01abdacafc1cf7587b870e43261efa5908e91b42f323080d9268` |
| `ExecutionRuntime/runtime/control/provider_binding_currentness_v2.go` | `2a446ee035c05385938e9bfab8cc896a06075f8491f2f51d9144734db0563cc1` |
| `ExecutionRuntime/runtime/ports/evidence_v2.go` | `5c026980a028a495832c2fc0c9c24083a31ae703fbb98f10a5a85b80ce2ce8bf` |
| `ExecutionRuntime/runtime/ports/evidence_subject_current_v1.go` | `cb76417d3134b6ad4bf92de3041ee16f873e2555144b0f173a52044c96fac49b` |
| `ExecutionRuntime/runtime/ports/governance_v2.go` | `22b6fd09cd24fa97d5bbcb5966747a788a0ebbf691cf26e98782a0014ecdd097` |
| `ExecutionRuntime/runtime/ports/execution_governance_v2.go` | `dbb4976a9a3a4cbd93970176b9147a870fbb0fe1a1d8475daf77a5f1f57a422d` |
| `ExecutionRuntime/runtime/ports/operation_review_authorization_v4.go` | `4bf7daedfaef762e7febc7a8b5fe8b5c24c1489dd4c4e48d011f1d0c05e07a46` |
| `ExecutionRuntime/runtime/ports/generation_binding_v1.go` | `f6cafd1fe4c174bbcf105b99fb528531c1357e06bf953617b59053c2f342b992` |
| `ExecutionRuntime/harness/assemblycontract/types.go` | `e29ca82e9438faf9eea337ca68bc648b0991c679768bdb20348de392221e683a` |
| `ExecutionRuntime/review/ports/current.go` | `699e9eb8f19a7b033aa8c051b8ce9886ce4eeacfb93c6bf2ff8eec3ed4f2228d` |
| `ExecutionRuntime/review/contract/current.go` | `04a564980317051979d4e6ffbfdc1286e62073ea20b4b25fab26dccc8b230634` |
| `ExecutionRuntime/review/runtimeadapter/reader_v4.go` | `31f78fed354f0ed364819fad2a3e62382558fa41039dc42119c954c94fb73c9f` |
| `ExecutionRuntime/review/cmd/review-service/main.go` | `c4b8e8609092958b1c6d82744e2b6013e8b492256d031ee36c1404d078e7d228` |
| `ExecutionRuntime/review/service/service.go` | `db0a3348a2dcfbf311d0c5486c2b0ebe4b81aafcd9237ed33ae7130ef29b7f0c` |
| `ExecutionRuntime/review/conformance/store.go` | `611b7538efcd0bb18f29dcc3a95fa37b7450fe5d170b6acd8c86bf962b0610e0` |

## 3. 五类 Owner 与 root 精确盘点

| 依赖 | 现有可复用公开类型 / Reader | 缺失 exact 字段 / 语义 | Owner | 最小 Delta | 实现准入证据 | 当前 |
|---|---|---|---|---|---|---|
| Binding current | `ReviewComponentBindingRefV2`与`ProviderBindingRefV2`的六字段可显式无损映射；`ProviderBindingCurrentnessPortV2`与`ProviderBindingCurrentnessAdapterV2`已复读完整BindingSet、全member Fact、Set/Fact Grant和真实min TTL；[第四候选](rev-d11-binding-authoritative-current-port-delta-v1.md)仅是Review侧候选 | live仍缺Review Assignment/Target subject-current projection/history/index及host consumer association/current证明；Provider nominal不可alias或type-pun为Review Source/association | Runtime Binding Owner | 复用现有Provider currentness内核，只增Review nominal subject projection、bound consumer association、Owner-only publish/CAS/history/current及S1/S2 exact复读；Review只拿Reader | BIND-01..28 ordinary100/race20/full/race/vet；双独立0/0/0；指纹回读 | **OPEN / NO-GO / Binding P0=1；第四候选不是实现授权** |
| Evidence applicability/current | `EvidenceSubjectCurrentReaderV1`已提供Record+Source stable Projection/Index/history/current、Registration、Source Policy、Scope、Authority、Producer/Consumer Binding、Readability/Presence和可选full `EvidenceOwnerFactRefV2`；`ReviewEvidenceRefV2`与`EvidenceOwnerFactRefV2`字段仍可无损引用 | `ReviewEvidenceRefV2.Ref` 仍是弱字符串；live无Owner-sealed `ReviewEvidenceRefV2 -> EvidenceSubjectKeyV1 + EvidenceOwnerFactRefV2`的exact Target/Scope applicability association；Review不得自行猜Record/Source或补签Projection | Evidence Owner | 不新建Evidence Store或第二subject-current index；只增Review Evidence applicability association/Reader，内部引用已验收R-CTY projection/index和exact OwnerFact | full Ref/classification/digest换内容、Target/Scope/跨tenant、N项独立TTL、OwnerFact缺失、R-CTY旧Ref/ABA/lost-reply/rollback conformance；独立0/0/0；指纹回读 | **OPEN / NO-GO / Evidence P0=1** |
| Policy current | `ReviewPolicyBindingRefV2`、`ReviewPolicyFactV2`、`ReviewPolicyFactReaderV2`与`ReviewPolicyFactV2.ValidateCurrent`可复用权威Fact、canonical digest和exact Candidate校验 | 弱字符串Inspect不提供full Source/Target/Run/Scope subject-current resolve；缺immutable ProjectionRef/Revision/Checked/ProjectionDigest、historical exact Inspect和closed errors | Policy Owner | 不复制Policy Fact；只加Review-decision policy exact-current/historical wrapper、current index与Owner-sealed projection identity | Target/Run/Scope/Policy binding漂移、revoke/tombstone/TTL、same-ID conflict、ABA/lost-reply/rollback/deep-clone conformance；独立0/0/0；指纹回读 | **OPEN / NO-GO / Policy P0=1** |
| Actor / Reviewer Authority current | `AuthorityBindingRefV2`、`AuthorityFactReaderV2`、`DispatchAuthorityFactV2`与`ValidateCurrent`可复用authority binding/fact、exact Scope/ActionScope/Epoch校验；`OperationGovernanceCurrentReaderV3`只可复用subject-current模式 | 弱字符串Reader没有Actor/Reviewer分角色exact subject；V3 operation aggregate不能替代Target+Assignment/Reviewer的两个Owner projection | Authority Owner | 不复制Authority Fact；版本化增加Actor/Reviewer role-aware exact-current/historical Reader，或显式role-discriminated sealed union | 两role独立漂移/TTL/revoke、跨reviewer/assignment/tenant/scope、role-pun、ABA/lost-reply/rollback/deep-clone conformance；独立0/0/0；指纹回读 | **OPEN / NO-GO / Authority P0=1** |
| Scope current | `ExecutionScopeBindingRefV2`、`ExecutionScopeFactReaderV2`、`ExecutionScopeCurrentFactV2`、Fact digest及现有`ValidateCurrent`可复用；`OperationGovernanceFactRefV3`仅可复用ref形态 | 现有`ValidateCurrent`硬绑`EffectIntentV2`、active Run与capability digest，不适用全部Review Target；仍缺REV-D11 Source+Target/Run/Scope subject-current projection/history/index和exact Inspect | Scope Owner（Runtime governance/current-scope Owner） | 不新建Scope Fact；版本化增Review-decision scope exact-current/historical wrapper，封存existing Fact ref与projection identity | Run/Target/scope/capability/activation/instance/binding漂移、TTL/tombstone/跨tenant、ABA/lost-reply/rollback/deep-clone conformance；独立0/0/0；指纹回读 | **OPEN / NO-GO / Scope P0=1** |
| Review conformance / production root | `DecisionExternalCurrentReaderV1`/Request/Projection已是Review聚合seam；`GenerationBindingAssociationCurrentReaderV1`与Harness `AssemblyHandoffV1`/`AssemblyBindingConformanceV1`可作为root的现有装配输入 | production `DecisionExternalCurrentReaderV1`实现不存在；`CurrentFactSourceV4`仅有test fixture；`review-service` root只组装SQLite `service.New`+HTTP，未组装`decisioncurrent.SourceV1`/`verdictowner.Owner`；V4 Reader是Verdict后下游投影，不可反向冒充五Owner source | Review Owner负责聚合/conformance；production composition Owner负责root | 五类Reader全部独立验收后，才可实现Review只读聚合、reusable suite与宿主内部Decide协调；root不持Owner写口/Fake/mutable registry，HTTP不暴露direct Verdict写口 | 五类Owner准入满足；Review targeted100/race20/full ordinary/race/vet/import-DAG/root integration；独立0/0/0；用户明确授权production root | **OPEN / NO-GO / composition-root P0=1** |

## 4. 可复用边界与禁止升级

1. live exact refs与Owner Facts可以作为未来公开projection的被封存输入；它们当前不是REV-D11 immutable current projection。
2. `ReviewComponentBindingRefV2`可逐六字段显式映射到`ProviderBindingRefV2`后调用现有currentness Reader，但不能alias/type-pun；`ProviderBindingCurrentProjectionV2`不是Review subject projection或consumer association。
3. `OperationGovernanceCurrentReaderV3`是operation subject聚合读面，不能替代Actor Authority、Reviewer Authority或Scope三个nominal Reader。
4. `EvidenceSubjectCurrentReaderV1`能证明exact Record+Source的subject currentness，但不能凭`ReviewEvidenceRefV2.Ref`推导Record/Source，也不能自动证明该Evidence对Case/Target/Scope适用。
5. `OperationReviewCurrentReaderV4`消费已决定Review及其下游current facts，不能形成Verdict Decide的循环source。
6. Owner Fact自身的`Digest`、`ProjectionWatermark`或Review aggregate digest都不能补作Owner `ProjectionDigest`。
7. 任何缺Reader、弱查询、未冻结error closed set、Owner文件指纹漂移或production root缺失都必须Fail Closed；不得以test fake、by-name/latest或Review私有兼容接口升级。

## 5. 最小接线波次

以下只是跨Owner收口顺序，不是Go实现授权；前一波未通过各自Owner准入与独立复审时，后续波不得进入production组装。

1. **Evidence applicability薄层**：复用已验收R-CTY `EvidenceSubjectCurrentReaderV1`，只冻结Review Evidence applicability association/Reader及Owner conformance；不新建Evidence Store/index。
2. **Binding consumer association与Review nominal current**：复用`ProviderBindingCurrentnessPortV2`，关闭host consumer association、Review Assignment/Target subject、history/current/index与BIND-01..28。
3. **Policy/Authority/Scope三个窄exact-current wrapper**：可并行设计与Owner-local conformance，但各自必须独立0/0/0；不复制原Fact。
4. **Review production aggregate**：仅在前三波全绿后实现`DecisionExternalCurrentReaderV1`、S1 resolve+exact Inspect、S2同exact Ref复读、全输入真实min TTL和reusable conformance。
5. **production composition/root**：最后组装Generation/Binding/Harness conformance、五类read-only capability、`decisioncurrent.SourceV1`与内部`verdictowner.Owner`协调；不向HTTP、SDK或组件暴露direct Verdict写口。

## 6. 实现准入总门

只有以下证据同时成立，Review Owner才可请求进入下一实施门；本盘点没有使任何一项成立：

1. 五类Owner分别冻结公开nominal Source/Subject/ProjectionRef/Projection/Reader/closed errors，且字段满足REV-D11；
2. 每类Owner证明linearizable current-index resolve、create-once immutable projection、historical exact Inspect、deep clone、S1/S2 index stability、TTL与rollback；
3. 五类Owner conformance与独立复审均为0/0/0；
4. Review reusable external-current conformance获得单独实现授权并实际通过，不由fixture伪造Owner事实；
5. production composition Owner冻结只读依赖DAG和生命周期，明确无Owner写口、无全局mutable registry、无production Fake；
6. 用户明确授权Go实现和production root，且live SHA-256重新盘点无stale。

因此当前结论保持：**Review-owned asset P0/P1/P2=0；production准入P0=6仍全部OPEN；五类Owner与production root全部NO-GO**。
