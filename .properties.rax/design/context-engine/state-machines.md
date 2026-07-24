# Context Engine状态机与恢复

## 1. Recipe发布状态机

```text
draft -> validated -> evaluated -> review_pending -> published
  |         |            |              |             |
  +------> rejected <----+--------------+             +-> superseded
                                                   \-> revoked
published --rollback release--> published(历史版本的新Current Binding)
```

- `validated`只证明Schema/静态规则；
- `evaluated`绑定固定Evaluation Set和结果；
- Review由政策决定，但“无需Review”也必须是明确事实；
- `published`必须CAS预期Base Revision；
- rollback不改历史事实。

## 2. Frame Attempt状态机

```text
reserved -> collecting -> admitted -> manifest_frozen -> rendered -> frame_frozen
    |           |            |              |              |
    +---------- failed <-----+--------------+--------------+
    |                                                      |
    +---------------------- reconciling <--- lost reply ----+
                                      \-> exact Inspect -> resume|failed
frame_frozen -> delivered -> outcome_pending -> outcome_recorded
```

不变量：

- 先reserve-once再读取可能漂移的Source；
- 每次CAS revision单调+1；Frame Fact自身冻结为rev1；
- Manifest冻结后不再解析新Source；
- Create/CAS回包未知只Inspect exact Attempt/FrameID；
- 同FrameID不同Digest为EvidenceConflict；
- Frame过期后不能重新授权消费，但保留历史Inspect。

### 2.1 per-turn Refresh与Harness Turn

```text
G6A settled ToolResultV2
  + current V4 Inspection
  + verified Association
  -> Application-owned SettledActionContextSourceCurrentReaderV1
  -> Application calls RefreshContextTurnV1
  -> test fixture manual injection (A/B isolation) OR production composition root (C enablement)
  -> G6B S1 owner-current reread
  -> refresh_reserved(deterministic Attempt/Frame/Generation IDs; Tool=1, others=0)
  -> sources_collected
  -> manifest_frozen
  -> frame_frozen
  -> pending_domain_result_recorded (no current pointer)
  -> Application calls ApplyContextTurnRefreshV1
  -> S2 fresh owner-current reread
  -> atomic context_apply_settlement + expected-generation-current CAS
  -> applied_current / G6B acceptance candidate
  -> [enablement only] Application requests harness continuation(next Turn, exact FrameRef+Digest)
```

- 首切面必须且只能消费一个G6A已验证的settled ToolResultV2；ToolResult、V4 Inspection与Association任一exact ref不一致都不得reserve；
- `CTX-D09-R1`已冻结为零Runtime Settlement；状态机A/B-local、Application Adapter与Memory/Knowledge B-cross fixture已完成。Apply输入不得包含V4、additive Runtime settlement或Tool settlement；
- Source cardinality固定`Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`；Memory/Knowledge先经Owner V2 Reader的S1/S2 exact association与每Owner64KiB物化上限，非此基数必须在任何Owner Store/CAS写入前拒绝；
- S1/S2都必须核验Session revision/Turn、PendingAction、父Frame/Generation、Assembly Generation/Binding/Activation、Authority、全部Source/Artifact/Cache TTL与currentness；
- 父Frame不可变；StablePrefix与SemiStable逐项复用exact `ContentRef{Ref,Digest,Length}`，仅DynamicTail追加；PrefixDigest或稳定cache identity漂移必须拒绝；
- `Refresh`核验settled Tool exact chain并只产生pending Context DomainResult；`Apply`必须先S2，再原子提交ApplySettlement+Generation current CAS；`Inspect`只读；三段不得合并或跳序；
- Refresh/Create/Apply/CAS回包未知只`InspectContextTurnRefreshV1`原Attempt/Frame/Generation；不得用新ID补偿；cancel/deadline在写后同样进入`waiting_inspect`；
- S2遇到TTL crossing/current drift时最多保留pending/diagnostic fact，不得写ApplySettlement成功或使Generation current pointer可见；
- G6B验收前不得调用Harness；启用后Harness Candidate创建/CAS未知仍由Harness按exact Candidate/Session Inspect恢复，Context不得构造Continuation或替它推进Turn；
- Frame交付后新Source只能进入下一Turn/Generation。

Refresh Attempt状态冻结为：

```text
absent -> reserved -> pending_domain_result -> s2_checking -> atomic_applying -> applied_current
             |              |                    |                 |
             +----------> rejected               +-------------> rejected
             +----------> waiting_inspect <------ unknown/cancel/deadline/lost reply
                                                \-> Inspect original attempt -> pending|applied|rejected
```

`waiting_inspect`不是成功或失败推断；只允许exact Inspect。写前cancel/deadline保持零状态，写后不得返回stale Prepared/Result投影。ApplySettlement与Generation current CAS必须同一原子可见性边界；观察到任一单边成功均为Conflict并保持current不可见。

## 3. Candidate Admission状态机

```text
submitted -> inspected -> admitted
                     \-> excluded(optional only)
                     \-> rejected
                     \-> stale -> inspect current version or fail
```

权限、Scope、Authority、Freshness或Source Digest无法验证时不得admit。Required Source不可用导致Frame失败；Optional按Recipe降级并留下Residual。

## 4. Cache状态机

```text
planned -> write_admitted -> dispatch_permitted -> executing
                                           |          |
                                           |          +-> observation
                                           |          +-> unknown -> inspect_only
                                           |                         \-> observation|not_applied|failed
                                           +-> expired/denied
observation -> owner_inspected -> committed -> hit_current
                                   |             |
                                   +-> conflict  +-> invalidated|expired|authority_stale
```

- Plan不是Entry；Receipt不是hit；
- 远程或写入均需Effect；
- `unknown`禁止重新Dispatch；Remote Inspect是独立Operation Effect；
- hit时复读Partition、Authority、Capability、TTL和Invalidation Generation；
- Cleanup/删除同样走Effect与Settlement。

## 5. Artifact Anchor状态机

```text
current -> unchanged
current -> version_changed -> delta_available -> rebased/current
                         \-> delta_conflict -> rematerialize -> current
current -> expired|authority_stale|generation_dropped -> invalid
```

Anchor不声明文件“现在没变”，只记录某Frame已见版本。`unchanged`必须来自Artifact Owner的当前Inspect。

压缩切换到新Generation时，未出现在`RetainedAnchorSet`中的旧Anchor直接进入`generation_dropped -> invalid`；仅摘要提及文件不构成保留。Delta的BaseAnchor、base/target文件版本或范围任一不一致都走`rematerialize`。

## 6. Compaction/Generation状态机

```text
generation_active -> compaction_candidate -> compacted_generation_active
        |                    |                         |
        |                    +-> rejected             +-> later_compaction
        +-> frozen_history (Continuity retains)
```

压缩失败不破坏当前Generation。新Generation必须引用旧Generation、摘要来源范围、保留Anchor、Open Effects与Outstanding Work。Generation current切换只由Context Owner使用expected revision+digest CAS；丢回包只Inspect原Generation ID。

首个Owner-local实现切面分两步：`Prepare`只验证exact current、冻结Summary与候选Generation，候选不可见；`Apply`必须在同一Owner backend内S2复读并以expected Generation current CAS原子发布。当前步骤不得把Prepare成功解释为current，也不得让Continuity Event参与CAS裁决。

## 7. Expected/Actual Injection状态机

```text
expected_frozen -> actual_observed -> inspected -> matched|allowed_residual|rejected|unknown
                               \-> observation_unknown -> unknown
```

`unknown`不能冒充通过；强制Instruction/Authority/Secret/Workspace/Tool Surface漂移为`rejected`。

`matched`还要求Expected处于`Created <= current < Expires`，Actual携带非空、规范排序且唯一的类型化Observation refs，并对Execution/Route/Attempt/Frame/source sequence/Revision/Digest逐项Inspect；partial/unavailable fidelity进入`unknown`。

## 8. Prompt反馈状态机

```text
feedback_observed -> candidate -> evaluated -> review_pending -> release_CAS -> published
                             \-> declined     \-> expired      \-> conflict -> re-evaluate
```

模型自评、Provider Usage或单次成功均不足以发布。发布结果回包未知时Inspect exact Release Attempt和Effect Settlement。

### 8.1 PromptAsset Owner-local pre-release

```text
immutable PromptAssetV1
  -> draft -> validated -> evaluated -> review_pending
       |          |            |               |
       +----------+------------+---------------> rejected
```

Prompt与Recipe lifecycle nominal严格分离，不允许同字符串FactRef互换。资产片段只产生Candidate；Recipe仍决定Admission/区域/最终顺序。并发successor只有一个CAS赢家；Create/Advance回包未知只Inspect exact asset/lifecycle head。production `published/superseded/revoked`不在Owner-local状态闭集内。

## 9. 恢复原则

1. 所有真实外部Effect严格按既有Runtime治理顺序推进并消费其Owner支持的Operation Settlement；CTX-D09本地迁移不属于该链，固定为`pending DomainResult → S2 → atomic ApplySettlement+Generation current CAS`；
2. Begin前失败可确认not_applied；Begin后未知进入reconciling，只Inspect原attempt；
3. Remote Inspect是绑定原Effect revision的独立Effect；
4. Provider Observation经领域Owner Inspect/CAS后才形成Context/Cache DomainResultFact；
5. 外部Effect的Runtime Operation Settlement与对应Context领域事实不互相冒充；CTX-D09不得把现有V4或上游Tool settlement作为本地Apply依据；
6. Rewind创建新Instance/Generation，不宣称Provider Cache、文件或外部世界回滚；
7. Residual、Unknown Effect、Cleanup分别报告并占用冲突域。
8. Context不向Timeline分配序列、不直接写Continuity Store或RocksDB；Continuity只在Context事实落定后投影exact Owner refs。
