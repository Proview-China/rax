# Runtime对抗反例验收矩阵

## 1. 定位

本矩阵把三轮独立反审中的失败反例转成设计验收输入。当前仅证明合同可判定，不代表实现测试已经通过。每个ID必须在后续测试中映射为单元、合同、白盒、黑盒、故障注入或集成用例。

## 2. 身份、生命周期与替换

| ID | 反例输入 | 必须结果 | 合同 |
|---|---|---|---|
| GATE-01 | 文本/图/反例未落地却标记设计收口 | 撤销门禁，回到修正设计并重新审核 | acceptance |
| OBJ-01 | Harness Digest变化但复用Lineage | 拒绝，要求新Plan/Lineage | concepts |
| OBJ-02 | 两个Control Plane激活同Identity | 只有一个取得IdentityExecutionLease | concepts/control-plane |
| OBJ-03 | 旧epoch命令作用新Instance | stale instance epoch | concepts |
| OBJ-04 | Harness完成但必需Effect未知 | execution indeterminate/needs reconciliation | concepts/effects |
| OBJ-05 | Artifact被Review拒绝 | Execution可完成，Task不得自动成功 | concepts |
| OBJ-06 | 同一Instance同时启动两个Run | 后到请求返回run conflict，不隐式排队 | concepts/kernel |
| OBJ-07 | proposed Instance Schema要求先有SandboxLease ID | 拒绝循环引用；proposed阶段只需Lineage和预留epoch | concepts/admission |
| OBJ-08 | reserved Identity Lease提交业务Effect | 拒绝；reserved只允许同attempt激活/清理 | concepts/admission |
| OWNER-01 | 同一Effect绑定两个Settlement权威所有者 | Admission拒绝，必须解析到唯一归并点 | concepts/effects |
| OWNER-02 | Runtime因负责协调Intent而宣称拥有全部Intent语义 | 拒绝；每个Effect仍绑定一个Intent事实所有者，Runtime只经Port协调/关联 | README/concepts |
| STATE-01 | 节点断电 | 不得直接Cleanup Complete | kernel |
| STATE-02 | Harness自报Ready但无进程 | 不得Ready | kernel/evidence |
| STATE-03 | 旧Instance迟到Ready | 记录迟到，不复活 | kernel |
| STATE-04 | Stop后Remote Batch仍运行 | 分维度报告，不得完全清理 | kernel/effects |
| STATE-05 | 同Plan重建 | 新Instance ID和更高epoch | kernel |
| STATE-06 | 写入running + fenced | 状态验证拒绝并要求stopping/fenced | kernel |
| STATE-07 | terminal但本地Cleanup仍pending | 合法分维度状态，不得伪造complete | kernel |
| STATE-08 | 旧epoch迟到事件要求terminal→ready | 只作迟到证据，不迁移状态 | kernel/evidence |
| STATE-09 | Preflight Probe回包丢失且可能创建远程对象 | 原子进入stopping/unknown/indeterminate，不得进入Snapshot | kernel/admission |
| STATE-10 | 修改ExecutionCertainty时自动重写Lifecycle/Cleanup | 拒绝；三维分别校验和持久化，不存在依赖链 |
| STATE-11 | 原Effect仍为UnknownOutcome时自动派发Compensation | 拒绝`recovery_effect_not_permitted`；先以权威Receipt/Inspect确定原效果，再独立授权补偿 | kernel/admission |
| STATE-12 | cleanup pending但实现把unknown解释为“只能Inspect”，永久拒绝Release | 错误；允许不扩大权限且具有新Intent/Fence/Authority/适用Budget/Evidence的Cleanup/Release | kernel/admission |

## 3. Fence、Secret与控制

| ID | 反例输入 | 必须结果 | 合同 |
|---|---|---|---|
| FENCE-01 | 旧Token（离线Token）仍在策略有效期 | 新Instance不能取得冲突域 | safety |
| FENCE-02 | 旧Instance重连提交写入 | 最终边界按epoch拒绝 | safety |
| FENCE-03 | 离线撤销参数未配置 | 禁用离线Effect | safety/profile |
| FENCE-04 | 两个强制执行点持有不同Fence epoch | 旧epoch侧拒绝并产生EvidenceConflict；执行点不自封Owner | safety/concepts |
| SECRET-01 | Harness已读取长期Secret明文后卸载文件 | 只报告路径撤销，明文清除不可证明 | safety |
| REPLACE-01 | 旧付款Operation未知 | 新实例不得取得付款域 | safety/effects |
| REPLACE-02 | Lease失联且网络隔离不明 | 不签发ReplacementPermit | safety |
| CMD-01 | Resume未执行时Stop线性化 | Resume superseded | control-plane |
| CMD-02 | Approve后参数Digest变化 | 审批失效 | control-plane/effects |
| CMD-03 | 网络分区双主 | 少数/失联侧推进fail closed | control-plane |
| CMD-04 | 权威Intent无法持久化 | 不接受新Effect命令 | control-plane/evidence |
| CMD-05 | Deny晚于Effect派发 | 不得报告未发生，继续结算 | control-plane/effects |
| CMD-06 | online_strict Intent派发前失去Authority事实源 | 最终边界fail closed | control-plane/safety |
| CMD-07 | leased_offline Token已过期仍尝试新步骤 | 拒绝派发，仅允许已发生效果结算 | control-plane/safety |

## 4. Admission、Saga与Harness

| ID | 反例输入 | 必须结果 | 合同 |
|---|---|---|---|
| ADM-01 | Probe发送真实Prompt | 归类Activation，不得声称无副作用 | admission |
| ADM-02 | Preflight后扩展Trust撤销 | Activation失败 | admission/extensions |
| ADM-03 | Preflight后Route/Entitlement证据过期 | Activation线性化前重验并失败 | admission/profile |
| ADM-04 | Snapshot字段要求真实Lease ID | Schema拒绝；Snapshot只绑定proposed身份与需求摘要 | admission/concepts |
| ADM-05 | Sandbox reserved后ActivationCommit失败 | 保持Quarantined、Fence/Inspect/Release，不启动Harness | admission/sandbox |
| ADM-06 | Preflight Probe回包丢失且可能留下远程对象 | 原子进入stopping/unknown/cleanup，不得形成Snapshot | admission/kernel |
| SAGA-01 | MCP Attach成功但回包丢失 | unknown effect，不盲重试 | admission |
| SAGA-02 | 发送邮件后删除本地记录 | 不构成原效果补偿 | admission/effects |
| SAGA-03 | Required绑定释放未知 | Instance不得Ready | admission/kernel |
| HARNESS-01 | CLI有不可拦截网络和长期明文 | rejected conformance | contracts |
| HARNESS-02 | 无Pause但Effect均可Fence | restricted controlled，不暴露Pause | contracts |
| HARNESS-03 | Harness称Cleaned但Batch仍运行 | Cleanup不得Complete | contracts/effects |
| CONTRACT-01 | Capability自报但认证过期 | 不作为bound能力 | contracts |
| CONTRACT-02 | 未知扩展命名空间 | 不猜测、不跨Provider透传 | contracts/extensions |

## 5. Effect、Budget与正式提交

| ID | 反例输入 | 必须结果 | 合同 |
|---|---|---|---|
| EFF-01 | 模型外发Context无EffectIntent | 拒绝派发 | effects/evidence |
| EFF-02 | Hosted Tool可持久写但不可观察 | Harness降级或拒绝 | effects/contracts |
| EFF-03 | 付款超时无Receipt | unknown outcome，禁止盲重试 | effects |
| EFF-04 | Sandbox Allocate只有Action ID、没有Effect Intent身份 | 拒绝；所有Effect统一使用Intent ID/revision | effects/safety |
| BUD-01 | 并发预算超卖：预算100，同时Reserve 70+70 | 只能一个成功 | effects |
| BUD-02 | UnknownOutcome后释放全部预算 | 禁止，保留最坏占额 | effects |
| BUD-03 | 无Token/时长上限的stream或Batch请求Hard Budget | 拒绝或显式降级Soft并重审 | effects |
| REMOTE-01 | Sandbox释放但远程Batch残留活动 | remote continuation outstanding | effects |
| REMOTE-02 | Provider Cache无法删除 | 报告retained/retention unknown | effects/continuity |
| COMMIT-01 | 旧Instance迟到Memory Candidate | 默认拒绝或重新采纳 | effects |
| COMMIT-02 | Commit回包丢失 | 查询原Intent，不重复提交 | effects |
| COMMIT-03 | Review Verdict为Rejected仍创建Commit Intent | 拒绝；Rejected终止提交支路 | effects/continuity |
| COMP-01 | Authority撤销后自动退款 | 未有独立补偿授权则拒绝 | admission/effects |

## 6. Evidence、Checkpoint与Cache

| ID | 反例输入 | 必须结果 | 合同 |
|---|---|---|---|
| EVID-01 | Harness签名Ready，Inspect无进程 | 不得Ready | evidence |
| EVID-02 | Harness称EffectFailed，目标状态已变化 | EvidenceConflict/已发生或未知 | evidence |
| EVID-03 | 权威Intent Store不可用 | 新外部Effect fail closed | evidence |
| EVID-04 | 投影服务不可用但Intent+Outbox已持久 | 可派发，稍后投影 | evidence |
| EVID-05 | Observer伪造Source身份 | 认证拒绝 | evidence |
| EVID-06 | 原生事件含Secret | 正文不得进入普通Projection | evidence |
| EVID-07 | 旧epoch合法签名事件 | 只作迟到证据 | evidence |
| EVID-08 | 同一SourceEvent至少一次重复到达 | 按source ID/epoch/event ID去重，只产生一个Ledger事实 | evidence |
| EVID-09 | Source sequence从10跳到12 | 产生EventGap；安全结论不得假定11不存在 |
| EVID-10 | CommandRecord已写但DesiredState/Outbox逻辑提交失败 | 命令不得Accepted；恢复时按同一revision补全或回滚可见性 | control-plane/evidence |
| CKPT-01 | Memory恢复、Harness恢复失败 | 不得Ready | continuity |
| CKPT-02 | Checkpoint含已撤销Capability | 恢复重验并拒绝/收紧 | continuity |
| CKPT-03 | 模型Effect已dispatch未settle却未进Checkpoint | 不得标consistent | continuity/effects |
| CKPT-04 | Sandbox Effect已settled但Remote Batch active | Checkpoint保留Remote水位与冲突域 | continuity/effects |
| CACHE-01 | 管理员与普通用户Prompt相同 | 不共享权限敏感缓存 | continuity |
| CACHE-02 | Harness Digest变化 | 旧缓存分区失效 | continuity |
| CACHE-03 | Provider隐式缓存隔离不明 | 敏感Context禁用缓存 | continuity |
| CACHE-04 | 为命中率改变Prompt语义顺序 | 拒绝 | continuity/context |
| CACHE-05 | 独立Provider Cache lookup发送Key并计费但无Intent | 拒绝；作为独立/子Effect治理 | continuity/effects |
| CACHE-06 | Provider Cache命中后Authority缩小 | 返回内容前重鉴权并拒绝 | continuity/context |

## 7. 扩展、API与Sandbox

| ID | 反例输入 | 必须结果 | 合同 |
|---|---|---|---|
| EXT-01 | 签名合法但Publisher未获授权 | 拒绝 | extensions |
| EXT-02 | 扩展无限Health事件 | 限流、隔离、Unhealthy | extensions |
| EXT-03 | Observer直接写权威Ledger | 拒绝 | extensions/evidence |
| EXT-04 | Artifact Digest改变但版本不变 | 拒绝旧Plan | extensions |
| EXT-05 | 扩展与Kernel同一地址空间但声称“默认隔离” | 不按拓扑词判断；必须提供威胁模型要求的等价隔离证明 | extensions |
| API-01 | REPL/API处理旧epoch Stop不同 | 合同失败，必须同一领域结果 | interfaces |
| API-02 | Facade直接写Review为Runtime状态 | 拒绝，路由给领域所有者 | interfaces |
| API-03 | 分区侧Resume | fail closed | interfaces/control-plane |
| API-04 | 写命令未知字段 | 默认拒绝 | interfaces |
| API-05 | 同一幂等Key更换Payload Digest | 所有Transport返回相同conflict/reason code | interfaces |
| API-06 | Instance命令缺适用lease epoch | precondition failed，不从投影猜补 | interfaces |
| SBX-01 | 容器删除但网络撤销未知 | Cleanup indeterminate | sandbox |
| SBX-02 | 一个Lease绑定两个Instance | 拒绝 | sandbox |
| SBX-03 | 节点失联无ReplacementPermit | 新Instance保持Quarantined | sandbox |
| SBX-04 | 后端缺强制网络隔离 | Admission拒绝 | sandbox |
| SBX-05 | reserved_quarantined Lease在Commit前启动Harness | Provider拒绝 | sandbox/admission |
| SBX-06 | ActivationCommit失败且Release回包丢失 | release indeterminate并阻断冲突域 | sandbox/admission |
| SBX-07 | 本地Sandbox已安全释放但远程Batch仍活动 | 本地Lease可released；远程记录继续占冲突域 | sandbox/effects |

## 8. Profile与Route交接

| ID | 反例输入 | 必须结果 | 合同 |
|---|---|---|---|
| PROFILE-01 | 同模型更换Harness却复用旧Route Profile | 拒绝，生成新Plan/Lineage | profile-assembly |
| PROFILE-02 | Provider专属字段覆盖Authority门禁 | 拒绝 | profile-assembly |
| PROFILE-03 | 离线撤销参数缺失却启用离线Effect | 禁用离线能力 | profile-assembly/safety |
| PROFILE-04 | Profile alias解析到新Digest但复用Lineage | 新Plan/Lineage | profile-assembly/concepts |

## 9. Plan门禁

| ID | 反例输入 | 必须结果 | 合同 |
|---|---|---|---|
| PLAN-01 | Plan未经用户决定写死`module/runtime`落点 | 拒绝；只写“用户批准的说明资产落点” | plan/acceptance |

## 10. 三轮审计问题追溯

| 审计问题 | 反例ID |
|---|---|
| R1-P0-1 阶段门禁冲突 | GATE-01 |
| R1-P0-2 旧执行面外部效果 | FENCE-01、FENCE-02、FENCE-04、REPLACE-01、EFF-04 |
| R1-P0-3 安全不确定态 | STATE-01、STATE-04、STATE-07、STATE-09～STATE-12、SBX-01 |
| R1-P0-4 Instance与epoch语义 | OBJ-01、OBJ-07、STATE-05 |
| R1-P0-5 Run/Session所有权 | OBJ-06、OBJ-04 |
| R1-P0-6 Harness准入 | HARNESS-01～HARNESS-03 |
| R1-P0-7 Binding恢复语义 | SAGA-01～SAGA-03 |
| R1-P0-8 Admission副作用边界 | ADM-01 |
| R1-P0-9 Authority TOCTOU | CMD-02、CMD-05、CMD-06、FENCE-04、EFF-01 |
| R1-P0-10 事件与事实源 | OWNER-01、OWNER-02、EVID-04、EVID-05、EVID-07～EVID-10 |
| R1-P1-1 Checkpoint一致性 | CKPT-01～CKPT-04 |
| R1-P1-2 Cache污染 | CACHE-01～CACHE-06 |
| R1-P1-3 扩展供应链 | EXT-01～EXT-05 |
| R1-P1-4 API语义边界 | API-01～API-06 |
| R1-P1-5 Exactly Once与未知结果 | EFF-03、COMMIT-02 |
| R2-P0-1 离线撤销延迟 | FENCE-01、FENCE-03、CMD-06、CMD-07 |
| R2-P0-2 Secret无法失忆 | SECRET-01 |
| R2-P0-3 Lineage与Plan绑定 | OBJ-01、STATE-05、PROFILE-01、PROFILE-04 |
| R2-P0-4 多维结果所有者 | OBJ-04、OBJ-05、OWNER-01、OWNER-02、COMMIT-03 |
| R2-P0-5 命令竞争 | CMD-01、CMD-02、CMD-05 |
| R2-P0-6 替换最低隔离证明 | REPLACE-01、REPLACE-02、SBX-03 |
| R2-P1-1 补偿也是Effect | STATE-11、COMP-01、SAGA-02 |
| R2-P1-2 Preflight到Activation漂移 | ADM-02～ADM-05 |
| R2-P1-3 Budget结算 | BUD-01～BUD-03 |
| R2-P1-4 敏感Evidence与防篡改 | EVID-05～EVID-07 |
| R2-P1-5 Identity并发 | OBJ-02 |
| R2-P1-6 Artifact/Memory提交 | COMMIT-01、COMMIT-02 |
| R3-1 策略参数实现中立 | FENCE-03 |
| R3-2 Budget为能力而非预设模块 | BUD-01、BUD-02 |
| R3-3 统一Effect无旁路 | OWNER-02、EFF-01、EFF-02、EFF-04、CACHE-05、REMOTE-01 |
| R3-4 来源不等于事实 | STATE-02、EVID-01、EVID-02 |
| R3-5 CAP fail-closed | CMD-03、API-03 |
| R3-6 Write-ahead Evidence | CMD-04、EVID-03、EVID-04、EVID-10 |
| R3-7 Provider长期状态 | STATE-04、REMOTE-01、REMOTE-02 |

## 11. 覆盖要求

矩阵覆盖对象创建/状态、唯一所有权、Fence/Secret、双主、Admission/Saga、Harness、统一Effect身份、Hosted Tool、Hard/Soft Budget、UnknownOutcome、Remote Batch、迟到Commit、Evidence重复/缺口/逻辑提交、Checkpoint统一Effect切面、Cache读写、Profile/Route漂移、扩展隔离、API一致性和Sandbox本地/远程释放。任何合同新增或语义变化必须同步新增或修改反例ID；总数由验证脚本从表格计算，不在正文手工写死。
