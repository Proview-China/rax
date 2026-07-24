# Sandbox v2 验收合同

## 1. 当前验收边界

普通 lifecycle、Workspace commit、Checkpoint Participant/Provider/Evidence/Settlement/Apply、
Snapshot `reserved -> available`与加密Content Store、fresh Instance Restore、SQLite、SDK/API/CLI、
Host root及真实Host/containerd/QEMU-KVM/Wasmtime后端已经实现。

Snapshot terminal Retention/Legal Hold、Runtime governed purge/cleanup sibling、Management
CurrentIndex/Tombstone、Agent Host最终注册和deployment certification仍是跨Owner门；下述SA门继续
约束未来跨Owner实现，Sandbox不得用私有DTO补齐。

1. Runtime唯一拥有SandboxLease、Fence、Instance epoch与Operation Settlement；Sandbox只拥有allocate/activate/open/inspect/release/workspace/checkpoint participation/cleanup等DomainResultFact与ApplySettlement。
2. `allocate`、`activate`、`open`、`checkpoint`、`restore`、`cancel`、`close`、`fence`、`release`、`inspect`、`cleanup`、`workspace-commit` 各为独立 Effect，不能合并偷步。
3. 所有 Effect 都映射到 Operation V3 的 Subject/Intent/Admission/Governance/Delegation/Observation/Settlement 链，不另建 Sandbox Gateway。
4. Provider/Enforcer 只返回 Observation/Receipt；Sandbox Owner 必须独立 Inspect、验证精确绑定并 ExpectedRevision CAS 后才能提交领域事实。
5. Begin 后 Unknown 只 Inspect 原 Attempt；Provider NotFound、进程死亡、超时均不恢复自动重派权。
6. Close 不 Cancel active Run；Release 不证明 Cleanup；Restore 创建新 Instance、更高 epoch、新 Runtime Lease，外部世界不回滚。
7. Host/Container/MicroVM/WASM 按 capability/conformance 评估；Remote 仅为 locality；不可 Fence、不可 Inspect 或存在 raw bypass 的能力不得路由高风险 Effect。
8. Workspace 使用 view→overlay→diff→独立 governed commit；Checkpoint 只形成参与者事实，不自称全局一致。
9. Slot/Phase 仅声明 Observer/Filter/Gate/Port 贡献需求，不发明公共枚举，不接入 Harness 私有 Port。
10. 所有公共缺口以 `port-delta.md` 的结构化 Delta 上报；实现前必须取得相应 Owner 裁决，不能私建兼容接口。

任一项未获联合评审确认，Sandbox v2 均不得进入实现阶段。

## 2. 合同行为验收

| 编号 | 必须证明的行为 | 失败判据 |
|---|---|---|
| A01 | 缺 Tenant/Scope/Revision/Digest/TTL、有限资源上限、显式网络策略或合法 SecretRef 的请求 fail closed | Provider 被调用或请求被补默认权限 |
| A02 | Runtime LeaseRef/Instance/Fence/Scope 使用值语义校验；深拷贝等值通过，任一 epoch/digest 漂移拒绝 | 比较指针地址，或旧 epoch 执行 |
| A03 | 同一 Provider slot 的未终结 Allocation/Residual 由 tenant-stable conflict domain 阻止错误复用 | 通过新 Run/Instance 缩窄冲突域绕过 |
| A04 | Admission 与实际执行点均重验 Intent/Fence/Identity/Authority/Review/Budget/Scope/Policy/Binding | 第二门禁失败后仍有 Provider 调用 |
| A05 | 各 Effect 有独立 operation kind、attempt、receipt、inspect、settlement | `close+release+cleanup` 或 `checkpoint+restore` 被合并 |
| A06 | Provider回包不能直接推进Sandbox Fact或Runtime Lease；Inspect后只能CAS DomainResultFact，必须再经Runtime Operation Settlement与Sandbox ApplySettlement | Provider `active/released`字符串成为权威事实 |
| A07 | Begin 回包丢失、NotFound、进程死亡时只 Inspect 原 Attempt | 创建新 Attempt、换 Provider 或自动重派 |
| A08 | 不可 Fence/Inspect 的持久能力、不可控网络、长期明文 Secret、raw bypass 对高风险 Requirement 直接 rejected | 静默降级或仅记录警告后执行 |
| A09 | Cleanup 分进程、文件挂载、网络、Secret、后台任务、远端继续、Provider 保留七维结算 | 单 boolean 或进程死亡被当 cleanup complete |
| A10 | unresolved/indeterminate Residual 保留 Owner、Scope、ConflictDomain、Evidence 与处置状态 | 残留被丢弃后槽位复用 |
| A11 | Checkpoint未来执行严格遵循ReservePhase→InspectCurrent→Admission→Review/Auth→Permit→Begin→prepare/execute Enforcement→Provider→Evidence→DomainResult→Settlement→Apply；缺任一门Provider=0 | 跳门、交换顺序、Begin直接授执行，或prepare ref冒充execute ref |
| A12 | Restore 新 Instance/更高 epoch/新 Lease；外部网络、交易、邮件、数据库事实不回滚 | 复用旧 Lease 或宣称外部世界 rewind |
| A13 | Workspace overlay 不等于真实提交；commit 有独立 Review/Effect/Receipt/Inspect/CAS/Settlement | Provider write success 直接成为 workspace committed |
| A14 | Run Settlement 只提交 Sandbox requirement result；Runtime 聚合并选择 Outcome | Sandbox 直接 CompleteRun 或选择 Runtime Outcome |
| A15 | Backend capability 必须有 artifact/contract、conformance evidence、TTL、residual；名称不蕴含保证 | 因“container/microVM”标签自动判为安全 |
| A16 | Harness 仅消费已装配 Endpoint/Scope；无 Sandbox Controller 直连或私有 Port 适配 | Sandbox 导入 Harness kernel/fakes/internal 或私有 Port |
| A17 | Sandbox控制域独立`SnapshotArtifactOwnerV2`将无TTL stable Subject identity、带revision/TTL的versioned exact/current SubjectRef、StorageArtifactRef、FactRef、Entry/Envelope、CurrentIndex严格分层；AggregateCurrent按state-active TTL闭集，terminal不可回退，retention不授执行资格 | stable identity伪装current或进入BindingSet TTL；混用StorageArtifactRef/FactRef；Provider receipt直接成为Fact；历史Fact TTL杀current；用retention延长资格 |
| A18 | 与Runtime P14一致：仅prepared closure可create-once选择commit XOR abort；failed→incomplete、not_applied→confirmed_not_applied且均无后继；unknown只Inspect/Reconcile、未决最终indeterminate | failed/not_applied/unknown创建Abort；unknown本地改写；同closure双分支；重开prepare、切分支或revision/digest回退 |
| A19 | Checkpoint/Restore只使用冻结的typed scope、Evidence/Settlement与current Reader；不得复用activation scope、Evidence V3或Settlement V4 | 走普通lifecycle兼容翻译、缺门调用Provider或复用旧Instance/Lease |
| A20 | Compatibility漂移使旧Snapshot对新Backend/Workspace/Generation失效；不得靠Provider名称或快照存在推断可Restore | artifact/schema/OS/arch/conformance变化仍复用旧Compatibility |
| A21 | Assembly release只发布公共`sandbox.execution`与已冻结的Phase/Port descriptor；未冻结Phase保持缺失 | 复用无关Phase或创建Sandbox私有Hook/枚举 |
| A22 | Snapshot deletion/retention、Restore cleanup与Residual各自独立Operation并可从lost reply恢复 | NotFound/进程死亡推导删除或cleanup complete；换ID/Attempt重派 |
| A23 | current Reader拒绝typed-nil、presence/value矛盾与TTL超界；后继expiry不晚于PreviousPhase closure和全部上游最早TTL，`now == expiry`失效 | typed-nil绕过门禁；派生TTL延寿；过期终态触发新Phase/Provider |

### 2.1 Snapshot Artifact切片联合验收门（全部未勾选）

| 编号 | 联合Review必须确认 | NO-GO反例 |
|---|---|---|
| SA01 | Retention Policy/Legal Hold与Artifact-local Retention Application单一Owner分工 | Sandbox与Continuity双主CAS同一retention语义 |
| SA02 | Reservation DTO、stable key、Owner派生ID、RequestedNotAfter和内容冲突闭表 | 同source attempt换Schema/content/policy/ID/TTL创建第二份 |
| SA03 | 无Revision/TTL的stable SubjectIdentity与带Revision/TTL的versioned exact/current SubjectRef分层；SubjectRef→PayloadBodyDigest/FactRef→EntryBodyDigest/EntryRef→EnvelopeBodyDigest/EnvelopeRef四层canonical，每层排除own Ref/Digest | stable identity携TTL/current或进入BindingSet；versioned SubjectRef换revision/TTL复用旧digest；self-digest循环；Entry/Envelope计算含自身Digest |
| SA04 | StorageArtifactRef与SnapshotArtifactFactRef使用独立完整exact DTO、type URL/version/revision/digest domain；Fact不反向含Retention/Entry/Envelope ref | raw handle；Storage冒充FactRef；错type/version/domain；同digest跨domain重放 |
| SA05 | AggregateCurrent按state-active TTL闭集；CurrentIndex/Tombstone exact DTO canonical覆盖全部presence/TTL并排除own Ref/Digest | 无条件min历史；presence/value矛盾；TTL篡改旧digest；Fact过期使terminal复活 |
| SA06 | Index ExpectedCurrent、NoActive、Carry及From/To Index均为full exact Ref；四Reader逐方法closed errors，current/history与权威NotFound分离 | expected只给revision；NotFound当NoActive；Unknown当absent；过期history授新purge资格；lost reply换index/proof key |
| SA07 | no-hold Projection绑定Index exact且watermark generation等值；S1→S2词典序，跨代连续Carry exact proof；Projection排除caller RequestedNotAfter | sequence/generation回退；跨代无carry/断链/coverage漂移；强制Projection revision相等；caller bound重封 |
| SA08 | AttemptState与AggregateState分离、one-active-attempt；failed关闭后只允许新stable key；terminal tombstone/index不可回退 | unknown时新Attempt；复用failed Attempt；confirmed/indeterminate复活；expiry自动删除或回absent |
| SA09 | Runtime/Continuity只消费中立`SnapshotArtifactFactRefV2`，不消费StorageArtifactRef | Sandbox写Runtime/Manifest、暴露storage/backend ref，或其他Owner导入Sandbox实现 |
| SA10 | 公共面仅coordinate-only Reserve/Commit/Inspect；Commit经Owner S1/S2及包内expected-current CAS，raw committer不可导出/跨包 | 公共Apply/raw CAS、generic mutate、caller payload直接成为Fact或跨包committer |
| SA11 | Sandbox-owned对象Owner clock/RequestedNotAfter/min、fresh pre/post与`now==expires`规则闭表；跨Owner proof不收caller bound | caller/Provider时间授权；clock rollback延寿；post漂移仍提交旧candidate |
| SA12 | Reservation replay、CAS winner lost reply、Provider Unknown/NotFound三类恢复分离 | 非权威NotFound新建ID；CAS未知盲重放；Provider NotFound换Attempt/Provider |
| SA13 | delete request不等于deleted；Purge创建链为Reservation→Request→Attempt，Request不含Attempt exact ref且Attempt单向绑定PurgeRequestDigest；purge是唯一物理删除Effect；主Purge与cleanup Settlement全族均有ContractVersion/TypeURL/DigestDomain/canonical/Expires完整shape；cleanup独立Operation/Evidence/current Reader/Apply；只在purge DomainResult→Runtime sibling Settlement current→Sandbox Apply CAS后deleted | Request↔Attempt digest环；lost reply重封Request；主Purge Settlement比cleanup缩水；重复SettlementRef；request/Begin/Receipt/NotFound/cleanup成功直接deleted；purge隐式cleanup；任一占位shape或TTL延长 |
| SA14 | live Checkpoint V5 Reader保持一方法；Sandbox versioned exact源真实携Owner/Kind/Revision/Schema/Expires，runtimeadapter仅source→neutral单向构造并逐字段等值校验；无TTL stable Subject identity不参与BindingSet映射；Purge/cleanup各18个命名Evidence Request/Result具备canonical request identity；Record+Chain+Cursor+Consumption+Qualification-consumed由Evidence Owner原子提交；DAG无SCC | Adapter把stable identity伪造成current neutral ref或提供neutral→source；推导/默认填充字段；Result绑定另一Request；typed-nil/presence绕过；Record/Consume分写；Runtime反向导入Sandbox；raw Fact Port/plain Reader wiring |
| SA15 | ExpectedAggregateRef full CAS、append-only history/current与64路单赢家/no-ABA | 只给revision、stale ref覆盖、current回退、同revision换digest、alias改Store |
| SA16 | 不同Deletion caller bound读取同一natural no-hold current exact ref；Sandbox仅在Attempt/Aggregate取min | proof body/digest含RequestedNotAfter；短/长bound得到不同proof；长bound越过proof natural Expires |
| SA17 | CurrentIndex exact body覆盖type/version/ID/revision/state、全部presence、present ref TTL、closure/time字段并排除own Ref/Digest | activeAttempt+tombstone并存；terminal无tombstone；absent带value；漏TTL；own ref参与digest |
| SA18 | Tombstone exact body覆盖terminal state、pre-terminal ref、cause/settlement/residual/previous presence与TTL；续期保持lineage/state | 引用含自身terminal Envelope形成环；deleted切indeterminate；清空tombstone；previous断链 |
| SA19 | StorageArtifactRef完整DTO禁止backend locator/Credential并与FactRef digest domain隔离 | path/bucket/key/VM handle；revision变化复用digest；namespace/content/schema/TTL篡改；Fact/Storage互填 |

## 3. Backend Conformance 验收

每个 Backend+artifact+contract 组合独立出具报告，逐项标为 `enforced | observed_only | unsupported`：文件、网络、进程、Secret、资源、设备/syscall、Fence、prepared-attempt local inspect、provider-operation inspect、Cleanup、Checkpoint、Workspace overlay、Residual。

Conformance 结果只允许：

- `admitted`：全部 required property 已证明且证据未过期；
- `admitted_with_explicit_residual`：Requirement 明确允许该 Residual，重新 Admission 后接受；
- `observe_only`：只能观察，不能承载需要强制隔离的生产 Effect；
- `rejected`：required capability 缺失、不可 Fence/Inspect、证据过期或存在禁止 residual。

Fake 只能验证控制逻辑，永远不能把某 Backend 标成生产 admitted 或承诺 SLA。

## 4. 恢复与 Settlement 验收

- Unknown、failed、residual、cleanup_pending 是独立维度，不能压成一个 `error`。
- Inspect 自身 Unknown 时进入 blocked indeterminate，等待 Owner 事实变化或人工处置；不得递归生成无限 Inspect 链。
- Cleanup/compensation 是新治理 Operation，必须重新校验 current Authority/Review/Budget/Scope/Fence。
- Runtime Lease/Fence迁移与Sandbox DomainResultFact/ApplySettlement分开CAS；中间必须引用精确Runtime Operation Settlement，任一方未知都不得推导另一方成功。
- Application Coordinator 必须能在进程重启后从持久 Attempt/Settlement 继续，不靠内存回包。

## 5. 验证证据要求

实现获授权后，验收报告必须列出实际执行命令、退出码、失败注入点和证据路径，覆盖单元、白盒、黑盒、故障注入、Conformance、race、vet、Application 集成与系统测试。完整矩阵见 `.properties.rax/plan/sandbox/test-matrix.md`。
