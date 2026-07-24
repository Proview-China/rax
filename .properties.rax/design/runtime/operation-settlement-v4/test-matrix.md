# Operation Settlement V4测试矩阵

## 1. 验收原则

测试必须证明“强类型Evidence V3消费链能够形成唯一Runtime V4终态”，而不是只证明API返回成功。fake只能验证确定性、CAS和恢复语义，不能宣称生产持久性、Provider控制或SLA。

## 2. 公共合同与canonical

| 编号 | 场景 | 预期 |
|---|---|---|
| OS4-C01 | Submission与Evidence Binding重复Seal | digest稳定，nil/empty按字段规则等价 |
| OS4-C02 | map顺序、重复集合项、重复JSON键、未知required、尾随文档 | 全部拒绝 |
| OS4-C03 | V3/V4同Settlement ID双向创建 | 第二版本Conflict+IdempotencyPayloadMismatch；首记录不变 |
| OS4-C04 | V2 Evidence Record ref伪装V3 Consumption/Record | type-pun拒绝，零写 |
| OS4-C05 | 仅schema+payload digest伪装DomainResult Fact | 拒绝，Reader计数为零或返回typed error |
| OS4-C06 | 任一ID/revision/digest/phase/Attempt/Scope字段重Seal | Validate或Gateway拒绝 |

## 3. Evidence binding完整性

| 编号 | 场景 | 预期 |
|---|---|---|
| OS4-E01 | prepare与execute各一份、完整exact关联 | 可进入current复读 |
| OS4-E02 | 缺prepare或缺execute | fail closed，零Settlement写 |
| OS4-E03 | phase交换、重复、额外phase | fail closed |
| OS4-E04 | prepare Consumption用于execute | Consumption/phase mismatch |
| OS4-E05 | issued Qualification与final Qualification不同ID或非法revision | Conflict |
| OS4-E06 | final状态为issued/ingest_only/revoked/expired/consumed_observation | 拒绝Settlement |
| OS4-E07 | Record、Candidate digest、Handoff、Attempt任一错配 | Conflict |
| OS4-E08 | Enforcement 4.1 phase ref错配、历史Receipt重Seal或phase ref沿用旧digest | fail closed |
| OS4-E09 | normalized full OperationScope逐字段漂移 | Conflict，零写 |
| OS4-E10 | prepare/execute属于不同Effect revision、Operation或Attempt链 | fail closed |

## 4. DomainResult与current Reader

| 编号 | 场景 | 预期 |
|---|---|---|
| OS4-D01 | exact kind-routed authoritative current Fact | 允许进入Owner Commit |
| OS4-D02 | Provider Receipt、Observation、Claim或opaque ref冒充Fact | 拒绝 |
| OS4-D03 | unknown kind、重复Owner、Reader未装配/不可用/NotFound | 零权威写、fail closed |
| OS4-D04 | Fact ID/revision/digest/Effect/Operation/Attempt任一漂移 | Conflict |
| OS4-D05 | Reader第一次读后、Owner Commit前Fact变化 | Owner最后复读拒绝 |
| OS4-D06 | Reader回包丢失或观察租约到期 | Inspect/重读，不推导成功 |

## 5. Owner原子性与恢复

| 编号 | 场景 | 预期 |
|---|---|---|
| OS4-A01 | Evidence已consumed_current，Settlement尚未写时崩溃 | 原Evidence不回滚；按exact refs恢复，不调用Provider |
| OS4-A02 | Runtime Commit在四项写入边界注入故障 | Settlement/Association/guard/projection全无或全有，无半写 |
| OS4-A03 | Commit成功但回包丢失 | Inspect原ID返回已提交事实；重交同内容幂等 |
| OS4-A04 | 同Settlement ID换任一内容 | Conflict；首事实不变 |
| OS4-A05 | 64并发同内容提交 | 仅一次线性化，其余幂等读取同ref |
| OS4-A06 | 64并发不同内容提交 | 仅一个胜者，其余Conflict |
| OS4-A07 | Association或terminal projection被Store故障模拟为缺失 | current Inspect fail closed并报告Owner合同破坏 |

## 6. V3/V4共享terminal guard

| 编号 | 场景 | 预期 |
|---|---|---|
| OS4-G01 | 既有V3 settled后提交V4 | guard occupied；V4 NotFound；无sidecar |
| OS4-G02 | V4胜出后提交V3 settled CAS | V3 Conflict；V3 Fact未伪造settled |
| OS4-G03 | V3/V4 64并发竞争同一Effect | 全局仅一个terminal winner |
| OS4-G04 | 不同version-specific Settlement ID但同Effect | shared guard仍互斥 |
| OS4-G05 | V3 settled历史Fact没有显式V4 guard记录 | Owner逻辑仍判定guard占用，不修改旧digest |
| OS4-G06 | 尝试给V3 terminal追加V4 association sidecar | 拒绝 |
| OS4-G07 | V3-first后用不同Settlement ID和不同OperationDigest提交V4 | shared guard拒绝；零V4四对象 |
| OS4-G08 | V4-first后分别提交不同ID的V3/V4终态 | 两条路径均Conflict；首终态不变 |
| OS4-G09 | current-by-Effect索引被另一合法对象占用/变化后，用旧Guard ref历史Inspect | 仍按Settlement ID读回原历史Guard，不借current索引 |
| OS4-G10 | 不同Tenant使用相同Effect ID各自提交V4 | 各一次成功，历史/current闭包互不串读 |

## 7. TTL、历史真实性与late

| 编号 | 场景 | 预期 |
|---|---|---|
| OS4-T01 | Provider执行与Evidence current消费后，历史Permit/Policy到期 | exact关联仍可truthful settlement，不授新dispatch |
| OS4-T02 | Qualification在Consume前到期并降级consumed_observation | 永不具Settlement资格 |
| OS4-T03 | late observation在bounded ingest window内落Ledger | 仅Observation；Settlement Gateway拒绝 |
| OS4-T04 | 时钟回拨 | 不复活已过期Reader观察租约或Evidence资格 |
| OS4-T05 | 历史Permit/Policy ref被换revision/digest | 即使新版本current也拒绝旧链替换 |

## 8. Closed matrix与零Provider

| 编号 | 场景 | 预期 |
|---|---|---|
| OS4-M01 | `OperationScopeKind=activation_attempt` + `PolicyProfile=praxis.runtime/activation-evidence` + 三个允许的Sandbox EffectKind | 可进入V4 Gateway |
| OS4-M02 | backend-discovery/cancel/rollback/close/release/termination | Issue/Settle拒绝 |
| OS4-M03 | run/admin/custom/recovery profile | 拒绝 |
| OS4-M04 | Action Gateway或Run/Session/Turn/Action/Context required输入 | 当前版本unsupported |
| OS4-M05 | Settlement重试路径 | Provider调用计数始终为零 |

## 9. 白盒、黑盒与集成门禁

### 白盒

- 每个Validate、canonical、clone、typed ref、closed enum、phase集合与terminal guard分支；
- Fact Store四对象单事务、V3/V4共享锁、lost-reply注入点；
- Gateway两次复读与Owner最终复读间的逐字段漂移；
- DomainResult kind routing、Binding漂移和Reader失效。

### 黑盒/Conformance

- 只依赖公共V4 Port运行；禁止导入Runtime `control/kernel/fakes`内部实现；
- 自定义组件只能通过注册的namespaced kind与可信Reader接入，不能自授Owner/Settlement资格；
- fake/external adapter不得宣称production、durability或SLA；
- Observation、Receipt、Claim、Evidence消费均不能单独产生terminal projection。

### 集成顺序

1. V4公共合同与canonical；
2. V4 Fact Store与shared terminal guard；
3. Evidence V3 exact Reader/Association；
4. DomainResult kind-routed Reader；
5. Governance Gateway；
6. activation first slice；
7. Action Gateway另案解冻。

每层必须通过定向普通测试、`-race`、`go vet`、`gofmt -l`、`git diff --check`和必要fuzz/property；中央组合测试不能替代单元和故障反例。

## 10. 最终执行记录

- Owner自测：full ordinary、full shuffle、full race、`go vet`、`gofmt -l`、`git diff --check`全部PASS；
- 中央独立高重复：目标`count=100` PASS（127.334s），目标`race count=20` PASS（238.537s）；
- staged failure覆盖1—5，lost reply、64并发同内容/换内容、V3/V4 64竞态、跨Tenant隔离与历史四对象闭包均实际命中；
- 独立Review最终裁决：YES；
- fake/Conformance不宣称production durability、availability或SLA。
