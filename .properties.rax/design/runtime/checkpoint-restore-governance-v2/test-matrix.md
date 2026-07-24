# Checkpoint/Restore Governance V2测试矩阵

状态：**Checkpoint-first V2第三次独立代码终审YES（P0/P1/P2=0）**。第一波只验Checkpoint；Restore运行能力保持unsupported。

## 1. public shape与canonical

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-U01 | strict decode、unknown field、nil/empty、bounds、type-pun | canonical deterministic；未知required/重复/尾随拒绝 |
| CKP2-U02 | Attempt/Barrier/EffectCut/Consistency/Finalize refs互换 | exact type/version拒绝 |
| CKP2-U03 | ManifestFact、candidate或RestorePlan冒充ManifestSeal | Consistency Validate拒绝 |
| CKP2-U04 | Participant Reservation phase/owner/attempt/barrier/cut漂移 | current projection拒绝 |
| CKP2-U05 | legacy BarrierID/Epoch/Snapshot字符串补字段 | 不能构造任一V2 Ref/Fact |
| CKP2-U06 | Restore shape进入ValidateCurrent | unsupported；零Fact、零Provider |
| CKP2-U07 | historical Attempt/Finalization Inspect冒充terminal current | 类型/入口分离；historical存在不产生current Projection |

## 2. Attempt+Barrier原子bundle

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-A01 | 首次Create | Attempt+Barrier同时存在且exact关联 |
| CKP2-A02 | 只写Attempt或只写Barrier的staged failpoint | 两者均NotFound；后续同canonical可成功 |
| CKP2-A03 | lost create reply | detached exact Inspect恢复完整bundle，不新建Barrier |
| CKP2-A04 | 同ID同canonical重复 | 幂等返回同bundle |
| CKP2-A05 | 同Attempt ID换Barrier/Scope/Run/Generation/Participant set | Conflict，原bundle不变 |
| CKP2-A06 | 64并发同canonical | 一个线性化bundle；无半对象/data race |
| CKP2-A07 | 64并发changed-content | 仅一个内容胜出，其余Conflict |
| CKP2-A08 | backend回错bundle | Gateway Validate/exact compare拒绝 |
| CKP2-A09 | Barrier Policy MaxTTL非正、expiry加法overflow、任一NotAfter已到期 | 写前拒绝；Attempt/Barrier均NotFound |
| CKP2-A10 | caller expected Barrier expiry与Policy派生值不一致 | Conflict/PreconditionFailed；caller不能延长或缩短Owner派生TTL |
| CKP2-A11 | Policy换ref/digest或Create后换Policy | Attempt冻结ref/semantic不变；新Cut/phase/consistent拒绝漂移，非成功deadline closure仍按冻结值收口 |
| CKP2-A12 | nil/typed-nil Store、Policy Reader或Clock | backend调用0；零半对象；不panic |

## 3. Barrier、EffectCut与dispatch竞态

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-B01 | Begin先于Barrier线性化 | Effect必须进入Cut |
| CKP2-B02 | Barrier先于Begin/actual execution point | Begin和execution point拒绝 |
| CKP2-B03 | Freeze与Effect inventory漂移 | 不漏Effect；拒绝并reconcile |
| CKP2-B04 | Begin Effect缺Settlement/inspection | 不得从Cut消失 |
| CKP2-B05 | 聚合watermark相同但逐Attempt不同 | Conflict/拒绝 |
| CKP2-B06 | Barrier `now == expires` | 不得Freeze新Cut/Reserve新phase/consistent |
| CKP2-B07 | Freeze staged failure | Cut与Attempt revision全有或全无 |

## 4. Participant ReservePhase顺序

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-P01 | 无Reservation直接Admission | 零Admission、零Provider |
| CKP2-P02 | Reservation后按Admission→Review/Auth→Permit→Begin→双Enforcement | 唯一合法Provider路径 |
| CKP2-P03 | phase swap/missing/duplicate/extra | 拒绝；零Provider |
| CKP2-P04 | prepare Reservation复用于commit/abort | exact phase拒绝 |
| CKP2-P05 | Reservation owner/binding/revision/digest/TTL漂移 | fresh current失败，零Provider |
| CKP2-P06 | commit缺prepare Settlement/ApplySettlement | 不得Reserve/Admit commit |
| CKP2-P07 | 64并发Reserve同stable phase | 一个Reservation；changed-content Conflict |
| CKP2-P08 | Reservation回包丢失 | 只Inspect原Reservation，不新建phase/Effect |
| CKP2-P09 | prepare携非nil PreviousPhase；commit/abort缺或换prepare闭包 | 拒绝；零Admission/零Provider |
| CKP2-P10 | commit与abort 64并发不同Operation/Effect ID | branch guard仅一个分支胜出；另一分支Conflict |
| CKP2-P11 | commit/abort unknown后切换兄弟分支 | 拒绝；只Inspect原phase/attempt |
| CKP2-P12 | Participant terminal后覆盖PreviousPhase/DomainResult/Settlement/Apply ref | Conflict；历史immutable |
| CKP2-P13 | nil/typed-nil Reservation/Domain/Evidence/Settlement Reader | backend调用0；零Admission/零Provider；不panic |
| CKP2-P14 | prepare failed/not_applied/unknown后尝试commit或abort | 两种后继均拒绝；failed→incomplete输入，not_applied→confirmed-not-applied输入，unknown只Inspect/Reconcile并最终indeterminate |

## 5. Evidence V1与Settlement V5

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-E01 | Issue Qualification | 不推进source cursor/sequence |
| CKP2-E02 | wrong phase/attempt/barrier/cut/reservation/lease | Consume拒绝 |
| CKP2-E03 | late/consumed_observation | 可审计，不得进入V5 |
| CKP2-E04 | consumed_current exact closure | 可成为V5输入 |
| CKP2-E05 | typed DomainResult/Participant Fact/Enforcement drift | V5零写 |
| CKP2-E06 | V3 Evidence或V4 Settlement type-pun | 拒绝，不扩旧闭表 |
| CKP2-E07 | Provider reply unknown | 只Inspect原Attempt，不重新Consume/Provider |
| CKP2-E08 | Evidence/Domain/Enforcement reader unavailable | V5零写 |
| CKP2-E09 | Provider Observation直接作为DomainResult或Settlement输入 | type-pun拒绝；必须先Evidence consumed_current再由Owner CAS DomainResult |

## 6. Consistency、Finalize与ManifestSeal

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-C01 | exact Cut+all required closures+immutable ManifestSeal | Attempt consistent、Barrier closed、Consistency同时提交 |
| CKP2-C02 | Consistency或CloseBarrier staged publish任一点失败 | 三对象全无变化；后续同canonical可提交 |
| CKP2-C03 | Commit回包丢失 | exact Inspect三对象；不重Commit |
| CKP2-C04 | ManifestFact verified_candidate但Seal缺失 | 不得consistent |
| CKP2-C05 | Seal属于另一Attempt/Barrier/Cut | 拒绝，零Consistency |
| CKP2-C06 | mutable Manifest/RestorePlan进入Commit | public Validate拒绝 |
| CKP2-C07 | known missing/failed且无unknown | Runtime派生incomplete并原子CloseBarrier |
| CKP2-C08 | 显式安全取消且所有Begin闭合 | Runtime派生aborted并原子CloseBarrier |
| CKP2-C09 | 任一外部结果无法证明 | Runtime派生indeterminate并原子CloseBarrier |
| CKP2-C10 | caller直接提交terminal状态 | schema无该字段/请求拒绝 |
| CKP2-C11 | Commit与Finalize并发 | expected revisions只允许一个赢家 |
| CKP2-C12 | Finalize回包丢失 | Inspect terminal Attempt+closed Barrier；不重开Barrier |
| CKP2-C13 | Barrier过期与Commit竞态 | `now >= expires`不得consistent，只能Finalize |
| CKP2-C14 | Prepare caller省略expected diagnostics/residuals，但Owner枚举含unknown | 不得省略Owner读取；Closure含unknown，deadline前reconcile且不能aborted |
| CKP2-C15 | Prepare caller expected set漏unknown或Residual | exact compare失败；零Closure、零Finalize |
| CKP2-C16 | caller换Policy或伪造safe-cancel | schema无Policy/Trigger输入；Attempt冻结Policy复读后拒绝aborted |
| CKP2-C17 | fake-safe-cancel：部分已Begin Effect缺abort/not-applied闭包 | 不得aborted；完整枚举决定incomplete/indeterminate |
| CKP2-C18 | Diagnostics/Residuals Finalization Owner Port unavailable或typed-nil | 零Seal、零Closure、零Finalize、Barrier不关闭、零Provider；不panic |
| CKP2-C19 | ManifestSeal Reader/Fact Owner typed-nil | backend读取0、零Consistency、零Barrier变化 |
| CKP2-C20 | Diagnostics/Residuals首次Seal | 两个Owner各自create-once immutable Seal，均绑定同一Cut及Owner、source epoch/sequence、ledger/root、complete-set digest |
| CKP2-C21 | Seal后发布因果水位不晚于Cut的unknown | Owner publish Conflict且ledger/seal不变；若恶意Owner接受，Finalize零写/current terminal不可见 |
| CKP2-C22 | unknown在Cut之后新发生 | 只进入post-cut审计分区，不覆盖Owner Seal、Closure或historical terminal |
| CKP2-C23 | Diagnostics Seal完成后Residuals更新，或Residuals Seal后Diagnostics更新 | pre-cut更新必须先进入相应Seal或与Seal Conflict；Runtime Closure不得混用自由set/watermark |
| CKP2-C24 | 同Owner Seal/Closure ID换Owner、epoch、sequence、root、set、Cut或EffectCut | Conflict，原Seal/Closure不变 |
| CKP2-C25 | ManifestSeal exact lookup缺完整Continuity Owner/Tenant/Scope或Owner任一字段漂移 | public Validate/Reader拒绝，零Consistency |
| CKP2-C26 | Runtime typed Participant closure与Continuity RuntimeClosureRef缺失、重复、ID/Owner/digest漂移 | Reader Conflict，零Consistency |
| CKP2-C27 | Context/Artifact closure或external SHA-256 canonical spelling漂移 | invalid/conflict；不得使用临时digest规则 |
| CKP2-C28 | Seal/Participant closure S1与S2不同或Inspect回包丢失 | Conflict/Indeterminate；只重读同一exact coordinate |
| CKP2-C25 | Barrier仍current+unknown+`now < deadline` | 只Inspect/Reconcile；不创建后继phase、不提前关闭Barrier |
| CKP2-C26 | Barrier过期+unknown+`now >= deadline` | exact Closure驱动原子indeterminate+close Barrier，不永久卡住 |
| CKP2-C27 | Policy deny/缺失unknown-at-deadline terminalization | Create Attempt写前拒绝，零Attempt/Barrier |
| CKP2-C28 | Reconciliation TTL非正、overflow、deadline>Barrier expiry或expected mismatch | Create写前拒绝；零Attempt/Barrier |
| CKP2-C29 | Policy在Create后换deadline mode/ref/digest | Attempt冻结值不变；current漂移不得阻止基线deadline closure，也不得改结论 |
| CKP2-C30 | Finalization Cut或Attempt revision staged failure | 二者全有或全无；不得留下无Attempt绑定的Cut |
| CKP2-C31 | Input Closure或Attempt history binding staged failure | 二者全有或全无；后续同canonical可恢复提交 |
| CKP2-C32 | Residuals Seal B2后、Finalize CAS前并发pre-cut unknown | unknown publish与Owner Seal只能一个顺序胜出；若Seal先胜publish Conflict，若publish先胜必须进入Seal |
| CKP2-C33 | Finalize成功后晚到pre-cut unknown | Owner入口Conflict；若Owner违反，historical terminal保留审计但current terminal projection不可见 |
| CKP2-C34 | Owner Seal create/Inspect回包丢失 | 只按deterministic Seal ID Inspect；不重封、不改source sequence/root |
| CKP2-C35 | historical terminal存在，但Diagnostics Seal root/sequence/set漂移 | historical Inspect仍可审计；terminal current返回Conflict且Projection不可见 |
| CKP2-C36 | historical terminal存在，但Residuals Owner violation或pre-cut非法append | terminal current Fail Closed；不得包装旧Closure为current成功 |
| CKP2-C37 | terminal current Reader依赖为nil/typed-nil | backend读取前拒绝；不panic、不返回Projection、零Provider |
| CKP2-C38 | Seal current Reader Unavailable或返回Indeterminate | current terminal分别Unavailable/Indeterminate且不可见；historical Fact不受改写 |
| CKP2-C39 | exact Closure与两个Owner Seal current | `CheckpointAttemptTerminalCurrentProjectionV2` Validate/Seal成功，refs逐字段exact |
| CKP2-C40 | consistent terminal携Finalization sidecars，或非成功terminal缺Closure/任一Seal | public Validate/current Inspect拒绝 |

## 7. shared terminal guard

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-G01 | V3先settled后V5 | V5 Conflict、零sidecar |
| CKP2-G02 | V4先settled后V5 | V5 Conflict、零sidecar |
| CKP2-G03 | V5先settled后V3/V4 | V3/V4 Conflict、原V5不变 |
| CKP2-G04 | V3/V4/V5 64并发同Tenant+Effect | 同一Owner锁下仅一个terminal |
| CKP2-G05 | 同Tenant+Effect换OperationDigest/SettlementID | 仍互斥 |
| CKP2-G06 | 跨Tenant相同EffectID | 各自可独立成功 |
| CKP2-G07 | V5四对象或Effect terminal staged failure | 全有或全无 |

## 8. legacy、Restore与Owner反例

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-L01 | legacy CheckpointParticipantReport自报committed | 不能生成Reservation、DomainResult、V5或Consistency |
| CKP2-L02 | legacy CheckpointSet consistent | 不能生成ManifestSeal、Consistency V2或Restore资格 |
| CKP2-L03 | Continuity尝试写Runtime Attempt/Barrier/Consistency | Owner mismatch，零状态变化 |
| CKP2-L04 | Participant尝试CloseBarrier或选择Consistency | Owner mismatch |
| CKP2-L05 | Runtime尝试写ManifestFact/SealFact | Owner mismatch |
| CKP2-L06 | RestorePlan/Consistency触发Restore运行路径 | unsupported，Provider=0 |
| CKP2-L07 | Fake/Conformance声明production eligible/SLA | 必须false |

## 9. lost-reply、单调历史与no-ABA

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-R01 | Create reply丢失后Attempt已推进到cut_frozen/collecting | 同immutable bundle可Inspect恢复；不重Create |
| CKP2-R02 | Freeze reply丢失后Attempt已合法推进 | exact同Cut ref可恢复；换Cut拒绝 |
| CKP2-R03 | Participant mutation reply丢失后同分支合法推进 | stored revision≥expected且历史exact时Inspect-only；零Provider重派 |
| CKP2-R04 | commit reply丢失但abort分支先出现 | 不是幂等后继；Conflict/Owner violation |
| CKP2-R05 | same revision换digest、revision rewind或低revision | Fail Closed；不覆盖current |
| CKP2-R06 | terminal→非terminal→同terminal ABA | 状态机/CAS拒绝；revision与terminal history不回绕 |
| CKP2-R07 | Finalize reply丢失但集合或派生terminal不同 | Conflict；只接受同完整set、同结论、同closed Barrier |
| CKP2-R08 | typed-nil Gateway dependency的lost-reply路径 | 不调用detached backend、不panic、不误报成功 |
| CKP2-R09 | Finalization Cut/Closure create回包丢失 | exact Inspect原Cut/Closure；不重冻Cut、不混用新Owner水位 |
| CKP2-R10 | deadline indeterminate+close回包丢失 | Inspect同Closure、同terminal与closed Barrier；不重开Barrier、不换Policy |

## 10. 实现后门禁

仅在联合Design Review YES并获代码授权后执行：targeted ordinary `count=100`、race `count=20`、全Runtime ordinary/shuffle/race/vet、canonical fuzz、gofmt、XML、relative links、diff-check。生产Provider、backend、持久性、availability和SLA必须另有独立证据。

## 11. 独立审计返修反例

| ID | 场景 | 必须证明 |
|---|---|---|
| CKP2-A41 | EffectCut terminal unknown kind、alias、混合sidecar或disposition不匹配 | public Validate与Freeze均拒绝；Attempt仍barrier_acquired、零Cut |
| CKP2-A42 | Freeze后Effect inventory root/entry set在Consistency S1/S2漂移 | final CAS前Conflict；零Consistency、Barrier仍active |
| CKP2-A43 | Attempt创建后Policy unavailable/revoked/expired，typed non-success需收口 | 只使用Attempt冻结模式；deadline indeterminate或冻结not-applied规则不被阻断 |
| CKP2-A44 | Qualification同stable ref替换Barrier/Cut/Reservation或伪长TTL | Fact Validate/Gateway拒绝；零Qualification写 |
| CKP2-A45 | Qualification Attempt/Input/Reservation任一current source在S1/S2漂移 | Conflict；零Qualification；Provider=0 |
| CKP2-A46 | Create回复丢失且Attempt已合法推进 | historical初始bundle exact则恢复；ABA/identity漂移拒绝 |
| CKP2-A47 | historical非成功终态与typed Seal classifications不一致 | historical仍可审计；terminal-current不可见 |
| CKP2-A48 | EffectCut outer EffectID/Intent revision/digest与Attempt A/B splice；`failed` alias | public Validate/Freeze拒绝，零Cut |
| CKP2-A49 | Consistency inventory仅同Attempt ID但revision/digest不同，或Barrier ref漂移 | S1/S2均Conflict，零Consistency/零Close |
| CKP2-A50 | Consistency Owner正常回错bundle；lost reply Inspect回错Fact/bundle | canonical exact拒绝，不返回伪成功 |
| CKP2-A51 | Run current revision/digest/status/scope与ExpectedRunRevision漂移 | Create前拒绝，Attempt/Barrier均NotFound |
| CKP2-A52 | create lost reply lineage缺revision、非法跳转、ABA或revision rewind | Conflict，不接受仅revision下界 |
| CKP2-A53 | Permit/Policy/source epoch-sequence/schema/payload/current TTL任一漂移或caller伪长TTL | Qualification零写、cursor不动 |
| CKP2-A54 | Handoff/Consume lost reply、same ID换Attempt/source/self digest、current漂移 | historical exact Inspect；换内容Conflict；零Provider |
| CKP2-A55 | terminal-current任一分支依赖typed-nil | 首个backend read前component_missing，read count=0 |
| CKP2-A56 | V4 terminal保持Effect/Operation但替换DomainResult nested Attempt的Intent/Permit/AttemptID | full canonical Attempt比较拒绝，零Cut |
| CKP2-A57 | fresh V5 terminal仅凭旧SettlementRef进入EffectCut V2 | public Validate明确unsupported，Freeze零写；V3/V4不受影响 |
| CKP2-A58 | Evidence Record Reader nil/typed-nil/Unavailable；S1/S2返回不同valid mapping | ErrorUnavailable/ComponentMissing或Conflict；Consumption/游标零写 |
| CKP2-A59 | fresh Consumption ID拼接另一valid Record的source epoch/sequence、schema、payload、scope、Authority、ledger scope/chain | exact Qualification与Owner chain校验拒绝；Consumption/游标零写 |

最终实测：Owner targeted ordinary `count=100` PASS（tests/fakes 172.053s），targeted race `count=20` PASS（tests/fakes 347.009s）；中央与第三独立审查的full ordinary、full race、vet、gofmt、diff-check均PASS。该记录不是production backend、Restore、Provider或SLA证据。
