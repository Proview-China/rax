# Workspace Read Command Publication V2

## 状态与目标

状态：`implementation-software-test-yes / owner-local / uncommitted`。

本切片保留既有事实类型 `WorkspaceReadCommandV1`，新增由 Sandbox 唯一拥有的
command publication/current/ensure 闭环：

```text
exact neutral Source command coordinate
  -> WorkspaceReadSourceCurrentReaderV2
  -> Runtime Effect current S1
  -> Runtime Prepared current S1
  -> Workspace current S1
  -> Runtime Effect/Prepared、Source、Workspace current S2
  -> WorkspaceReadCommandV1
  -> immutable WorkspaceReadCommandPublicationV2
  -> fresh WorkspaceReadCommandOwnerCurrentV2
```

该闭环只把已存在的跨 Owner current 输入编译为 Sandbox-owned command，并把
Publication+OwnerCurrent fresh资格设为physical与completion的强制前置。它不在
Publication阶段执行物理读取，不调用Provider、不写Runtime，也不宣称
cross-owner production ready。

## Owner 与非 Owner

- Sandbox 拥有：
  - `WorkspaceReadCommandV1`；
  - `WorkspaceReadCommandPublicationV2`；
  - `WorkspaceReadCommandOwnerCurrentV2`；
  - Ensure 的 create-once、current refresh、CAS、历史 Inspect 与 SQLite v19。
- neutral Source command 事实仍由其外部领域 Owner 持有。Sandbox 只定义消费
  Projection/Reader，不导入 `ExecutionRuntime/tool-mcp`，也不把配置值当成
  Source current。
- Runtime 继续拥有 Operation、Effect、Prepared、Attempt、Permit、Delegation、
  Fence 与 Runtime current。Sandbox 只复读 public current projection。
- Workspace 是 Sandbox 自身领域对象，但 Ensure 仍须通过 exact current Reader
  独立 S1/S2 复读，不能使用 Source projection 内嵌快照代替。
- Tool 后续实现 neutral Source Reader adapter；本切片不实现 Tool adapter。
- Application、Harness、Host、Console、Provider 与 production root 不在本切片。

## 公共对象

### WorkspaceReadSourceCommandRefV2

coordinate-only exact ref：

- `Owner EffectOwnerRefV2`，role固定为`settlement`；
- `Kind = praxis.tool/workspace-read-execution-command`；
- `ID`；
- `Revision`；
- `Digest`。

它不携 `current=true`、stable key、caller snapshot 或执行资格。

### WorkspaceReadSourceCurrentProjectionV2

Sandbox-neutral Projection 至少封存：

- `ContractVersion / TypeURL`；
- exact Source command coordinate；
- `SourceOwner EffectOwnerRefV2`，至少包含role/component/manifest；
- Runtime `OperationSubjectV3` 与 `OperationDigest`；
- exact Prepared ref、Prepared semantic digest；
- full `OperationDispatchAttemptRefV3`，包括 Permit 与 Delegation presence；
- Runtime Effect intent digest、Fact revision、state；
- payload schema、digest、revision；
- 显式 bounded canonical inline bytes 与长度；
- 由 canonical bytes严格派生的Workspace ref、relative path、range、
  requested-not-after；
- Source created/not-after；
- `CheckedUnixNano / ExpiresUnixNano / ProjectionDigest`。

Source exact coordinate整体必须出现在Effect.Intent.Owners exact集合中，并与
Runtime Effect.Provider、Prepared.Provider逐轴闭合；仅有
ID/revision/digest而Owner或manifest不同的同名Source必须Fail Closed。
Source时间还须满足：

- `SourceCreatedUnixNano >= Prepared.PreparedUnixNano`；
- `SourceNotAfterUnixNano <= RequestedNotAfterUnixNano`；
- `SourceNotAfterUnixNano <= Prepared.ExpiresUnixNano`；
- 与Effect current闭合时，
  `SourceNotAfterUnixNano <= Effect.Intent.ExpiresUnixNano`。

canonical inline bytes必须：

- 严格JSON解码，拒绝未知字段、trailing token和noncanonical encoding；
- 重新编码后与原bytes逐字节相等；
- 非空，且不超过 Runtime inline 上界；
- 其sha256严格等于payload digest；
- 只允许inline payload；generic/ref payload与伪ObjectRef拒绝。

Source command Ref digest固定为raw lowercase 64 hex；payload digest固定为
`sha256:`加lowercase 64 hex。两种表示不可互换，不接受uppercase、prefix alias
或同一摘要的多种文本表示。

`WorkspaceReadSourceCurrentReaderV2`只接受exact Source coordinate。Tool未来
adapter必须从authoritative Tool exact/current事实投影，不能由caller配置冒充。

### WorkspaceReadCommandPublicationV2

immutable publication封存：

- exact `WorkspaceReadCommandV1`；
- exact neutral Source coordinate、SourceOwner与stable semantic digest；
- Runtime Effect intent、Prepared、full Attempt的stable exact closure；
- exact Workspace Fact/Lease stable closure；
- create-time owner watermarks；
- publication canonical digest与stable absolute semantic NotAfter。

Publication不创造 `WorkspaceReadCommandV2` 事实，也不授予物理执行权。
Publication identity和body equality禁止包含Source/Effect/Prepared/Workspace每次
重签的`Checked/Expires/ProjectionDigest`。所有transient current proof只进入
OwnerCurrent；同一稳定Source再次Ensure不得产生兄弟Publication。
create-time owner watermarks只允许Source Fact created、Prepared issued及Workspace
Fact issued等稳定absolute水位；本次S1/S2 checked与owner fresh clock同样禁止。
这些上游水位只形成`EffectiveCreatedLower`。Command/Publication的实际
`Meta.Created`使用Sandbox owner fresh commitNow，必须满足
`lower <= commitNow < semantic NotAfter`；Created不进入稳定ID/digest/equality，
不得把Sandbox事实倒签成上游发行时间。Command与Publication必须由同一次initial
owner事务、同一个commitNow创建，二者`Created == Updated == commitNow`；不同
commitNow产生的合法并发候选必须保持相同stable exact refs，由SQLite保留winner
的真实Created body。

### WorkspaceReadCommandOwnerCurrentV2

fresh owner current封存：

- exact Command与Publication refs；
- Source、Effect、Prepared、Workspace current binding及fresh outer digests；
- owner current revision；
- `CheckedUnixNano / ExpiresUnixNano / ProjectionDigest`。

OwnerCurrent为版本化append-only history加current pointer。刷新必须CAS旧current，
不得覆盖历史、不得ABA；`now == ExpiresUnixNano`即失效。

OwnerCurrent只能通过两个owner seal创建：

- initial：固定revision 1、`Created == Checked == initial commitNow`，并与
  Command/Publication共享同一个initial owner时间；
- next：固定`revision = expected current revision + 1`，沿用revision 1的
  Created，Checked不得回退；store必须再确认expected exact ref仍是current
  pointer。

next不要求旧current在refresh时仍有效。旧15秒transient current到期后只失去使用
资格；fresh Source/Effect/Prepared/Workspace S1/S2仍可基于exact predecessor
CAS创建新current。OwnerCurrent expiry不得由caller指定，必须精确等于
`min(semantic not-after, 四类fresh expiry, Workspace Lease expiry,
checked + 15s)`。

任何current资格都必须执行两个不同的join：

1. stored三事实join：Command、Publication、OwnerCurrent exact refs、semantic
   digest、Source、Workspace、NotAfter、derived IDs/revisions逐轴闭合；
2. fresh owner join：复读Source/Effect/Prepared并逐字段匹配OwnerCurrent中的
   projection digest/checked/expires；复读Workspace并重证完整body、digest、
   Meta/Lease expiry与currentness。`WorkspaceCheckedUnixNano`只是Sandbox owner
   的本地S2观察水位，只要求不晚于OwnerCurrent checked/use-time now，不能伪称
   Workspace Owner签发的projection字段。

`OwnerCurrent.ValidateShape`只证明自洽envelope，不能替代上述join。

## Ensure 与S1/S2

公开Ensure只接受 `WorkspaceReadSourceCommandRefV2`：

```go
EnsureWorkspaceReadCommandV2(
    context.Context,
    WorkspaceReadSourceCommandRefV2,
) (WorkspaceReadCommandOwnerCurrentV2, error)
```

调用顺序固定为：

1. exact Source current S1；
2. 按Source中的exact Operation/Effect读取Runtime Effect current S1；
3. 按Source中的exact Prepared读取Runtime Prepared current S1；
4. 按canonical bytes派生的exact Workspace读取Workspace current S1；
5. 分别完成Source、Effect、Prepared、Workspace S2；
6. 取fresh owner clock，拒绝回退、到期或跨界；
7. 由Prepared snapshot取得full Runtime Attempt，不新造Attempt Reader；
8. 在同一SQLite事务中create-once Command、Publication、OwnerCurrent；
9. commit回包丢失时只用预先派生的exact refs做bounded Inspect。

S1/S2必须逐字段保持：

- Source exact coordinate、payload与derived request不漂移；
- Effect状态严格为 `dispatch_intent`；
- Effect Operation/Intent/Provider/payload与Source闭合；
- Prepared、Attempt、Permit、Delegation、Provider与Source闭合；
- Workspace ref、revision、digest、Lease与scope不漂移；
- `WorkspaceReadCommandV1.Meta.ExpiresUnixNano <= RequestedNotAfterUnixNano`。

Delegation的权威关系是：Attempt与Prepared declared delegation都non-nil/合法，
ID相等，且`Attempt.Delegation.Revision`严格大于
`Prepared.DeclaredDelegation.Revision`；Attempt完整digest/坐标由Runtime Attempt
digest与Prepared snapshot exact join封存，不能把两者误写为exact相等。

Workspace `Lease.ObservedRevision`不等于Operation
`CurrentProjectionRevision`，不得用后者伪造Runtime Lease observation。Factory只
通过Workspace S1/S2 full-body equality捕获ObservedRevision漂移；未来physical
current gate继续复读RuntimeLease current并逐轴核验。

TTL严格分两层，时间区间均为半开 `[checked, expires)`：

- immutable Command/Publication semantic NotAfter只取Source Fact NotAfter、
  Runtime Effect Intent、Prepared、Workspace Meta/Lease及caller
  requested-not-after等稳定绝对上界；不得把15秒transient current envelope
  固化进Command/Publication语义TTL；
- OwnerCurrent expiry取fresh Source/Effect/Prepared/Workspace current上界、
  15秒owner上界、Command.Meta.Expires和Publication semantic NotAfter的最小值。

初次Command+Publication+OwnerCurrent三路原子创建。后续refresh只能对同一
immutable Publication重新S1/S2并CAS追加OwnerCurrent history/pointer；不得创建
第二Publication，不得越过最初语义上界。

## Owner-private writer

`WorkspaceReadOwnerStoreV1`不再公开raw Command create；concrete
`(*sqlite.Store).CreateWorkspaceReadCommandV1`也必须删除或私有化，不能留下
跨包可调用的旁路。Command/Publication/OwnerCurrent writer只接受Sandbox
`internal/owner/...` nominal capability；capability只能由kernel完成全部S1/S2
与fresh-clock closure后构造。

public `Seal`或canonical helper只负责形状与摘要，不赋权。Tool、Runtime和其他
Owner不能导入internal capability，也不能直接写Sandbox Owner Fact。

## SQLite v19

v19必须在同一事务中持久化：

- Command canonical body与body seal；
- immutable Publication history；
- OwnerCurrent history与current pointer；
- v19 schema ledger仅由Open/migration事务创建或推进。

每次Ensure事实事务只写Command canonical/body seal、Publication history、
OwnerCurrent history+pointer；不得耦合DDL、`user_version`或schema ledger推进。

Open/migration必须避免`CREATE IF NOT EXISTS`式silent repair：

1. `BEGIN IMMEDIATE`后在事务内重新读取`user_version`；
2. 枚举完整v19 namespace，并检查`Rows.Err`与`Close`；
3. existing v19先strict verify且执行零DDL；
4. 旧版仅在v19 namespace全空时执行exact CREATE；任意partial namespace均
   Conflict；
5. strict物理shape验证及正向probe通过后，ledger与`user_version`在同一事务推进；
6. 最后commit。

并发Open不得使用事务外缓存的stale version。raw SQLite方法只能是
`InspectStored...`等storage primitive，不能直接实现public current Reader；
public current Reader必须依次完成stored三事实join与fresh四Reader join。

不变量：

- 相同Source与相同闭包幂等返回同一winner；
- 相同Source异payload、相同Attempt异Source、A/B Source splice均Conflict；
- Command-only、Publication-only、OwnerCurrent-only及任意三路partial均Conflict；
- v18 legacy Command保持historical exact可读，但不得backfill/reseal为V2
  Publication/OwnerCurrent，也不得获得Ensure/current/physical资格；
- legacy `InspectWorkspaceReadCommandCurrentV1`必须要求V2
  Publication+OwnerCurrent闭包；只有v18/raw Command时Fail Closed，不能仅凭
  Command TTL恢复current或physical资格；
- v19 ledger存在时缺表、缺索引、shape/DDL/index漂移一律Conflict且不repair；
- 无ledger但出现任意partial v19 namespace同样Conflict。

## Unknown、失败与错误闭集

- pre-commit失败：`ErrorInvalidArgument`、`ErrorConflict`、
  `ErrorPreconditionFailed/ReasonBindingExpired`或`ErrorUnavailable`，零写；
- commit结果Unknown：只bounded exact Inspect预派生的Command/Publication/
  OwnerCurrent refs；不得重跑S1/S2、不得再次commit；
- exact三者全存在且闭合：返回winner；
- 三者全不存在：返回原Unknown；
- 任意partial、digest/lineage不一致：`Conflict`；
- NotFound只对权威Owner确认的exact历史不存在成立；store/clock/reader不确定为
  `Unavailable`或`Conflict`，不得转换成可重试执行权。

所有失败路径均为Provider调用0、physical read 0、Runtime write 0。

lost-reply分为两个协议：initial只Inspect预封的Command+Publication+OwnerCurrent
rev1；refresh只Inspect预封next OwnerCurrent。二者都不得重跑S1/S2；refresh不得
再写Command/Publication或换revision。CAS stale只读current pointer并验证仍绑定
同一Publication。

## 兼容、依赖与NO-GO

- V1 wire、digest与historical Reader不原地修改；
- Runtime public Effect/Prepared current是实现依赖；Workspace只注入含
  `InspectWorkspaceViewCurrentV1`的窄只读接口，不注入broad
  `WorkspaceCurrentReaderV1`；
- neutral Source Reader由Sandbox定义、未来Tool adapter实现；
- Runtime production current readers、Tool neutral adapter、Application
  composition、Provider、physical read与production root仍NO-GO；
- 本切片只可宣称 `owner-local factory ready`，不得设置CorePack Executable。
