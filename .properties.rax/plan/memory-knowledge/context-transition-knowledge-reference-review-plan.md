# P0-2/P0-5 联合审查与实现前退出计划

> 状态：**historical_review_plan / superseded_by_reference_implementation**。本文保留P0-2/P0-5实现前审查门；Context/Application/Harness/双Owner reference实现现已YES。production root与远程Retrieval仍未授权。

## 1. 候选资产

| External P0 | 候选资产 | 当前Owner结论 |
|---|---|---|
| P0-2 Context TransitionProof | [Memory Owner提交的联合候选](../../design/memory-engine/context-transition-proof-external-p0-candidate.md) | payload完整，可送Context/Application/Turn Owner审查；Context Owner未YES |
| P0-5 `knowledge_reference` | [Knowledge Owner提交的联合候选](../../design/knowledge-engine/knowledge-reference-external-p0-candidate.md) | payload完整，可送Context/Application审查；Context Owner未YES |

Owner-local V2合同继续以[Context Source Current Reader V2冻结合同](./context-source-reader-v2-frozen.md)为真值；P0-2/P0-5不得建立第二套Memory/Knowledge DTO或current。

## 2. 最短无SCC依赖DAG

```text
A. P0-1 named Turn exact Ref/Reader
          |
          +-----------------------+
          v                       v
B. P0-2 Context TransitionProof   C. P0-5 knowledge_reference
          \                       /
           v                     v
D. P0-3 Application namespaced 3-stage Port
                          |
                          v
E. P0-4 Memory/Knowledge adapters + nonzero cardinality + root
                          |
                          v
F. Context published exact Frame -> Harness
```

A/B/C的设计评审可并行；B/C实现都必须等待A公开nominal。D必须等待Context Owner分别接受B/C public nominal与Reader；E必须串行等待A-D。Harness只在F消费Context exact Frame，不新增Harness私有Hook/Port。

## 3. 固定调用顺序

```text
Runtime settled Tool refs
 -> Turn Owner exact current
 -> Memory/Knowledge S1 Owner-current/read exact
 -> Context pre-frame request
 -> Context pending DomainResult/Manifest/Frame/Generation seal（不可见）
 -> Context final TransitionProof seal
 -> Memory/Knowledge S2 fresh Owner-current/read exact
 -> Context复读stable集合、fresh associations、TTL/current
 -> Context atomic local ApplySettlement + Generation current CAS
 -> publish exact Frame
 -> Harness consume Frame
```

Observation → Owner DomainResult → Evidence → Runtime Settlement → Owner Apply 的领域链不被本计划替换。Remote Retrieval Gateway仍unsupported/Provider=0。

## 4. 分阶段退出条件

| 阶段 | 唯一Owner | 必须交付 | 退出证据 | 可并行/串行 |
|---|---|---|---|---|
| A Turn exact | Harness Owner + Harness-owned Application Adapter | live `CommittedPendingActionReaderV3`及Session/Turn applicability coordinate已存在；无损映射到Application既有Session/Turn nominal并携带TTL | 映射独立审计YES；target ordinary100/race20；Harness/Application full test/race/vet | 可与B/C资产审查并行；不新增Turn Store/Reader |
| B TransitionProof | Context Owner | pre-frame request、Proof Ref/StableBody/fresh Projection/Reader、同backend/lock、S1/S2、atomic Apply/CAS binding | Context独立审计YES；target100/race20；Context full/race/vet | 与C并行；D前必须YES |
| C knowledge_reference | Context Owner + Knowledge合同消费确认 | FragmentKind、Binding Ref/StableBody/fresh Projection/Reader、exact source chain、TTL/errors | 双Owner独立审计YES；target100/race20；各自full/race/vet | 与B并行；D前必须YES |
| D Application Port | Application Owner | namespaced prepare/pre-frame、apply/advance、inspect三阶段Port，只传exact refs | Application独立审计YES；target100/race20；full/race/vet | 串行等待A/B/C |
| E adapters/root | Memory/Knowledge/Application/Context各自Owner | 两Owner Adapter、Capability/Binding、nonzero cardinality、root接线；无跨实现导入 | 联合独立审计YES；target100/race20；各模块full/race/vet；G6B黑盒/故障注入 | 串行等待D |
| F Harness消费 | Harness Owner | 只消费Context published exact Frame；无Memory/Knowledge直连 | Harness/系统集成审计YES；blackbox/failure/race/vet | 串行等待E |

本轮没有Go，因此不运行或宣称上述测试证据；只做Markdown/link/diff/hash轻门。

## 5. Fail Closed验收矩阵

| 场景 | 必须结果 |
|---|---|
| P0-1 Ref缺失或ordinal/legacy不一致 | B/C/D均不启动；来源数0 |
| pre-frame request含未seal输出ref | `ErrInvalid/ErrConflict`，零pending发布 |
| proof stable body与pending/Frame/Generation漂移 | Conflict，零S2/零Apply |
| proof fresh Projection过期 | Expired，必须Inspect原attempt，不新建attempt |
| Knowledge stable chain、NextCursor、Result/Evidence、Citation/License/Conflict漂移 | Conflict，零Candidate/零Frame |
| fresh Projection ref/time变化但stable closure相同 | 允许fresh digest不同；继续比较S2 stable exact集合 |
| S2失败、clock rollback、Get跨TTL、evicted/poisoned | Fail Closed，零body/零Apply/零publish |
| Apply回包丢失 | Inspect Context原attempt/proof；Unknown不重试Execute |
| Apply成功但Generation CAS失败 | 零current Frame可见性，进入Context cleanup/residual |
| Context未接受kind/Reader或P0-3/P0-4未闭合 | `ErrUnsupported`，Memory=0、Knowledge=0 |

## 6. Review问题与当前P0/P1/P2

联合Owner必须明确回答：

1. Context Owner是否接受候选domain/version/ObjectKind、字段顺序与canonical envelope；
2. Context Owner是否接受TransitionProof只拥有stable body、fresh association仅在Projection/Apply中；
3. Context Owner是否接受`knowledge_reference`及`SourceBindingRef`替代不足的`SourceRef+SourceRevision`；
4. 是否接受复用Harness V3 Reader与Application既有Session/Turn nominal，并由Harness-owned Adapter无损投影；
5. Application三阶段Port如何无损承载exact refs而不复制Owner DTO；
6. Context atomic Apply+Generation CAS如何绑定ProofRef与S2 association；
7. G6B首次非零来源的Capability/Binding generation和回滚门。

当前真值：Owner-local P0=0/P1=0/P2=0保持；本轮候选未登记新的Owner-local缺口。**External P0仍为5**：P0-1、P0-2、P0-3、P0-4、P0-5均未因资产化关闭。候选交独立审后由对应Owner给出P0/P1/P2裁决；Memory/Knowledge不得自行宣布External YES。
